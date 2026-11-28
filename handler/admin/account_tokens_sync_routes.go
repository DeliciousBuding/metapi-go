package admin

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/go-chi/chi/v5"
)

func (h *accountTokensHandler) syncAccount(w http.ResponseWriter, r *http.Request) {
	accountIDStr := chi.URLParam(r, "accountId")
	accountID, err := strconv.ParseInt(accountIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid account ID")
		return
	}

	row, err := service.GetAccountWithSiteByID(h.db.DB, accountID)
	if err != nil {
		writeErrorCode(w, http.StatusNotFound, ErrorCodeAccountNotFound, "account not found")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, syncErr := executeAccountTokenSync(ctx, h.db, h.cfg, row)
	if syncErr != nil {
		writeError(w, http.StatusBadGateway, syncErr.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *accountTokensHandler) syncAll(w http.ResponseWriter, r *http.Request) {
	var body payloads.AccountTokenSyncAllPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		// Empty body is allowed for background mode.
		body = payloads.AccountTokenSyncAllPayload{}
	}

	wait := body.Wait != nil && *body.Wait
	db := h.db
	cfg := h.cfg

	if wait {
		summary, results := executeSyncAllAccountTokens(context.Background(), db, cfg)
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"summary": summary,
			"results": results,
		})
		return
	}

	task, reused := StartBackgroundTask(BackgroundTaskStartOptions{
		Type:      "sync-all-account-tokens",
		Title:     "Sync all account tokens",
		DedupeKey: "sync-all-account-tokens",
	}, func() (any, error) {
		summary, results := executeSyncAllAccountTokens(context.Background(), db, cfg)
		return map[string]any{
			"summary": summary,
			"results": results,
		}, nil
	})

	writeJSON(w, http.StatusAccepted, map[string]any{
		"success": true,
		"queued":  true,
		"reused":  reused,
		"jobId":   task.ID,
		"taskId":  task.ID,
		"status":  string(task.Status),
		"message": "all-account token sync started; check the program logs later",
	})
}
