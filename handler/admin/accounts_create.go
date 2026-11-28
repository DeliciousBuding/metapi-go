package admin

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/alert"
	"github.com/deliciousbuding/metapi-go/store"
)

func (h *accountsHandler) createAccount(w http.ResponseWriter, r *http.Request) {
	var body payloads.AccountCreatePayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid account payload: "+err.Error())
		return
	}

	if body.SiteID <= 0 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "Invalid siteId. Expected positive number.")
		return
	}

	// Check site exists
	var site store.Site
	if err := h.db.Get(&site, "SELECT "+service.SiteSelectColumns+" FROM sites WHERE id = ?", body.SiteID); err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "site not found")
		return
	}

	if body.ProxyURL != nil {
		normalizedProxyURL, valid := normalizeProxyURLInput(body.ProxyURL)
		if !valid {
			writeErrorWithRequest(w, r, http.StatusBadRequest, invalidProxyURLMessage)
			return
		}
		// A per-account proxy redirects this account's upstream credential
		// (and auto-relogin traffic), so setting one needs the master token.
		if requireProxyURLReauth(w, r, normalizedProxyURL) {
			return
		}
		body.ProxyURL = &normalizedProxyURL
	}

	credentialMode, requestedTokens := accountCreateTokens(body)
	if len(requestedTokens) == 0 {
		writeErrorWithRequest(w, r, http.StatusBadRequest, "token is required")
		return
	}

	// Batch API key creation (multiple tokens)
	if credentialMode == service.CredentialModeAPIKey && len(requestedTokens) > 1 {
		h.createAPIKeyBatch(w, r, body, site, requestedTokens)
		return
	}

	// Single token creation
	created, err := h.createSingleAccount(r.Context(), body, site, credentialMode, requestedTokens[0], body.Username)
	if err != nil {
		slog.Error("Account creation failed", "err", err)
		message := err.Error()
		requiresVerification := false
		var verifyErr *accountCreateError
		if errors.As(err, &verifyErr) {
			message = verifyErr.Message
			requiresVerification = verifyErr.RequiresVerification
			if credentialMode != service.CredentialModeAPIKey {
				message = alert.AppendSessionTokenRebindHint(message)
			}
		} else if credentialMode != service.CredentialModeAPIKey {
			message = alert.AppendSessionTokenRebindHint(message)
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success":              false,
			"requiresVerification": requiresVerification,
			"message":              message,
		})
		return
	}

	// Discover models only after the session's relay keys have been synced.
	// Verification alone did not persist availability, so fresh imports stayed
	// empty and their first route could not relay until a scheduled refresh.
	syncReport := syncTokensAfterAccountCreate(r.Context(), h.db, h.cfg, created.ID)
	modelCount, modelRefresh := h.refreshModelsAfterAccountCreate(r.Context(), body, created.ID)

	// Return the initialized credential state, not the pre-sync snapshot.
	var account store.Account
	if err := h.db.Get(&account, "SELECT * FROM accounts WHERE id = ?", created.ID); err != nil {
		writeErrorWithRequest(w, r, http.StatusInternalServerError, "Account saved, but its initialized state could not be loaded.")
		return
	}
	caps := service.BuildCapabilitiesForAccount(&account)
	routing.InvalidateCache()
	globalAccountsCache.clear()
	writeJSON(w, http.StatusOK, map[string]any{
		"id":               account.ID,
		"siteId":           account.SiteID,
		"username":         account.Username,
		"accessToken":      account.AccessToken,
		"status":           account.Status,
		"tokenType":        created.TokenType,
		"credentialMode":   string(service.ResolveStoredCredentialMode(&account)),
		"capabilities":     caps,
		"modelCount":       modelCount,
		"modelRefresh":     modelRefresh,
		"apiTokenFound":    account.APIToken != nil && strings.TrimSpace(*account.APIToken) != "",
		"usernameDetected": created.UsernameDetected,
		"queued":           false,
		"tokenCount":       syncReport.TokenCount,
		"tokenSyncStatus":  syncReport.Status,
		"tokenSyncMessage": syncReport.Message,
	})
}

