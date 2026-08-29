package admin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/notify"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterSiteAnnouncementsRoutes registers all /api/site-announcements routes.
// Also wires the background SiteAnnouncementScheduler to real SyncSiteAnnouncements
// . Routes register before StartBackgroundServices, so the package-level
// default is installed for NewSiteAnnouncementScheduler to pick up.
func RegisterSiteAnnouncementsRoutes(r chi.Router, db *sqlx.DB) {
	wireSiteAnnouncementSchedulerSync()

	handler := &siteAnnouncementsHandler{db: db}

	r.Get("/api/site-announcements", handler.listAnnouncements)
	r.Post("/api/site-announcements/{id}/read", handler.markRead)
	r.Post("/api/site-announcements/read-all", handler.markAllRead)
	r.Delete("/api/site-announcements", handler.deleteAll)
	r.Post("/api/site-announcements/sync", handler.syncAnnouncements)
}

// wireSiteAnnouncementSchedulerSync injects admin SyncSiteAnnouncements into the
// scheduler package without creating an app→admin import cycle (admin already
// imports scheduler; app cannot import admin).
func wireSiteAnnouncementSchedulerSync() {
	scheduler.SetDefaultSiteAnnouncementSyncFunc(func(db *sqlx.DB) scheduler.SiteAnnouncementSyncResult {
		result := SyncSiteAnnouncements(db, nil)
		return scheduler.SiteAnnouncementSyncResult{
			ScannedSites:  result.ScannedSites,
			Inserted:      result.Inserted,
			Updated:       result.Updated,
			Unsupported:   result.Unsupported,
			Notifications: result.Notifications,
			Events:        result.Events,
			Failed:        result.Failed,
		}
	})
}

type siteAnnouncementsHandler struct {
	db *sqlx.DB
}

// SiteAnnouncementSyncResult mirrors the TS syncSiteAnnouncements result shape.
// Used by the admin HTTP task path and by SiteAnnouncementScheduler via SyncSiteAnnouncements.
type SiteAnnouncementSyncResult struct {
	ScannedSites  int                          `json:"scannedSites"`
	Inserted      int                          `json:"inserted"`
	Updated       int                          `json:"updated"`
	Unsupported   int                          `json:"unsupported"`
	Notifications int                          `json:"notifications"`
	Events        int                          `json:"events"`
	Failed        int                          `json:"failed"`
	FailedSites   []SiteAnnouncementFailedSite `json:"failedSites"`
}

// SiteAnnouncementFailedSite is one site that failed during announcement sync.
type SiteAnnouncementFailedSite struct {
	SiteID   int64  `json:"siteId"`
	SiteName string `json:"siteName"`
	Message  string `json:"message"`
}

