package store

import (
	"fmt"
	"log/slog"

	"github.com/deliciousbuding/metapi-go/config"
)

// LoadRuntimeSettings reads the settings table and applies runtime overrides
// to the config. Mirrors TS runtimeSettingsHydration behavior.
func LoadRuntimeSettings(db *DB, cfg *config.Config, rt *config.RuntimeSettings) error {
	if db == nil {
		return fmt.Errorf("settings: database not initialized")
	}

	settingsStore := NewSettingsStore(db)
	settingsMap, err := settingsStore.GetAll()
	if err != nil {
		return err
	}

	if len(settingsMap) == 0 {
		slog.Info("settings: no runtime settings found in DB")
	} else {
		slog.Info("settings: loaded runtime settings", "count", len(settingsMap))
		// Apply runtime overrides to the static + runtime drafts.
		ApplyRuntimeSettings(cfg, rt, settingsMap)
	}
	if err := MigrateLegacyProxyToken(db, rt, cfg.ProxyGlobalTokenRPM); err != nil {
		return fmt.Errorf("migrate legacy downstream key: %w", err)
	}

	// Which log-retention regime owns the log tables is decided by explicit
	// operator intent only: an admin-saved log-cleanup setting, or an env
	// toggle explicitly set to true (config.LogCleanupEnvEnabled). It used to
	// be inferred from "retention > 0", but config.Load floors retention at 1
	// day, so the inference fired as soon as the settings table held any row at
	// all: it silently flipped an explicit LOG_CLEANUP_*_ENABLED=false to true
	// and disabled the legacy proxy-log retention scheduler. Saving an
	// unrelated setting (the site name, say) must not switch the regime, and
	// an env-only deployment must not lose the regime it asked for either.
	//
	// The two sources are asymmetric on purpose: env can turn the regime ON but
	// cannot "claim" it while off, because claiming it disables the legacy
	// PROXY_LOG_RETENTION_DAYS pruner and would leave proxy_logs unpruned.
	// Values still come from the settings table when it has them (hydration ran
	// above); env only contributes the regime bit.
	dbExplicit := HasExplicitLogCleanupSettings(settingsMap)
	cfg.LogCleanupConfigured = dbExplicit || cfg.LogCleanupEnvEnabled

	slog.Info("settings: log retention regime",
		"regime", logRetentionRegime(cfg.LogCleanupConfigured),
		"configured", cfg.LogCleanupConfigured,
		"source", logRetentionRegimeSource(dbExplicit, cfg.LogCleanupEnvEnabled),
		"usage_logs_enabled", rt.LogCleanupUsageLogsEnabled,
		"program_logs_enabled", rt.LogCleanupProgramLogsEnabled,
		"retention_days", rt.LogCleanupRetentionDays)

	return nil
}

// logRetentionRegime names the scheduler that owns the log tables, for the
// single startup line above: "log_cleanup" prunes usage/program logs on
// LOG_CLEANUP_CRON, "legacy_fallback" leaves proxy_logs to the
// PROXY_LOG_RETENTION_DAYS pruner.
func logRetentionRegime(configured bool) string {
	if configured {
		return "log_cleanup"
	}
	return "legacy_fallback"
}

// logRetentionRegimeSource reports which kind of operator intent claimed the
// regime. Admin-saved settings win over the env toggle because they are also
// the source of the values, so the two never disagree about what is running.
func logRetentionRegimeSource(dbExplicit, envEnabled bool) string {
	switch {
	case dbExplicit:
		return "db_settings"
	case envEnabled:
		return "env_toggle"
	default:
		return "none"
	}
}

// logCleanupExplicitKeys are the settings keys whose presence proves the
// operator configured log cleanup themselves, so startup must neither infer
// nor override anything for that subsystem.
//
// The underscore spellings are what the admin write path persists
// (handler/admin/settings_apply.go). The dotted spellings are deprecated
// read-only aliases: no release ever wrote them, they only matter for
// hand-edited or externally imported rows, and they can be dropped once such
// rows are extinct.
var logCleanupExplicitKeys = []string{
	"log_cleanup_usage_logs_enabled",
	"log_cleanup_program_logs_enabled",
	"log_cleanup_retention_days",
	"log_cleanup.enabled",
	"log_cleanup.usage_logs_enabled",
	"log_cleanup.program_logs_enabled",
	"log_cleanup.retention_days",
}

