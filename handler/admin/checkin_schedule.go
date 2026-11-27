package admin

import (
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/schedulespec"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/service/settingsmigration"
	"github.com/deliciousbuding/metapi-go/store"
)

type checkinSchedulePatch struct {
	Mode          *string
	Cron          *string
	IntervalHours *int
	// E1: random-window mode bounds (HH:mm, 24h).
	WindowStart *string
	WindowEnd   *string
	Schedule    *schedulespec.ScheduleSpec
}

type checkinScheduleState struct {
	Mode          string
	Cron          string
	IntervalHours int
	// E1
	WindowStart string
	WindowEnd   string
}

func applyCheckinScheduleSettings(db *store.DB, rt *config.RuntimeSettings, patch checkinSchedulePatch) (checkinScheduleState, error) {
	state := resolveCheckinScheduleState(rt, patch)
	if state.Mode == "" {
		return checkinScheduleState{}, fmt.Errorf("mode must be cron, interval or window")
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
		if _, _, err := schedulespec.WindowBounds(state.WindowStart, state.WindowEnd); err != nil {
			return checkinScheduleState{}, err
		}
	}

	v2 := schedulespec.ScheduleSpec{Version: schedulespec.ScheduleSpecVersion}
	if patch.Schedule != nil {
		v2 = *patch.Schedule
		v2.Version = schedulespec.ScheduleSpecVersion
	} else {
		switch state.Mode {
		case "window":
			v2 = schedulespec.ScheduleSpec{Version: schedulespec.ScheduleSpecVersion, Type: "window", WindowStart: state.WindowStart, WindowEnd: state.WindowEnd, Cron: state.Cron}
		case "interval":
			v2 = schedulespec.ScheduleSpec{Version: schedulespec.ScheduleSpecVersion, Type: "interval", EveryHours: state.IntervalHours, Cron: state.Cron}
		default:
			v2 = schedulespec.CronToSchedule(state.Cron)
		}
	}
	v2.Cron = state.Cron
	if err := settingsmigration.SaveCheckinSchedule(db.DB, v2, state.Mode, state.Cron, state.IntervalHours, state.WindowStart, state.WindowEnd); err != nil {
		return checkinScheduleState{}, err
	}

	config.UpdateRuntime(func(r *config.RuntimeSettings) {
		r.CheckinScheduleMode = state.Mode
		r.CheckinCron = state.Cron
		r.CheckinIntervalHours = state.IntervalHours
		r.CheckinWindowStart = state.WindowStart
		r.CheckinWindowEnd = state.WindowEnd
	})
	if err := app.UpdateCheckinSchedule(state.Mode, state.Cron, state.IntervalHours, state.WindowStart, state.WindowEnd); err != nil {
		return checkinScheduleState{}, fmt.Errorf("settings: apply checkin schedule runtime update: %w", err)
	}
	return state, nil
}

func resolveCheckinScheduleState(rt *config.RuntimeSettings, patch checkinSchedulePatch) checkinScheduleState {
	mode := normalizeCheckinScheduleMode(rt.CheckinScheduleMode)
	if patch.Mode != nil {
		mode = normalizeCheckinScheduleMode(*patch.Mode)
	}

	cron := strings.TrimSpace(rt.CheckinCron)
	if cron == "" {
		cron = config.DefaultCheckinCron
	}
	if patch.Cron != nil {
		cron = strings.TrimSpace(*patch.Cron)
	}

	intervalHours := rt.CheckinIntervalHours
	if intervalHours < 1 || intervalHours > 24 {
		intervalHours = config.DefaultCheckinIntervalHours
	}
	if patch.IntervalHours != nil {
		intervalHours = *patch.IntervalHours
	}

	// E1: window bounds (HH:mm), env defaults when unset.
	windowStart := strings.TrimSpace(rt.CheckinWindowStart)
	if windowStart == "" {
		windowStart = "00:00"
	}
	if patch.WindowStart != nil {
		windowStart = strings.TrimSpace(*patch.WindowStart)
	}
	windowEnd := strings.TrimSpace(rt.CheckinWindowEnd)
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

// scheduleSpecForCheckin derives the semantic ScheduleSpec shown by GET
// /api/settings/runtime for the checkin task, honouring its mode.
func scheduleSpecForCheckin(rt *config.RuntimeSettings) schedulespec.ScheduleSpec {
	switch rt.CheckinScheduleMode {
	case "window":
		return schedulespec.ScheduleSpec{Version: schedulespec.ScheduleSpecVersion, Type: "window", WindowStart: rt.CheckinWindowStart, WindowEnd: rt.CheckinWindowEnd, Cron: rt.CheckinCron}
	case "interval":
		return schedulespec.ScheduleSpec{Version: schedulespec.ScheduleSpecVersion, Type: "interval", EveryHours: rt.CheckinIntervalHours, Cron: rt.CheckinCron}
	default:
		return schedulespec.CronToSchedule(rt.CheckinCron)
	}
}