// GET /api/site-announcements?limit=&offset=&siteId=&platform=&read=&status=
func (h *siteAnnouncementsHandler) listAnnouncements(w http.ResponseWriter, r *http.Request) {
	limit, _ := parseLimitOffset(r, 50, 500)
	offset := max(0, getQueryInt(r, "offset", 0))
	readFilter := strings.TrimSpace(r.URL.Query().Get("read"))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	siteIDFilter := strings.TrimSpace(r.URL.Query().Get("siteId"))
	platformFilter := strings.TrimSpace(r.URL.Query().Get("platform"))

	var conditions []string
	var args []any

	if siteIDFilter != "" {
		if id, err := strconv.ParseInt(siteIDFilter, 10, 64); err == nil && id > 0 {
			conditions = append(conditions, "site_id = ?")
			args = append(args, id)
		}
	}
	if platformFilter != "" {
		conditions = append(conditions, "platform = ?")
		args = append(args, platformFilter)
	}

	query := "SELECT * FROM site_announcements"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY first_seen_at DESC"

	rows, err := h.db.Queryx(h.db.Rebind(query), args...)
	if err != nil {
		slog.Error("Failed to load announcements", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load announcements"})
		return
	}
	defer rows.Close()

	var all []map[string]any
	for rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			continue
		}
		all = append(all, mapKeysToCamel(row))
	}

	// Apply read filter
	if readFilter == "true" {
		filtered := make([]map[string]any, 0)
		for _, row := range all {
			if hasValue(row["readAt"]) {
				filtered = append(filtered, row)
			}
		}
		all = filtered
	} else if readFilter == "false" {
		filtered := make([]map[string]any, 0)
		for _, row := range all {
			if !hasValue(row["readAt"]) {
				filtered = append(filtered, row)
			}
		}
		all = filtered
	}

	// Apply status filter
	now := time.Now()
	if statusFilter == "dismissed" {
		filtered := make([]map[string]any, 0)
		for _, row := range all {
			if hasValue(row["dismissedAt"]) {
				filtered = append(filtered, row)
			}
		}
		all = filtered
	} else if statusFilter == "active" {
		filtered := make([]map[string]any, 0)
		for _, row := range all {
			if !hasValue(row["dismissedAt"]) {
				endsAt := parseTime(row["endsAt"])
				if endsAt == nil || !endsAt.Before(now) {
					filtered = append(filtered, row)
				}
			}
		}
		all = filtered
	} else if statusFilter == "expired" {
		filtered := make([]map[string]any, 0)
		for _, row := range all {
			if !hasValue(row["dismissedAt"]) {
				endsAt := parseTime(row["endsAt"])
				if endsAt != nil && endsAt.Before(now) {
					filtered = append(filtered, row)
				}
			}
		}
		all = filtered
	}

	// Apply pagination after filters
	if offset >= len(all) {
		all = []map[string]any{}
	} else {
		end := offset + limit
		if end > len(all) {
			end = len(all)
		}
		all = all[offset:end]
	}

	writeJSON(w, http.StatusOK, normalizeSlice(all))
}

