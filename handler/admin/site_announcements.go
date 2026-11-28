package admin

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterSiteAnnouncementsRoutes registers all /api/site-announcements routes.
// It wires the scheduler to SyncSiteAnnouncements before background services start.
func RegisterSiteAnnouncementsRoutes(r chi.Router, db *store.DB) {
	wireSiteAnnouncementSchedulerSync()

	handler := &siteAnnouncementsHandler{db: db}

	r.Get("/api/site-announcements", handler.listAnnouncements)
	r.Get("/api/site-announcements/{id}/history", handler.history)
	r.Post("/api/site-announcements/{id}/read", handler.markRead)
	r.Post("/api/site-announcements/read-all", handler.markAllRead)
	r.Delete("/api/site-announcements", handler.deleteAll)
	r.Post("/api/site-announcements/sync", handler.syncAnnouncements)
}

type siteAnnouncementsHandler struct {
	db *store.DB
}

// POST /api/site-announcements/:id/read
func (h *siteAnnouncementsHandler) markRead(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := h.db.Exec("UPDATE site_announcements SET read_at = ? WHERE id = ?", now, id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to mark announcement read"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// POST /api/site-announcements/read-all
func (h *siteAnnouncementsHandler) markAllRead(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := h.db.Exec("UPDATE site_announcements SET read_at = ?", now); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to mark announcements read"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// DELETE /api/site-announcements
func (h *siteAnnouncementsHandler) deleteAll(w http.ResponseWriter, r *http.Request) {
	if _, err := h.db.Exec("DELETE FROM site_announcements"); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to delete announcements"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// POST /api/site-announcements/sync
// Queues a real background sync against active sites (or a single siteId).
func (h *siteAnnouncementsHandler) syncAnnouncements(w http.ResponseWriter, r *http.Request) {
	siteID, err := parseOptionalSiteAnnouncementSyncSiteID(r)
	if err != nil {
		writeErrorWithRequest(w, r, http.StatusBadRequest, err.Error())
		return
	}
	if siteID != nil {
		var exists int
		err := h.db.Get(&exists, `SELECT 1 FROM sites WHERE id = ?`, *siteID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeErrorCodeWithRequest(w, r, http.StatusNotFound, ErrorCodeSiteNotFound, "Site not found.")
			return
		case err != nil:
			slog.Error("site-announcement sync: failed to validate site", "siteId", *siteID, "error", err)
			writeErrorWithRequest(w, r, http.StatusInternalServerError, "Failed to validate site.")
			return
		}
	}

	title := "Sync site announcements"
	dedupeKey := "site-announcements:all"
	if siteID != nil {
		title = fmt.Sprintf("Sync site announcements #%d", *siteID)
		dedupeKey = fmt.Sprintf("site-announcements:%d", *siteID)
	}

	db := h.db
	task, reused := StartBackgroundTask(BackgroundTaskStartOptions{
		Type:      "site-announcements-sync",
		Title:     title,
		DedupeKey: dedupeKey,
	}, func() (any, error) {
		result := SyncSiteAnnouncements(db, siteID)
		if err := siteAnnouncementSyncTerminalError(result); err != nil {
			return result, err
		}
		return result, nil
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"queued":  true,
		"reused":  reused,
		"taskId":  task.ID,
	})
}

func parseOptionalSiteAnnouncementSyncSiteID(r *http.Request) (*int64, error) {
	raw, err := readAdminJSONBody(r.Body)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, nil
	}
	if raw[0] != '{' {
		return nil, fmt.Errorf("request body must be a JSON object")
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return nil, err
	}

	var body map[string]json.RawMessage
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	for key := range body {
		if key != "siteId" {
			return nil, fmt.Errorf("unknown request field %q", key)
		}
	}
	value := bytes.TrimSpace(body["siteId"])
	if len(value) == 0 {
		return nil, nil
	}
	if bytes.Equal(value, []byte("null")) {
		return nil, fmt.Errorf("siteId must be a positive integer")
	}

	if value[0] < '0' || value[0] > '9' {
		return nil, fmt.Errorf("siteId must be a positive integer")
	}
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil {
		return nil, fmt.Errorf("siteId must be a positive integer")
	}
	id, err := strconv.ParseInt(number.String(), 10, 64)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("siteId must be a positive integer")
	}
	return &id, nil
}

func siteAnnouncementSyncTerminalError(result SiteAnnouncementSyncResult) error {
	if result.Failed == 0 {
		return nil
	}
	if result.ScannedSites == 0 || (result.Inserted+result.Updated == 0 && result.Failed == result.ScannedSites) {
		return fmt.Errorf("site announcement sync failed for all targeted sites")
	}
	return nil
}
