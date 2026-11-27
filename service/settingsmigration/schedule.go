package settingsmigration

import (
	"fmt"

	"github.com/deliciousbuding/metapi-go/internal/schedulespec"
	"github.com/jmoiron/sqlx"
)

func SaveDualSchedule(db *sqlx.DB, legacyKey, cron, v2Key string, spec schedulespec.ScheduleSpec) error {
	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("settings: begin dual schedule write: %w", err)
	}
	defer tx.Rollback()
	if err := upsertScheduleTx(tx, legacyKey, cron); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	spec.Cron = cron
	if err := upsertScheduleTx(tx, v2Key, spec); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	return tx.Commit()
}

func SaveCheckinSchedule(db *sqlx.DB, spec schedulespec.ScheduleSpec, mode, cron string, interval int, windowStart, windowEnd string) error {
	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("settings: begin checkin schedule update: %w", err)
	}
	defer tx.Rollback()

	if err := upsertScheduleTx(tx, "checkin_schedule_mode", mode); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	if err := upsertScheduleTx(tx, "checkin_cron", cron); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	if err := upsertScheduleTx(tx, "checkin_interval_hours", interval); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	// E1: persist window bounds whenever window mode is involved.
	if mode == "window" {
		if err := upsertScheduleTx(tx, "checkin_window_start", windowStart); err != nil {
			return fmt.Errorf("settings: %w", err)
		}
		if err := upsertScheduleTx(tx, "checkin_window_end", windowEnd); err != nil {
			return fmt.Errorf("settings: %w", err)
		}
	}
	if err := upsertScheduleTx(tx, "checkin_schedule_v2", spec); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("settings: commit checkin schedule update: %w", err)
	}
	return nil
}
