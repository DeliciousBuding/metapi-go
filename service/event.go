package service

import (
	"time"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"
)

// MaxEventMessageBytes bounds events.message. A proxy request can carry an
// arbitrarily long model name, and the all-failed path writes it into an
// event; without a cap one request can inflate the database (and break
// backups) by megabytes.
const MaxEventMessageBytes = 4096

// CreateEvent creates an event entry.
func CreateEvent(db *sqlx.DB, eventType string, title string, message string, level string, relatedID int64, relatedType string) error {
	message = truncateEventMessage(message)
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.Exec(
		db.Rebind(`INSERT INTO events (type, title, message, level, read, related_id, related_type, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`),
		eventType, title, message, level, false, relatedID, relatedType, now,
	)
	return err
}

// truncateEventMessage cuts message to MaxEventMessageBytes without splitting
// a UTF-8 rune and marks the truncation.
func truncateEventMessage(message string) string {
	if len(message) <= MaxEventMessageBytes {
		return message
	}
	const marker = "…[truncated]"
	keep := MaxEventMessageBytes - len(marker)
	for keep > 0 && !utf8.RuneStart(message[keep]) {
		keep--
	}
	return message[:keep] + marker
}
