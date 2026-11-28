package admin

import (
	"log/slog"
	"net/http"

	"github.com/deliciousbuding/metapi-go/app"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/schedulespec"
	"github.com/deliciousbuding/metapi-go/scheduler"
	"github.com/deliciousbuding/metapi-go/service/settingsmigration"
)

// applyCheckinSettings applies the checkin schedule fields.
func (h *settingsHandler) applyCheckinSettings(body map[string]any) *settingsApplyError {
	// #1027: global check-in kill switch. Persisted to the checkin_enabled
	// setting and hot-applied to the running scheduler independently of the
	// schedule fields below.
	if v, ok := body["checkinEnabled"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "checkinEnabled must be a boolean (true/false)")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.CheckinDisabled = !enabled })
		if err := upsertSettingDB(h.db, "checkin_enabled", enabled); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save checkinEnabled")
		}
		app.UpdateCheckinEnabled(enabled)
	}
	if hasAnyKey(body, "checkinCron", "checkinScheduleMode", "checkinIntervalHours") {
		patch := checkinSchedulePatch{}
		if v, ok := body["checkinCron"]; ok {
			cron := normalizeString(v)
			patch.Cron = &cron
		}
		if v, ok := body["checkinScheduleMode"]; ok {
			mode := normalizeString(v)
			patch.Mode = &mode
		}
		if v, ok := body["checkinIntervalHours"]; ok {
			hours, err := toFloat64Strict(v)
			if err != nil {
				return failSettings(http.StatusBadRequest, "checkinIntervalHours must be a number")
			}
			if hours != float64(int(hours)) {
				return failSettings(http.StatusBadRequest, "check-in interval must be an integer number of hours between 1 and 24")
			}
			intervalHours := int(hours)
			patch.IntervalHours = &intervalHours
		}
		if _, err := applyCheckinScheduleSettings(h.db, config.Runtime(), patch); err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
	}
	if v, ok := body["checkinSchedule"]; ok {
		spec, err := decodeScheduleSpec(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "invalid checkinSchedule format")
		}
		if err := spec.Validate(config.ValidateCronExpr); err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		patch := checkinSchedulePatch{Schedule: &spec}
		switch spec.Type {
		case "daily", "custom":
			mode := "cron"
			cron, err := schedulespec.ScheduleToCron(spec, config.ValidateCronExpr)
			if err != nil {
				return failSettings(http.StatusBadRequest, err.Error())
			}
			patch.Mode = &mode
			patch.Cron = &cron
		case "interval":
			mode := "interval"
			patch.Mode = &mode
			patch.IntervalHours = &spec.EveryHours
			if cron, err := schedulespec.ScheduleToCron(spec, config.ValidateCronExpr); err == nil && cron != "" {
				patch.Cron = &cron
			}
		case "window":
			mode := "window"
			patch.Mode = &mode
			patch.WindowStart = &spec.WindowStart
			patch.WindowEnd = &spec.WindowEnd
		}
		if _, err := applyCheckinScheduleSettings(h.db, config.Runtime(), patch); err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
	}
	return nil
}

// applyBalanceScheduleSettings applies the balance refresh cron/schedule.
func (h *settingsHandler) applyBalanceScheduleSettings(body map[string]any) *settingsApplyError {
	// #1027: global balance-refresh kill switch. Persisted to the
	// balance_refresh_enabled setting and hot-applied to the running
	// scheduler independently of the cron fields below.
	if v, ok := body["balanceRefreshEnabled"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "balanceRefreshEnabled must be a boolean (true/false)")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.BalanceRefreshDisabled = !enabled })
		if err := upsertSettingDB(h.db, "balance_refresh_enabled", enabled); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save balanceRefreshEnabled")
		}
		if err := app.UpdateBalanceEnabled(enabled); err != nil {
			slog.Warn("settings: balance refresh enable hot update failed", "error", err)
		}
	}
	// Balance refresh cron
	if v, ok := body["balanceRefreshCron"]; ok {
		cron := normalizeString(v)
		if !scheduler.ValidateCronExpr(cron) {
			return failSettings(http.StatusBadRequest, "balanceRefreshCron is not a valid cron expression")
		}
		if err := settingsmigration.SaveDualSchedule(h.db.DB, "balance_refresh_cron", cron, "balance_refresh_schedule_v2", schedulespec.CronToSchedule(cron)); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save balance refresh schedule")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.BalanceRefreshCron = cron })
		if err := app.UpdateBalanceCron(cron); err != nil {
			slog.Warn("settings: balance refresh cron hot update failed", "error", err)
		}
	}
	if v, ok := body["balanceRefreshSchedule"]; ok {
		spec, err := decodeScheduleSpec(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "invalid balanceRefreshSchedule format")
		}
		if err := spec.Validate(config.ValidateCronExpr); err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		cron, err := schedulespec.ScheduleToCron(spec, config.ValidateCronExpr)
		if err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		if cron == "" {
			return failSettings(http.StatusBadRequest, "balanceRefreshSchedule cannot be converted to a cron expression")
		}
		if err := settingsmigration.SaveDualSchedule(h.db.DB, "balance_refresh_cron", cron, "balance_refresh_schedule_v2", spec); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save balance refresh schedule")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.BalanceRefreshCron = cron })
		if err := app.UpdateBalanceCron(cron); err != nil {
			slog.Warn("settings: balance refresh cron hot update failed", "error", err)
		}
	}
	return nil
}

