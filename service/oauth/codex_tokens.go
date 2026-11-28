package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// codexCLIUserAgent mirrors the Codex CLI client fingerprint used by the
// upstream OAuth/token endpoints. Without it, OpenAI's auth gateway is
// more likely to classify requests as non-CLI traffic.
const codexCLIUserAgent = "codex-cli/0.91.0"

// codexTokenURL is a package var (not a const) so tests can swap in a local
// httptest server via withCodexEndpointSwap, mirroring the grok.go pattern.
var codexTokenURL = "https://auth.openai.com/oauth/token"

type codexJWTClaims struct {
	Email string `json:"email"`
	Auth  struct {
		ChatGPTAccountID string `json:"chatgpt_account_id"`
		ChatGPTPlanType  string `json:"chatgpt_plan_type"`
	} `json:"https://api.openai.com/auth"`
}

type codexTokenResponse struct {
	AccessToken  string      `json:"access_token"`
	RefreshToken string      `json:"refresh_token"`
	IDToken      string      `json:"id_token"`
	ExpiresIn    interface{} `json:"expires_in"`
}

func parseJWTClaims(token string) (*codexJWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT")
	}
	decoded, err := base64Decode(parts[1])
	if err != nil {
		return nil, err
	}
	var claims codexJWTClaims
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return nil, err
	}
	return &claims, nil
}

func parseExpiresIn(value interface{}) (int64, bool) {
	switch v := value.(type) {
	case float64:
		if v > 0 {
			return int64(v), true
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err == nil && parsed > 0 {
			return parsed, true
		}
	}
	return 0, false
}

func exchangeCodexToken(ctx context.Context, form url.Values, proxyURL *string, refresh bool) (*codexTokenResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", codexTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, refreshPreparationFailure(refresh, err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", codexCLIUserAgent)

	resp, err := doRefreshHTTP(req, proxyURL, refresh)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body := readOAuthErrorResponseBody(resp.Body)
		return nil, refreshHTTPFailure(ctx, refresh, resp.StatusCode, body, summarizeOAuthHTTPError("codex token exchange", resp.StatusCode, body))
	}

	body, err := readOAuthJSONResponseBody(resp.Body)
	if err != nil {
		return nil, successfulRefreshResponseError(refresh, "response read failed", err)
	}

	var payload codexTokenResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, successfulRefreshResponseError(refresh, "invalid payload", fmt.Errorf("codex token exchange returned invalid payload"))
	}

	accessToken := strings.TrimSpace(payload.AccessToken)
	refreshToken := strings.TrimSpace(payload.RefreshToken)
	idToken := strings.TrimSpace(payload.IDToken)
	expiresIn, hasExpiresIn := parseExpiresIn(payload.ExpiresIn)

	if accessToken == "" || (!refresh && (refreshToken == "" || idToken == "")) || !hasExpiresIn || expiresIn <= 0 {
		return nil, successfulRefreshResponseError(refresh, "missing required fields", fmt.Errorf("codex token exchange response missing required fields"))
	}
	return &payload, nil
}

func refreshCodexAccessToken(ctx context.Context, input RefreshTokenInput) (*TokenSet, error) {
	clientID := strings.TrimSpace(input.ClientID)
	if clientID == "" {
		var err error
		clientID, err = requireCodexClientID()
		if err != nil {
			return nil, refreshNotConsumed(err)
		}
	}
	form := url.Values{"grant_type": {"refresh_token"}, "client_id": {clientID}, "refresh_token": {input.RefreshToken}, "scope": {"openid profile email"}}
	if strings.TrimSpace(input.ClientID) != "" {
		form.Del("scope")
	}
	payload, err := exchangeCodexToken(ctx, form, input.ProxyURL, true)
	if err != nil {
		return nil, err
	}
	expiresIn, _ := parseExpiresIn(payload.ExpiresIn)
	tokens := &TokenSet{AccessToken: payload.AccessToken, RefreshToken: payload.RefreshToken, IDToken: payload.IDToken, TokenExpiresAt: time.Now().UnixMilli() + expiresIn*1000}
	identity := ParseCodexAccessToken(payload.AccessToken)
	if claims, err := parseJWTClaims(payload.IDToken); err == nil {
		identity = MergeCodexIdentity(&AccountIdentity{Email: claims.Email, ChatGPTAccountID: claims.Auth.ChatGPTAccountID, PlanType: claims.Auth.ChatGPTPlanType}, identity)
	}
	if identity != nil {
		tokens.Email = identity.Email
		tokens.AccountID = identity.ChatGPTAccountID
		tokens.AccountKey = identity.ChatGPTAccountID
		tokens.PlanType = identity.PlanType
	}
	if tokens.AccountID == "" {
		return nil, successfulRefreshResponseError(true, "missing chatgpt_account_id", fmt.Errorf("codex token refresh response missing chatgpt_account_id"))
	}
	return tokens, nil
}

// buildCodexTokenSetFromPayload converts a raw codex token response into a
// TokenSet, applying the access_token identity fallback codex2api uses: when
// the id_token omits email/account_id/plan_type, the access_token JWT (which
// surfaces email under the /profile namespace) recovers them. account_id is
// still required after the merge so an anonymous token fails loudly.
func buildCodexTokenSetFromPayload(payload *codexTokenResponse, origin string) (*TokenSet, error) {
	claims, err := parseJWTClaims(payload.IDToken)
	if err != nil {
		return nil, fmt.Errorf("codex %s response invalid id_token", origin)
	}

	identity := &AccountIdentity{
		Email:            strings.TrimSpace(claims.Email),
		ChatGPTAccountID: strings.TrimSpace(claims.Auth.ChatGPTAccountID),
		PlanType:         strings.TrimSpace(claims.Auth.ChatGPTPlanType),
	}
	if identityIncomplete(identity) {
		identity = MergeCodexIdentity(identity, ParseCodexAccessToken(payload.AccessToken))
	}

	accountID := strings.TrimSpace(identity.ChatGPTAccountID)
	if accountID == "" {
		return nil, fmt.Errorf("codex %s response missing chatgpt_account_id", origin)
	}

	expiresIn, _ := parseExpiresIn(payload.ExpiresIn)
	return &TokenSet{
		AccessToken:    payload.AccessToken,
		RefreshToken:   payload.RefreshToken,
		TokenExpiresAt: time.Now().UnixMilli() + expiresIn*1000,
		Email:          identity.Email,
		AccountID:      accountID,
		AccountKey:     accountID,
		PlanType:       identity.PlanType,
		IDToken:        payload.IDToken,
	}, nil
}

func base64Decode(encoded string) ([]byte, error) {
	// Handle standard and URL-safe base64.
	decoded, err := base64DecodeRaw(encoded)
	if err != nil {
		return nil, err
	}
	return decoded, nil
}

func base64DecodeRaw(encoded string) ([]byte, error) {
	// Add padding if needed.
	switch len(encoded) % 4 {
	case 2:
		encoded += "=="
	case 3:
		encoded += "="
	}
	return base64.URLEncoding.DecodeString(
		strings.NewReplacer("-", "+", "_", "/").Replace(encoded),
	)
}
