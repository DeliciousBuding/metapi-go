package admin

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
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

	// Every persisted availability row for the account — available AND
	// unavailable — so the panel renders honest source/availability state:
	// manual rows stay pinned available; auto rows a refresh no longer
	// observes upstream flip to unavailable instead of silently vanishing.
	type modelRow struct {
		ModelName string  `db:"model_name"`
		Available bool    `db:"available"`
		LatencyMs *int64  `db:"latency_ms"`
		IsManual  bool    `db:"is_manual"`
		CheckedAt *string `db:"checked_at"`
	}
	var modelRows []modelRow
	if err := h.db.Select(&modelRows, h.db.Rebind("SELECT model_name, available, latency_ms, is_manual, checked_at FROM model_availability WHERE account_id = ? ORDER BY model_name ASC"), id); err != nil {
		slog.Error("failed to load account model availability", "account_id", id, "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load model availability"})
		return
	}

	// Get disabled models for this site
	var disabledRows []string
	h.db.Select(&disabledRows, h.db.Rebind("SELECT model_name FROM site_disabled_models WHERE site_id = ?"), row.Account.SiteID)

	disabledSet := map[string]bool{}
	for _, m := range disabledRows {
		disabledSet[m] = true
	}

	models := make([]map[string]any, 0, len(modelRows))
	for _, r := range modelRows {
		models = append(models, map[string]any{
			"name":      r.ModelName,
			"available": r.Available,
			"latencyMs": r.LatencyMs,
			"disabled":  disabledSet[r.ModelName],
			"isManual":  r.IsManual,
			"checkedAt": r.CheckedAt,
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

// accountManualModelsRequest is the wire shape for
// POST /api/accounts/{id}/models/manual. It embeds the TS-parity add payload
// and extends it with an explicit remove list (#998): manual rows are
// operator-owned and can be deleted explicitly; auto-discovered rows belong
// to the upstream refresh owner and are never deleted through this endpoint.
type accountManualModelsRequest struct {
	payloads.AccountManualModelsPayload
	Remove []string `json:"remove"`
}

func (h *accountsHandler) manualModels(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var body accountManualModelsRequest
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid models. Expected string[]."})
		return
	}

	add := cleanModelList(body.Models)
	remove := cleanModelList(body.Remove)
	if len(add) == 0 && len(remove) == 0 {
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

	for _, m := range add {
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

	// Deletions run after the upserts, so when a name appears in both lists
	// the explicit remove wins. The is_manual predicate keeps auto rows —
	// owned by the refresh owner — untouched; a remove targeting one is a
	// silent no-op reflected in the reported removed count.
	var removed int64
	for _, m := range remove {
		res, err := tx.Exec(tx.Rebind(
			"DELETE FROM model_availability WHERE account_id = ? AND model_name = ? AND is_manual = ?",
		), id, m, true)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to remove manual model"})
			return
		}
		if n, err := res.RowsAffected(); err == nil {
			removed += n
		}
	}

	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to commit manual models"})
		return
	}
	committed = true

	invalidateRoutingCache()
	globalAccountsCache.clear()
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"added":   len(add),
		"removed": removed,
	})
}

// cleanModelList trims whitespace and drops empty/duplicate entries while
// preserving first-seen casing (manual model names are case-sensitive).
func cleanModelList(raw []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, m := range raw {
		m = strings.TrimSpace(m)
		if m != "" && !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
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
