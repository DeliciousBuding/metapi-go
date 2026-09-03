package admin

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	balanceService "github.com/deliciousbuding/metapi-go/service/balance"
	"github.com/deliciousbuding/metapi-go/store"
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

	// #1176: credentialMode is an explicit operator choice on the edit form.
	// Resolve it before the credential fields are touched so the api_token
	// mirror (an apikey-mode convenience) cannot stamp a session credential as
	// an API key, and so the mode that gets persisted is the one requested.
	var requestedMode service.AccountCredentialMode
	hasRequestedMode := false
	if body.CredentialMode != nil {
		mode := service.NormalizeCredentialMode(*body.CredentialMode)
		if mode != service.CredentialModeSession && mode != service.CredentialModeAPIKey {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid credentialMode. Expected session or apikey."})
			return
		}
		requestedMode, hasRequestedMode = mode, true
	}

	updates := map[string]any{}
	pendingExtraConfig := row.Account.ExtraConfig
	mergeExtraConfigUpdate := func(patch map[string]any) {
		merged := service.MergeExtraConfig(pendingExtraConfig, patch)
		if merged != nil {
			pendingExtraConfig = merged
			updates["extraConfig"] = *merged
		}
	}
	if body.Username != nil {
		updates["username"] = *body.Username
	}
	mirrorAPIKeyToken := false
	explicitSessionSwitch := hasRequestedMode && requestedMode == service.CredentialModeSession
	if body.AccessToken != nil {
		nextAccessToken := strings.TrimSpace(*body.AccessToken)
		updates["accessToken"] = nextAccessToken
		// A session JWT is not an API key: skip the apikey mirror when the
		// operator explicitly moved this account to session mode (#1176).
		mirrorAPIKeyToken = !explicitSessionSwitch && shouldMirrorAPIKeyToken(row.Account, body.APIToken)
		if mirrorAPIKeyToken {
			updates["apiToken"] = nextAccessToken
		}
	}
	if body.APIToken != nil && !mirrorAPIKeyToken {
		updates["apiToken"] = normalizeAPITokenUpdate(body.APIToken)
	}
	if body.Status != nil {
		status := normalizeAccountStatus(*body.Status)
		if status == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid account status. Expected active, disabled, or expired."})
			return
		}
		updates["status"] = status
	}
	if body.UnitCost != nil {
		if *body.UnitCost <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid unitCost value. Expected positive number."})
			return
		}
		updates["unitCost"] = *body.UnitCost
	}
	if body.ExtraConfig != nil {
		// Merge with existing extraConfig to preserve keys not in the update.
		// For sub2api, strip nested sub2apiAuth here and merge it via managed-auth
		// helpers later so partial patches cannot clobber unrelated auth keys.
		if ecMap, ok := body.ExtraConfig.(map[string]any); ok {
			if service.IsSub2ApiPlatform(row.Site.Platform) {
				stripped := make(map[string]any, len(ecMap))
				for k, v := range ecMap {
					if k == "sub2apiAuth" {
						continue
					}
					stripped[k] = v
				}
				if len(stripped) > 0 {
					mergeExtraConfigUpdate(stripped)
				}
			} else {
				mergeExtraConfigUpdate(ecMap)
			}
		}
	}
	nextAccount := row.Account
	nextAccount.ExtraConfig = pendingExtraConfig
	if accessToken, ok := updates["accessToken"].(string); ok {
		nextAccount.AccessToken = accessToken
	}
	if apiToken, ok := updates["apiToken"].(*string); ok {
		nextAccount.APIToken = apiToken
	} else if apiTokenStr, ok := updates["apiToken"].(string); ok {
		token := apiTokenStr
		nextAccount.APIToken = &token
	}
	if status, ok := updates["status"].(string); ok {
		nextAccount.Status = status
	}
	if service.BuildCapabilitiesForAccount(&nextAccount).CanCheckin {
		if body.CheckinEnabled != nil {
			updates["checkinEnabled"] = *body.CheckinEnabled
		}
	} else if row.Account.CheckinEnabled || body.CheckinEnabled != nil {
		updates["checkinEnabled"] = false
	}
	if body.IsPinned != nil {
		updates["isPinned"] = *body.IsPinned
	}
	if body.SortOrder != nil {
		so := service.NormalizeSortOrder(body.SortOrder)
		if so == nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid sortOrder value. Expected non-negative integer."})
			return
		}
		updates["sortOrder"] = int64(*so)
	}
	if body.ProxyURL != nil {
		normalizedProxyURL := strings.TrimSpace(*body.ProxyURL)
		if normalizedProxyURL != "" && !service.IsValidProxyURL(normalizedProxyURL) {
			writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid proxyUrl. Expected a valid http(s)/socks proxy URL.")
			return
		}
		// An explicitly cleared proxyUrl must persist as a deletion (issue
		// #1009 residual). MergeExtraConfig deletes keys whose patch value is
		// an untyped nil, so build the patch with a nil interface rather than
		// a typed (*string)(nil), which would marshal to "proxyUrl": null.
		var proxyPatch any
		if normalizedProxyURL != "" {
			proxyPatch = normalizedProxyURL
		}
		mergeExtraConfigUpdate(map[string]any{"proxyUrl": proxyPatch})
	}
	// credentialMode lives in extraConfig (same storage as the create path).
	// Merged after body.ExtraConfig so the explicit top-level field wins over a
	// nested copy in the same request, mirroring how proxyUrl is handled.
	if hasRequestedMode {
		mergeExtraConfigUpdate(map[string]any{"credentialMode": string(requestedMode)})
	}
	// PlatformUserID and skipModelFetch live in extraConfig (same storage as
	// the create path); merge instead of dropping the form fields.
	if body.PlatformUserID != nil {
		mergeExtraConfigUpdate(map[string]any{"platformUserId": *body.PlatformUserID})
	}
	if body.SkipModelFetch != nil {
		mergeExtraConfigUpdate(map[string]any{"skipModelFetch": *body.SkipModelFetch})
	}
	// Tags persist to the accounts.tags JSON-array column with the same
	// normalization as the dedicated PUT /api/accounts/{id}/tags endpoint
	// (trim + dedupe + drop empties).
	if body.Tags != nil {
		seen := make(map[string]struct{}, len(*body.Tags))
		normalizedTags := make([]string, 0, len(*body.Tags))
		for _, tag := range *body.Tags {
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			if _, duplicate := seen[tag]; duplicate {
				continue
			}
			seen[tag] = struct{}{}
			normalizedTags = append(normalizedTags, tag)
		}
		updates["tags"] = encodeTagsJSON(normalizedTags)
	}
	// Remark is a free-form human-readable note. An empty/whitespace string
	// clears the column (NULL); otherwise the trimmed value is persisted.
	// Do not store credentials here — remark is returned in plaintext on
	// admin list/search responses (unlike accessToken/apiToken which are masked).
	if body.Remark != nil {
		updates["remark"] = service.NormalizeNullable(body.Remark)
	}
	// Sub2API managed auth: merge top-level / nested refreshToken+tokenExpiresAt
	// into extraConfig.sub2apiAuth without clobbering unrelated keys.
	if service.IsSub2ApiPlatform(row.Site.Platform) {
		var extraPatch map[string]any
		if body.ExtraConfig != nil {
			if ecMap, ok := body.ExtraConfig.(map[string]any); ok {
				extraPatch = ecMap
			}
		}
		// Merge against the original stored config so nested extraConfig.sub2apiAuth
		// patches compose with top-level fields instead of overwriting wholesale.
		if mergedAuth := service.BuildMergedSub2ApiAuth(row.Account.ExtraConfig, body.RefreshToken, body.TokenExpiresAt, extraPatch); mergedAuth != nil {
			mergeExtraConfigUpdate(map[string]any{"sub2apiAuth": mergedAuth})
		}
	}
	// Fail loud instead of persisting a mode the stored credential cannot back:
	// such an account can never authenticate, and the edit dialog would keep
	// showing a mode that lies about what is on the row (#1176).
	if hasRequestedMode {
		hasSession := strings.TrimSpace(nextAccount.AccessToken) != ""
		hasAPIKey := nextAccount.APIToken != nil && strings.TrimSpace(*nextAccount.APIToken) != ""
		if requestedMode == service.CredentialModeSession && !hasSession {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credentialMode session requires a session credential: send accessToken in the same request."})
			return
		}
		if requestedMode == service.CredentialModeAPIKey && !hasAPIKey {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "credentialMode apikey requires an API key: send apiToken in the same request."})
			return
		}
	}

	// Expired API-key recovery (p3-sites-accounts.md 594-602 / TS accounts.ts):
	// when credentials change on an expired apikey account and status is not forced
	// disabled, refresh models with allowInactive and reactivate only on success.
	recovery := shouldRecoverExpiredAPIKey(row.Account, nextAccount, updates)
	if recovery {
		// preserveExpiredStatus: do not activate until model refresh succeeds.
		if status, ok := updates["status"].(string); ok && status == "active" {
			delete(updates, "status")
		}
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
		_ = service.SetAccountRuntimeHealth(h.db, id, service.RuntimeHealthEntry{
			State:  service.HealthUnhealthy,
			Reason: "connection credential expired; update the credential",
			Source: service.HealthSourceAuth,
		})
	}

	var modelRefresh map[string]any
	if recovery {
		modelRefresh = accountModelRefresher(r.Context(), h.db, id, true)
		if modelRefreshSucceeded(modelRefresh) {
			if err := service.UpdateAccountFields(h.db, id, map[string]any{"status": "active"}); err != nil {
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
			if err := service.UpdateAccountFields(h.db, id, map[string]any{"status": "expired"}); err != nil {
				slog.Warn("failed to preserve expired status after recovery failure", "account_id", id, "err", err)
			}
		}
	}

	updatedRow, err := service.GetAccountWithSiteByID(h.db, id)
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

// ---- Delete Account ----

func (h *accountsHandler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := service.DeleteAccount(h.db, id); err != nil {
		slog.Error("Failed to delete account", "err", err, "account_id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "Failed to delete account"})
		return
	}
	service.RebuildRoutesBestEffort()
	globalAccountsCache.clear()
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// ---- Batch Accounts ----

func (h *accountsHandler) batchAccounts(w http.ResponseWriter, r *http.Request) {
	var body payloads.AccountBatchPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid account batch payload."})
		return
	}

	if len(body.IDs) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "ids is required"})
		return
	}

	action := strings.TrimSpace(body.Action)
	validActions := map[string]bool{"enable": true, "disable": true, "delete": true, "refreshBalance": true}
	if !validActions[action] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "Invalid action"})
		return
	}

	successIDs := []int64{}
	failedItems := []map[string]any{}
	skippedItems := []map[string]any{}
	shouldRebuildRoutes := false
	shouldInvalidateRoutes := false

	for _, rawID := range body.IDs {
		id := int64(rawID)

		if action == "refreshBalance" {
			result, err := balanceService.RefreshBalance(h.cfg, h.db, id)
			if result == nil && err == nil {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "Account not found"})
				continue
			}
			if err != nil {
				slog.Warn("Batch balance refresh failed", "err", err, "account_id", id)
				failedItems = append(failedItems, map[string]any{"id": id, "message": "Balance refresh failed"})
				continue
			}
			if result.Skipped {
				skippedItems = append(skippedItems, map[string]any{"id": id, "reason": result.Reason})
				continue
			}
			successIDs = append(successIDs, id)
			continue
		}

		var existing store.Account
		err := h.db.Get(&existing, h.db.Rebind("SELECT * FROM accounts WHERE id = ?"), id)
		if err != nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "Account not found"})
			continue
		}

		now := time.Now().UTC().Format(time.RFC3339)
		var execErr error
		switch action {
		case "delete":
			_, execErr = h.db.Exec(h.db.Rebind("DELETE FROM accounts WHERE id = ?"), id)
		case "enable":
			_, execErr = h.db.Exec(h.db.Rebind("UPDATE accounts SET status = 'active', updated_at = ? WHERE id = ?"), now, id)
		case "disable":
			_, execErr = h.db.Exec(h.db.Rebind("UPDATE accounts SET status = 'disabled', updated_at = ? WHERE id = ?"), now, id)
		}
		if execErr != nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "Account update failed"})
			continue
		}
		if action == "delete" {
			shouldRebuildRoutes = true
		} else if action == "enable" || action == "disable" {
			shouldInvalidateRoutes = true
		}
		successIDs = append(successIDs, id)
	}

	if shouldRebuildRoutes {
		service.RebuildRoutesBestEffort()
	} else if shouldInvalidateRoutes {
		routing.InvalidateCache()
	}
	if len(successIDs) > 0 {
		globalAccountsCache.clear()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"successIds":   successIDs,
		"failedItems":  failedItems,
		"skippedItems": skippedItems,
	})
}
