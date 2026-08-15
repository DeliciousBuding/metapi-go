package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// ---- Model Name Redirects ----
//

// RegisterModelRedirectRoutes mounts the redirect endpoints.
func RegisterModelRedirectRoutes(r chi.Router, db *sqlx.DB) {
	h := &modelRedirectHandler{db: db}
	r.Get("/api/model-redirects", h.list)
	r.Put("/api/model-redirects/{id}", h.update)
	r.Delete("/api/model-redirects/{id}", h.remove)
	r.Post("/api/model-redirects/generate", h.generate)
	r.Post("/api/model-redirects/apply", h.apply)
}

type modelRedirectHandler struct {
	db *sqlx.DB
}

// RegisterModelRedirectFixRoutes mounts the model-governance fix-candidate
// endpoints. They wrap service.ListRedirectFixCandidates /
// service.ApplyRedirectFixes so the admin SPA can review and apply
// redirect-repairable disabled models without driving raw SQL.
func RegisterModelRedirectFixRoutes(r chi.Router, db *sqlx.DB) {
	h := &modelRedirectHandler{db: db}
	r.Get("/api/models/redirect-fix-candidates", h.listFixCandidates)
	r.Post("/api/models/redirect-fix-candidates", h.applyFixCandidates)
}

// GET /api/models/redirect-fix-candidates
func (h *modelRedirectHandler) listFixCandidates(w http.ResponseWriter, r *http.Request) {
	candidates, err := service.ListRedirectFixCandidates(r.Context(), h.db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to list fix candidates"})
		return
	}
	if candidates == nil {
		candidates = []service.RedirectFixCandidate{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": candidates, "count": len(candidates)})
}

// POST /api/models/redirect-fix-candidates — body {dryRun?: bool}.
// dryRun=true reports without deleting; the default (false) applies the
// fixes, records an event per candidate, and reloads the hot-path registry.
func (h *modelRedirectHandler) applyFixCandidates(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DryRun *bool `json:"dryRun"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}
	dryRun := body.DryRun != nil && *body.DryRun

	candidates, err := service.ListRedirectFixCandidates(r.Context(), h.db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to list fix candidates"})
		return
	}

	if dryRun {
		writeJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"dryRun":  true,
			"items":   candidates,
			"count":   len(candidates),
		})
		return
	}

	removed, err := service.ApplyRedirectFixes(r.Context(), h.db, candidates)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "apply failed: " + err.Error()})
		return
	}
	for _, c := range candidates {
		_ = recordRedirectEvent(h.db, c)
	}
	service.ReloadRedirectRegistry(r.Context(), h.db) // K1b: keep hot-path registry fresh
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"dryRun":  false,
		"removed": removed,
		"count":   len(candidates),
	})
}

// GET /api/model-redirects?accountId=&source=
func (h *modelRedirectHandler) list(w http.ResponseWriter, r *http.Request) {
	accountID := getQueryInt(r, "accountId", 0)
	source := strings.TrimSpace(r.URL.Query().Get("source"))

	q := `SELECT mr.id, mr.account_id, mr.canonical, mr.actual, mr.source, mr.last_seen_at, mr.created_at, mr.updated_at,
		COALESCE(a.username, '') AS username, COALESCE(s.name, '') AS site_name
		FROM model_name_redirects mr
		LEFT JOIN accounts a ON a.id = mr.account_id
		LEFT JOIN sites s ON s.id = a.site_id`
	var args []any
	var conds []string
	if accountID > 0 {
		conds = append(conds, "mr.account_id = ?")
		args = append(args, accountID)
	}
	if source == "sync" || source == "manual" {
		conds = append(conds, "mr.source = ?")
		args = append(args, source)
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += " ORDER BY mr.updated_at DESC, mr.id DESC"

	rows, err := queryRowsErr(h.db, q, args...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list model redirects")
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, map[string]any{
			"id":         coerceInt64(row["id"]),
			"accountId":  coerceInt64(row["accountId"]),
			"username":   coerceString(row["username"]),
			"siteName":   coerceString(row["siteName"]),
			"canonical":  coerceString(row["canonical"]),
			"actual":     coerceString(row["actual"]),
			"source":     coerceString(row["source"]),
			"lastSeenAt": row["lastSeenAt"],
			"createdAt":  coerceString(row["createdAt"]),
			"updatedAt":  coerceString(row["updatedAt"]),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// PUT /api/model-redirects/{id} — switch to manual / correct actual.
func (h *modelRedirectHandler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var body struct {
		Actual *string `json:"actual"`
		Source *string `json:"source"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}
	if body.Actual != nil {
		actual := strings.TrimSpace(*body.Actual)
		if actual == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"message": "actual must not be empty"})
			return
		}
		*body.Actual = actual
	}
	if body.Source != nil && *body.Source != "manual" && *body.Source != "sync" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "source must be sync or manual"})
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)

	// Build the SET clause dynamically with explicit column assignment.
	var setParts []string
	var args []any
	if body.Actual != nil {
		setParts = append(setParts, "actual = ?")
		args = append(args, *body.Actual)
	}
	if body.Source != nil {
		setParts = append(setParts, "source = ?")
		args = append(args, *body.Source)
	}
	if len(setParts) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "nothing to update"})
		return
	}
	setParts = append(setParts, "updated_at = ?")
	args = append(args, now, id)

	res, err := h.db.Exec(rebindAdminQuery(h.db, "UPDATE model_name_redirects SET "+strings.Join(setParts, ", ")+" WHERE id = ?"), args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to update redirect"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "redirect not found"})
		return
	}
	service.ReloadRedirectRegistry(r.Context(), h.db) // K1b: keep hot-path registry fresh
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// DELETE /api/model-redirects/{id}
func (h *modelRedirectHandler) remove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := h.db.Exec(rebindAdminQuery(h.db, "DELETE FROM model_name_redirects WHERE id = ?"), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to delete redirect"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "redirect not found"})
		return
	}
	service.ReloadRedirectRegistry(r.Context(), h.db) // K1b: keep hot-path registry fresh
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// POST /api/model-redirects/generate — body {accountId?, models?}
// Runs sync generation for one account (or all accounts with availability).
// Regeneration is idempotent; manual mappings are never overwritten.
func (h *modelRedirectHandler) generate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccountID int64    `json:"accountId"`
		Models    []string `json:"models"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}

	if body.AccountID > 0 {
		models := body.Models
		if len(models) == 0 {
			// Deterministic order: insertion order ≈ upstream return order.
			rows, err := queryRowsErr(h.db, "SELECT model_name FROM model_availability WHERE account_id = ? AND available = 1 ORDER BY id ASC", body.AccountID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to load account models")
				return
			}
			for _, row := range rows {
				models = append(models, coerceString(row["modelName"]))
			}
		}
		created, err := service.GenerateModelRedirects(r.Context(), h.db, body.AccountID, models)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "generate failed: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "created": created})
		service.ReloadRedirectRegistry(r.Context(), h.db) // K1b: keep hot-path registry fresh
		return
	}

	// All accounts: iterate accounts with available models.
	type acctModel struct {
		AccountID int64  `db:"account_id"`
		ModelName string `db:"model_name"`
	}
	var rows []acctModel
	if err := h.db.Select(&rows, "SELECT account_id, model_name FROM model_availability WHERE available = 1"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "generate failed: " + err.Error()})
		return
	}
	byAccount := make(map[int64][]string)
	for _, row := range rows {
		byAccount[row.AccountID] = append(byAccount[row.AccountID], row.ModelName)
	}
	total := 0
	for accountID, models := range byAccount {
		n, err := service.GenerateModelRedirects(r.Context(), h.db, accountID, models)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "generate failed: " + err.Error()})
			return
		}
		total += n
	}
	service.ReloadRedirectRegistry(r.Context(), h.db) // K1b: keep hot-path registry fresh
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "created": total, "accounts": len(byAccount)})
}

// POST /api/model-redirects/apply — body {dryRun: true}
// Lists disabled-model entries fixable via redirects; dryRun=true reports
// without deleting (default). dryRun=false deletes and records events.
func (h *modelRedirectHandler) apply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		DryRun *bool `json:"dryRun"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return
	}
	dryRun := true
	if body.DryRun != nil {
		dryRun = *body.DryRun
	}

	candidates, err := service.ListRedirectFixCandidates(r.Context(), h.db)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to list fix candidates"})
		return
	}

	if dryRun {
		writeJSON(w, http.StatusOK, map[string]any{
			"success":    true,
			"dryRun":     true,
			"candidates": candidates,
			"count":      len(candidates),
		})
		return
	}

	removed, err := service.ApplyRedirectFixes(r.Context(), h.db, candidates)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "apply failed: " + err.Error()})
		return
	}
	// Record an event per removal for the attention surface.
	for _, c := range candidates {
		_ = recordRedirectEvent(h.db, c)
	}
	service.ReloadRedirectRegistry(r.Context(), h.db) // K1b: keep hot-path registry fresh
	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"dryRun":  false,
		"removed": removed,
		"count":   len(candidates),
	})
}

// recordRedirectEvent logs a disabled-model re-enable to the events table
// (type model_redirect_applied) so the attention surface / ops can trace it.
func recordRedirectEvent(db *sqlx.DB, c service.RedirectFixCandidate) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(rebindAdminQuery(db, `
		INSERT INTO events (type, title, message, level, related_id, related_type, created_at, read)
		VALUES ('model_redirect_applied', ?, ?, 'info', ?, 'site', ?, 0)`),
		"已修复禁用模型（映射自动恢复）",
		"站点 "+c.SiteName+" 的禁用模型 "+c.ModelName+" 已通过映射 "+c.Canonical+" → "+c.Actual+" 恢复可用。",
		c.SiteID, now)
	return err
}
