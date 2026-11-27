package oauth

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

// CallbackInput is the input to HandleCallback.
type CallbackInput struct {
	Provider string `json:"provider"`
	State    string `json:"state"`
	Code     string `json:"code,omitempty"`
	Error    string `json:"error,omitempty"`
}

// CallbackResult is the result of HandleCallback.
type CallbackResult struct {
	AccountID int64 `json:"accountId"`
	SiteID    int64 `json:"siteId"`
}

// SessionStatusResult is the result of GetSessionStatus.
type SessionStatusResult struct {
	Provider          string        `json:"provider"`
	State             string        `json:"state"`
	Status            SessionStatus `json:"status"`
	AccountID         int64         `json:"accountId,omitempty"`
	SiteID            int64         `json:"siteId,omitempty"`
	Error             string        `json:"error,omitempty"`
	RetryAfterSeconds int           `json:"retryAfterSeconds,omitempty"`
	CreatedAt         time.Time     `json:"createdAt"`
	UpdatedAt         time.Time     `json:"updatedAt"`
}

// ManualCallbackInput holds the input for manual callback submission.
type ManualCallbackInput struct {
	State       string `json:"state"`
	CallbackURL string `json:"callbackUrl"`
}

// GetSessionStatus returns the current status of an OAuth session by state.
// For pending Device Flow sessions it also performs one RFC 8628 token poll;
// authorization_pending and slow_down remain pending instead of failing the
// session. The caller's request context bounds the outbound poll.
func GetSessionStatus(ctx context.Context, state string) *SessionStatusResult {
	session := GetSession(state)
	if session == nil {
		return nil
	}
	if session.Status == SessionPending {
		def := GetProviderDefinition(session.Provider)
		if def != nil && def.Metadata.SupportsDeviceFlow {
			_ = pollDeviceFlowSession(ctx, session, def)
			session = GetSession(state)
			if session == nil {
				return nil
			}
		}
	}
	return sessionStatusResult(session)
}

func sessionStatusResult(session *SessionRecord) *SessionStatusResult {
	retryAfterSeconds := 0
	if session.Status == SessionPending {
		if entry, ok := lookupDeviceFlow(session.State); ok && entry.interval > 0 {
			retryAfterSeconds = maxInt(int(entry.interval/time.Second), 1)
		}
	}
	return &SessionStatusResult{
		Provider:          session.Provider,
		State:             session.State,
		Status:            session.Status,
		AccountID:         session.AccountID,
		SiteID:            session.SiteID,
		Error:             session.Error,
		RetryAfterSeconds: retryAfterSeconds,
		CreatedAt:         session.CreatedAt,
		UpdatedAt:         session.UpdatedAt,
	}
}

func pollDeviceFlowSession(ctx context.Context, session *SessionRecord, def *OAuthProviderDefinition) error {
	token, err := def.ExchangeAuthorizationCode(ctx, ExchangeCodeInput{
		State:    session.State,
		ProxyURL: resolveOAuthSessionProxyURL(session),
	})
	if err != nil {
		if isDeviceFlowPending(err) {
			return err
		}
		MarkSessionError(session.State, err.Error())
		return err
	}

	_, err = completeOAuthSession(ctx, session.State, session, def, token)
	return err
}

// HandleCallback processes an OAuth callback.
func HandleCallback(input CallbackInput) (*CallbackResult, error) {
	session := GetSession(input.State)
	if session == nil || session.Provider != input.Provider {
		return nil, fmt.Errorf("oauth session not found or provider mismatch")
	}

	def := GetProviderDefinition(input.Provider)
	if def == nil {
		MarkSessionError(input.State, fmt.Sprintf("unsupported oauth provider: %s", input.Provider))
		return nil, fmt.Errorf("unsupported oauth provider: %s", input.Provider)
	}

	if input.Error != "" {
		MarkSessionError(input.State, input.Error)
		return nil, fmt.Errorf("%s", input.Error)
	}

	code := strings.TrimSpace(input.Code)
	if code == "" {
		MarkSessionError(input.State, "missing oauth code")
		return nil, fmt.Errorf("missing oauth code")
	}

	token, err := def.ExchangeAuthorizationCode(context.Background(), ExchangeCodeInput{
		Code:         code,
		State:        input.State,
		RedirectURI:  session.RedirectURI,
		CodeVerifier: session.CodeVerifier,
		ProjectID:    session.ProjectID,
		ProxyURL:     resolveOAuthSessionProxyURL(session),
	})
	if err != nil {
		MarkSessionError(input.State, err.Error())
		return nil, err
	}

	return completeOAuthSession(context.Background(), input.State, session, def, token)
}