// HasExplicitLogCleanupSettings checks whether log cleanup was explicitly
// configured via the settings table. Mirrors TS behavior: checks for specific
// setting keys that indicate user-intended log cleanup config.
func HasExplicitLogCleanupSettings(settingsMap map[string]string) bool {
	for _, key := range logCleanupExplicitKeys {
		if _, ok := settingsMap[key]; ok {
			return true
		}
	}
	return false
}

// nonHydratedSettingKeys are settings that legitimately live in the settings
// table but are deliberately NOT folded into the startup snapshot by
// ApplyRuntimeSettings, each with the reason why. Two shapes belong here: keys
// whose consumer reads the settings table itself at use time (and re-validates
// what it reads), and keys whose value is consumed before hydration runs.
//
// Everything else that production code can persist must have a case in
// ApplyRuntimeSettings. settings_rehydration_gate_test.go fails when it does
// not, because a persisted key that nothing reads back means the operator's
// change evaporates on the next restart. That gate reads every writer in the
// repo (R1 for the admin write side, R6 for the service/scheduler/store side),
// so a key written from outside handler/admin cannot slip past it: an
// unallowlisted key there is what made every boot log a hydration warning for
// oauth.identity_backfill_complete, a marker its own writer reads back.
var nonHydratedSettingKeys = map[string]string{
	LegacyProxyTokenMigrationKey: "consumed by MigrateLegacyProxyToken at startup",
	// Resolved by store.EnsureRuntimeDatabase from the env-derived snapshot:
	// the database is already open (and is the very thing being read) by the
	// time LoadRuntimeSettings runs.
	"db_type": "consumed by store.EnsureRuntimeDatabase before hydration runs",
	"db_url":  "consumed by store.EnsureRuntimeDatabase before hydration runs",
	"db_ssl":  "consumed by store.EnsureRuntimeDatabase before hydration runs",
	// v2 ScheduleSpec mirrors of the cron keys, read by the schedule
	// migration service and the admin schedule endpoints.
	"checkin_schedule_v2":         "read by service/settingsmigration + handler/admin/checkin_schedule.go",
	"balance_refresh_schedule_v2": "read by service/settingsmigration",
	"log_cleanup_schedule_v2":     "read by service/settingsmigration",
	// Secret read at use time by its own handler; never published to the
	// runtime snapshot, where a stale copy would outlive rotation.
	"monitor_ldoh_cookie": "read at use time by handler/admin/monitor.go",
	// Backup destination + resume cursor, read at use time by the scheduler.
	"backup_webdav_config_v1": "read at use time by scheduler/backup_webdav.go",
	"backup_webdav_state_v1":  "read at use time by scheduler/backup_webdav.go",
	// Read at use time by the daily-summary scheduler.
	"daily_summary_cron": "read at use time by scheduler/daily_summary.go",
	// Startup-migration completion marker written by service/oauth and read
	// back at use time by the very function that writes it. It carries no
	// operator intent, so there is nothing for the snapshot to hold.
	"oauth.identity_backfill_complete": "read at use time by service/oauth.RunOauthIdentityBackfillOnce",
	// Catalog auto-sync toggle, read at use time by its own store on every
	// sync pass; a snapshot copy would only be a stale second owner.
	"catalog_auto_sync_enabled": "read at use time by service/catalogsync.Store.AutoSyncEnabled",
	// Removed setting: it was stored but never rendered.
	// Legacy rows are intentionally ignored, and listed here so they do not
	// trip the unknown-key warning on every boot.
	"home_page_content": "setting removed; legacy rows intentionally ignored",
}

// unknownKeyLogLimit bounds the aggregated unknown-key warning so a database
// carrying many legacy rows cannot produce an unbounded log line.
const unknownKeyLogLimit = 20

// truncateKeyList caps a key list destined for a single log line, appending a
// "+N more" marker when it had to cut entries.
func truncateKeyList(keys []string, limit int) []string {
	if len(keys) <= limit {
		return keys
	}
	out := append([]string{}, keys[:limit]...)
	return append(out, fmt.Sprintf("(+%d more)", len(keys)-limit))
}
