package admin

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func (h *accountTokensHandler) batchTokens(w http.ResponseWriter, r *http.Request) {
	var body payloads.AccountTokenBatchPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid account token batch payload.")
		return
	}

	if len(body.IDs) == 0 {
		writeError(w, http.StatusBadRequest, "ids is required")
		return
	}

	action := strings.TrimSpace(body.Action)
	validActions := map[string]bool{"enable": true, "disable": true, "delete": true}
	if !validActions[action] {
		writeError(w, http.StatusBadRequest, "Invalid action")
		return
	}

	var successIDs []int64
	var failedItems []map[string]any

	for _, rawID := range body.IDs {
		id := int64(rawID)

		existing, err := service.GetTokenByID(h.db.DB, id)
		if err != nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "Token not found"})
			continue
		}

		owner, ownerErr := service.GetAccountByID(h.db.DB, existing.AccountID)
		if ownerErr != nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "Account not found"})
			continue
		}
		if owner == nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "Account not found"})
			continue
		}
		if service.IsAPIKeyConnection(owner) {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "API key connections do not support managing account tokens"})
			continue
		}

		if action == "delete" {
			if err := h.deleteTokenWithUpstream(r.Context(), existing); err != nil {
				slog.Error("Token deletion failed", "err", err, "token_id", id)
				failedItems = append(failedItems, map[string]any{"id": id, "message": err.Error()})
				continue
			}
		} else {
			if service.IsMaskedPendingAccountToken(existing) {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "pending tokens cannot change their enabled state; complete the plaintext token first"})
				continue
			}
			now := time.Now().UTC().Format(time.RFC3339)
			if _, err := h.db.Exec("UPDATE account_tokens SET enabled = ?, updated_at = ? WHERE id = ?", action == "enable", now, id); err != nil {
				slog.Error("Token status update failed", "err", err, "token_id", id)
				failedItems = append(failedItems, map[string]any{"id": id, "message": "Token status update failed"})
				continue
			}

			if existing.IsDefault && action == "disable" {
				if _, err := service.RepairDefaultToken(h.db.DB, existing.AccountID); err != nil {
					slog.Error("Default token repair failed", "err", err, "account_id", existing.AccountID)
					failedItems = append(failedItems, map[string]any{"id": id, "message": "Default token repair failed"})
					continue
				}
			}
		}
		successIDs = append(successIDs, id)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"successIds":  successIDs,
		"failedItems": failedItems,
	})
}

