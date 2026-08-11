package admin

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/jmoiron/sqlx"
)

type checkinSchedulePatch struct {
	Mode          *string
	Cron          *string
	IntervalHours *int
	// E1: random-window mode bounds (HH:mm, 24h).
	WindowStart *string
	WindowEnd   *string
	Schedule    *scheduler.ScheduleSpec
}

type checkinScheduleState struct {
	Mode          string
	Cron          string
	IntervalHours int
	// E1
	WindowStart string
	WindowEnd   string
}

func applyCheckinScheduleSettings(db *sqlx.DB, cfg *config.Config, patch checkinSchedulePatch) (checkinScheduleState, error) {
	state := resolveCheckinScheduleState(cfg, patch)
	if state.Mode == "" {
		return checkinScheduleState{}, fmt.Errorf("mode must be cron or interval")
	}
	if patch.Cron != nil || state.Mode == "cron" {
		if state.Cron == "" {
			return checkinScheduleState{}, fmt.Errorf("cron is required when mode is cron")
		}
		if !scheduler.ValidateCronExpr(state.Cron) {
			return checkinScheduleState{}, fmt.Errorf("invalid cron expression")
		}
	}
	if patch.IntervalHours != nil || state.Mode == "interval" {
		if state.IntervalHours < 1 || state.IntervalHours > 24 {
			return checkinScheduleState{}, fmt.Errorf("intervalHours must be between 1 and 24")
		}
	}
	// E1: window mode requires valid HH:mm bounds with start <= end.
	if patch.WindowStart != nil || patch.WindowEnd != nil || state.Mode == "window" {
		if _, err := scheduler.RandomCronInWindow(state.WindowStart, state.WindowEnd); err != nil {
			return checkinScheduleState{}, err
		}
	}

	tx, err := db.Beginx()
	if err != nil {
		return checkinScheduleState{}, fmt.Errorf("settings: begin checkin schedule update: %w", err)
	}
	defer tx.Rollback()

	if err := upsertSettingTx(db, tx, "checkin_schedule_mode", state.Mode); err != nil {
		return checkinScheduleState{}, err
	}
	if err := upsertSettingTx(db, tx, "checkin_cron", state.Cron); err != nil {
		return checkinScheduleState{}, err
	}
	if err := upsertSettingTx(db, tx, "checkin_interval_hours", state.IntervalHours); err != nil {
		return checkinScheduleState{}, err
	}
	// E1: persist window bounds whenever window mode is involved.
	if state.Mode == "window" {
		if err := upsertSettingTx(db, tx, "checkin_window_start", state.WindowStart); err != nil {
			return checkinScheduleState{}, err
		}
		if err := upsertSettingTx(db, tx, "checkin_window_end", state.WindowEnd); err != nil {
			return checkinScheduleState{}, err
		}
	}
	v2 := scheduler.ScheduleSpec{Version: scheduler.ScheduleSpecVersion}
	if patch.Schedule != nil {
		v2 = *patch.Schedule
		v2.Version = scheduler.ScheduleSpecVersion
	} else {
		switch state.Mode {
		case "window":
			v2 = scheduler.ScheduleSpec{Version: scheduler.ScheduleSpecVersion, Type: "window", WindowStart: state.WindowStart, WindowEnd: state.WindowEnd, Cron: state.Cron}
		case "interval":
			v2 = scheduler.ScheduleSpec{Version: scheduler.ScheduleSpecVersion, Type: "interval", EveryHours: state.IntervalHours, Cron: state.Cron}
		default:
			v2 = scheduler.CronToSchedule(state.Cron)
		}
	}
	if err := upsertSettingTx(db, tx, "checkin_schedule_v2", v2); err != nil {
		return checkinScheduleState{}, err
	}

	if err := tx.Commit(); err != nil {
		return checkinScheduleState{}, fmt.Errorf("settings: commit checkin schedule update: %w", err)
	}

	cfg.CheckinScheduleMode = state.Mode
	cfg.CheckinCron = state.Cron
	cfg.CheckinIntervalHours = state.IntervalHours
	cfg.CheckinWindowStart = state.WindowStart
	cfg.CheckinWindowEnd = state.WindowEnd
	if err := app.UpdateCheckinSchedule(state.Mode, state.Cron, state.IntervalHours, state.WindowStart, state.WindowEnd); err != nil {
		return checkinScheduleState{}, fmt.Errorf("settings: apply checkin schedule runtime update: %w", err)
	}
	return state, nil
}

func resolveCheckinScheduleState(cfg *config.Config, patch checkinSchedulePatch) checkinScheduleState {
	mode := normalizeCheckinScheduleMode(cfg.CheckinScheduleMode)
	if patch.Mode != nil {
		mode = normalizeCheckinScheduleMode(*patch.Mode)
	}

	cron := strings.TrimSpace(cfg.CheckinCron)
	if cron == "" {
		cron = config.DefaultCheckinCron
	}
	if patch.Cron != nil {
		cron = strings.TrimSpace(*patch.Cron)
	}

	intervalHours := cfg.CheckinIntervalHours
	if intervalHours < 1 || intervalHours > 24 {
		intervalHours = config.DefaultCheckinIntervalHours
	}
	if patch.IntervalHours != nil {
		intervalHours = *patch.IntervalHours
	}

	// E1: window bounds (HH:mm), env defaults when unset.
	windowStart := strings.TrimSpace(cfg.CheckinWindowStart)
	if windowStart == "" {
		windowStart = "00:00"
	}
	if patch.WindowStart != nil {
		windowStart = strings.TrimSpace(*patch.WindowStart)
	}
	windowEnd := strings.TrimSpace(cfg.CheckinWindowEnd)
	if windowEnd == "" {
		windowEnd = "23:59"
	}
	if patch.WindowEnd != nil {
		windowEnd = strings.TrimSpace(*patch.WindowEnd)
	}

	return checkinScheduleState{
		Mode:          mode,
		Cron:          cron,
		IntervalHours: intervalHours,
		WindowStart:   windowStart,
		WindowEnd:     windowEnd,
	}
}

func normalizeCheckinScheduleMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", "cron":
		return "cron"
	case "interval":
		return "interval"
	case "window":
		return "window"
	default:
		return ""
	}
}

func upsertSettingTx(db *sqlx.DB, tx *sqlx.Tx, key string, value any) error {
	jsonValue, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("settings: marshal %q: %w", key, err)
	}
	query := db.Rebind(`INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`)
	if _, err := tx.Exec(query, key, string(jsonValue)); err != nil {
		return fmt.Errorf("settings: upsert %q: %w", key, err)
	}
	return nil
}

// scheduleSpecForCheckin derives the semantic ScheduleSpec shown by GET
// /api/settings/runtime for the checkin task, honouring its mode.
func scheduleSpecForCheckin(cfg *config.Config) scheduler.ScheduleSpec {
	switch cfg.CheckinScheduleMode {
	case "window":
		return scheduler.ScheduleSpec{Version: scheduler.ScheduleSpecVersion, Type: "window", WindowStart: cfg.CheckinWindowStart, WindowEnd: cfg.CheckinWindowEnd, Cron: cfg.CheckinCron}
	case "interval":
		return scheduler.ScheduleSpec{Version: scheduler.ScheduleSpecVersion, Type: "interval", EveryHours: cfg.CheckinIntervalHours, Cron: cfg.CheckinCron}
	default:
		return scheduler.CronToSchedule(cfg.CheckinCron)
	}
}
