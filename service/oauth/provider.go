package oauth

import (
	"context"
	"time"
)

// ---- OAuthProviderId ----

type OAuthProviderId string

const (
	ProviderCodex       OAuthProviderId = "codex"
	ProviderClaude      OAuthProviderId = "claude"
	ProviderGeminiCli   OAuthProviderId = "gemini-cli"
	ProviderAntigravity OAuthProviderId = "antigravity"
	ProviderGrok        OAuthProviderId = "grok"
)

// ---- ProviderMetadata ----

type ProviderMetadata struct {
	Provider                   OAuthProviderId `json:"provider"`
	Label                      string          `json:"label"`
	Platform                   string          `json:"platform"`
	Enabled                    bool            `json:"enabled"`
	LoginType                  string          `json:"loginType"`
	RequiresProjectId          bool            `json:"requiresProjectId"`
	SupportsDirectAccountRouting bool           `json:"supportsDirectAccountRouting"`
	SupportsCloudValidation    bool            `json:"supportsCloudValidation"`
	SupportsNativeProxy        bool            `json:"supportsNativeProxy"`
}

// ---- ProviderSiteConfig ----

type ProviderSiteConfig struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Platform string `json:"platform"`
}

// ---- LoopbackConfig ----

type LoopbackConfig struct {
	Host        string `json:"host"`
	Port        int    `json:"port"`
	Path        string `json:"path"`
	RedirectURI string `json:"redirectUri"`
}

// ---- TokenSet ----

type TokenSet struct {
	AccessToken    string                 `json:"accessToken"`
	RefreshToken   string                 `json:"refreshToken,omitempty"`
	TokenExpiresAt int64                  `json:"tokenExpiresAt,omitempty"`
	Email          string                 `json:"email,omitempty"`
	AccountKey     string                 `json:"accountKey,omitempty"`
	AccountID      string                 `json:"accountId,omitempty"`
	PlanType       string                 `json:"planType,omitempty"`
	ProjectID      string                 `json:"projectId,omitempty"`
	IDToken        string                 `json:"idToken,omitempty"`
	SessionToken   string                 `json:"sessionToken,omitempty"`
	ProviderData   map[string]interface{} `json:"providerData,omitempty"`
}

// ---- ProxyHeaderInput ----

type ProxyHeaderInput struct {
	OAuth             ProxyHeaderOAuth            `json:"oauth"`
	DownstreamHeaders map[string]interface{}      `json:"downstreamHeaders,omitempty"`
}

type ProxyHeaderOAuth struct {
	Provider     string                 `json:"provider"`
	AccountKey   string                 `json:"accountKey,omitempty"`
	AccountID    string                 `json:"accountId,omitempty"`
	ProjectID    string                 `json:"projectId,omitempty"`
	ProviderData map[string]interface{} `json:"providerData,omitempty"`
}

// ---- SessionRecord ----

type SessionStatus string

const (
	SessionPending SessionStatus = "pending"
	SessionSuccess SessionStatus = "success"
	SessionError   SessionStatus = "error"
)

type SessionRecord struct {
	Provider        string        `json:"provider"`
	State           string        `json:"state"`
	Status          SessionStatus `json:"status"`
	CodeVerifier    string        `json:"codeVerifier"`
	RedirectURI     string        `json:"redirectUri"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
	ExpiresAt       time.Time     `json:"expiresAt"`
	AccountID       int64         `json:"accountId,omitempty"`
	SiteID          int64         `json:"siteId,omitempty"`
	Error           string        `json:"error,omitempty"`
	RebindAccountID int64         `json:"rebindAccountId,omitempty"`
	ProjectID       string        `json:"projectId,omitempty"`
	ProxyURL        string        `json:"proxyUrl,omitempty"`
	UseSystemProxy  bool          `json:"useSystemProxy,omitempty"`
}

// ---- Provider interface ----

type BuildAuthURLInput struct {
	State        string
	RedirectURI  string
	CodeVerifier string
	ProjectID    string
}

type ResolveRedirectURIInput struct {
	RequestOrigin string
}

type ExchangeCodeInput struct {
	Code         string
	State        string
	RedirectURI  string
	CodeVerifier string
	ProjectID    string
	ProxyURL     *string
}

type RefreshTokenInput struct {
	RefreshToken string
	OAuth        *RefreshOAuthContext
	ProxyURL     *string
}

type RefreshOAuthContext struct {
	ProjectID    string
	ProviderData map[string]interface{}
}

// SessionTokenInput carries the ChatGPT web session token used by the
// session-cookie fallback when a refresh token has been revoked but the web
// session is still valid.
type SessionTokenInput struct {
	SessionToken string
	ProxyURL     *string
}

// OAuthProviderDefinition defines the interface for an OAuth provider.
type OAuthProviderDefinition struct {
	Metadata ProviderMetadata `json:"metadata"`
	Site     ProviderSiteConfig `json:"site"`
	Loopback LoopbackConfig   `json:"loopback"`

	BuildAuthorizationURL     func(ctx context.Context, input BuildAuthURLInput) (string, error)
	ResolveRedirectURI        func(ctx context.Context, input ResolveRedirectURIInput) (string, error)
	ExchangeAuthorizationCode func(ctx context.Context, input ExchangeCodeInput) (*TokenSet, error)
	RefreshAccessToken        func(ctx context.Context, input RefreshTokenInput) (*TokenSet, error)
	BuildProxyHeaders         func(ctx context.Context, input ProxyHeaderInput) map[string]string

	// RefreshWithSessionToken is an optional capability: when non-nil, the
	// refresh orchestrator falls back to it after a non-retryable RT refresh
	// failure (e.g. invalid_grant). Providers that do not support a web
	// session fallback leave this nil.
	RefreshWithSessionToken func(ctx context.Context, input SessionTokenInput) (*TokenSet, error)

	// ParseAccessToken is an optional capability: when non-nil, the refresh
	// orchestrator uses it to recover identity fields (email, account id,
	// plan type) that the id_token omitted, by parsing the access_token JWT.
	// Providers whose access tokens are not JWTs or carry no identity leave
	// this nil.
	ParseAccessToken func(accessToken string) *AccountIdentity
}
