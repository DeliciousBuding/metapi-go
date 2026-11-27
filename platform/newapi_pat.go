package platform

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const newAPIPATVerificationScope = "access_token.generate"
const newAPIManualPATImport = "complete verification in New API and import a durable dashboard PAT manually"

// These scopes cover the Metapi account's profile/model discovery, balance and
// check-in, and relay API-key listing/reveal/management. No admin grant is needed.
var newAPIMetapiPATScopes = []string{
	"api_key:read", "api_key:reveal", "api_key:write", "profile:read", "wallet:read", "wallet:write",
}

// promoteV1LoginCredential turns a short-lived New API v1 session JWT into the
// durable dashboard PAT that Metapi can safely persist. There is deliberately
// no refresh-token subsystem here: New API already owns the durable credential,
// and using it removes a lifecycle rather than adding one.
//
// Legacy New API rotates its dashboard PAT through GET /api/user/token. Current
// New API removes that route and uses scoped PATs instead. An owned stored PAT
// is reused before any new token is requested, avoiding quota exhaustion on
// repeated logins.
func (n *NewApiAdapter) promoteV1LoginCredential(
	ctx context.Context,
	baseURL, sessionJWT, password, cookieHeader, sessionID, existing string,
	platformUserID *int,
	proxy *ProxyConfig,
) (string, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	sessionID = strings.TrimSpace(sessionID)
	headers := n.authHeaders(sessionJWT, platformUserID)
	if sessionID != "" {
		headers["X-Auth-Session"] = sessionID
		// Refresh cookies are scoped to /api/user/auth, not /api/verify or
		// /api/user/token. Keep them for logout; the JWT authenticates the
		// proof ceremony. Revoke failed promotions as well, so rejected
		// step-up attempts do not exhaust the upstream login-session quota.
		logoutHeaders := n.authHeaders(sessionJWT, platformUserID)
		logoutHeaders["X-Auth-Session"] = sessionID
		if strings.TrimSpace(cookieHeader) != "" {
			logoutHeaders["Cookie"] = cookieHeader
		}
		if origin, err := url.Parse(baseURL); err == nil {
			logoutHeaders["Origin"] = origin.Scheme + "://" + origin.Host
		}
		defer func() {
			_, err := fetchNewAPIV1SessionJSON(ctx, baseURL+"/api/user/auth/logout", http.MethodPost, nil, logoutHeaders, proxy)
			if err != nil {
				// Only the fixed diagnostic/status is safe to log. An
				// upstream error body can echo passwords, proofs or tokens.
				slog.Warn("new-api login: transient session logout failed", "error", err)
			}
		}()
	} else {
		slog.Warn("new-api login: upstream returned no session id; transient session could not be revoked")
	}

	catalog, catalogErr := fetchNewAPIV1SessionJSON(ctx, baseURL+"/api/user/access_tokens/catalog", http.MethodGet, nil, headers, proxy)
	if catalogErr == nil {
		data, _ := getMap(catalog.Parsed, "data")
		if _, ok := data["groups"].([]interface{}); !ok {
			return "", fmt.Errorf("login succeeded, but New API returned an invalid scoped PAT catalog; %s", newAPIManualPATImport)
		}
		return n.scopedV1LoginCredential(ctx, baseURL, password, existing, sessionJWT, headers, proxy)
	}
	// Older New API has no scoped PAT catalog. Some versions serve 404; on
	// others the path is caught by a protected /api/user/:id route. Only those
	// recognized missing-route shapes permit the legacy rotating-token path.
	catalogCode, _ := getString(catalog.Parsed, "code")
	legacyCatalog := catalog.Status == http.StatusNotFound || catalog.Status == http.StatusMethodNotAllowed ||
		(catalog.Status == http.StatusForbidden && catalogCode == "AUTH_INSUFFICIENT_PRIVILEGE") ||
		(catalog.Status == http.StatusOK && catalog.Parsed == nil && strings.HasPrefix(catalog.ContentType, "text/html"))
	if !legacyCatalog {
		return "", fmt.Errorf("login succeeded, but New API scoped PAT capability could not be checked: %w; %s", catalogErr, newAPIManualPATImport)
	}

	answer, err := fetchNewAPIV1SessionJSON(ctx, baseURL+"/api/user/token", http.MethodGet, nil, headers, proxy)
	var proof string
	if err != nil {
		if !requiresNewAPISecurityProof(answer) {
			return "", fmt.Errorf("login succeeded, but New API could not issue a durable dashboard token: %w; %s", err, newAPIManualPATImport)
		}
		proof, err = newAPIV1PasswordProof(ctx, baseURL, password, nil, headers, proxy)
		if err != nil {
			return "", fmt.Errorf("login succeeded, but New API security verification failed: %w; %s", err, newAPIManualPATImport)
		}
		// The proof is single-use and bound to this session and operation.
		// Retry PAT issuance exactly once, never verify/replay in a loop.
		headers["X-Security-Proof"] = proof
		answer, err = fetchNewAPIV1SessionJSON(ctx, baseURL+"/api/user/token", http.MethodGet, nil, headers, proxy)
		delete(headers, "X-Security-Proof")
		if err != nil {
			return "", fmt.Errorf("login succeeded, but New API could not issue a durable dashboard token after security verification: %w; %s", err, newAPIManualPATImport)
		}
	}
	durableToken, _ := getString(answer.Parsed, "data")
	durableToken = strings.TrimSpace(durableToken)
	if durableToken == "" || durableToken == sessionJWT || durableToken == proof {
		return "", fmt.Errorf("login succeeded, but New API returned no durable dashboard PAT; %s", newAPIManualPATImport)
	}
	// Logout remains best-effort: never discard a PAT already issued because
	// session revocation failed. The deferred cleanup uses the session JWT,
	// never the PAT or the consumed security proof.
	return durableToken, nil
}

