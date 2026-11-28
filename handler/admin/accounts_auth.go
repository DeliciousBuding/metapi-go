package admin

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

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
	if err := h.db.Get(&site, "SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?", body.SiteID); err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeSiteNotFound, "site not found")
		return
	}

	adp := platform.GetAdapter(site.Platform)
	if adp == nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "unsupported platform: "+site.Platform)
		return
	}
	var existing store.Account
	err := h.db.Get(&existing,
		"SELECT * FROM accounts WHERE site_id = ? AND username = ?",
		body.SiteID, body.Username,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "Failed to load account.")
		return
	}
	reused := err == nil
	existingCredential := ""
	if reused {
		existingCredential = existing.AccessToken
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	loginResult, err := platform.LoginReusingCredential(adp, ctx, site.URL, body.Username, body.Password, nil, service.BuildPlatformProxyConfigForToken(h.cfg, &site, body.Username), existingCredential)
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
			`UPDATE accounts SET access_token = ?, checkin_enabled = ?, status = 'active',
			 extra_config = ?, updated_at = ? WHERE id = ?`,
			loginAccessToken, true, extraConfigStr, now, existing.ID,
		); err != nil {
			slog.Error("Failed to update login account", "err", err, "account_id", existing.ID)
			writeErrorWithRequest(w, r, http.StatusInternalServerError, "Failed to save account.")
			return
		}
	} else {
		sortOrder, _ := service.GetNextAccountSortOrder(h.db.DB)
		if _, err := h.db.Exec(
			`INSERT INTO accounts (site_id, username, access_token, checkin_enabled, status,
			 is_pinned, sort_order, extra_config, created_at, updated_at)
			 VALUES (?, ?, ?, ?, 'active', ?, ?, ?, ?, ?)`,
			body.SiteID, body.Username, loginAccessToken, true, false, sortOrder, extraConfigStr, now, now,
		); err != nil {
			slog.Error("Failed to insert login account", "err", err, "site_id", body.SiteID)
			writeErrorWithRequest(w, r, http.StatusInternalServerError, "Failed to save account.")
			return
		}
	}

	// Fetch the created/updated account for response
	var loginAcct store.Account
	if err := h.db.Get(&loginAcct, "SELECT * FROM accounts WHERE site_id = ? AND username = ?", body.SiteID, body.Username); err != nil {
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