func (h *accountTokensHandler) updateToken(w http.ResponseWriter, r *http.Request) {
	tokenID, ok := pathID(w, r)
	if !ok {
		return
	}

	var body payloads.AccountTokenUpdatePayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid account token payload.")
		return
	}

	existing, err := service.GetTokenByID(h.db.DB, tokenID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeTokenNotFound, "token not found")
		return
	}

	owner, err := service.GetAccountWithSiteByID(h.db.DB, existing.AccountID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeAccountNotFound, "account not found")
		return
	}
	if owner == nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeAccountNotFound, "account not found")
		return
	}
	if service.IsAPIKeyConnection(&owner.Account) {
		writeErrorCode(w, http.StatusBadRequest, ErrorCodeOperationNotSupported, "API key connections do not support managing account tokens")
		return
	}

	updates := map[string]any{}

	nextValueStatus := service.ResolveAccountTokenValueStatus(existing)

	if body.Name != nil {
		updates["name"] = strings.TrimSpace(*body.Name)
	}
	if body.Token != nil {
		tv := strings.TrimSpace(*body.Token)
		if tv == "" {
			writeError(w, http.StatusBadRequest, "token must not be empty")
			return
		}
		updates["token"] = tv
		if IsMaskedTokenValue(tv) {
			nextValueStatus = service.TokenValueStatusMaskedPending
		} else {
			nextValueStatus = service.TokenValueStatusReady
		}
		updates["valueStatus"] = nextValueStatus
	}
	if body.Group != nil {
		updates["tokenGroup"] = nullIfEmpty(strings.TrimSpace(*body.Group))
	}
	if body.Source != nil {
		updates["source"] = strings.TrimSpace(*body.Source)
	}

	if nextValueStatus == service.TokenValueStatusMaskedPending {
		updates["enabled"] = false
		updates["isDefault"] = false
	} else {
		if body.Enabled != nil {
			updates["enabled"] = *body.Enabled
		}
		if body.IsDefault != nil {
			updates["isDefault"] = *body.IsDefault
		}
	}

	if err := service.UpdateTokenFields(h.db.DB, tokenID, updates); err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	// Refresh and handle default logic
	latest, err := service.GetTokenByID(h.db.DB, tokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	if latest == nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	if body.IsDefault != nil && *body.IsDefault && service.IsUsableAccountToken(latest) {
		if ok, err := service.SetDefaultToken(h.db.DB, tokenID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to set default token")
			return
		} else if !ok {
			writeError(w, http.StatusBadRequest, "token cannot be set as default")
			return
		}
	} else if latest.IsDefault && service.IsUsableAccountToken(latest) {
		if ok, err := service.SetDefaultToken(h.db.DB, tokenID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to set default token")
			return
		} else if !ok {
			writeError(w, http.StatusBadRequest, "token cannot be set as default")
			return
		}
	} else if existing.IsDefault && !service.IsUsableAccountToken(latest) {
		if _, err := service.RepairDefaultToken(h.db.DB, existing.AccountID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to repair the default token")
			return
		}
	} else if body.IsDefault != nil && !(*body.IsDefault) && existing.IsDefault {
		if _, err := service.RepairDefaultToken(h.db.DB, existing.AccountID); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to repair the default token")
			return
		}
	}

	latest, err = service.GetTokenByID(h.db.DB, tokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read tokens")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"token":   tokenToMap(*latest, owner.Site.Platform),
	})
}

func (h *accountTokensHandler) deleteToken(w http.ResponseWriter, r *http.Request) {
	tokenID, ok := pathID(w, r)
	if !ok {
		return
	}

	token, err := service.GetTokenByID(h.db.DB, tokenID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeTokenNotFound, "token not found")
		return
	}

	owner, err := service.GetAccountByID(h.db.DB, token.AccountID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeTokenNotFound, "token not found")
		return
	}
	if owner == nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeTokenNotFound, "token not found")
		return
	}
	if service.IsAPIKeyConnection(owner) {
		writeErrorCode(w, http.StatusBadRequest, ErrorCodeOperationNotSupported, "API key connections do not support managing account tokens")
		return
	}

	if err := h.deleteTokenWithUpstream(r.Context(), token); err != nil {
		slog.Error("Token deletion failed", "err", err, "token_id", tokenID)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *accountTokensHandler) deleteTokenWithUpstream(ctx context.Context, token *store.AccountToken) error {
	if token == nil {
		return fmt.Errorf("token not found")
	}

	// Known limitation: masked_pending tokens only store redacted key material, so remote
	// delete cannot match a real upstream key and is intentionally skipped.
	shouldSkipUpstream := service.IsMaskedPendingAccountToken(token) || IsMaskedTokenValue(token.Token)

	row, err := service.GetAccountWithSiteByID(h.db.DB, token.AccountID)
	if err != nil {
		return service.DeleteTokenByID(h.db.DB, token.ID)
	}

	if !shouldSkipUpstream {
		siteDisabled := strings.EqualFold(strings.TrimSpace(row.Site.Status), "disabled")
		accessToken := strings.TrimSpace(row.Account.AccessToken)
		adapter := platform.GetAdapter(row.Site.Platform)

		if !siteDisabled && accessToken != "" && adapter != nil {
			callCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			proxyCfg := service.BuildPlatformProxyConfig(h.cfg, &row.Account, &row.Site)
			platformUserID := service.ResolvePlatformUserIDPtr(&row.Account)
			if delErr := adapter.DeleteAPIToken(callCtx, row.Site.URL, accessToken, token.Token, platformUserID, proxyCfg); delErr != nil {
				return fmt.Errorf("failed to delete the upstream token: %w", delErr)
			}
		}
	}

	return service.DeleteTokenByID(h.db.DB, token.ID)
}
