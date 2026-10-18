package store

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

func buildSiteAnnouncementHistoryDDL(d string) string {
	id := "INTEGER PRIMARY KEY AUTOINCREMENT"
	if isPG(d) {
		id = "SERIAL PRIMARY KEY"
	}
	return `CREATE TABLE IF NOT EXISTS site_announcement_history (
		id ` + id + `,
		announcement_id INTEGER NOT NULL REFERENCES site_announcements(id) ON DELETE CASCADE,
		snapshot TEXT NOT NULL
	)`
}

// SaveSiteAnnouncementRevision preserves the previous version in the same
// transaction that replaces its current content.
func SaveSiteAnnouncementRevision(tx *sqlx.Tx, announcement SiteAnnouncement) error {
	snapshot, err := json.Marshal(announcement)
	if err != nil {
		return err
	}
	_, err = tx.Exec(tx.Rebind(`INSERT INTO site_announcement_history (announcement_id, snapshot) VALUES (?, ?)`), announcement.ID, string(snapshot))
	return err
}

// Single-notice endpoints used content hashes as identities before timelines.
// Only that exact legacy format is eligible for automatic merging.
func legacyNoticeKey(key string) bool {
	value := strings.TrimPrefix(key, "notice:")
	if value == key || len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func migrateSiteAnnouncementTimeline(db *DB) error {
	exists, err := tableExists(db, "site_announcements")
	if err != nil || !exists {
		return err
	}
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var rows []SiteAnnouncement
	if err := tx.Select(&rows, `SELECT * FROM site_announcements WHERE source_key = 'notice' OR source_key LIKE 'notice:%' ORDER BY first_seen_at, id`); err != nil {
		return err
	}
	groups := map[int64][]SiteAnnouncement{}
	for _, row := range rows {
		if legacyNoticeKey(row.SourceKey) || row.SourceKey == "notice" {
			// The timeline snapshot is JSON, outside the ordinary *_at sweep.
			// Normalize the SQL-shaped UTC timestamps written by old syncs.
			for _, value := range []*string{&row.FirstSeenAt, &row.LastSeenAt} {
				if parsed, err := time.Parse("2006-01-02 15:04:05", *value); err == nil {
					*value = parsed.UTC().Format(time.RFC3339)
				}
			}
			groups[row.SiteID] = append(groups[row.SiteID], row)
		}
	}
	for _, versions := range groups {
		// last_seen_at identifies the content most recently observed, including
		// sites that reverted to an older notice. IDs break same-second ties.
		sort.SliceStable(versions, func(i, j int) bool {
			if versions[i].LastSeenAt == versions[j].LastSeenAt {
				return versions[i].ID < versions[j].ID
			}
			return versions[i].LastSeenAt < versions[j].LastSeenAt
		})
		current := versions[len(versions)-1]
		for _, old := range versions[:len(versions)-1] {
			if old.Title != current.Title || old.Content != current.Content || old.Level != current.Level {
				version := old
				version.ID = current.ID
				if err := SaveSiteAnnouncementRevision(tx, version); err != nil {
					return fmt.Errorf("preserve announcement %d: %w", old.ID, err)
				}
			}
			if _, err := tx.Exec(tx.Rebind(`UPDATE events SET related_id = ? WHERE related_type = 'site_announcement' AND related_id = ?`), current.ID, old.ID); err != nil {
				return err
			}
			if _, err := tx.Exec(tx.Rebind(`DELETE FROM site_announcements WHERE id = ?`), old.ID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(tx.Rebind(`UPDATE site_announcements SET source_key = 'notice' WHERE id = ?`), current.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}
