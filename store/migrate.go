package store

import (
	"fmt"
	"log/slog"
)

// AutoMigrate creates all 35 tables with indexes, unique constraints, foreign keys,
// and check constraints. Uses CREATE TABLE IF NOT EXISTS for idempotency.
// After the base bootstrap it runs the additive enterprise steps
// (schema_migrations bookkeeping + ordered ALTER TABLE upgrades) and logs a
// one-line summary when legacy-schema convergence actually happened. Run on
// startup after Open().
func AutoMigrate(db *DB) error {
	dialect := db.Dialect
	slog.Info("store: running auto-migration", "dialect", dialect)

	migrations := []struct {
		name string
		sql  string
	}{
		// Table 1: sites
		{"sites", buildSitesDDL(dialect)},
		// Table 2: site_api_endpoints
		{"site_api_endpoints", buildSiteAPIEndpointsDDL(dialect)},
		// Table 3: site_disabled_models
		{"site_disabled_models", buildSiteDisabledModelsDDL(dialect)},
		// Table 4: accounts
		{"accounts", buildAccountsDDL(dialect)},
		// Table 5: account_tokens
		{"account_tokens", buildAccountTokensDDL(dialect)},
		// Table 6: checkin_logs
		{"checkin_logs", buildCheckinLogsDDL(dialect)},
		// Table 7: model_availability
		{"model_availability", buildModelAvailabilityDDL(dialect)},
		// Table 8: token_model_availability
		{"token_model_availability", buildTokenModelAvailabilityDDL(dialect)},
		// Table 9: token_routes
		{"token_routes", buildTokenRoutesDDL(dialect)},
		// Table 10: route_group_sources
		{"route_group_sources", buildRouteGroupSourcesDDL(dialect)},
		// Table 11: oauth_route_units
		{"oauth_route_units", buildOAuthRouteUnitsDDL(dialect)},
		// Table 12: oauth_route_unit_members
		{"oauth_route_unit_members", buildOAuthRouteUnitMembersDDL(dialect)},
		// Table 13: route_channels
		{"route_channels", buildRouteChannelsDDL(dialect)},
		// Table 14: proxy_logs
		{"proxy_logs", buildProxyLogsDDL(dialect)},
		// Table 15: proxy_debug_traces
		{"proxy_debug_traces", buildProxyDebugTracesDDL(dialect)},
		// Table 16: proxy_debug_attempts
		{"proxy_debug_attempts", buildProxyDebugAttemptsDDL(dialect)},
		// Table 17: proxy_video_tasks
		{"proxy_video_tasks", buildProxyVideoTasksDDL(dialect)},
		// Table 18: proxy_files
		{"proxy_files", buildProxyFilesDDL(dialect)},
		// Table 19: settings (text PK)
		{"settings", buildSettingsDDL(dialect)},
		// Table 20: admin_snapshots
		{"admin_snapshots", buildAdminSnapshotsDDL(dialect)},
		// Table 21: analytics_projection_checkpoints (text PK)
		{"analytics_projection_checkpoints", buildAnalyticsProjectionCheckpointsDDL(dialect)},
		// Table 22: site_day_usage
		{"site_day_usage", buildSiteDayUsageDDL(dialect)},
		// Table 23: site_hour_usage
		{"site_hour_usage", buildSiteHourUsageDDL(dialect)},
		// Table 24: model_day_usage
		{"model_day_usage", buildModelDayUsageDDL(dialect)},
		// Table 25: downstream_api_keys
		{"downstream_api_keys", buildDownstreamAPIKeysDDL(dialect)},
		// Table 26: site_announcements
		{"site_announcements", buildSiteAnnouncementsDDL(dialect)},
		// Table 27: events
		{"events", buildEventsDDL(dialect)},
		// Table 28: admin_background_tasks
		{"admin_background_tasks", buildAdminBackgroundTasksDDL(dialect)},
		// Table 29: balance_history
		{"balance_history", buildBalanceHistoryDDL(dialect)},
		// Table 30: model_verify_history
		{"model_verify_history", buildModelVerifyHistoryDDL(dialect)},
		// Table 31: product_announcements
		{"product_announcements", buildProductAnnouncementsDDL(dialect)},
		// Table 32: announcement_dismissals
		{"announcement_dismissals", buildAnnouncementDismissalsDDL(dialect)},
		// Table 33: model_name_redirects
		{"model_name_redirects", buildModelNameRedirectsDDL(dialect)},
		// Table 34: admin_audit_logs
		{"admin_audit_logs", buildAdminAuditLogsDDL(dialect)},
		// Table 35: model_probe_results
		{"model_probe_results", buildModelProbeResultsDDL(dialect)},
	}

	// Non-UNIQUE indexes are created separately via CREATE INDEX IF NOT EXISTS
	// for both SQLite and PostgreSQL. SQLite inline CREATE TABLE only supports
	// PRIMARY KEY and UNIQUE constraints, not plain indexes.
	migrations = append(migrations, buildIndexes()...)

	for _, m := range migrations {
		if _, err := db.Exec(m.sql); err != nil {
			return fmt.Errorf("store: migrate %s: %w", m.name, err)
		}
	}

	// Additive upgrades for existing installs (ALTER TABLE ADD COLUMN, etc.).
	// CREATE TABLE IF NOT EXISTS alone never mutates an already-created table.
	// The counted variant reports how many steps actually executed so startup
	// logs can summarize schema convergence for old databases.
	appliedAdditive, err := applyAdditiveMigrationsCounted(db, enterpriseAdditiveSteps)
	if err != nil {
		return err
	}
	if appliedAdditive > 0 {
		slog.Info("store: converged legacy schema",
			"additive_migrations", appliedAdditive,
			"dialect", dialect,
		)
	}

	slog.Info("store: auto-migration complete", "dialect", dialect)
	return nil
}

