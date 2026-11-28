package admin

import (
	"log/slog"
	"net/http"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
)

// ---- Update Account ----

func (h *accountsHandler) updateAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var body payloads.AccountUpdatePayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid account payload: " + err.Error()})
		return
	}

	// A per-account proxy redirects the stored upstream credential: validate it
	// and require the master token when the request sets a non-empty value.
	// Clearing the field stays possible from a session because it cannot
	// redirect traffic.
	if body.ProxyURL != nil {
		normalizedProxyURL, valid := normalizeProxyURLInput(body.ProxyURL)
		if !valid {
			writeErrorWithRequest(w, r, http.StatusBadRequest, invalidProxyURLMessage)
			return
		}
		if requireProxyURLReauth(w, r, normalizedProxyURL) {
			return
		}
		body.ProxyURL = &normalizedProxyURL
	}
	// extraConfig is merged verbatim below, so the same field can arrive
	// nested; it must pass the same validation and reauth gate.
	if patch, nestedProxyURL, found := extraConfigProxyURL(body.ExtraConfig); found {
		if err := service.ValidateProxyURL(nestedProxyURL); err != nil {
			writeErrorWithRequest(w, r, http.StatusBadRequest, invalidProxyURLMessage)
			return
		}
		if !auth.ReauthConfirmed(r) {
			auth.WriteReauthRequired(w)
			return
		}
		patch["proxyUrl"] = nestedProxyURL
	}

	// Wrap the read-merge-write in a transaction so concurrent patches of
	// extra_config compose instead of overwriting each other (lost updates).
	// PostgreSQL takes a FOR UPDATE row lock on the read; SQLite serializes via
	// its single-connection pool. The deferred Rollback is a harmless no-op once
	// committed and unwinds the tx on every early-return path.
	tx, err := h.db.Beginx()
	if err != nil {
		slog.Error("Failed to begin account update transaction", "err", err, "account_id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "Failed to update account"})
		return
	}
	defer tx.Rollback()

	row, err := service.GetAccountWithSiteByIDForUpdate(tx, id)
	if err != nil {
		writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeAccountNotFound, "account not found")
		return
	}

	updates, recovery, err := buildAccountUpdate(row, body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if err := service.UpdateAccountFieldsTx(tx, id, updates); err != nil {
		slog.Error("Failed to update account", "err", err, "account_id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "Failed to update account"})
		return
	}
	if err := tx.Commit(); err != nil {
		slog.Error("Failed to commit account update", "err", err, "account_id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "Failed to update account"})
		return
	}

	// When status is forced to expired, align stored runtime health with  auth class.
	if status, ok := updates["status"].(string); ok && status == "expired" {
		_ = service.SetAccountRuntimeHealth(h.db.DB, id, service.RuntimeHealthEntry{
			State:  service.HealthUnhealthy,
			Reason: "connection credential expired; update the credential",
			Source: service.HealthSourceAuth,
		})
	}

	var modelRefresh map[string]any
	if recovery {
		modelRefresh = accountModelRefresher(r.Context(), h.db, id, true)
		if modelRefreshSucceeded(modelRefresh) {
			if err := service.UpdateAccountFields(h.db.DB, id, map[string]any{"status": "active"}); err != nil {
				slog.Error("Failed to reactivate account after model refresh", "err", err, "account_id", id)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "model refresh succeeded but account activation failed"})
				return
			}
			// Best-effort: clear stale auth-unhealthy runtime health after successful recovery.
			_ = clearAccountAuthRuntimeHealth(h.db, id)
		} else {
			// Keep expired; never claim active on failed model refresh.
			msg := modelRefreshErrorMessage(modelRefresh)
			slog.Warn("expired API-key recovery model refresh failed", "account_id", id, "message", msg)
			if err := service.UpdateAccountFields(h.db.DB, id, map[string]any{"status": "expired"}); err != nil {
				slog.Warn("failed to preserve expired status after recovery failure", "account_id", id, "err", err)
			}
		}
	}

	updatedRow, err := service.GetAccountWithSiteByID(h.db.DB, id)
	if err != nil {
		slog.Error("Failed to load updated account", "err", err, "account_id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "Failed to load updated account", "errorCode": "resourceLoadFailed"})
		return
	}
	updated := updatedRow.Account
	caps := service.BuildCapabilitiesForAccount(&updated)
	sessionCapable := caps.CanRefreshBalance
	hasDiscoveredModels := false
	if recovery && modelRefreshSucceeded(modelRefresh) {
		hasDiscoveredModels = true
	}
	resp := map[string]any{
		"id":                 updated.ID,
		"siteId":             updated.SiteID,
		"username":           updated.Username,
		"accessToken":        updated.AccessToken,
		"apiToken":           updated.APIToken,
		"balance":            updated.Balance,
		"balanceUsed":        updated.BalanceUsed,
		"quota":              updated.Quota,
		"unitCost":           updated.UnitCost,
		"valueScore":         updated.ValueScore,
		"status":             updated.Status,
		"isPinned":           updated.IsPinned,
		"sortOrder":          updated.SortOrder,
		"checkinEnabled":     updated.CheckinEnabled,
		"lastCheckinAt":      updated.LastCheckinAt,
		"lastBalanceRefresh": updated.LastBalanceRefresh,
		"oauthProvider":      updated.OAuthProvider,
		"oauthAccountKey":    updated.OAuthAccountKey,
		"oauthProjectId":     updated.OAuthProjectID,
		"extraConfig":        updated.ExtraConfig,
		"createdAt":          updated.CreatedAt,
		"updatedAt":          updated.UpdatedAt,
		"remark":             updated.Remark,
		"credentialMode":     string(service.ResolveStoredCredentialMode(&updated)),
		"capabilities":       caps,
		"runtimeHealth": service.BuildRuntimeHealthForAccount(service.RuntimeHealthInput{
			AccountStatus:       updated.Status,
			SiteStatus:          updatedRow.Site.Status,
			ExtraConfig:         updated.ExtraConfig,
			SessionCapable:      &sessionCapable,
			HasDiscoveredModels: hasDiscoveredModels,
			OAuthProvider:       updated.OAuthProvider,
		}),
		"site": map[string]any{
			"id":       updatedRow.Site.ID,
			"name":     updatedRow.Site.Name,
			"url":      updatedRow.Site.URL,
			"platform": updatedRow.Site.Platform,
			"status":   updatedRow.Site.Status,
		},
	}
	if recovery {
		resp["modelRefresh"] = modelRefresh
		if !modelRefreshSucceeded(modelRefresh) {
			resp["message"] = "credential updated, but model refresh failed; account remains expired: " + modelRefreshErrorMessage(modelRefresh)
		}
	}
	// Same credential policy as the list surface: without this, a no-op update
	// ({"sortOrder": n}) answers with the plaintext accessToken/apiToken and the
	// autoRelogin passwordCipher that GET /api/accounts masks — i.e. the write
	// endpoint would be a harvest primitive around the read-side redaction.
	service.RedactAccountSecrets(resp)
	routing.InvalidateCache()
	globalAccountsCache.clear()
	writeJSON(w, http.StatusOK, resp)
}
