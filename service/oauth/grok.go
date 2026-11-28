package oauth

import (
	"context"
	"time"
)

// xAI/Grok Device OAuth constants.
//
// Endpoints and the public client_id are sourced from the grok2api-chenyme
// reference implementation (backend/internal/infra/provider/cli/oauth.go).
// xAI exposes a public Device OAuth client for the Grok CLI; the client_id is
// not secret and is hardcoded here (mirrors the Antigravity placeholder pattern
// for vendor-published public clients).
// grokDeviceCodeURL and grokTokenURL are package vars (not consts) so tests
// can swap in a local httptest server without monkey-patching net.Dialer.
var (
	grokDeviceCodeURL = "https://auth.x.ai/oauth2/device/code"
	grokTokenURL      = "https://auth.x.ai/oauth2/token"
)

const (
	grokClientID            = "b1a00492-073a-47ea-816f-4c329264a828"
	grokDefaultScope        = "openid profile email offline_access grok-cli:access api:access"
	grokUpstreamBaseURL     = "https://api.x.ai"
	grokUserAgent           = "grok-cli/1.0 (oauth)"
	grokLoopbackPort        = 0 // Device OAuth has no redirect callback.
	grokLoopbackPath        = ""
	grokLoopbackRedirectURI = "" // Device OAuth does not use a redirect URI.
	// grokHTTPTimeout bounds each outbound call to the xAI auth endpoints.
	grokHTTPTimeout = 30 * time.Second
	// grokDeviceCodeTTL bounds how long a device_code remains usable in our
	// side-channel store. xAI typically returns expires_in=1800 (30 min);
	// we keep a small margin above that.
	grokDeviceCodeTTL = 35 * time.Minute
	// grokDefaultPollInterval is the fallback poll interval when xAI does not
	// return an explicit `interval` field.
	grokDefaultPollInterval = 5 * time.Second
)

// Device OAuth is split across two provider hooks: BuildAuthorizationURL
// stores the device_code in the shared Device Flow side channel, and
// ExchangeAuthorizationCode polls it. See device_flow.go.

// ---- Registration ----

func init() {
	RegisterProvider(&OAuthProviderDefinition{
		Metadata: ProviderMetadata{
			Provider:                     ProviderGrok,
			Label:                        "Grok",
			Platform:                     "grok",
			Enabled:                      true,
			LoginType:                    "oauth",
			RequiresProjectId:            false,
			SupportsDirectAccountRouting: true,
			SupportsCloudValidation:      false,
			SupportsNativeProxy:          true,
			SupportsDeviceFlow:           true,
		},
		Site: ProviderSiteConfig{
			Name:     "xAI Grok OAuth",
			URL:      grokUpstreamBaseURL,
			Platform: "grok",
		},
		SessionTTL: grokDeviceCodeTTL,
		// Loopback is intentionally zero-valued: Device OAuth has no redirect
		// callback. The shared flow layer still starts a loopback listener for
		// every provider, but for grok it simply never receives a callback —
		// completion is driven by polling the token endpoint.
		Loopback: LoopbackConfig{
			Host:        "127.0.0.1",
			Port:        grokLoopbackPort,
			Path:        grokLoopbackPath,
			RedirectURI: grokLoopbackRedirectURI,
		},
		BuildAuthorizationURL:     buildGrokAuthorizationURL,
		ExchangeAuthorizationCode: exchangeGrokAuthorizationCode,
		RefreshAccessToken:        refreshGrokAccessToken,
		BuildProxyHeaders:         buildGrokProxyHeaders,
	})
}

func buildGrokProxyHeaders(ctx context.Context, input ProxyHeaderInput) map[string]string {
	// The Authorization: Bearer <access_token> header is applied centrally by
	// the proxy layer from the account's stored access token. Here we only add
	// the xAI/Grok client identity headers that the upstream expects on every
	// chat-completion request.
	return map[string]string{
		"User-Agent": grokUserAgent,
	}
}
