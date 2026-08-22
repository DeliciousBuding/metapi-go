package admin

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func (h *accountsHandler) getAccountModels(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	row, err := service.GetAccountWithSiteByID(h.db, id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "account not found"})
		return
	}

	// Get available models for this account
	type modelRow struct {
		ModelName string `db:"model_name"`
		Available int    `db:"available"`
		LatencyMs *int64 `db:"latency_ms"`
		IsManual  int    `db:"is_manual"`
	}
	var modelRows []modelRow
	h.db.Select(&modelRows, h.db.Rebind("SELECT model_name, CASE WHEN available THEN 1 ELSE 0 END AS available, latency_ms, CASE WHEN is_manual THEN 1 ELSE 0 END AS is_manual FROM model_availability WHERE account_id = ?"), id)

	// Get disabled models for this site
	var disabledRows []string
	h.db.Select(&disabledRows, h.db.Rebind("SELECT model_name FROM site_disabled_models WHERE site_id = ?"), row.Account.SiteID)

	disabledSet := map[string]bool{}
	for _, m := range disabledRows {
		disabledSet[m] = true
	}

	var models []map[string]any
	for _, r := range modelRows {
		if r.Available == 0 {
			continue
		}
		models = append(models, map[string]any{
			"name":      r.ModelName,
			"latencyMs": r.LatencyMs,
			"disabled":  disabledSet[r.ModelName],
			"isManual":  r.IsManual == 1,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"siteId":        row.Account.SiteID,
		"siteName":      row.Site.Name,
		"models":        models,
		"totalCount":    len(models),
		"disabledCount": countDisabled(models),
	})
}

func (h *accountsHandler) manualModels(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var body payloads.AccountManualModelsPayload
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid models. Expected string[]."})
		return
	}

	if len(body.Models) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model list must not be empty"})
		return
	}

	// Deduplicate
	seen := map[string]bool{}
	var models []string
	for _, m := range body.Models {
		m = strings.TrimSpace(m)
		if m != "" && !seen[m] {
			seen[m] = true
			models = append(models, m)
		}
	}
	if len(models) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "model list must not be empty"})
		return
	}

	var account store.Account
	if err := h.db.Get(&account, h.db.Rebind("SELECT * FROM accounts WHERE id = ?"), id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "account not found"})
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := h.db.Beginx()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to start transaction"})
		return
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	for _, m := range models {
		// Single dialect-aware upsert replaces the SELECT-then-UPDATE-or-INSERT
		// pattern (3 round-trips/model → 1). ON CONFLICT (account_id, model_name)
		// targets the model_availability_account_model_unique constraint. Manual
		// models set is_manual = true on both insert and conflict update so the
		// operator-pinned flag is always reasserted (matches the old behavior).
		if _, err := tx.Exec(tx.Rebind(`
			INSERT INTO model_availability (account_id, model_name, available, is_manual, latency_ms, checked_at)
			VALUES (?, ?, ?, ?, NULL, ?)
			ON CONFLICT (account_id, model_name) DO UPDATE SET
				available = EXCLUDED.available,
				is_manual = EXCLUDED.is_manual,
				latency_ms = NULL,
				checked_at = EXCLUDED.checked_at
		`), id, m, true, true, now); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to upsert manual model"})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit manual models"})
		return
	}
	committed = true

	routing.InvalidateCache()
	globalAccountsCache.clear()
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// ---- Helpers ----

func buildBatchAPIKeyName(username string, index, total int) string {
	if username == "" {
		return fmt.Sprintf("key-%d", index+1)
	}
	return fmt.Sprintf("%s-%d", username, index+1)
}

func countDisabled(models []map[string]any) int {
	count := 0
	for _, m := range models {
		if d, ok := m["disabled"].(bool); ok && d {
			count++
		}
	}
	return count
}
