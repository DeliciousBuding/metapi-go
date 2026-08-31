package admin

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// ---- Product Announcements ----

// Operator-authored severity-ranked banners (替代邮件群发). Dashboard pulls
// GET /api/announcements/active and renders severity-colored banners with a
// dismiss action. Editing content resets the dismissal so a new revision
// surfaces again (dismiss-revision semantics).

// RegisterAnnouncementsRoutes mounts the announcement endpoints.
func RegisterAnnouncementsRoutes(r chi.Router, db *sqlx.DB) {
	h := &announcementsHandler{db: db}
	r.Get("/api/announcements", h.listAll)
	r.Get("/api/announcements/active", h.listActive)
	r.Post("/api/announcements", h.create)
	r.Put("/api/announcements/{id}", h.update)
	r.Delete("/api/announcements/{id}", h.remove)
	r.Post("/api/announcements/{id}/dismiss", h.dismiss)
}

type announcementsHandler struct {
	db *sqlx.DB
}

func validAnnouncementSeverity(s string) bool {
	switch s {
	case "info", "warning", "critical":
		return true
	}
	return false
}

// coerceBool normalizes dialect-dependent boolean scans (SQLite 0/1, PG bool).
func coerceBool(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case int64:
		return b != 0
	case float64:
		return b != 0
	case string:
		return b == "true" || b == "1" || b == "t"
	}
	return false
}

// loadAnnouncements returns announcements with their dismissed state.
func (h *announcementsHandler) loadAnnouncements(enabledOnly bool) ([]map[string]any, error) {
	q := `SELECT a.id, a.title, a.message, a.severity, a.link, a.enabled, a.created_at, a.updated_at,
		(d.dismissed_at IS NOT NULL) AS dismissed, d.dismissed_at AS dismissed_at
		FROM product_announcements a
		LEFT JOIN announcement_dismissals d ON d.announcement_id = a.id`
	var args []any
	if enabledOnly {
		q += ` WHERE a.enabled = TRUE`
	}
	q += ` ORDER BY CASE a.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END, a.updated_at DESC`

	rows, err := queryRowsErr(h.db, q, args...)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		item := map[string]any{
			"id":          coerceInt64(row["id"]),
			"title":       coerceString(row["title"]),
			"message":     coerceString(row["message"]),
			"severity":    coerceString(row["severity"]),
			"link":        row["link"],
			"enabled":     coerceBool(row["enabled"]),
			"dismissed":   coerceBool(row["dismissed"]),
			"dismissedAt": row["dismissedAt"],
			"createdAt":   coerceString(row["createdAt"]),
			"updatedAt":   coerceString(row["updatedAt"]),
		}
		items = append(items, item)
	}
	return items, nil
}

// GET /api/announcements — admin view: all announcements with dismiss state.
func (h *announcementsHandler) listAll(w http.ResponseWriter, r *http.Request) {
	items, err := h.loadAnnouncements(false)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load announcements", "errorCode": "resourceLoadFailed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// GET /api/announcements/active — enabled and not dismissed (Dashboard).
func (h *announcementsHandler) listActive(w http.ResponseWriter, r *http.Request) {
	items, err := h.loadAnnouncements(true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load announcements", "errorCode": "resourceLoadFailed"})
		return
	}
	visible := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if item["dismissed"] == true {
			continue
		}
		visible = append(visible, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": visible})
}

type announcementBody struct {
	Title    string  `json:"title"`
	Message  string  `json:"message"`
	Severity string  `json:"severity"`
	Link     *string `json:"link"`
	Enabled  *bool   `json:"enabled"`
}

// decodeAnnouncementBody validates a create/update payload; writes a 400 and
// returns ok=false on error.
func decodeAnnouncementBody(w http.ResponseWriter, r *http.Request) (announcementBody, bool) {
	var body announcementBody
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "invalid JSON body"})
		return body, false
	}
	body.Title = strings.TrimSpace(body.Title)
	body.Message = strings.TrimSpace(body.Message)
	if body.Title == "" || body.Message == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "title and message are required"})
		return body, false
	}
	if body.Severity == "" {
		body.Severity = "info"
	}
	if !validAnnouncementSeverity(body.Severity) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "severity must be info, warning or critical"})
		return body, false
	}
	if body.Link != nil {
		link := strings.TrimSpace(*body.Link)
		body.Link = &link
	}
	return body, true
}

// POST /api/announcements
func (h *announcementsHandler) create(w http.ResponseWriter, r *http.Request) {
	body, ok := decodeAnnouncementBody(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	_, err := h.db.Exec(rebindAdminQuery(h.db, `
		INSERT INTO product_announcements (title, message, severity, link, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`),
		body.Title, body.Message, body.Severity, body.Link, enabled, now, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to create announcement"})
		return
	}
	items, err := h.loadAnnouncements(false)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "created but failed to reload", "errorCode": "resourceLoadFailed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "items": items})
}

// PUT /api/announcements/{id} — edit; content changes reset the dismissal so
// the new revision is seen again.
func (h *announcementsHandler) update(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var current struct {
		Title   string  `db:"title"`
		Message string  `db:"message"`
		Link    *string `db:"link"`
	}
	if err := h.db.Get(&current, rebindAdminQuery(h.db, "SELECT title, message, link FROM product_announcements WHERE id = ?"), id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "announcement not found"})
		return
	}
	body, ok := decodeAnnouncementBody(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}

	// Content revision: title/message/link changed → drop the dismissal.
	contentChanged := current.Title != body.Title || current.Message != body.Message ||
		(current.Link == nil) != (body.Link == nil) ||
		(current.Link != nil && body.Link != nil && *current.Link != *body.Link)

	tx, err := h.db.Beginx()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to update announcement"})
		return
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(tx.Rebind(`UPDATE product_announcements
		SET title = ?, message = ?, severity = ?, link = ?, enabled = ?, updated_at = ?
		WHERE id = ?`),
		body.Title, body.Message, body.Severity, body.Link, enabled, now, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to update announcement"})
		return
	}
	if contentChanged {
		_, err = tx.Exec(tx.Rebind("DELETE FROM announcement_dismissals WHERE announcement_id = ?"), id)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to update announcement"})
			return
		}
	}
	if err := tx.Commit(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to update announcement"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "revision": contentChanged})
}

// DELETE /api/announcements/{id}
func (h *announcementsHandler) remove(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	res, err := h.db.Exec(rebindAdminQuery(h.db, "DELETE FROM product_announcements WHERE id = ?"), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to delete announcement"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "announcement not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// POST /api/announcements/{id}/dismiss
func (h *announcementsHandler) dismiss(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var exists int
	if err := h.db.Get(&exists, "SELECT COUNT(*) FROM product_announcements WHERE id = ?", id); err != nil || exists == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "announcement not found"})
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := h.db.Exec(rebindAdminQuery(h.db, `
		INSERT INTO announcement_dismissals (announcement_id, dismissed_at)
		VALUES (?, ?)
		ON CONFLICT (announcement_id) DO UPDATE SET dismissed_at = excluded.dismissed_at`), id, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to dismiss announcement"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
