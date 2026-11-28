package admin

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

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
		normalizedProxyURL, valid := normalizeProxyURLInput(body.ProxyURL)
		if !valid {
			writeErrorWithRequest(w, r, http.StatusBadRequest, invalidProxyURLMessage)
			return
		}
		body.ProxyURL = &normalizedProxyURL
	}

	// Get site
	var site store.Site
	if err := h.db.Get(&site, "SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?", body.SiteID); err != nil {
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
		writeErrorWithRequest(w, r, http.StatusBadRequest, credentialVerificationFailureMessage(site.Platform))
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

	row, err := service.GetAccountWithSiteByID(h.db.DB, accountID)
	if err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeAccountNotFound, "account not found")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// Sub2API managed auth: merge refreshToken/tokenExpiresAt into extraConfig.sub2apiAuth
	// without clobbering unrelated keys.
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
		"UPDATE accounts SET access_token = ?, status = 'active', extra_config = ?, updated_at = ? WHERE id = ?",
		nextAccessToken, extraConfigStr, now, accountID,
	); err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "failed to update account")
		return
	}

	// Fetch updated account for response
	var rebindAcct store.Account
	if err := h.db.Get(&rebindAcct, "SELECT * FROM accounts WHERE id = ?", accountID); err != nil {
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
