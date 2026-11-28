package admin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

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

	query := `SELECT site_announcements.*, (SELECT COUNT(*) FROM site_announcement_history h WHERE h.announcement_id = site_announcements.id) AS revision_count FROM site_announcements`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY first_seen_at DESC, id DESC"

	rows, err := h.db.Queryx(query, args...)
	if err != nil {
		slog.Error("Failed to load announcements", "err", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load announcements", "errorCode": "resourceLoadFailed"})
		return
	}
	defer rows.Close()

	var all []map[string]any
	for rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load announcements"})
			return
		}
		all = append(all, mapKeysToCamel(row))
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load announcements"})
		return
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

func (h *siteAnnouncementsHandler) history(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var current store.SiteAnnouncement
	if err := h.db.Get(&current, `SELECT * FROM site_announcements WHERE id = ?`, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Announcement not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load announcement history", "errorCode": "resourceLoadFailed"})
		return
	}
	var snapshots []struct {
		ID       int64  `db:"id"`
		Snapshot string `db:"snapshot"`
	}
	if err := h.db.Select(&snapshots, `SELECT id, snapshot FROM site_announcement_history WHERE announcement_id = ? ORDER BY id DESC`, id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load announcement history", "errorCode": "resourceLoadFailed"})
		return
	}
	type version struct {
		store.SiteAnnouncement
		RevisionID int64 `json:"revisionId"`
	}
	versions := []version{{SiteAnnouncement: current}}
	for _, snapshot := range snapshots {
		var announcement store.SiteAnnouncement
		if err := json.Unmarshal([]byte(snapshot.Snapshot), &announcement); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Failed to load announcement history", "errorCode": "resourceLoadFailed"})
			return
		}
		versions = append(versions, version{SiteAnnouncement: announcement, RevisionID: snapshot.ID})
	}
	writeJSON(w, http.StatusOK, versions)
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