// ---- DDL helper functions ----

// btype returns the boolean column type for a given dialect.
func btype(d string) string {
	if d == DialectPostgres {
		return "BOOLEAN"
	}
	return "INTEGER" // SQLite stores 0/1
}

// rtype returns the real/float column type for a given dialect.
func rtype(d string) string {
	if d == DialectPostgres {
		return "DOUBLE PRECISION"
	}
	return "REAL"
}

// serialPK returns the auto-increment PK column definition.
func serialPK(d string) string {
	if d == DialectPostgres {
		return "SERIAL PRIMARY KEY"
	}
	return "INTEGER PRIMARY KEY AUTOINCREMENT"
}

// textPK returns the text PK column definition (for settings, checkpoints).
func textPK(d string) string {
	return "TEXT PRIMARY KEY"
}

// isPostgres is a short helper.
func isPG(d string) bool { return d == DialectPostgres }

// AllTableNames returns the 18 application tables transferred between
// dialects by cmd/migrate, in canonical schema order (parents before
// children). This is the single source of truth for the migration table set;
// cmd/migrate must not maintain its own copy.
func AllTableNames() []string {
	return []string{
		"sites",
		"site_api_endpoints",
		"site_disabled_models",
		"accounts",
		"account_tokens",
		"checkin_logs",
		"model_availability",
		"token_model_availability",
		"token_routes",
		"route_group_sources",
		"route_channels",
		"proxy_logs",
		"proxy_video_tasks",
		"proxy_files",
		"settings",
		"downstream_api_keys",
		"site_announcements",
		"events",
	}
}

// ClearTableNames returns the 18 application tables in FK-safe delete order
// (children before parents) used to wipe a target database before re-insert.
// A simple reversal of AllTableNames is NOT FK-safe (proxy_logs references
// downstream_api_keys, which sorts later in schema order), so this curated
// order is kept as a second canonical list.
func ClearTableNames() []string {
	return []string{
		"route_channels",
		"route_group_sources",
		"token_model_availability",
		"model_availability",
		"checkin_logs",
		"proxy_logs",
		"proxy_video_tasks",
		"proxy_files",
		"account_tokens",
		"accounts",
		"site_announcements",
		"site_disabled_models",
		"site_api_endpoints",
		"token_routes",
		"sites",
		"downstream_api_keys",
		"events",
		"settings",
	}
}

