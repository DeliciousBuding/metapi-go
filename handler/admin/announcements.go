package admin

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// ---- Product Announcements ----

// Operator-authored severity-ranked banners (替代邮件群发). Dashboard pulls
// GET /api/announcements/active and renders severity-colored banners with a
// dismiss action. Editing content resets the dismissal so a new revision
// surfaces again (dismiss-revision semantics).

// RegisterAnnouncementsRoutes mounts the announcement endpoints.
func RegisterAnnouncementsRoutes(r chi.Router, db *store.DB) {
	h := &announcementsHandler{db: db}
	r.Get("/api/announcements", h.listAll)
	r.Get("/api/announcements/active", h.listActive)
	r.Post("/api/announcements", h.create)
	r.Put("/api/announcements/{id}", h.update)
	r.Delete("/api/announcements/{id}", h.remove)
	r.Post("/api/announcements/{id}/dismiss", h.dismiss)
}

type announcementsHandler struct {
	db *store.DB
}

func validAnnouncementSeverity(s string) bool {
	switch s {
	case "info", "warning", "critical":
		return true
	}
	return false
}

// GET /api/announcements — admin view: all announcements with dismiss state.
func (h *announcementsHandler) listAll(w http.ResponseWriter, r *http.Request) {
	items, err := h.db.ProductAnnouncements(false)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load announcements", "errorCode": "resourceLoadFailed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// GET /api/announcements/active — enabled and not dismissed (Dashboard).
func (h *announcementsHandler) listActive(w http.ResponseWriter, r *http.Request) {
	items, err := h.db.ProductAnnouncements(true)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to load announcements", "errorCode": "resourceLoadFailed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

type announcementBody struct {
	Title    string  `json:"title"`
	Message  string  `json:"message"`
	Severity string  `json:"severity"`
	Link     *string `json:"link"`
	Enabled  *bool   `json:"enabled"`
}

func (body announcementBody) record() store.ProductAnnouncement {
	return store.ProductAnnouncement{
		Title: body.Title, Message: body.Message, Severity: body.Severity,
		Link: body.Link, Enabled: body.Enabled == nil || *body.Enabled,
	}
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
	err := h.db.CreateProductAnnouncement(body.record())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to create announcement"})
		return
	}
	items, err := h.db.ProductAnnouncements(false)
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
	current, err := h.db.AnnouncementContent(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "announcement not found"})
		return
	}
	body, ok := decodeAnnouncementBody(w, r)
	if !ok {
		return
	}
	contentChanged, err := h.db.ReviseProductAnnouncement(id, current, body.record())
	if err != nil {
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
	found, err := h.db.DeleteProductAnnouncement(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to delete announcement"})
		return
	}
	if !found {
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
	err := h.db.DismissProductAnnouncement(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "announcement not found"})
		return
	}

	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to dismiss announcement"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
