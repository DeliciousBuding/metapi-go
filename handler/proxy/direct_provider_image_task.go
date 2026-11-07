package proxyhandler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/tidwall/sjson"
)

const maxImageTaskResponseBytes = 16 << 20

type directImageTaskOperation struct {
	cfg          *UpstreamConfig
	wire         *directProviderWire
	proxyConfig  *platform.ProxyConfig
	budgetMs     int64
	pollInterval time.Duration
	beforeQuery  func(context.Context) (string, error)
	format       string
}

func sendDirectImageTaskRequest(cfg *UpstreamConfig, original, downstream *http.Request, proxyCtx *Ctx, selected *routing.SelectedChannel, wire *directProviderWire, proxyConfig *platform.ProxyConfig, budgetMs int64) (*http.Response, error) {
	format, _ := proxyCtx.Body["response_format"].(string)
	operation := directImageTaskOperation{cfg: cfg, wire: wire, proxyConfig: proxyConfig, budgetMs: budgetMs, pollInterval: 2 * time.Second, format: format}
	revalidateDriver := func(ctx context.Context) error {
		binding := selected.Direct.RequestConfig.Provider
		if binding == nil || cfg.ChannelPlugins == nil {
			return channelplugins.ErrBinding
		}
		// Availability follows the current installation. A compatible upgrade
		// does not replace this attempt's already captured program or options.
		_, _, err := cfg.ChannelPlugins.DescribeProvider(ctx, *binding)
		return err
	}
	operation.beforeQuery = func(ctx context.Context) (string, error) {
		request := downstream.WithContext(ctx)
		if _, err := revalidateDirectOperationDispatch(request, proxyCtx, cfg, selected, proxyCtx.dispatchPolicy); err != nil {
			return "", channelplugins.ErrBinding
		}
		if err := revalidateDriver(ctx); err != nil {
			return "", err
		}
		token := selected.TokenValue
		if cfg.ResolveDirectCredential != nil && selected.Direct.CredentialID > 0 {
			var proxyURL *string
			if proxyConfig != nil && proxyConfig.ProxyURL != "" {
				proxyURL = &proxyConfig.ProxyURL
			}
			credential, err := cfg.ResolveDirectCredential(ctx, selected.Direct.CredentialID, proxyURL, false)
			if err != nil || credential == nil || credential.AccessToken == "" || credential.Kind != selected.Direct.CredentialKind {
				return "", channelplugins.ErrBinding
			}
			if _, err := revalidateDirectOperationDispatch(request, proxyCtx, cfg, selected, proxyCtx.dispatchPolicy); err != nil {
				return "", channelplugins.ErrBinding
			}
			if err := revalidateDriver(ctx); err != nil {
				return "", err
			}
			token = credential.AccessToken
		} else if selected.Direct.CredentialKind != store.DirectCredentialAPIKey {
			return "", channelplugins.ErrUnavailable
		}
		return token, nil
	}
	return operation.run(original)
}

