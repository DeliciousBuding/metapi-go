package admin

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// ---- Login Account ----

func (h *accountsHandler) loginAccount(w http.ResponseWriter, r *http.Request) {
	var body payloads.AccountLoginPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid login payload: "+err.Error())
		return
	}

	if body.SiteID <= 0 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid siteId. Expected positive number.")
		return
	}
	if strings.TrimSpace(body.Username) == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid username. Expected string.")
		return
	}
	if strings.TrimSpace(body.Password) == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid password. Expected string.")
		return
	}

	// Get site
	var site store.Site
	if err := h.db.Get(&site, h.db.Rebind("SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?"), body.SiteID); err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeSiteNotFound, "site not found")
		return
	}

	adp := platform.GetAdapter(site.Platform)
	if adp == nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "unsupported platform: "+site.Platform)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	loginResult, err := adp.Login(ctx, site.URL, body.Username, body.Password, nil, service.BuildPlatformProxyConfigForToken(h.cfg, &site, body.Username))
	if err != nil {
		slog.Warn("Account login failed", "err", err, "site_id", site.ID, "platform", site.Platform)
		writeErrorWithRequest(w, r, http.StatusUnauthorized, "login failed")
		return
	}
	if loginResult == nil || !loginResult.Success || strings.TrimSpace(loginResult.AccessToken) == "" {
		message := "login failed"
		if loginResult != nil && strings.TrimSpace(loginResult.Message) != "" {
			message = strings.TrimSpace(loginResult.Message)
		}
		writeErrorWithRequest(w, r, http.StatusUnauthorized, message)
		return
	}
	loginAccessToken := strings.TrimSpace(loginResult.AccessToken)

	// Check for existing account (reusedAccount)
	var existing store.Account
	reused := false
	err = h.db.Get(&existing,
		h.db.Rebind("SELECT * FROM accounts WHERE site_id = ? AND username = ?"),
		body.SiteID, body.Username,
	)
	if err == nil {
		reused = true
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Encrypt password for autoRelogin
	passwordCipher, encErr := service.EncryptPassword(h.cfg, body.Password)
	if encErr != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "Failed to encrypt password.")
		return
	}

	extraConfig := map[string]any{
		"credentialMode": "session",
		"autoRelogin": map[string]any{
			"username":       body.Username,
			"passwordCipher": passwordCipher,
			"updatedAt":      now,
		},
	}

	extraConfigStr := service.MarshalExtraConfig(extraConfig)

	if reused {
		// Update existing account
		if _, err := h.db.Exec(
			h.db.Rebind(`UPDATE accounts SET access_token = ?, checkin_enabled = ?, status = 'active',
			 extra_config = ?, updated_at = ? WHERE id = ?`),
			loginAccessToken, true, extraConfigStr, now, existing.ID,
		); err != nil {
			slog.Error("Failed to update login account", "err", err, "account_id", existing.ID)
			writeErrorWithRequest(w, r, http.StatusInternalServerError, "Failed to save account.")
			return
		}
	} else {
		sortOrder, _ := service.GetNextAccountSortOrder(h.db)
		if _, err := h.db.Exec(
			h.db.Rebind(`INSERT INTO accounts (site_id, username, access_token, checkin_enabled, status,
			 is_pinned, sort_order, extra_config, created_at, updated_at)
			 VALUES (?, ?, ?, ?, 'active', ?, ?, ?, ?, ?)`),
			body.SiteID, body.Username, loginAccessToken, true, false, sortOrder, extraConfigStr, now, now,
		); err != nil {
			slog.Error("Failed to insert login account", "err", err, "site_id", body.SiteID)
			writeErrorWithRequest(w, r, http.StatusInternalServerError, "Failed to save account.")
			return
		}
	}

	// Fetch the created/updated account for response
	var loginAcct store.Account
	if err := h.db.Get(&loginAcct, h.db.Rebind("SELECT * FROM accounts WHERE site_id = ? AND username = ?"), body.SiteID, body.Username); err != nil {
		slog.Error("Failed to load login account", "err", err, "site_id", body.SiteID)
		writeErrorCodeWithRequest(w, r, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "Failed to load account.")
		return
	}
	loginAcctMap := map[string]any{
		"id":             loginAcct.ID,
		"siteId":         loginAcct.SiteID,
		"username":       loginAcct.Username,
		"accessToken":    loginAcct.AccessToken,
		"apiToken":       loginAcct.APIToken,
		"balance":        loginAcct.Balance,
		"status":         loginAcct.Status,
		"isPinned":       loginAcct.IsPinned,
		"sortOrder":      loginAcct.SortOrder,
		"checkinEnabled": loginAcct.CheckinEnabled,
		"extraConfig":    loginAcct.ExtraConfig,
		"createdAt":      loginAcct.CreatedAt,
		"updatedAt":      loginAcct.UpdatedAt,
	}
	// Auto-sync upstream tokens for the session-credential account (#1002);
	// report the real persisted count instead of a fixed tokenCount.
	syncReport := syncTokensAfterAccountCreate(r.Context(), h.db, h.cfg, loginAcct.ID)

	// A successful login is the only moment we know the control credential is
	// fresh. Discover and persist models now instead of leaving a brand-new
	// account at totalCount=0 until the periodic scheduler happens to run. That
	// empty interval made manual route creation produce channels with no usable
	// model and was the user-visible #1179 failure. The refresh is reported
	// honestly but remains partial initialization: the verified account and its
	// synced relay keys stay saved when a particular upstream cannot list models.
	modelRefresh := accountModelRefresher(r.Context(), h.db, loginAcct.ID, false)

	routing.InvalidateCache()
	globalAccountsCache.clear()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":          true,
		"account":          loginAcctMap,
		"apiTokenFound":    loginAcct.APIToken != nil,
		"tokenCount":       syncReport.TokenCount,
		"tokenSyncStatus":  syncReport.Status,
		"tokenSyncMessage": syncReport.Message,
		"modelRefresh":     modelRefresh,
		"reusedAccount":    reused,
	})
}