func (n *NewApiAdapter) scopedV1LoginCredential(ctx context.Context, baseURL, password, existing, sessionJWT string, headers map[string]string, proxy *ProxyConfig) (string, error) {
	if strings.HasPrefix(existing, "nap_") {
		usable, err := n.ownedScopedPAT(ctx, baseURL, existing, headers, proxy)
		if err != nil {
			return "", fmt.Errorf("login succeeded, but existing New API PAT could not be reused: %w; %s", err, newAPIManualPATImport)
		}
		if usable {
			return existing, nil
		}
	}

	request := map[string]interface{}{"name": "Metapi", "scopes": newAPIMetapiPATScopes, "expires_at": 0}
	answer, err := fetchNewAPIV1SessionJSON(ctx, baseURL+"/api/user/access_tokens", http.MethodPost, request, headers, proxy)
	var proof string
	if err != nil {
		if !requiresNewAPISecurityProof(answer) {
			return "", fmt.Errorf("login succeeded, but New API could not create a scoped PAT: %w; %s", err, newAPIManualPATImport)
		}
		context := map[string]interface{}{"scopes": newAPIMetapiPATScopes, "expires_at": 0}
		proof, err = newAPIV1PasswordProof(ctx, baseURL, password, context, headers, proxy)
		if err != nil {
			return "", fmt.Errorf("login succeeded, but New API security verification failed: %w; %s", err, newAPIManualPATImport)
		}
		headers["X-Security-Proof"] = proof
		answer, err = fetchNewAPIV1SessionJSON(ctx, baseURL+"/api/user/access_tokens", http.MethodPost, request, headers, proxy)
		delete(headers, "X-Security-Proof")
		if err != nil {
			return "", fmt.Errorf("login succeeded, but New API rejected scoped PAT creation after verification: %w; %s", err, newAPIManualPATImport)
		}
	}
	data, _ := getMap(answer.Parsed, "data")
	token, _ := getString(data, "token")
	token = strings.TrimSpace(token)
	if !strings.HasPrefix(token, "nap_") || token == sessionJWT || token == proof {
		return "", fmt.Errorf("login succeeded, but New API returned no durable scoped PAT; %s", newAPIManualPATImport)
	}
	return token, nil
}

// The listing exposes only token fingerprints, never plaintext. Compare the
// stored candidate's fingerprint and require every scope Metapi actually uses.
func (n *NewApiAdapter) ownedScopedPAT(ctx context.Context, baseURL, existing string, headers map[string]string, proxy *ProxyConfig) (bool, error) {
	answer, err := fetchNewAPIV1SessionJSON(ctx, baseURL+"/api/user/access_tokens", http.MethodGet, nil, headers, proxy)
	if err != nil {
		return false, err
	}
	data, _ := getMap(answer.Parsed, "data")
	items, ok := data["items"].([]interface{})
	if !ok {
		return false, fmt.Errorf("upstream returned no token listing")
	}
	digest := sha256.Sum256([]byte(existing))
	wantRef := hex.EncodeToString(digest[:])
	for _, raw := range items {
		item, ok := raw.(map[string]interface{})
		if !ok {
			return false, fmt.Errorf("upstream returned an invalid token listing")
		}
		ref, _ := getString(item, "token_ref")
		if ref != wantRef {
			continue
		}
		expires, ok := getFloat(item, "expires_at")
		if !ok {
			return false, fmt.Errorf("existing PAT has no readable expiry")
		}
		if expires != 0 && expires <= float64(time.Now().Unix()) {
			return false, nil
		}
		rawScopes, ok := item["scopes"].([]interface{})
		if !ok {
			return false, fmt.Errorf("existing PAT has no readable scopes")
		}
		granted := make(map[string]bool, len(rawScopes))
		for _, rawScope := range rawScopes {
			scope, ok := rawScope.(string)
			if !ok {
				return false, fmt.Errorf("existing PAT has invalid scopes")
			}
			granted[scope] = true
		}
		for _, scope := range newAPIMetapiPATScopes {
			if !granted[scope] {
				return false, fmt.Errorf("existing PAT lacks required scopes; update its grant or import a suitable PAT")
			}
		}
		return true, nil
	}
	return false, nil
}

