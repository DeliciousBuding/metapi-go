package service

import (
	"database/sql"
	"errors"
	"log/slog"
	"strings"

	"github.com/jmoiron/sqlx"
)

// Marketplace stats read token_model_availability. Backfilling a matching
// token is best-effort after the account snapshot has committed.
func backfillAccountTokenModels(db *sqlx.DB, accountID int64, token string, models []string, now string) bool {
	tokenID, ok := resolveAccountTokenID(db, accountID, token)
	if !ok {
		return false
	}
	if err := persistTokenModelAvailability(db, tokenID, models, now); err != nil {
		slog.Warn("model-refresh: token_model_availability backfill failed",
			"account_id", accountID, "token_id", tokenID, "error", err)
		return false
	}
	return true
}

func persistAccountModelAvailability(db *sqlx.DB, accountID int64, models []string, now string) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Preserve manual rows; mark non-manual previous rows unavailable then upsert.
	// `NOT is_manual` is dialect-safe: PostgreSQL BOOLEAN and SQLite INTEGER 0/1
	// both evaluate NULL->NULL (excluded) and false/0->true in WHERE.
	if _, err := tx.Exec(tx.Rebind(`
		UPDATE model_availability
		SET available = ?, checked_at = ?
		WHERE account_id = ? AND NOT is_manual
	`), false, now, accountID); err != nil {
		return err
	}

	for _, model := range models {
		// Single dialect-aware upsert replaces the SELECT-then-UPDATE-or-INSERT
		// pattern (3 round-trips/model -> 1). ON CONFLICT (account_id, model_name)
		// targets the model_availability_account_model_unique constraint. The
		// is_manual column is deliberately NOT updated on conflict so the
		// auto-refresh path preserves operator-set manual rows (mirroring the
		// old UPDATE which only touched available/latency_ms/checked_at).
		if _, err := tx.Exec(tx.Rebind(`
			INSERT INTO model_availability (account_id, model_name, available, is_manual, latency_ms, checked_at)
			VALUES (?, ?, ?, ?, NULL, ?)
			ON CONFLICT (account_id, model_name) DO UPDATE SET
				available = EXCLUDED.available,
				latency_ms = NULL,
				checked_at = EXCLUDED.checked_at
		`), accountID, model, true, false, now); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

// resolveAccountTokenID finds the account_tokens row id matching the resolved
// refresh token. Tokens are stored normalized (TrimSpace, no Bearer prefix);
// the resolved token is also normalized, so an exact match is sufficient.
// Returns (id, true) on match, (0, false) when no row matches — in which case
// the caller skips the token_model_availability backfill (#675).
func resolveAccountTokenID(db *sqlx.DB, accountID int64, token string) (int64, bool) {
	normalized := strings.TrimSpace(token)
	normalized = strings.TrimPrefix(normalized, "Bearer ")
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return 0, false
	}
	var tokenID int64
	err := db.Get(&tokenID, db.Rebind(
		"SELECT id FROM account_tokens WHERE account_id = ? AND token = ? ORDER BY id DESC LIMIT 1",
	), accountID, normalized)
	if err != nil {
		return 0, false
	}
	return tokenID, true
}

// persistTokenModelAvailability mirrors persistAccountModelAvailability for
// the token_model_availability table: mark all existing rows for the token
// unavailable, then upsert the current model set as available. Uses its own
// transaction so a failure here never rolls back the account-level write.
func persistTokenModelAvailability(db *sqlx.DB, tokenID int64, models []string, now string) error {
	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Mark all existing rows for this token unavailable before upserting the
	// current set — mirrors the account-level "mark non-manual unavailable"
	// pattern. token_model_availability has no is_manual column, so we clear
	// everything and re-enable the freshly observed models.
	if _, err := tx.Exec(tx.Rebind(`
		UPDATE token_model_availability
		SET available = ?, checked_at = ?
		WHERE token_id = ?
	`), false, now, tokenID); err != nil {
		return err
	}

	for _, model := range models {
		var existingID int64
		err := tx.Get(&existingID, tx.Rebind(`
			SELECT id FROM token_model_availability WHERE token_id = ? AND model_name = ?
		`), tokenID, model)
		if err == nil {
			if _, err := tx.Exec(tx.Rebind(`
				UPDATE token_model_availability
				SET available = ?, latency_ms = NULL, checked_at = ?
				WHERE id = ?
			`), true, now, existingID); err != nil {
				return err
			}
			continue
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.Exec(tx.Rebind(`
			INSERT INTO token_model_availability (token_id, model_name, available, latency_ms, checked_at)
			VALUES (?, ?, ?, NULL, ?)
		`), tokenID, model, true, now); err != nil {
			// Unique constraint violation: fall back to UPDATE.
			if _, err2 := tx.Exec(tx.Rebind(`
				UPDATE token_model_availability
				SET available = ?, latency_ms = NULL, checked_at = ?
				WHERE token_id = ? AND model_name = ?
			`), true, now, tokenID, model); err2 != nil {
				return err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}