// applyModelSyncScheduleSettings validates and persists the scheduling/creation
// controls together. An invalid cron must never partially enable automatic
// route creation, and a database error must not publish an unpersisted setting.
func (h *settingsHandler) applyModelSyncScheduleSettings(body map[string]any) *settingsApplyError {
	var create *bool
	var cron *string
	if v, ok := body["autoCreateModelRoutes"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "autoCreateModelRoutes must be a boolean (true/false)")
		}
		create = &enabled
	}
	if v, ok := body["modelSyncCron"]; ok {
		value := normalizeString(v)
		if !scheduler.ValidateCronExpr(value) {
			return failSettings(http.StatusBadRequest, "modelSyncCron is not a valid cron expression")
		}
		cron = &value
	}
	if create == nil && cron == nil {
		return nil
	}
	tx, err := h.db.Beginx()
	if err != nil {
		return failSettings(http.StatusInternalServerError, "failed to save model sync settings")
	}
	defer func() { _ = tx.Rollback() }()
	if create != nil {
		if err := upsertSettingDB(tx, "auto_create_model_routes", *create); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save automatic model route setting")
		}
	}
	if cron != nil {
		if err := upsertSettingDB(tx, "model_sync_cron", *cron); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save model sync schedule")
		}
	}
	if err := tx.Commit(); err != nil {
		return failSettings(http.StatusInternalServerError, "failed to commit model sync settings")
	}
	config.UpdateRuntime(func(rt *config.RuntimeSettings) {
		if create != nil {
			rt.AutoCreateModelRoutes = *create
		}
		if cron != nil {
			rt.ModelSyncCron = *cron
		}
	})
	if cron != nil {
		if err := app.UpdateModelSyncCron(*cron); err != nil {
			slog.Warn("settings: model sync cron hot update failed", "error", err)
		}
	}
	return nil
}

// applyLogCleanupSettings applies the log cleanup fields and hot-reloads the
// cleanup task when the cron schedule changed.
func (h *settingsHandler) applyLogCleanupSettings(body map[string]any) *settingsApplyError {
	// Log cleanup settings
	logCleanupChanged := false
	if v, ok := body["logCleanupCron"]; ok {
		cron := normalizeString(v)
		if !scheduler.ValidateCronExpr(cron) {
			return failSettings(http.StatusBadRequest, "logCleanupCron is not a valid cron expression")
		}
		if err := settingsmigration.SaveDualSchedule(h.db.DB, "log_cleanup_cron", cron, "log_cleanup_schedule_v2", schedulespec.CronToSchedule(cron)); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save log cleanup schedule")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.LogCleanupCron = cron })
		logCleanupChanged = true
	}
	if v, ok := body["logCleanupUsageLogsEnabled"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "logCleanupUsageLogsEnabled must be a boolean (true/false)")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.LogCleanupUsageLogsEnabled = enabled })
		upsertSettingDB(h.db, "log_cleanup_usage_logs_enabled", enabled)
	}
	if v, ok := body["logCleanupProgramLogsEnabled"]; ok {
		enabled, err := toBoolStrict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "logCleanupProgramLogsEnabled must be a boolean (true/false)")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.LogCleanupProgramLogsEnabled = enabled })
		upsertSettingDB(h.db, "log_cleanup_program_logs_enabled", enabled)
	}
	if v, ok := body["logCleanupRetentionDays"]; ok {
		days, err := toFloat64Strict(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "logCleanupRetentionDays must be a number")
		}
		if days < 1 {
			return failSettings(http.StatusBadRequest, "log cleanup retention days must be an integer >= 1")
		}
		retentionDays := int(days)
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.LogCleanupRetentionDays = retentionDays })
		upsertSettingDB(h.db, "log_cleanup_retention_days", retentionDays)
	}
	if v, ok := body["logCleanupSchedule"]; ok {
		spec, err := decodeScheduleSpec(v)
		if err != nil {
			return failSettings(http.StatusBadRequest, "invalid logCleanupSchedule format")
		}
		if err := spec.Validate(config.ValidateCronExpr); err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		cron, err := schedulespec.ScheduleToCron(spec, config.ValidateCronExpr)
		if err != nil {
			return failSettings(http.StatusBadRequest, err.Error())
		}
		if cron == "" {
			return failSettings(http.StatusBadRequest, "logCleanupSchedule cannot be converted to a cron expression")
		}
		if err := settingsmigration.SaveDualSchedule(h.db.DB, "log_cleanup_cron", cron, "log_cleanup_schedule_v2", spec); err != nil {
			return failSettings(http.StatusInternalServerError, "failed to save log cleanup schedule")
		}
		config.UpdateRuntime(func(r *config.RuntimeSettings) { r.LogCleanupCron = cron })
		logCleanupChanged = true
	}
	if logCleanupChanged {
		// One snapshot for the whole hot-reload argument set.
		rt := config.Runtime()
		if err := app.UpdateLogCleanupSettings(rt.LogCleanupCron, rt.LogCleanupUsageLogsEnabled, rt.LogCleanupProgramLogsEnabled, rt.LogCleanupRetentionDays); err != nil {
			slog.Warn("settings: log cleanup hot update failed", "error", err)
		}
	}
	return nil
}