// POST /api/site-announcements/:id/read
func (h *siteAnnouncementsHandler) markRead(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	h.db.Exec(h.db.Rebind("UPDATE site_announcements SET read_at = ? WHERE id = ?"), now, id)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// POST /api/site-announcements/read-all
func (h *siteAnnouncementsHandler) markAllRead(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC().Format(time.RFC3339)
	h.db.Exec(h.db.Rebind("UPDATE site_announcements SET read_at = ?"), now)
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// DELETE /api/site-announcements
func (h *siteAnnouncementsHandler) deleteAll(w http.ResponseWriter, r *http.Request) {
	h.db.Exec("DELETE FROM site_announcements")
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
		err := h.db.Get(&exists, h.db.Rebind(`SELECT 1 FROM sites WHERE id = ?`), *siteID)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			writeErrorWithRequest(w, r, http.StatusNotFound, "Site not found.")
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

// SyncSiteAnnouncements fetches announcements for active sites (or one site)
// via platform adapters and upserts site_announcements. Exported so the
// background SiteAnnouncementScheduler can reuse this path without HTTP.
func SyncSiteAnnouncements(db *sqlx.DB, siteID *int64) SiteAnnouncementSyncResult {
	result := SiteAnnouncementSyncResult{
		FailedSites: []SiteAnnouncementFailedSite{},
	}

	type siteRow struct {
		ID       int64   `db:"id"`
		Name     string  `db:"name"`
		URL      string  `db:"url"`
		Platform string  `db:"platform"`
		APIKey   *string `db:"api_key"`
	}

	var sites []siteRow
	var err error
	if siteID != nil && *siteID > 0 {
		err = db.Select(&sites, db.Rebind(`SELECT id, name, url, platform, api_key FROM sites WHERE id = ?`), *siteID)
	} else {
		err = db.Select(&sites, `SELECT id, name, url, platform, api_key FROM sites WHERE status = 'active'`)
	}
	if err != nil {
		slog.Error("site-announcement sync: failed to query sites", "error", err)
		result.Failed++
		result.FailedSites = append(result.FailedSites, SiteAnnouncementFailedSite{
			SiteID:   0,
			SiteName: "",
			Message:  err.Error(),
		})
		return result
	}

	ctx := context.Background()
	for _, site := range sites {
		result.ScannedSites++
		adapter := platform.GetAdapter(site.Platform)
		if adapter == nil {
			result.Unsupported++
			continue
		}

		accessToken := strings.TrimSpace(stringPtrValue(site.APIKey))
		if accessToken == "" {
			accessToken = resolveSiteAccessToken(db, site.ID)
		}

		anns, annErr := adapter.GetSiteAnnouncements(ctx, site.URL, accessToken, nil, nil)
		if annErr != nil {
			result.Failed++
			result.FailedSites = append(result.FailedSites, SiteAnnouncementFailedSite{
				SiteID:   site.ID,
				SiteName: site.Name,
				Message:  annErr.Error(),
			})
			continue
		}

		seenAt := service.FormatUtcSqlDateTime(time.Now())
		for _, announcement := range anns {
			sourceKey := strings.TrimSpace(announcement.SourceKey)
			if sourceKey == "" {
				continue
			}

			rawPayload := ""
			if len(announcement.RawPayload) > 0 {
				rawPayload = string(announcement.RawPayload)
			}

			tx, beginErr := db.Beginx()
			if beginErr != nil {
				recordSiteAnnouncementFailure(&result, site.ID, site.Name, "begin transaction", beginErr)
				continue
			}
			var announcementID int64
			insertErr := tx.QueryRowx(tx.Rebind(`
				INSERT INTO site_announcements (
					site_id, platform, source_key, title, content, level, source_url,
					starts_at, ends_at, upstream_created_at, upstream_updated_at,
					first_seen_at, last_seen_at, raw_payload
				) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT (site_id, source_key) DO NOTHING
				RETURNING id
			`),
				site.ID,
				site.Platform,
				sourceKey,
				announcement.Title,
				announcement.Content,
				defaultLevel(announcement.Level),
				emptyToNil(announcement.SourceURL),
				emptyToNil(announcement.StartsAt),
				emptyToNil(announcement.EndsAt),
				emptyToNil(announcement.UpstreamCreatedAt),
				emptyToNil(announcement.UpstreamUpdatedAt),
				seenAt,
				seenAt,
				emptyToNil(rawPayload),
			).Scan(&announcementID)
			isNew := insertErr == nil
			if insertErr != nil && !errors.Is(insertErr, sql.ErrNoRows) {
				_ = tx.Rollback()
				recordSiteAnnouncementFailure(&result, site.ID, site.Name, "insert", insertErr)
				continue
			}

			if !isNew {
				updateResult, updateErr := tx.Exec(tx.Rebind(`
					UPDATE site_announcements
					SET platform = ?, title = ?, content = ?, level = ?, source_url = ?,
					    starts_at = ?, ends_at = ?, upstream_created_at = ?, upstream_updated_at = ?,
					    last_seen_at = ?, raw_payload = ?
					WHERE site_id = ? AND source_key = ?
				`),
					site.Platform,
					announcement.Title,
					announcement.Content,
					defaultLevel(announcement.Level),
					emptyToNil(announcement.SourceURL),
					emptyToNil(announcement.StartsAt),
					emptyToNil(announcement.EndsAt),
					emptyToNil(announcement.UpstreamCreatedAt),
					emptyToNil(announcement.UpstreamUpdatedAt),
					seenAt,
					emptyToNil(rawPayload),
					site.ID,
					sourceKey,
				)
				if updateErr != nil {
					_ = tx.Rollback()
					recordSiteAnnouncementFailure(&result, site.ID, site.Name, "update", updateErr)
					continue
				}
				rows, rowsErr := updateResult.RowsAffected()
				if rowsErr != nil || rows != 1 {
					_ = tx.Rollback()
					if rowsErr == nil {
						rowsErr = fmt.Errorf("updated %d rows after conflict, want 1", rows)
					}
					recordSiteAnnouncementFailure(&result, site.ID, site.Name, "update", rowsErr)
					continue
				}
				if commitErr := tx.Commit(); commitErr != nil {
					recordSiteAnnouncementFailure(&result, site.ID, site.Name, "commit", commitErr)
					continue
				}
				result.Updated++
				continue
			}

			title := "Site announcement: " + site.Name
			message := buildAnnouncementMessage(announcement)
			if _, eventErr := tx.Exec(tx.Rebind(`
				INSERT INTO events (type, title, message, level, related_id, related_type, created_at, read)
				VALUES ('site_notice', ?, ?, ?, ?, 'site_announcement', ?, ?)
			`), title, message, defaultLevel(announcement.Level), announcementID, seenAt, false); eventErr != nil {
				_ = tx.Rollback()
				recordSiteAnnouncementFailure(&result, site.ID, site.Name, "event", eventErr)
				continue
			}
			if commitErr := tx.Commit(); commitErr != nil {
				recordSiteAnnouncementFailure(&result, site.ID, site.Name, "commit", commitErr)
				continue
			}
			result.Inserted++
			result.Events++

			// Notification delivery is best-effort and counted only when at least
			// one configured channel actually succeeds.
			rt := safeRuntimeSettings()
			if rt != nil {
				dispatch, notifyErr := notify.SendNotification(rt, title, message, defaultLevel(announcement.Level), nil)
				if notifyErr != nil {
					slog.Warn("site-announcement sync: notification failed", "siteId", site.ID, "sourceKey", sourceKey, "error", notifyErr)
				} else if dispatch != nil && dispatch.Succeeded > 0 {
					result.Notifications++
				}
			}

		}
	}

	return result
}

func recordSiteAnnouncementFailure(result *SiteAnnouncementSyncResult, siteID int64, siteName, stage string, err error) {
	message := stage + ": " + err.Error()
	slog.Error("site-announcement sync: write failed", "siteId", siteID, "siteName", siteName, "stage", stage, "error", err)
	for i := range result.FailedSites {
		if result.FailedSites[i].SiteID == siteID {
			result.FailedSites[i].Message += "; " + message
			return
		}
	}
	result.Failed++
	result.FailedSites = append(result.FailedSites, SiteAnnouncementFailedSite{
		SiteID:   siteID,
		SiteName: siteName,
		Message:  message,
	})
}

func resolveSiteAccessToken(db *sqlx.DB, siteID int64) string {
	var token *string
	err := db.Get(&token, db.Rebind(`
		SELECT access_token FROM accounts
		WHERE site_id = ? AND status = 'active'
		ORDER BY id ASC
		LIMIT 1
	`), siteID)
	if err != nil || token == nil {
		return ""
	}
	return strings.TrimSpace(*token)
}

func buildAnnouncementMessage(row platform.SiteAnnouncement) string {
	title := strings.TrimSpace(row.Title)
	content := strings.TrimSpace(row.Content)
	if title != "" && content != "" && title != content && strings.ToLower(title) != "site notice" {
		return title + "\n" + content
	}
	if content != "" {
		return content
	}
	return title
}

func defaultLevel(level string) string {
	level = strings.TrimSpace(level)
	if level == "" {
		return "info"
	}
	return level
}

func emptyToNil(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

func stringPtrValue(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

// safeRuntimeSettings returns the current runtime-settings snapshot, or nil
// before boot publication. Notification channels read only runtime-mutable
// values, so the static Config is not needed here.
func safeRuntimeSettings() *config.RuntimeSettings {
	return config.RuntimeSafe()
}

func hasValue(v any) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val) != ""
	case []byte:
		return strings.TrimSpace(string(val)) != ""
	default:
		return true
	}
}

func parseTime(v any) *time.Time {
	if v == nil {
		return nil
	}
	var s string
	switch val := v.(type) {
	case string:
		s = val
	case []byte:
		s = string(val)
	case time.Time:
		return &val
	default:
		return nil
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		// Try other formats
		t, err = time.Parse("2006-01-02 15:04:05", s)
		if err != nil {
			return nil
		}
	}
	return &t
}