// refreshModelsAfterAccountCreate uses the same discovery/persistence owner as
// password login. A discovery failure is partial initialization, never grounds
// to erase an already verified account. Explicit no-probe imports remain so.
func (h *accountsHandler) refreshModelsAfterAccountCreate(ctx context.Context, body payloads.AccountCreatePayload, accountID int64) (int, map[string]any) {
	if body.SkipModelFetch != nil && *body.SkipModelFetch {
		return 0, map[string]any{"success": true, "skipped": true, "reason": "skipModelFetch"}
	}
	result := accountModelRefresher(ctx, h.db, accountID, false)
	count := 0
	if success, _ := result["success"].(bool); success {
		if refresh, ok := result["refresh"].(map[string]any); ok {
			count, _ = refresh["modelCount"].(int)
		}
	}
	return count, result
}

func accountCreateTokens(body payloads.AccountCreatePayload) (service.AccountCredentialMode, []string) {
	mode := service.ResolveRequestedCredentialMode(body.CredentialMode)
	var tokens []string
	if len(body.AccessTokens) > 0 {
		mode = service.CredentialModeAPIKey
		for _, token := range body.AccessTokens {
			if token = strings.TrimSpace(token); token != "" {
				tokens = append(tokens, token)
			}
		}
	} else if body.AccessToken != nil {
		if token := strings.TrimSpace(*body.AccessToken); token != "" {
			tokens = append(tokens, token)
		}
	}
	return mode, tokens
}

func (h *accountsHandler) createAPIKeyBatch(w http.ResponseWriter, r *http.Request, body payloads.AccountCreatePayload, site store.Site, requestedTokens []string) {
	var items []map[string]any
	createdCount := 0
	for i, token := range requestedTokens {
		username := body.Username
		if username != nil {
			name := *username
			if len(name) > 0 {
				n := buildBatchAPIKeyName(name, i, len(requestedTokens))
				username = &n
			}
		}

		created, err := h.createSingleAccount(r.Context(), body, site, service.CredentialModeAPIKey, token, username)
		if err != nil {
			slog.Error("Batch account creation failed", "err", err, "index", i)
			item := map[string]any{
				"index":   i,
				"status":  "failed",
				"message": err.Error(),
			}
			var verifyErr *accountCreateError
			if errors.As(err, &verifyErr) {
				item["message"] = verifyErr.Message
				item["requiresVerification"] = verifyErr.RequiresVerification
			}
			items = append(items, item)
		} else {
			modelCount, modelRefresh := h.refreshModelsAfterAccountCreate(r.Context(), body, created.ID)
			createdCount++
			items = append(items, map[string]any{
				"index":        i,
				"status":       "created",
				"id":           created.ID,
				"username":     coalescePtr(created.Username, ""),
				"queued":       false,
				"message":      nil,
				"modelCount":   modelCount,
				"modelRefresh": modelRefresh,
			})
		}
	}

	if createdCount == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"success":      false,
			"batch":        true,
			"totalCount":   len(requestedTokens),
			"createdCount": 0,
			"failedCount":  len(requestedTokens),
			"message":      "batch add failed (0/" + strconv.Itoa(len(requestedTokens)) + ")",
			"items":        items,
		})
		return
	}

	routing.InvalidateCache()
	globalAccountsCache.clear()
	writeJSON(w, http.StatusOK, map[string]any{
		"success":      true,
		"batch":        true,
		"totalCount":   len(requestedTokens),
		"createdCount": createdCount,
		"failedCount":  len(requestedTokens) - createdCount,
		"message":      "batch add completed: " + strconv.Itoa(createdCount) + " succeeded, " + strconv.Itoa(len(requestedTokens)-createdCount) + " failed",
		"items":        items,
	})
}
