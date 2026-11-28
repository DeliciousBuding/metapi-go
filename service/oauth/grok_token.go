package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// grokTokenResponse models the xAI /oauth2/token response. The same shape is
// reused for device-code grant and refresh-token grant.
type grokTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	// Error / RFC 8628 polling error fields.
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

// grokIDTokenClaims is the subset of JWT claims we read from the xAI id_token.
type grokIDTokenClaims struct {
	Email string `json:"email"`
	Sub   string `json:"sub"`
	Name  string `json:"name"`
}

func refreshGrokAccessToken(ctx context.Context, input RefreshTokenInput) (*TokenSet, error) {
	refreshToken := strings.TrimSpace(input.RefreshToken)
	if refreshToken == "" {
		return nil, fmt.Errorf("grok oauth: missing refresh_token")
	}

	callCtx, cancel := context.WithTimeout(ctx, grokHTTPTimeout)
	defer cancel()

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", grokClientID)
	form.Set("refresh_token", refreshToken)

	payload, err := postOAuthForm(callCtx, grokTokenURL, form, input.ProxyURL)
	if err != nil {
		return nil, fmt.Errorf("grok oauth: refresh request failed: %w", err)
	}

	var token grokTokenResponse
	if err := json.Unmarshal(payload, &token); err != nil {
		return nil, successfulRefreshResponseError(true, "invalid payload", fmt.Errorf("grok oauth: invalid refresh payload: %w", err))
	}
	if token.Error != "" {
		msg := token.Error
		if token.ErrorDescription != "" {
			msg = msg + ": " + token.ErrorDescription
		}
		return nil, refreshNotConsumed(fmt.Errorf("grok oauth: %s", msg))
	}
	if strings.TrimSpace(token.AccessToken) == "" {
		return nil, successfulRefreshResponseError(true, "missing access token", fmt.Errorf("grok oauth: refresh response missing access_token"))
	}

	expiresIn := token.ExpiresIn
	if expiresIn <= 0 {
		expiresIn = 3600
	}

	email, sub := parseGrokIDToken(token.IDToken)
	accountKey := email
	if accountKey == "" {
		accountKey = sub
	}
	// Preserve provider data from the previous token set when the refresh
	// response omits it.
	if input.OAuth != nil && input.OAuth.ProviderData != nil {
		if _, ok := providerDataGet(input.OAuth.ProviderData, "scope"); !ok && token.Scope != "" {
			providerDataSet(input.OAuth.ProviderData, "scope", token.Scope)
		}
		merged := mergeGrokProviderData(input.OAuth.ProviderData, token)
		return &TokenSet{
			AccessToken:    token.AccessToken,
			RefreshToken:   firstGrokNonEmpty(token.RefreshToken, refreshToken),
			TokenExpiresAt: time.Now().UnixMilli() + int64(expiresIn)*1000,
			Email:          email,
			AccountKey:     accountKey,
			AccountID:      sub,
			IDToken:        token.IDToken,
			ProviderData:   merged,
		}, nil
	}

	providerData := map[string]interface{}{}
	if token.TokenType != "" {
		providerData["tokenType"] = token.TokenType
	}
	if token.Scope != "" {
		providerData["scope"] = token.Scope
	}
	if token.IDToken != "" {
		providerData["idToken"] = token.IDToken
	}

	return &TokenSet{
		AccessToken:    token.AccessToken,
		RefreshToken:   firstGrokNonEmpty(token.RefreshToken, refreshToken),
		TokenExpiresAt: time.Now().UnixMilli() + int64(expiresIn)*1000,
		Email:          email,
		AccountKey:     accountKey,
		AccountID:      sub,
		IDToken:        token.IDToken,
		ProviderData:   providerData,
	}, nil
}

// parseGrokIDToken decodes the JWT id_token returned by xAI and extracts the
// email and sub claims. Returns empty strings (no error) when the id_token is
// absent or not a parseable JWT — callers treat absence as a soft fallback.
func parseGrokIDToken(idToken string) (email, sub string) {
	idToken = strings.TrimSpace(idToken)
	if idToken == "" {
		return "", ""
	}
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return "", ""
	}
	decoded, err := base64Decode(parts[1])
	if err != nil {
		return "", ""
	}
	var claims grokIDTokenClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return "", ""
	}
	return strings.TrimSpace(claims.Email), strings.TrimSpace(claims.Sub)
}

func firstGrokNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func providerDataGet(data map[string]interface{}, key string) (interface{}, bool) {
	if data == nil {
		return nil, false
	}
	value, ok := data[key]
	return value, ok
}

func providerDataSet(data map[string]interface{}, key string, value interface{}) {
	if data == nil {
		return
	}
	data[key] = value
}

func mergeGrokProviderData(existing map[string]interface{}, token grokTokenResponse) map[string]interface{} {
	merged := make(map[string]interface{})
	for key, value := range existing {
		merged[key] = value
	}
	if token.TokenType != "" {
		merged["tokenType"] = token.TokenType
	}
	if token.Scope != "" {
		merged["scope"] = token.Scope
	}
	if token.IDToken != "" {
		merged["idToken"] = token.IDToken
	}
	return merged
}