// ---- Verify Token ----

func (h *accountsHandler) verifyToken(w http.ResponseWriter, r *http.Request) {
	var body payloads.AccountVerifyTokenPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid verify-token payload: "+err.Error())
		return
	}

	if body.SiteID <= 0 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid siteId. Expected positive number.")
		return
	}

	accessToken := ""
	if body.AccessToken != nil {
		accessToken = strings.TrimSpace(*body.AccessToken)
	}
	if accessToken == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "token must not be empty")
		return
	}

	if body.ProxyURL != nil {
		normalizedProxyURL := strings.TrimSpace(*body.ProxyURL)
		if normalizedProxyURL != "" && !service.IsValidProxyURL(normalizedProxyURL) {
			writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid proxyUrl. Expected a valid http(s)/socks proxy URL.")
			return
		}
		body.ProxyURL = &normalizedProxyURL
	}

	// Get site
	var site store.Site
	if err := h.db.Get(&site, h.db.Rebind("SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?"), body.SiteID); err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeSiteNotFound, "site not found")
		return
	}

	adp := platform.GetAdapter(site.Platform)
	if adp == nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "unsupported platform: "+site.Platform)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	result, err := adp.VerifyToken(ctx, site.URL, accessToken, body.PlatformUserID, service.BuildPlatformProxyConfigForTokenWithProxyURL(h.cfg, &site, accessToken, coalescePtr(body.ProxyURL, "")))
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if result == nil || result.TokenType == "" || result.TokenType == "unknown" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "token verification failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":       true,
		"tokenType":     result.TokenType,
		"modelCount":    len(result.Models),
		"models":        result.Models,
		"userInfo":      result.UserInfo,
		"balance":       result.Balance,
		"apiToken":      result.APIToken,
		"apiTokenFound": result.APIToken != "",
	})
}

func shouldMirrorAPIKeyToken(account store.Account, requestedAPIToken any) bool {
	if service.ResolveStoredCredentialMode(&account) != service.CredentialModeAPIKey {
		return false
	}
	if requestedAPIToken == nil {
		return true
	}
	requested, ok := requestedAPIToken.(string)
	if !ok {
		return false
	}
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return true
	}
	current := ""
	if account.APIToken != nil {
		current = strings.TrimSpace(*account.APIToken)
	}
	return requested == current
}

func normalizeAPITokenUpdate(value any) any {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return value
}

func normalizeAccountStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "active", "disabled", "expired":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return ""
	}
}

// shouldRecoverExpiredAPIKey detects the expired API-key credential recovery path:
// previous status expired, next mode apikey, credentials changed, next status not forced disabled.
func shouldRecoverExpiredAPIKey(prev, next store.Account, updates map[string]any) bool {
	if !strings.EqualFold(strings.TrimSpace(prev.Status), "expired") {
		return false
	}
	nextStatus := strings.TrimSpace(prev.Status)
	if status, ok := updates["status"].(string); ok && strings.TrimSpace(status) != "" {
		nextStatus = strings.TrimSpace(status)
	}
	if strings.EqualFold(nextStatus, "disabled") {
		return false
	}
	mode := service.ResolveStoredCredentialMode(&next)
	if mode != service.CredentialModeAPIKey {
		return false
	}
	return credentialFieldsChanged(prev, updates)
}

func credentialFieldsChanged(prev store.Account, updates map[string]any) bool {
	if accessToken, ok := updates["accessToken"].(string); ok {
		if strings.TrimSpace(accessToken) != strings.TrimSpace(prev.AccessToken) {
			return true
		}
	}
	if apiTokenRaw, ok := updates["apiToken"]; ok {
		prevAPI := ""
		if prev.APIToken != nil {
			prevAPI = strings.TrimSpace(*prev.APIToken)
		}
		switch v := apiTokenRaw.(type) {
		case string:
			if strings.TrimSpace(v) != prevAPI {
				return true
			}
		case *string:
			nextAPI := ""
			if v != nil {
				nextAPI = strings.TrimSpace(*v)
			}
			if nextAPI != prevAPI {
				return true
			}
		default:
			if v != nil {
				return true
			}
		}
	}
	return false
}

