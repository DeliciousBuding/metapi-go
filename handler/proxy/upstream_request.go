package proxyhandler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
)

// defaultUpstreamClient is used as a safety fallback when the RuntimeExecutor
// has not been wired (e.g., during tests). It carries a 90s timeout so a hung
// upstream never leaks a goroutine, and the shared RejectCrossOriginRedirect
// policy so a public origin cannot 302 into metadata/loopback when Executor is
// nil. Production deployments should always wire the Executor field via
// SetUpstreamConfig.
var defaultUpstreamClient = &http.Client{
	Timeout: proxy.DefaultRequestCeiling,
	Transport: httpclient.NewTransport(httpclient.Options{
		ResponseHeaderTimeout: proxy.DefaultRequestCeiling,
		SiteDialGuard:         true, // data-plane dial guard, see proxy.NewStreamTransport
	}),
	CheckRedirect: platform.RejectCrossOriginRedirect,
}

// defaultStreamUpstreamClient mirrors defaultUpstreamClient for SSE streams
// on the fallback path (Executor unwired, e.g. tests): no whole-request
// timeout, so a flowing stream is governed per chunk by the relay's idle
// guard (PROXY_STREAM_IDLE_TIMEOUT_SEC) instead of total elapsed time. The
// shared header-budget owner in sendUpstreamRequest bounds the initial wait;
// the reusable stream transport has no competing response-header timer.
var defaultStreamUpstreamClient = &http.Client{
	Transport:     proxy.NewStreamTransport(),
	CheckRedirect: platform.RejectCrossOriginRedirect,
}

// siteRequestURL rebuilds native Gemini resource paths after routing has resolved
// the actual model. The shared URL builder preserves semantic base paths and
// avoids appending the API version twice.
func siteRequestURL(baseURL, path, model string, stream bool) string {
	if bit := proxy.DirectProtocolForPath(path); bit == store.DirectProtocolGemini || bit == store.DirectProtocolGeminiEmbeddings {
		version, _, action := ParseGeminiPath(path)
		path = "/" + version + "/models/" + url.PathEscape(strings.TrimPrefix(model, "models/")) + ":" + action
		if stream {
			path += "?alt=sse"
		}
	}
	// Defense in depth at the final concatenation: the dispatcher already
	// validated the downstream path, but a rewrite must never reintroduce an
	// escaping path. An empty URL fails request construction before any
	// upstream I/O.
	if !validUpstreamDispatchPath(path) {
		return ""
	}
	return proxy.BuildUpstreamURL(baseURL, path)
}

// Native API keys use provider headers; OAuth credentials and compatible gateway
// sites retain their Bearer contract even when a client uses a native protocol.
func siteNativeAPIKeyHeader(selected *routing.SelectedChannel, path string) string {
	if selected.Direct != nil {
		return ""
	}
	endpoint, _ := proxy.EndpointFromPath(path)
	provider := platform.NormalizePlatformAlias(selected.Site.Platform)
	header := ""
	if provider == "claude" && endpoint == proxy.EndpointMessages {
		header = "x-api-key"
	} else if provider == "gemini" && (endpoint == proxy.EndpointGemini || proxy.DirectProtocolForPath(path) == store.DirectProtocolGeminiEmbeddings) {
		header = "x-goog-api-key"
	}
	if header != "" && oauth.GetOauthInfoFromAccount(&selected.Account) != nil {
		return ""
	}
	return header
}

func applyProxyCustomHeaders(req *http.Request, proxyConfig *platform.ProxyConfig) {
	if proxyConfig == nil {
		return
	}
	// Shared deny-list: Authorization/Host/hop-by-hop/Cookie/Proxy-*/metapi control.
	// site-wins when CustomHeadersOverrideRequest; else request-wins.
	platform.ApplyCustomHeadersWithOptions(req, proxyConfig.CustomHeaders, platform.ApplyCustomHeadersOptions{
		OverrideRequest: proxyConfig.CustomHeadersOverrideRequest,
	})
	// Per-site anti-bot identity: cf_clearance cookie + browser UA override.
	platform.ApplySiteIdentity(req, proxyConfig)
}

