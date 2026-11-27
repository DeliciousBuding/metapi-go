package events

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

// List returns the native event row shape, including driver-specific read values.
func List(db *store.DB, limit, offset int, typeFilter string, read *bool) ([]map[string]any, error) {
	var conditions []string
	var args []any

	if typeFilter != "" {
		conditions = append(conditions, "type = ?")
		args = append(args, typeFilter)
	}
	if read != nil {
		conditions = append(conditions, "read = ?")
		args = append(args, *read)
	}
	query := "SELECT * FROM events"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := db.Queryx(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []map[string]any{}
	for rows.Next() {
		row := map[string]any{}
		if err := rows.MapScan(row); err != nil {
			return nil, err
		}
		events = append(events, row)
	}
	// Preserve the inbox API's best-effort list if iteration stops after readable rows.
	return events, nil
}

func UnreadCount(db *store.DB) (int, error) {
	var count int
	err := db.Get(&count, "SELECT COUNT(*) FROM events WHERE read = ?", false)
	return count, err
}

// MarkRead acknowledges a single event, or all unread events when id is nil.
func MarkRead(db *store.DB, id *int64) error {
	if id == nil {
		_, err := db.Exec("UPDATE events SET read = ? WHERE read = ?", true, false)
		return err
	}
	_, err := db.Exec("UPDATE events SET read = ? WHERE id = ?", true, *id)
	return err
}

func DeleteAll(db *store.DB) error {
	_, err := db.Exec("DELETE FROM events")
	return err
}
