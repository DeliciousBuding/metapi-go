package store

import (
	"database/sql"
	"time"
)

type AnnouncementWithDismissal struct {
	ProductAnnouncement
	Enabled     scannedBool `db:"enabled" json:"enabled"`
	Dismissed   bool        `db:"dismissed" json:"dismissed"`
	DismissedAt *string     `db:"dismissed_at" json:"dismissedAt"`
}

func (db *DB) ProductAnnouncements(activeOnly bool) ([]AnnouncementWithDismissal, error) {
	q := `SELECT a.id, a.title, a.message, a.severity, a.link, a.enabled, a.created_at, a.updated_at,
		(d.dismissed_at IS NOT NULL) AS dismissed, d.dismissed_at AS dismissed_at
		FROM product_announcements a
		LEFT JOIN announcement_dismissals d ON d.announcement_id = a.id`
	if activeOnly {
		q += ` WHERE a.enabled = TRUE AND d.dismissed_at IS NULL`
	}
	q += ` ORDER BY CASE a.severity WHEN 'critical' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END, a.updated_at DESC`

	items := []AnnouncementWithDismissal{}
	err := db.Select(&items, q)
	return items, err
}

func (db *DB) CreateProductAnnouncement(a ProductAnnouncement) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(`INSERT INTO product_announcements (title, message, severity, link, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`, a.Title, a.Message, a.Severity, a.Link, a.Enabled, now, now)
	return err
}

// AnnouncementContent reads the revision fields before the HTTP edit body is parsed.
func (db *DB) AnnouncementContent(id int64) (ProductAnnouncement, error) {
	var a ProductAnnouncement
	err := db.Get(&a, "SELECT title, message, link FROM product_announcements WHERE id = ?", id)
	return a, err
}

// ReviseProductAnnouncement atomically publishes the edit and resets its dismissal
// only when a content field changed. Severity and visibility are not revisions.
func (db *DB) ReviseProductAnnouncement(id int64, previous, a ProductAnnouncement) (bool, error) {
	changed := previous.Title != a.Title || previous.Message != a.Message ||
		(previous.Link == nil) != (a.Link == nil) ||
		(a.Link != nil && previous.Link != nil && *previous.Link != *a.Link)
	tx, err := db.Beginx()
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.Exec(tx.Rebind(`UPDATE product_announcements
		SET title = ?, message = ?, severity = ?, link = ?, enabled = ?, updated_at = ? WHERE id = ?`),
		a.Title, a.Message, a.Severity, a.Link, a.Enabled, time.Now().UTC().Format(time.RFC3339), id)
	if err != nil {
		return false, err
	}
	if changed {
		if _, err := tx.Exec(tx.Rebind("DELETE FROM announcement_dismissals WHERE announcement_id = ?"), id); err != nil {
			return false, err
		}
	}
	return changed, tx.Commit()
}

func (db *DB) DeleteProductAnnouncement(id int64) (bool, error) {
	res, err := db.Exec("DELETE FROM product_announcements WHERE id = ?", id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n != 0, nil
}

func (db *DB) DismissProductAnnouncement(id int64) error {
	var exists int
	if err := db.Get(&exists, "SELECT COUNT(*) FROM product_announcements WHERE id = ?", id); err != nil || exists == 0 {
		return sql.ErrNoRows
	}
	_, err := db.Exec(`INSERT INTO announcement_dismissals (announcement_id, dismissed_at) VALUES (?, ?)
		ON CONFLICT (announcement_id) DO UPDATE SET dismissed_at = excluded.dismissed_at`, id, time.Now().UTC().Format(time.RFC3339))
	return err
}