// sendUpstreamRequest dispatches an upstream HTTP request with optional observed
// header timeout. firstByteTimeoutMs is milliseconds; streams use the shared
// finite default when it is non-positive, while buffered calls retain their bounds.
// Config PROXY_FIRST_BYTE_TIMEOUT_SEC is seconds; convert via proxy.FirstByteTimeoutMs.
//
// isStream selects the SSE dispatch variants, which carry no whole-request
// timeout: a flowing stream must not be capped by total elapsed time
// (PROXY_REQUEST_TIMEOUT_SEC / executor ceiling), because that would also
// kill long-but-healthy streams. Body-phase liveness is instead enforced per
// chunk by the relay's idle guard (PROXY_STREAM_IDLE_TIMEOUT_SEC); the
// stream header phase is owned by the shared header budget, without a shorter
// transport timer racing it. Buffered dispatch retains its transport/total bounds.
// sendUpstreamRequest keeps response-header waiting separate from the body:
// the caller's remaining retry window is resolved before this dispatch, while
// a healthy stream keeps its own idle and byte limits after headers arrive.
// Media polling/downloads and asynchronous task lifetimes are not capped here.
func sendUpstreamRequest(cfg *UpstreamConfig, req *http.Request, proxyConfig *platform.ProxyConfig, firstByteTimeoutMs int64, isStream bool) (*http.Response, error) {
	hasProxyCfg := proxyConfig != nil && (proxyConfig.ProxyURL != "" || proxyConfig.InsecureSkipTLS)
	if hasProxyCfg && req.URL != nil && service.IsForbiddenSiteTargetURL(req.URL.String()) {
		return nil, fmt.Errorf("refusing to dispatch proxied request to forbidden metadata or link-local target")
	}
	if !hasProxyCfg && cfg != nil && cfg.Executor != nil {
		if isStream {
			return cfg.Executor.DoStreamWithObservedFirstByte(req.Context(), req, firstByteTimeoutMs)
		}
		return cfg.Executor.DoWithObservedFirstByte(req.Context(), req, firstByteTimeoutMs)
	}
	if isStream && hasProxyCfg {
		return platform.DoWithProxyStreamBudget(req.Context(), req, proxyConfig, firstByteTimeoutMs)
	}
	do := defaultUpstreamClient.Do
	if isStream {
		do = defaultStreamUpstreamClient.Do
	} else if hasProxyCfg {
		do = func(r *http.Request) (*http.Response, error) {
			return platform.DoWithProxy(r.Context(), r, proxyConfig)
		}
	}
	if isStream || firstByteTimeoutMs > 0 {
		return httpclient.DoWithHeaderBudget(req, firstByteTimeoutMs, do)
	}
	return do(req)
}

// swapModelInJSON rewrites the top-level "model" field of a JSON request body
// to upstreamModel for one upstream attempt. The dominant no-mapping case (the
// selected channel serves the requested model) short-circuits via an
// allocation-free top-level scan and returns the original bytes untouched; a
// real model_mapping splices only the model value span, keeping the rest of
// the payload byte-identical. Irregular bodies fall back to the legacy
// shallow re-encode (legacySwapModelInJSON).
func swapModelInJSON(bodyBytes []byte, upstreamModel string) []byte {
	if len(bodyBytes) == 0 {
		// Empty body: synthesize a minimal JSON object with only the model field.
		modelJSON, _ := json.Marshal(upstreamModel)
		return append(append([]byte(`{"model":`), modelJSON...), '}')
	}
	s, found, ok := findTopLevelValue(bodyBytes, "model")
	if ok {
		if found {
			if spanStringEquals(bodyBytes, s, upstreamModel) {
				// Zero-copy short-circuit: the body already carries the target
				// model (the no-mapping norm); no rewrite, no allocation.
				return bodyBytes
			}
			// Real model_mapping: splice only the model value, keep the rest of
			// the payload byte-identical.
			modelJSON, _ := json.Marshal(upstreamModel)
			return replaceSpan(bodyBytes, s, string(modelJSON))
		}
		// Clean object without a top-level model key (e.g. Gemini path-model
		// bodies): insert it as the first entry. The scan above proves no
		// escaped-key "model" hiding at the top level.
		openIdx, empty, bracesOK := objectBraces(bodyBytes)
		if bracesOK {
			modelJSON, _ := json.Marshal(upstreamModel)
			return insertTopLevelEntry(bodyBytes, openIdx, `"model":`+string(modelJSON), empty)
		}
	}
	// Irregular body: exact legacy shallow re-encode semantics.
	return legacySwapModelInJSON(bodyBytes, upstreamModel)
}

// legacySwapModelInJSON is the original shallow re-encode kept for irregular
// bodies the allocation-free scanner declines to edit.
func legacySwapModelInJSON(bodyBytes []byte, upstreamModel string) []byte {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		// Body is already validated in PrepareCtx; fallback to original bytes.
		return bodyBytes
	}
	modelJSON, _ := json.Marshal(upstreamModel)
	raw["model"] = json.RawMessage(modelJSON)
	result, err := json.Marshal(raw)
	if err != nil {
		return bodyBytes
	}
	return result
}

func bytesReader(b []byte) io.Reader {
	if len(b) == 0 {
		return nil
	}
	return bytes.NewReader(b)
}
