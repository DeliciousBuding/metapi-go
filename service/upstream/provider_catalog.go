package upstream

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/internal/ssrf"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

func effectiveCatalogProxy(channelProxy string, useSystemProxy bool) string {
	if channelProxy == "" && useSystemProxy {
		if runtime := config.RuntimeSafe(); runtime != nil {
			return runtime.SystemProxyUrl
		}
	}
	return channelProxy
}

func catalogHTTPClient(channelProxy string, useSystemProxy bool) (*http.Client, func(), error) {
	proxyURL := effectiveCatalogProxy(channelProxy, useSystemProxy)
	proxy := httpclient.NoProxy
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" || ssrf.IsForbiddenSiteHostname(parsed.Hostname()) || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "socks5" && parsed.Scheme != "socks5h") {
			return nil, nil, invalid("Invalid channel proxy URL")
		}
		proxy = http.ProxyURL(parsed)
	}
	transport := httpclient.NewTransport(httpclient.Options{Proxy: proxy, SiteDialGuard: true, DialTimeout: 5 * time.Second, ResponseHeaderTimeout: connectDiscoveryTimeout})
	client := httpclient.NewClient(transport, connectDiscoveryTimeout, func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse })
	return client, transport.CloseIdleConnections, nil
}

func providerCatalogChannelIdentity(channel *catalogChannelRow) string {
	if channel == nil {
		return ""
	}
	raw, _ := json.Marshal([]any{channel.Provider, channel.Enabled, channel.EndpointConfig, channel.RequestConfig.Provider, channel.UseSystemProxy, channel.ChannelProxy, channel.CustomHeaders})
	hash := sha256.Sum256(raw)
	return hex.EncodeToString(hash[:])
}