func completeOAuthSession(ctx context.Context, state string, session *SessionRecord, def *OAuthProviderDefinition, token *TokenSet) (*CallbackResult, error) {
	persistResult, err := activatePersistedOAuthAccount(ctx, session.db, ActivateInput{
		Definition:      def,
		Exchange:        token,
		RebindAccountID: session.RebindAccountID,
		ProxyURL:        session.ProxyURL,
		UseSystemProxy:  session.UseSystemProxy,
	})
	if err != nil {
		MarkSessionError(state, err.Error())
		return nil, err
	}

	MarkSessionSuccess(state, persistResult.AccountID, persistResult.SiteID)
	return &CallbackResult{
		AccountID: persistResult.AccountID,
		SiteID:    persistResult.SiteID,
	}, nil
}

// resolveOAuthSessionProxyURL keeps the three-tier fallback in one owner:
// explicit session proxy, then system proxy, then provider default.
func resolveOAuthSessionProxyURL(session *SessionRecord) *string {
	if session.ProxyURL != "" {
		return &session.ProxyURL
	}
	if session.UseSystemProxy {
		systemProxy := strings.TrimSpace(config.Runtime().SystemProxyUrl)
		if systemProxy != "" {
			return &systemProxy
		}
	}
	return resolveOauthProviderProxyUrl(session.Provider)
}

// SubmitManualCallback handles a manual callback URL submission.
func SubmitManualCallback(input ManualCallbackInput) error {
	session := GetSession(input.State)
	if session == nil {
		return fmt.Errorf("oauth session not found")
	}

	parsed, err := parseManualCallbackURL(input.CallbackURL)
	if err != nil {
		return err
	}

	if parsed.State != input.State {
		return fmt.Errorf("oauth callback state mismatch")
	}

	_, err = HandleCallback(CallbackInput{
		Provider: session.Provider,
		State:    parsed.State,
		Code:     parsed.Code,
		Error:    parsed.Error,
	})
	return err
}

type manualCallbackParsed struct {
	State string
	Code  string
	Error string
}

func parseManualCallbackURL(callbackURL string) (*manualCallbackParsed, error) {
	raw := strings.TrimSpace(callbackURL)
	if raw == "" {
		return nil, fmt.Errorf("invalid oauth callback url")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid oauth callback url")
	}
	state := strings.TrimSpace(parsed.Query().Get("state"))
	code := strings.TrimSpace(parsed.Query().Get("code"))
	errorParam := strings.TrimSpace(parsed.Query().Get("error"))
	errorDesc := strings.TrimSpace(parsed.Query().Get("error_description"))

	if state == "" || (code == "" && errorParam == "") {
		return nil, fmt.Errorf("invalid oauth callback url")
	}

	errStr := errorParam
	if errStr != "" && errorDesc != "" {
		errStr = errStr + ": " + errorDesc
	}

	return &manualCallbackParsed{
		State: state,
		Code:  code,
		Error: errStr,
	}, nil
}

// resolveOauthProviderProxyUrl returns the provider-specific default proxy URL.
// Each provider may have its own proxy configuration.
func resolveOauthProviderProxyUrl(provider string) *string {
	rt := config.Runtime()
	switch provider {
	case "codex":
		// Codex provider uses system proxy if configured.
		if rt.SystemProxyUrl != "" {
			return &rt.SystemProxyUrl
		}
	case "claude":
		if rt.SystemProxyUrl != "" {
			return &rt.SystemProxyUrl
		}
	case "gemini-cli":
		if rt.SystemProxyUrl != "" {
			return &rt.SystemProxyUrl
		}
	case "antigravity":
		if rt.SystemProxyUrl != "" {
			return &rt.SystemProxyUrl
		}
	}
	return nil
}