// newAPIV1PasswordProof implements New API's advertised password ceremony for
// access_token.generate. MFA and encrypted-password flows are not implemented
// here; require manual PAT import rather than downgrading to plaintext.
func newAPIV1PasswordProof(ctx context.Context, baseURL, password string, operationContext map[string]interface{}, headers map[string]string, proxy *ProxyConfig) (string, error) {
	answer, err := fetchNewAPIV1SessionJSON(ctx, baseURL+"/api/verify/methods?scope="+newAPIPATVerificationScope, http.MethodGet, nil, headers, proxy)
	if err != nil {
		return "", fmt.Errorf("could not obtain password verification methods: %w", err)
	}
	data, _ := getMap(answer.Parsed, "data")
	scope, _ := getString(data, "scope")
	if scope != newAPIPATVerificationScope {
		return "", fmt.Errorf("upstream returned unsupported verification requirements")
	}
	methods, _ := data["methods"].([]interface{})
	passwordAllowed := false
	for _, raw := range methods {
		option, _ := raw.(map[string]interface{})
		method, _ := getString(option, "method")
		available, _ := getBool(option, "available")
		if method == "password" && available {
			passwordAllowed = true
		}
	}
	if !passwordAllowed {
		return "", fmt.Errorf("upstream requires MFA/interactive verification or password verification is unavailable")
	}
	encrypted, hasEncryptionPolicy := getBool(data, "password_encryption_enabled")
	if !hasEncryptionPolicy || encrypted {
		return "", fmt.Errorf("upstream password encryption requirements are not supported by this login flow")
	}
	if password == "" {
		return "", fmt.Errorf("no password was supplied for verification")
	}
	body := map[string]interface{}{"method": "password", "scope": newAPIPATVerificationScope, "password": password}
	if operationContext != nil {
		body["context"] = operationContext
	}
	answer, err = fetchNewAPIV1SessionJSON(ctx, baseURL+"/api/verify", http.MethodPost, body, headers, proxy)
	if err != nil {
		return "", fmt.Errorf("password verification was rejected: %w", err)
	}
	data, _ = getMap(answer.Parsed, "data")
	proof, _ := getString(data, "proof_token")
	method, _ := getString(data, "method")
	scope, _ = getString(data, "scope")
	expires, _ := getFloat(data, "expires_at")
	if strings.TrimSpace(proof) == "" || method != "password" || scope != newAPIPATVerificationScope || expires <= float64(time.Now().Unix()) {
		return "", fmt.Errorf("upstream returned no valid password verification proof")
	}
	return proof, nil
}

// fetchNewAPIV1SessionJSON retains the status and structured error code for
// step-up detection, but never exposes upstream bodies or transport errors
// (which may contain credentials/URLs) to a login result or a log entry.
func fetchNewAPIV1SessionJSON(ctx context.Context, url, method string, body interface{}, headers map[string]string, proxy *ProxyConfig) (loginResponse, error) {
	answer, err := fetchLoginSessionResponse(ctx, url, method, body, headers, proxy)
	if err != nil {
		return answer, fmt.Errorf("request failed")
	}
	if answer.Status < 200 || answer.Status >= 300 {
		return answer, fmt.Errorf("HTTP %d", answer.Status)
	}
	if answer.Parsed == nil {
		return answer, fmt.Errorf("invalid JSON response (HTTP %d)", answer.Status)
	}
	if success, _ := getBool(answer.Parsed, "success"); !success {
		return answer, fmt.Errorf("unsuccessful response (HTTP %d)", answer.Status)
	}
	return answer, nil
}

// Only the explicit machine contract permits a proof retry.
func requiresNewAPISecurityProof(answer loginResponse) bool {
	code, _ := getString(answer.Parsed, "code")
	success, hasSuccess := getBool(answer.Parsed, "success")
	return answer.Status == http.StatusForbidden && code == "SECURITY_PROOF_REQUIRED" && hasSuccess && !success
}