// Catalog callbacks describe and parse read-only pages. The host owns current
// credentials, exact target, cursor injection, byte/model/page bounds and writes.
func discoverProviderCatalog(ctx context.Context, manager *channelplugins.Manager, binding store.DirectProviderBinding, channelProxy, rawHeaders string, useSystemProxy bool, credential func(context.Context, []string) (catalogCredential, error), verify func(context.Context) error) ([]string, Discovery, string, error) {
	ctx, cancel := context.WithTimeout(ctx, connectDiscoveryTimeout)
	defer cancel()
	if binding.Catalog == nil || binding.Catalog.Validate() != nil {
		return nil, Discovery{}, "", invalid("Configure a provider catalog endpoint")
	}
	base, err := url.Parse(binding.Catalog.URL)
	if err != nil || ssrf.IsForbiddenSiteHostname(base.Hostname()) {
		return nil, Discovery{}, "", invalid("Invalid upstream model URL")
	}
	manifest, fingerprint, err := manager.DescribeProvider(ctx, binding)
	if err != nil {
		return nil, Discovery{}, "", invalid("Provider catalog is unavailable")
	}
	custom, err := modelCatalogCustomHeaders(rawHeaders)
	if err != nil {
		return nil, Discovery{}, "", err
	}
	client, closeClient, err := catalogHTTPClient(channelProxy, useSystemProxy)
	if err != nil {
		return nil, Discovery{}, "", err
	}
	defer closeClient()
	names := []string{}
	cursor := ""
	seen := map[string]bool{}
	remaining := connectDiscoveryMaxBytes
	for page := 0; page < 100; page++ {
		if verify != nil {
			if err := verify(ctx); err != nil {
				return nil, Discovery{}, "", err
			}
		}
		_, current, err := manager.DescribeProvider(ctx, binding)
		if err != nil || current != fingerprint {
			return nil, Discovery{}, "", conflict("Provider changed during model discovery; retry discovery")
		}
		identity, err := credential(ctx, manifest.CredentialKinds)
		if err != nil || identity.Secret == "" {
			return nil, Discovery{}, "", invalid("Channel has no current credential for provider discovery")
		}
		if verify != nil {
			if err := verify(ctx); err != nil {
				return nil, Discovery{}, "", err
			}
		}
		invocation, err := manager.ResolveCatalog(ctx, binding, identity.Kind)
		if err != nil || invocation.Fingerprint() != fingerprint {
			return nil, Discovery{}, "", conflict("Provider changed during model discovery; retry discovery")
		}
		raw, err := invocation.Call(ctx, "modelsRequest", channelplugin.DriverInput{})
		if err != nil {
			return nil, Discovery{}, "", connectDiscoveryError("Provider could not prepare model discovery")
		}
		var descriptor channelplugin.RequestDescriptor
		if requestoverride.Decode(raw, &descriptor) != nil || descriptor.Method != http.MethodGet || descriptor.ContentType != "" {
			return nil, Discovery{}, "", connectDiscoveryError("Invalid provider catalog request")
		}
		target, err := descriptor.ResolveURL(binding.Catalog.URL)
		if err != nil || target.String() != base.String() {
			return nil, Discovery{}, "", connectDiscoveryError("Provider catalog target is not authorized")
		}
		if cursor != "" {
			query := target.Query()
			query.Set(binding.Catalog.CursorParameter, cursor)
			target.RawQuery = query.Encode()
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			return nil, Discovery{}, "", invalid("Invalid upstream model URL")
		}
		request.Header.Set("Accept", "application/json")
		for key, value := range custom {
			request.Header.Set(key, value)
		}
		for key, value := range descriptor.Headers {
			request.Header.Set(key, value)
		}
		switch binding.Catalog.Auth {
		case store.DirectAuthBearer:
			request.Header.Set("Authorization", "Bearer "+identity.Secret)
		case store.DirectAuthAPIKey:
			request.Header.Set("X-Api-Key", identity.Secret)
		case store.DirectAuthGoogle:
			request.Header.Set("X-Goog-Api-Key", identity.Secret)
		}
		response, err := client.Do(request)
		if err != nil {
			return nil, Discovery{}, "", connectDiscoveryError("Could not fetch upstream models; check the address and proxy settings")
		}
		limit := remaining
		if limit > channelplugin.MaxInputBytes/2 {
			limit = channelplugin.MaxInputBytes / 2
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, int64(limit)+1))
		_ = response.Body.Close()
		if response.StatusCode == 401 || response.StatusCode == 403 {
			return nil, Discovery{}, "", invalid("Upstream rejected the credential; check its permissions")
		}
		if response.StatusCode != 200 {
			return nil, Discovery{}, "", connectDiscoveryError("Upstream model discovery failed")
		}
		if readErr != nil || len(body) > limit {
			return nil, Discovery{}, "", connectDiscoveryError("Upstream model catalog exceeds the response size limit")
		}
		remaining -= len(body)
		raw, err = invocation.Call(ctx, "modelsResponse", channelplugin.DriverInput{Body: string(body), StatusCode: response.StatusCode})
		if err != nil {
			return nil, Discovery{}, "", connectDiscoveryError("Provider could not decode model discovery")
		}
		var result struct {
			Models     *[]string `json:"models"`
			NextCursor string    `json:"nextCursor,omitempty"`
		}
		if requestoverride.Decode(raw, &result) != nil || result.Models == nil {
			return nil, Discovery{}, "", connectDiscoveryError("Provider returned an invalid model catalog")
		}
		names = append(names, (*result.Models)...)
		if len(names) > connectDiscoveryMaxModels {
			return nil, Discovery{}, "", connectDiscoveryError("Upstream model catalog exceeds the 10000 model limit")
		}
		if result.NextCursor == "" {
			models, err := normalizeConnectModels(names)
			if err != nil {
				return nil, Discovery{}, "", err
			}
			if verify != nil {
				if err := verify(ctx); err != nil {
					return nil, Discovery{}, "", err
				}
			}
			_, current, err := manager.DescribeProvider(ctx, binding)
			if err != nil || current != fingerprint {
				return nil, Discovery{}, "", conflict("Provider changed during model discovery; retry discovery")
			}
			discovery := Discovery{Status: "discovered"}
			if len(models) == 0 {
				discovery = Discovery{Status: "empty", Message: "Upstream returned an empty model catalog"}
			}
			return models, discovery, fingerprint, nil
		}
		if binding.Catalog.CursorParameter == "" || len(result.NextCursor) > 4096 || seen[result.NextCursor] || strings.ContainsAny(result.NextCursor, "\r\n\x00") {
			return nil, Discovery{}, "", connectDiscoveryError("Upstream returned unsupported or repeated model pagination")
		}
		seen[result.NextCursor] = true
		cursor = result.NextCursor
	}
	return nil, Discovery{}, "", connectDiscoveryError("Upstream model pagination exceeds the page limit")
}
