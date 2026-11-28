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

// ---- Delete Account ----

func (h *accountsHandler) deleteAccount(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := service.DeleteAccount(h.db.DB, id); err != nil {
		slog.Error("Failed to delete account", "err", err, "account_id", id)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "Failed to delete account"})
		return
	}
	service.RebuildRoutesBestEffort(h.db.DB)
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
			result, err := balanceService.RefreshBalance(h.cfg, h.db.DB, id)
			if result == nil && err == nil {
				failedItems = append(failedItems, map[string]any{"id": id, "message": "Account not found"})
				continue
			}
			if err != nil {
				// #1210: the batch surface answers the same question as the
				// single-account one, so it gets the same classified reason rather
				// than a bare failure. The raw upstream text stays in the WARN line.
				reason := balanceService.ExplainRefreshFailure(err)
				slog.Warn("Batch balance refresh failed", "err", err, "account_id", id, "reason", reason)
				message := batchBalanceRefreshFailedMessage
				if reason != "" {
					message += ": " + reason
				}
				failedItems = append(failedItems, map[string]any{"id": id, "message": message})
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
		err := h.db.Get(&existing, "SELECT * FROM accounts WHERE id = ?", id)
		if err != nil {
			failedItems = append(failedItems, map[string]any{"id": id, "message": "Account not found"})
			continue
		}

		now := time.Now().UTC().Format(time.RFC3339)
		var execErr error
		switch action {
		case "delete":
			_, execErr = h.db.Exec("DELETE FROM accounts WHERE id = ?", id)
		case "enable":
			_, execErr = h.db.Exec("UPDATE accounts SET status = 'active', updated_at = ? WHERE id = ?", now, id)
		case "disable":
			_, execErr = h.db.Exec("UPDATE accounts SET status = 'disabled', updated_at = ? WHERE id = ?", now, id)
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
		service.RebuildRoutesBestEffort(h.db.DB)
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
