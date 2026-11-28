package admin

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service/notify"
	"github.com/deliciousbuding/metapi-go/store"
)

func persistSiteAnnouncement(db *store.DB, site announcementSyncSite, announcement platform.SiteAnnouncement, seenAt string, result *SiteAnnouncementSyncResult) {
	sourceKey := strings.TrimSpace(announcement.SourceKey)
	if sourceKey == "" {
		return
	}

	rawPayload := ""
	if len(announcement.RawPayload) > 0 {
		rawPayload = string(announcement.RawPayload)
	}

	tx, beginErr := db.Beginx()
	if beginErr != nil {
		recordSiteAnnouncementFailure(result, site.ID, site.Name, "begin transaction", beginErr)
		return
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
		recordSiteAnnouncementFailure(result, site.ID, site.Name, "insert", insertErr)
		return
	}

	if !isNew {
		var previous store.SiteAnnouncement
		query := `SELECT * FROM site_announcements WHERE site_id = ? AND source_key = ?`
		if db.Dialect == store.DialectPostgres {
			query += " FOR UPDATE"
		}
		if err := tx.Get(&previous, tx.Rebind(query), site.ID, sourceKey); err != nil {
			_ = tx.Rollback()
			recordSiteAnnouncementFailure(result, site.ID, site.Name, "load previous version", err)
			return
		}
		changed := previous.Title != announcement.Title || previous.Content != announcement.Content || previous.Level != defaultLevel(announcement.Level)
		firstSeenAt, readAt, dismissedAt := previous.FirstSeenAt, previous.ReadAt, previous.DismissedAt
		if changed {
			if err := store.SaveSiteAnnouncementRevision(tx, previous); err != nil {
				_ = tx.Rollback()
				recordSiteAnnouncementFailure(result, site.ID, site.Name, "save previous version", err)
				return
			}
			firstSeenAt, readAt, dismissedAt = seenAt, nil, nil
		}
		updateResult, updateErr := tx.Exec(tx.Rebind(`
					UPDATE site_announcements
					SET platform = ?, title = ?, content = ?, level = ?, source_url = ?,
					    starts_at = ?, ends_at = ?, upstream_created_at = ?, upstream_updated_at = ?,
					    last_seen_at = ?, raw_payload = ?, first_seen_at = ?, read_at = ?, dismissed_at = ?
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
			firstSeenAt, readAt, dismissedAt,
			site.ID,
			sourceKey,
		)
		if updateErr != nil {
			_ = tx.Rollback()
			recordSiteAnnouncementFailure(result, site.ID, site.Name, "update", updateErr)
			return
		}
		rows, rowsErr := updateResult.RowsAffected()
		if rowsErr != nil || rows != 1 {
			_ = tx.Rollback()
			if rowsErr == nil {
				rowsErr = fmt.Errorf("updated %d rows after conflict, want 1", rows)
			}
			recordSiteAnnouncementFailure(result, site.ID, site.Name, "update", rowsErr)
			return
		}
		if !changed {
			if commitErr := tx.Commit(); commitErr != nil {
				recordSiteAnnouncementFailure(result, site.ID, site.Name, "commit", commitErr)
				return
			}
			result.Updated++
			return
		}
		announcementID = previous.ID
	}

	title := "Site announcement: " + site.Name
	message := buildAnnouncementMessage(announcement)
	if _, eventErr := tx.Exec(tx.Rebind(`
				INSERT INTO events (type, title, message, level, related_id, related_type, created_at, read)
				VALUES ('site_notice', ?, ?, ?, ?, 'site_announcement', ?, ?)
			`), title, message, defaultLevel(announcement.Level), announcementID, seenAt, false); eventErr != nil {
		_ = tx.Rollback()
		recordSiteAnnouncementFailure(result, site.ID, site.Name, "event", eventErr)
		return
	}
	if commitErr := tx.Commit(); commitErr != nil {
		recordSiteAnnouncementFailure(result, site.ID, site.Name, "commit", commitErr)
		return
	}
	if isNew {
		result.Inserted++
	} else {
		result.Updated++
	}
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
		return string(notify.LevelInfo)
	}
	return level
}

func emptyToNil(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

// safeRuntimeSettings returns the current runtime-settings snapshot, or nil
// before boot publication. Notification channels read only runtime-mutable
// values, so the static Config is not needed here.
func safeRuntimeSettings() *config.RuntimeSettings {
	return config.RuntimeSafe()
}