// The whole submit/poll/result facade has one attempt and one ceiling. Neither
// a failed poll nor a decoder/download error can replay its billable POST.
func (op directImageTaskOperation) run(original *http.Request) (result *http.Response, err error) {
	if op.wire == nil || op.wire.Driver == nil || op.wire.DriverImageQuery == nil || op.beforeQuery == nil || op.format != "" && op.format != "url" && op.format != "b64_json" {
		return nil, channelplugins.ErrBinding
	}
	ctx, cancel := context.WithTimeout(original.Context(), proxy.RequestCeiling(int((op.budgetMs+999)/1000)))
	defer cancel()
	usage := ParsedUsage{Source: usageSourceUnknown}
	started := time.Now()
	submission := original.Clone(platform.WithoutRequestRedirects(ctx))
	submission.GetBody = nil
	response, err := sendUpstreamRequest(op.cfg, submission, op.proxyConfig, op.budgetMs, false)
	firstByteMs := time.Since(started).Milliseconds()
	defer func() {
		copy := usage
		op.wire.DriverImageUsage = &copy
		if result != nil {
			result.Request = original.WithContext(context.WithValue(original.Context(), directMediaTimingKey{}, &firstByteMs))
		}
		if err != nil {
			err = &directMediaTransportError{cause: err, usage: usage}
		}
	}()
	if err != nil {
		return nil, err
	}
	taskID := ""
	polls, totalBytes := 0, 0
	for {
		body, readErr := readDirectMediaResponse(response)
		if readErr != nil {
			return nil, readErr
		}
		normalized := proxy.NormalizeUpstreamBufferedBody(response.Header, body)
		totalBytes += len(normalized.Bytes)
		if !normalized.Readable || totalBytes > maxImageTaskResponseBytes {
			return nil, channelplugin.ErrResult
		}
		observed, failed, observeErr := observeDirectDriverBody(op.wire, normalized.Bytes, false)
		if observeErr != nil || taskID != "" && observed.upstreamTaskID != "" && taskID != observed.upstreamTaskID {
			return nil, channelplugin.ErrResult
		}
		usage = mergeUsagePreferLater(usage, observed)
		if taskID == "" && observed.upstreamTaskID != "" {
			taskID = observed.upstreamTaskID
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			if response.StatusCode >= 300 && response.StatusCode < 400 {
				return nil, channelplugin.ErrResult
			}
			return replayDirectMediaResponse(response, normalized.Bytes, true), nil
		}
		if failed || nativeDriverError(normalized.Bytes) {
			return nil, channelplugin.ErrResult
		}
		raw, callErr := op.wire.Driver.Call(ctx, "taskResult", channelplugin.DriverInput{Body: string(normalized.Bytes), Model: op.wire.PluginModel, TaskID: taskID, StatusCode: response.StatusCode})
		if callErr != nil {
			return nil, callErr
		}
		var decision struct {
			Status string `json:"status"`
			Body   string `json:"body,omitempty"`
		}
		if requestoverride.Decode(raw, &decision) != nil {
			return nil, channelplugin.ErrResult
		}
		if decision.Status == "completed" {
			output, completeErr := op.complete(ctx, response, []byte(decision.Body), usage)
			if completeErr != nil {
				return nil, completeErr
			}
			result = replayDirectMediaResponse(response, output, true)
			result.StatusCode, result.Status = http.StatusOK, "200 OK"
			return result, nil
		}
		if decision.Status != "pending" || decision.Body != "" || taskID == "" {
			return nil, channelplugin.ErrResult
		}
		if polls > 0 {
			timer := time.NewTimer(op.pollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
		address, addressErr := contract.ImageQueryURL(op.wire.DriverImageQuery.URL, taskID)
		if addressErr != nil {
			return nil, channelplugin.ErrResult
		}
		raw, callErr = op.wire.Driver.Call(ctx, "query", channelplugin.DriverInput{Body: string(normalized.Bytes), Model: op.wire.PluginModel, TaskID: taskID})
		if callErr != nil {
			return nil, callErr
		}
		var descriptor channelplugin.RequestDescriptor
		if requestoverride.Decode(raw, &descriptor) != nil || descriptor.Method != http.MethodGet || descriptor.Body != "" || descriptor.Parts != nil || descriptor.ContentType != "" && descriptor.ContentType != "application/json" || validateDriverOperationURL(descriptor, address) != nil {
			return nil, channelplugin.ErrResult
		}
		token, authErr := op.beforeQuery(ctx)
		if authErr != nil {
			return nil, authErr
		}
		query, queryErr := http.NewRequestWithContext(platform.WithoutRequestRedirects(ctx), http.MethodGet, address, nil)
		if queryErr != nil {
			return nil, channelplugin.ErrResult
		}
		if op.wire.DriverImageQuery.Auth != store.DirectAuthNone {
			query.Header = original.Header.Clone()
		}
		for _, name := range []string{"Content-Type", "Content-Length", "Range", "If-Range", "X-ModelScope-Async-Mode"} {
			query.Header.Del(name)
		}
		for name, value := range descriptor.Headers {
			query.Header.Set(name, value)
		}
		op.wire.Driver.ClearAuthentication(query.Header)
		query.Header.Set("Accept", "application/json")
		if op.wire.DriverImageQuery.Auth != store.DirectAuthNone {
			switch op.wire.DriverImageQuery.Auth {
			case store.DirectAuthAPIKey:
				query.Header.Set("X-Api-Key", token)
			case store.DirectAuthGoogle:
				query.Header.Set("X-Goog-Api-Key", token)
			default:
				query.Header.Set("Authorization", "Bearer "+token)
			}
			if authErr = op.wire.Driver.ApplyAuthentication(query.Header, token); authErr != nil {
				return nil, authErr
			}
		}
		queryProxy := op.proxyConfig
		if op.wire.DriverImageQuery.Auth == store.DirectAuthNone && queryProxy != nil {
			clean := *queryProxy
			clean.CustomHeaders, clean.ClearanceCookie = nil, ""
			queryProxy = &clean
		}
		response, err = sendUpstreamRequest(op.cfg, query, queryProxy, op.budgetMs, false)
		if err != nil {
			return nil, err
		}
		polls++
	}
}

// Host materialization never passes downloaded bytes to JS or derives tokens
// from them. Unchanged native fields/large integers remain JSON text.
func (op directImageTaskOperation) complete(ctx context.Context, response *http.Response, body []byte, usage ParsedUsage) ([]byte, error) {
	if requestoverride.RejectDuplicateJSONKeys(body) != nil || !responseControlAliasesMatch([]byte(`{}`), body) || !nativeDriverPayload(op.wire.PluginProtocol, body, false) || nativeDriverError(body) {
		return nil, channelplugin.ErrResult
	}
	decodedUsage := ParseUsageFromBody(body)
	if decodedUsage.invalidForUserBudget || decodedUsage.Found && (!usage.Found || !sameDriverObservedUsage(usage, decodedUsage)) || usage.UpstreamReportedModel != nil && decodedUsage.UpstreamReportedModel != nil && *usage.UpstreamReportedModel != *decodedUsage.UpstreamReportedModel {
		return nil, channelplugin.ErrResult
	}
	var payload struct {
		Created int64 `json:"created"`
		Data    []struct {
			URL    string `json:"url,omitempty"`
			Base64 string `json:"b64_json,omitempty"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &payload) != nil || len(payload.Data) == 0 || payload.Created < 0 {
		return nil, channelplugin.ErrResult
	}
	for index, image := range payload.Data {
		if image.Base64 != "" {
			decoded, err := base64.StdEncoding.DecodeString(image.Base64)
			if err != nil || len(decoded) == 0 {
				return nil, channelplugin.ErrResult
			}
		}
		dataURI := strings.HasPrefix(image.URL, "data:")
		if dataURI {
			if _, err := directImageDataURL(image.URL); err != nil {
				return nil, channelplugin.ErrResult
			}
		} else if image.URL != "" {
			target, err := url.Parse(image.URL)
			if err != nil || target.Host == "" || target.User != nil || target.Fragment != "" || target.Scheme != "http" && target.Scheme != "https" {
				return nil, channelplugin.ErrResult
			}
		}
		if op.format == "url" {
			if image.URL == "" {
				return nil, channelplugin.ErrResult
			}
			continue
		}
		if image.Base64 == "" {
			if image.URL == "" {
				return nil, channelplugin.ErrResult
			}
			encoded, err := directImageDataURL(image.URL)
			if !dataURI {
				request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, image.URL, nil)
				if requestErr != nil {
					return nil, channelplugin.ErrResult
				}
				download, downloadErr := downloadDirectVideoContent(op.cfg, request, op.proxyConfig, op.budgetMs, image.URL, response, nil)
				if downloadErr != nil {
					return nil, downloadErr
				}
				if download.StatusCode < 200 || download.StatusCode >= 300 {
					_ = download.Body.Close()
					return nil, channelplugin.ErrResult
				}
				bytes, readErr := readDirectMediaResponse(download)
				if readErr != nil || len(bytes) == 0 {
					return nil, channelplugin.ErrResult
				}
				encoded, err = base64.StdEncoding.EncodeToString(bytes), nil
			}
			if err != nil {
				return nil, channelplugin.ErrResult
			}
			body, err = sjson.SetBytes(body, "data."+strconv.Itoa(index)+".b64_json", encoded)
			if err != nil || int64(len(body)) > proxy.MaxBufferedResponseBodyBytes() {
				return nil, channelplugin.ErrResult
			}
		}
	}
	if payload.Created == 0 {
		var err error
		body, err = sjson.SetBytes(body, "created", time.Now().Unix())
		if err != nil {
			return nil, channelplugin.ErrResult
		}
	}
	return body, nil
}