// clearAccountAuthRuntimeHealth removes auth-sourced runtimeHealth best-effort after recovery.
func clearAccountAuthRuntimeHealth(db *sqlx.DB, accountID int64) error {
	var extraConfig *string
	if err := db.Get(&extraConfig, db.Rebind("SELECT extra_config FROM accounts WHERE id = ?"), accountID); err != nil {
		return err
	}
	cfg := service.ParseExtraConfig(extraConfig)
	if cfg == nil {
		return nil
	}
	healthRaw, ok := cfg["runtimeHealth"]
	if !ok || healthRaw == nil {
		return nil
	}
	b, err := json.Marshal(healthRaw)
	if err != nil {
		return nil
	}
	var entry service.RuntimeHealthEntry
	if err := json.Unmarshal(b, &entry); err != nil {
		return nil
	}
	if strings.ToLower(strings.TrimSpace(string(entry.Source))) != string(service.HealthSourceAuth) {
		return nil
	}
	merged := service.MergeExtraConfig(extraConfig, map[string]any{"runtimeHealth": nil})
	now := time.Now().UTC().Format(time.RFC3339)
	if merged == nil {
		_, err = db.Exec(db.Rebind("UPDATE accounts SET extra_config = NULL, updated_at = ? WHERE id = ?"), now, accountID)
		return err
	}
	_, err = db.Exec(db.Rebind("UPDATE accounts SET extra_config = ?, updated_at = ? WHERE id = ?"), *merged, now, accountID)
	return err
}

// ---- Rebind Session ----

func (h *accountsHandler) rebindSession(w http.ResponseWriter, r *http.Request) {
	accountID, ok := pathID(w, r)
	if !ok {
		return
	}

	var body payloads.AccountRebindSessionPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid rebind payload: "+err.Error())
		return
	}

	nextAccessToken := ""
	if body.AccessToken != nil {
		nextAccessToken = strings.TrimSpace(*body.AccessToken)
	}
	if nextAccessToken == "" {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "provide a new session token")
		return
	}

	row, err := service.GetAccountWithSiteByID(h.db, accountID)
	if err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeAccountNotFound, "account not found")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Sub2API managed auth: merge refreshToken/tokenExpiresAt into extraConfig.sub2apiAuth
	// without clobbering unrelated keys (spec p3-sites-accounts lines 528-542).
	extraConfigPatch := map[string]any{
		"credentialMode": "session",
	}
	if service.IsSub2ApiPlatform(row.Site.Platform) {
		if mergedAuth := service.BuildMergedSub2ApiAuth(row.Account.ExtraConfig, body.RefreshToken, body.TokenExpiresAt, nil); mergedAuth != nil {
			extraConfigPatch["sub2apiAuth"] = mergedAuth
		}
	}
	extraConfigStr := service.MergeExtraConfig(row.Account.ExtraConfig, extraConfigPatch)

	if _, err := h.db.Exec(
		h.db.Rebind("UPDATE accounts SET access_token = ?, status = 'active', extra_config = ?, updated_at = ? WHERE id = ?"),
		nextAccessToken, extraConfigStr, now, accountID,
	); err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update account")
		return
	}

	// Fetch updated account for response
	var rebindAcct store.Account
	if err := h.db.Get(&rebindAcct, h.db.Rebind("SELECT * FROM accounts WHERE id = ?"), accountID); err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to read updated account")
		return
	}
	rebindAcctMap := map[string]any{
		"id":             rebindAcct.ID,
		"siteId":         rebindAcct.SiteID,
		"username":       rebindAcct.Username,
		"accessToken":    rebindAcct.AccessToken,
		"apiToken":       rebindAcct.APIToken,
		"balance":        rebindAcct.Balance,
		"status":         rebindAcct.Status,
		"isPinned":       rebindAcct.IsPinned,
		"sortOrder":      rebindAcct.SortOrder,
		"checkinEnabled": rebindAcct.CheckinEnabled,
		"extraConfig":    rebindAcct.ExtraConfig,
		"createdAt":      rebindAcct.CreatedAt,
		"updatedAt":      rebindAcct.UpdatedAt,
	}
	// Masked like every other account surface: the caller supplied the new
	// session token itself, and the stored apiToken is not this endpoint's to
	// disclose (service.RedactAccountSecrets is the policy SSOT).
	service.RedactAccountSecrets(rebindAcctMap)
	routing.InvalidateCache()
	globalAccountsCache.clear()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":        true,
		"account":        rebindAcctMap,
		"tokenType":      "session",
		"credentialMode": "session",
		"capabilities":   service.BuildCapabilitiesFromCredentialMode(service.CredentialModeSession, true),
		"apiTokenFound":  false,
	})
}
