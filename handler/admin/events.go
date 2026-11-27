package admin

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/service/events"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterEventsRoutes registers all /api/events routes.
func RegisterEventsRoutes(r chi.Router, db *store.DB) {
	handler := &eventsHandler{db: db}

	r.Get("/api/events", handler.listEvents)
	r.Get("/api/events/count", handler.countUnread)
	r.Post("/api/events/{id}/read", handler.markRead)
	r.Post("/api/events/read-all", handler.markAllRead)
	r.Delete("/api/events", handler.deleteAll)
}

type eventsHandler struct {
	db *store.DB
}

// ---- List Events ----
// GET /api/events?limit=&offset=&type=&read=
func (h *eventsHandler) listEvents(w http.ResponseWriter, r *http.Request) {
	limit, _ := parseLimitOffset(r, 30, 500)
	offset := max(0, getQueryInt(r, "offset", 0))
	typeFilter := strings.TrimSpace(r.URL.Query().Get("type"))
	readFilter := strings.TrimSpace(r.URL.Query().Get("read"))

	var read *bool
	if readFilter == "true" || readFilter == "false" {
		value := readFilter == "true"
		read = &value
	}
	items, err := events.List(h.db, limit, offset, typeFilter, read)
	if err != nil {
		slog.Error("Failed to load events", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load events", "errorCode": "resourceLoadFailed"})
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// ---- Unread Count ----
// GET /api/events/count
func (h *eventsHandler) countUnread(w http.ResponseWriter, r *http.Request) {
	count, err := events.UnreadCount(h.db)
	if err != nil {
		slog.Error("Failed to count unread events", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load events", "errorCode": "resourceLoadFailed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": count})
}

// ---- Mark One Read ----
// POST /api/events/:id/read
func (h *eventsHandler) markRead(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	_ = events.MarkRead(h.db, &id)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// ---- Mark All Read ----
// POST /api/events/read-all
func (h *eventsHandler) markAllRead(w http.ResponseWriter, r *http.Request) {
	_ = events.MarkRead(h.db, nil)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// ---- Delete All ----
// DELETE /api/events
func (h *eventsHandler) deleteAll(w http.ResponseWriter, r *http.Request) {
	_ = events.DeleteAll(h.db)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}
