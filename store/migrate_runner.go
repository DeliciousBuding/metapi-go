// Package store's migrate_runner.go is the callable form of the metapi-migrate
// CLI: it transfers all 18 application tables between a SQLite database and a
// PostgreSQL database (either direction). cmd/migrate/main.go is now a thin
// wrapper around RunMigration, and handler/admin/settings_database.go queues
// RunMigration as an admin background task.
//
// The migration matches the TS databaseMigrationService.ts behaviour:
// - Per-column type coercion with fallback defaults
// - JSON column serialization (13 columns across 5 tables)
// - FK-safe DELETE order during overwrite
// - PostgreSQL sequence synchronization after insert (PG targets only;
// SQLite AUTOINCREMENT handles itself)
// - Single-transaction boundary with rollback on error
// - Settings key filtering (skips db_type, db_url, db_ssl)
package store

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---- RunMigrationOptions ----

// RunMigrationOptions configures a single RunMigration invocation. It is the
// callable equivalent of the metapi-migrate CLI flags; the CLI constructs it
// from parsed flags while the admin migrate handler constructs it from the
// HTTP request plus the live runtime database config.
type RunMigrationOptions struct {
	// FromPath is the source database: a SQLite path (plain, sqlite://, or
	// file://) or a PostgreSQL URL (postgres:// / postgresql://).
	FromPath string
	// ToURL is the target database (same encoding rules as FromPath).
	ToURL string
	// Overwrite clears target data in FK-safe order before inserting.
	// Defaults to true to match the TS behaviour; --overwrite=false is a no-op gate.
	Overwrite bool
	// DryRun validates and prints the migration plan without writing data.
	DryRun bool
	// Progress emits per-100-row progress lines to LogWriter when true.
	Progress bool
	// Verify computes row-count + hash checksums after migration.
	Verify bool
	// LogWriter receives structural + progress log lines. nil discards output;
	// the CLI passes os.Stderr, the admin handler passes a task-log-backed writer
	// so /api/tasks/{id} polling surfaces meaningful migration progress.
	LogWriter io.Writer
}

// ---- Type coercion helpers (matching TS) ----

func asString(v interface{}) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func asBoolean(v interface{}, fallback bool) bool {
	switch val := v.(type) {
	case bool:
		return val
	case float64:
		return val != 0
	case int64:
		return val != 0
	case string:
		switch strings.ToLower(strings.TrimSpace(val)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

func asNumber(v interface{}, fallback interface{}) interface{} {
	if v == nil {
		return fallback
	}
	switch val := v.(type) {
	case float64:
		return val
	case int64:
		return float64(val)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil {
			return fallback
		}
		return f
	}
	return fallback
}

func asNullableString(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	return fmt.Sprintf("%v", v)
}

// asNullableBool preserves NULL for nullable boolean columns (e.g.
// sites.resin_enabled). nil stays nil so per-site override semantics
// ("NULL = inherit global") survive the migration round-trip. Non-nil
// values are coerced via asBoolean so old SQLite rows (stored as 0/1
// INTEGER) and PG rows (stored as BOOLEAN) both round-trip correctly.
func asNullableBool(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
		return nil
	}
	return asBoolean(v, false)
}

// ---- JSON column serialization (matching TS serializeColumnValue) ----

// jsonColumnSet records which columns have logical type 'json'.
// 14 columns across 5 tables, matching the TS schemaContract.json logical types.
// (downstream_api_keys.tags is intentionally absent: it migrated as a plain
// TEXT passthrough before this refactor and keeps that behavior.)
var jsonColumnSet = map[string]bool{
	"sites.custom_headers":                         true,
	"accounts.extra_config":                        true,
	"token_routes.model_mapping":                   true,
	"token_routes.decision_snapshot":               true,
	"proxy_logs.billing_details":                   true,
	"proxy_video_tasks.status_snapshot":            true,
	"proxy_video_tasks.upstream_response_meta":     true,
	"downstream_api_keys.supported_models":         true,
	"downstream_api_keys.allowed_route_ids":        true,
	"downstream_api_keys.site_weight_multipliers":  true,
	"downstream_api_keys.excluded_site_ids":        true,
	"downstream_api_keys.excluded_credential_refs": true,
	"downstream_api_keys.allowed_site_ids":         true,
	"downstream_api_keys.allowed_credential_refs":  true,
}

func isJSONColumn(table, column string) bool {
	return jsonColumnSet[table+"."+column]
}

func serializeColumnValue(table, column string, v interface{}) interface{} {
	if isJSONColumn(table, column) {
		return serializeJSONValue(v)
	}
	return asNullableString(v)
}

func serializeJSONValue(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return string(b)
}

// ---- Runtime DB setting keys to skip (matching TS RUNTIME_DATABASE_SETTING_KEYS) ----

var runtimeDBSettingKeys = map[string]bool{
	"db_type": true,
	"db_url":  true,
	"db_ssl":  true,
}

// The migration table set, FK-safe delete order, and auto-increment table
// list all derive from the canonical store schema (store.AllTableNames /
// store.ClearTableNames) so a future store DDL change can never silently
// drift cmd/migrate again.

// sequenceTableNames returns every migrated table with a serial id column
// (settings has a text PK and is excluded, matching TS).
func sequenceTableNames() []string {
	var out []string
	for _, table := range AllTableNames() {
		if table == "settings" {
			continue
		}
		out = append(out, table)
	}
	return out
}

// ---- Migration summary ----

type MigrationSummary struct {
	Dialect    string         `json:"dialect"`
	Connection string         `json:"connection"`
	Overwrite  bool           `json:"overwrite"`
	Version    string         `json:"version"`
	Timestamp  int64          `json:"timestamp"`
	Rows       map[string]int `json:"rows"`
}

// ---- SQL helpers ----

func quoteIdentPG(s string) string {
	return `"` + s + `"`
}

func maskPassword(connStr string) string {
	u, err := url.Parse(connStr)
	if err != nil {
		return connStr
	}
	if u.User != nil {
		if _, ok := u.User.Password(); ok {
			u.User = url.UserPassword(u.User.Username(), "***")
		}
	}
	return u.String()
}

// ---- Migration flow ----

// RunMigration transfers all application tables between a SQLite database and
// a PostgreSQL database (either direction). It is the callable form of the
// metapi-migrate CLI: cmd/migrate/main.go wraps it for the CLI, and the admin
// migrate handler queues it as a background task. The migration SQL/logic is
// unchanged from the original CLI; only the log sink is parameterized via
// opts.LogWriter so progress can be surfaced through /api/tasks/{id}.
func RunMigration(opts RunMigrationOptions) (*MigrationSummary, error) {
	logw := opts.LogWriter
	if logw == nil {
		logw = io.Discard
	}
	fromPath := opts.FromPath
	toURL := opts.ToURL
	overwrite := opts.Overwrite
	dryRun := opts.DryRun
	progress := opts.Progress
	verify := opts.Verify

	fromPG := isPostgresURL(fromPath)
	toSQLite := isSQLiteTarget(toURL)
	switch {
	case fromPG && !toSQLite:
		return nil, fmt.Errorf("unsupported direction: PostgreSQL source to PostgreSQL target")
	case !fromPG && toSQLite:
		fmt.Fprintf(logw, "Direction: SQLite → SQLite (copy / dialect check)\n")
	case fromPG && toSQLite:
		fmt.Fprintf(logw, "Direction: PostgreSQL → SQLite (reverse migration, 2026-08-01)\n")
	default:
		fmt.Fprintf(logw, "Direction: SQLite → PostgreSQL (forward migration)\n")
	}

	// 1. Open source (SQLite path or PostgreSQL URL)
	srcDB, err := openSourceDB(fromPath)
	if err != nil {
		return nil, fmt.Errorf("open source: %w", err)
	}
	defer srcDB.Close()

	// 2. Read all 18 tables into memory (matching TS toBackupSnapshot)
	fmt.Fprintf(logw, "Reading source database...\n")
	snapshot, err := readAllTables(srcDB)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}

	// Print per-table row counts
	for _, t := range AllTableNames() {
		fmt.Fprintf(logw, "  %-28s %d rows\n", t+":", len(snapshot[t]))
	}

	// 3. Build insert statements with full type coercion (matching TS buildStatements)
	inserts := buildStatements(snapshot)

	if dryRun {
		fmt.Fprintf(logw, "\n[Dry-run] Would insert %d rows across %d tables.\n", len(inserts), len(snapshot))
		fmt.Fprintf(logw, "[Dry-run] No data written.\n")
		return buildSummary(snapshot, toURL, overwrite), nil
	}

	// 4. Open target (SQLite path or PostgreSQL URL) and create the canonical
	// schema via store.AutoMigrate — the same dual-dialect DDL the server uses,
	// so target tables always carry every column the insert builders expect.
	tgtDB, err := openTargetDB(toURL)
	if err != nil {
		return nil, fmt.Errorf("open target: %w", err)
	}
	defer tgtDB.Close()

	// 5. Drift guard: refuse to migrate when an insert builder's column list
	// no longer matches the store schema (turns silent data loss into a loud
	// failure instead).
	if err := verifyBuilderColumnsMatchTarget(tgtDB); err != nil {
		return nil, err
	}

	// 6. Check target state (reject if data exists and !overwrite, matching TS ensureTargetState)
	if !overwrite {
		var count int
		if err := tgtDB.QueryRow(`SELECT COUNT(*) FROM "sites"`).Scan(&count); err == nil && count > 0 {
			return nil, fmt.Errorf("target database already contains data. Use --overwrite to replace")
		}
	}

	// 7. Begin transaction
	tx, err := tgtDB.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // safe no-op after commit

	// 8. Clear target data in FK-safe order (if overwrite)
	if overwrite {
		fmt.Fprintf(logw, "\nClearing target data (FK-safe order)...\n")
		for _, table := range ClearTableNames() {
			if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM "%s"`, table)); err != nil {
				return nil, fmt.Errorf("clear %s: %w", table, err)
			}
		}
	}

	// 9. Insert all rows (dialect-specific placeholders)
	fmt.Fprintf(logw, "\nInserting %d rows...\n", len(inserts))
	inserted := 0
	start := time.Now()

	for _, stmt := range inserts {
		var sqlText string
		var args []interface{}
		if toSQLite {
			sqlText, args = buildInsertSQLite(stmt)
		} else {
			sqlText, args = buildInsertPG(stmt)
		}
		if _, err := tx.Exec(sqlText, args...); err != nil {
			return nil, fmt.Errorf("insert into %s: %w", stmt.table, err)
		}
		inserted++
		if progress && inserted%100 == 0 {
			elapsed := time.Since(start)
			fmt.Fprintf(logw, "  %d/%d rows inserted (%s elapsed)\n", inserted, len(inserts), elapsed.Round(time.Millisecond))
		}
	}

	if progress {
		elapsed := time.Since(start)
		fmt.Fprintf(logw, "  Done: %d rows in %s\n", inserted, elapsed.Round(time.Millisecond))
	}

	// 10. Sync sequences (PostgreSQL only; SQLite AUTOINCREMENT handles itself)
	if !toSQLite {
		fmt.Fprintf(logw, "\nSyncing PostgreSQL sequences...\n")
		for _, table := range sequenceTableNames() {
			q := fmt.Sprintf(`SELECT setval(pg_get_serial_sequence('%s', 'id'), COALESCE((SELECT MAX("id") FROM "%s"), 1), TRUE)`, table, table)
			if _, err := tx.Exec(q); err != nil {
				// Table might not exist if migrations haven't run; warn but continue
				fmt.Fprintf(logw, "  Warning: sync sequence for %s: %v\n", table, err)
			}
		}
	}

	// 11. Commit
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	summary := buildSummary(snapshot, toURL, overwrite)

	// 12. Verify checksums (if requested)
	if verify {
		fmt.Fprintf(logw, "\nVerifying checksums...\n")
		if err := verifyChecksums(srcDB, tgtDB, snapshot); err != nil {
			fmt.Fprintf(logw, "  Verification warning: %v\n", err)
		} else {
			fmt.Fprintf(logw, "  All checksums match.\n")
		}
	}

	return summary, nil
}

// ---- Dialect helpers (reverse-migration support, 2026-08-01) ----

func isPostgresURL(raw string) bool {
	return strings.HasPrefix(raw, "postgres://") || strings.HasPrefix(raw, "postgresql://")
}

// isSQLiteTarget treats "sqlite://path" and plain paths as SQLite targets.
func isSQLiteTarget(raw string) bool {
	return !isPostgresURL(raw)
}

// openSourceDB opens a SQLite path or a PostgreSQL URL as the source.
func openSourceDB(raw string) (*sql.DB, error) {
	if isPostgresURL(raw) {
		db, err := sql.Open("pgx", raw)
		if err != nil {
			return nil, err
		}
		if err := db.Ping(); err != nil {
			_ = db.Close()
			return nil, err
		}
		return db, nil
	}
	sourcePath, err := normalizeSQLitePath(raw)
	if err != nil {
		return nil, err
	}
	return sql.Open("sqlite", sourcePath+"?_journal_mode=WAL")
}

// openTargetDB opens a SQLite path or a PostgreSQL URL as the target via
// store.Open (WAL + pragmas for SQLite, sslmode/pool handling for PostgreSQL)
// and runs store.AutoMigrate so the target carries the canonical dual-dialect
// schema. The caller must Close the returned DB.
func openTargetDB(raw string) (*DB, error) {
	var (
		db  *DB
		err error
	)
	if isPostgresURL(raw) {
		db, err = Open(DialectPostgres, raw, false)
	} else {
		sqlitePath, pathErr := normalizeSQLitePath(raw)
		if pathErr != nil {
			return nil, pathErr
		}
		db, err = Open(DialectSQLite, sqlitePath, false)
	}
	if err != nil {
		return nil, err
	}

	if err := AutoMigrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("auto-migrate target schema: %w", err)
	}
	return db, nil
}

// normalizeSQLitePath handles sqlite:// prefix and plain paths (matching TS normalizeSqliteTarget).
func normalizeSQLitePath(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("path cannot be empty")
	}
	if raw == ":memory:" {
		return raw, nil
	}

	lower := strings.ToLower(raw)

	// file:// prefix
	if strings.HasPrefix(lower, "file://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", fmt.Errorf("invalid file:// URL: %w", err)
		}
		return u.Path, nil
	}

	// sqlite:// prefix
	if strings.HasPrefix(lower, "sqlite://") {
		return strings.TrimSpace(raw[len("sqlite://"):]), nil
	}

	// Guard against network URLs
	if strings.Contains(raw, "://") {
		return "", fmt.Errorf("SQLite connection cannot be a network URL; use plain file path or sqlite:// prefix")
	}

	return raw, nil
}

// readAllTables reads all 18 migration tables from the source into memory.
// The table set is sourced from store.AllTableNames so it can never drift
// from the canonical schema.
func readAllTables(db *sql.DB) (map[string][]map[string]interface{}, error) {
	snapshot := make(map[string][]map[string]interface{})

	for _, table := range AllTableNames() {
		rows, err := db.Query(fmt.Sprintf(`SELECT * FROM "%s"`, table))
		if err != nil {
			// Table might not exist if migrations haven't created it
			snapshot[table] = nil
			continue
		}

		cols, err := rows.Columns()
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("columns for %s: %w", table, err)
		}

		var tableRows []map[string]interface{}
		for rows.Next() {
			values := make([]interface{}, len(cols))
			valuePtrs := make([]interface{}, len(cols))
			for i := range values {
				valuePtrs[i] = &values[i]
			}
			if err := rows.Scan(valuePtrs...); err != nil {
				rows.Close()
				return nil, fmt.Errorf("scan %s: %w", table, err)
			}
			row := make(map[string]interface{})
			for i, col := range cols {
				row[col] = values[i]
			}
			tableRows = append(tableRows, row)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iter %s: %w", table, err)
		}

		snapshot[table] = tableRows
	}

	return snapshot, nil
}

// ---- Insert statement builder (matching TS buildStatements) ----

type insertStmt struct {
	table   string
	columns []string
	values  []interface{}
}

func buildStatements(snapshot map[string][]map[string]interface{}) []insertStmt {
	var stmts []insertStmt

	stmts = append(stmts, buildSites(snapshot["sites"])...)
	stmts = append(stmts, buildSiteAPIEndpoints(snapshot["site_api_endpoints"])...)
	stmts = append(stmts, buildSiteDisabledModels(snapshot["site_disabled_models"])...)
	stmts = append(stmts, buildSiteAnnouncements(snapshot["site_announcements"])...)
	stmts = append(stmts, buildAccounts(snapshot["accounts"])...)
	stmts = append(stmts, buildAccountTokens(snapshot["account_tokens"])...)
	stmts = append(stmts, buildCheckinLogs(snapshot["checkin_logs"])...)
	stmts = append(stmts, buildModelAvailability(snapshot["model_availability"])...)
	stmts = append(stmts, buildTokenModelAvailability(snapshot["token_model_availability"])...)
	stmts = append(stmts, buildTokenRoutes(snapshot["token_routes"])...)
	stmts = append(stmts, buildRouteChannels(snapshot["route_channels"])...)
	stmts = append(stmts, buildRouteGroupSources(snapshot["route_group_sources"])...)
	stmts = append(stmts, buildProxyLogs(snapshot["proxy_logs"])...)
	stmts = append(stmts, buildProxyVideoTasks(snapshot["proxy_video_tasks"])...)
	stmts = append(stmts, buildProxyFiles(snapshot["proxy_files"])...)
	stmts = append(stmts, buildDownstreamAPIKeys(snapshot["downstream_api_keys"])...)
	stmts = append(stmts, buildEvents(snapshot["events"])...)
	stmts = append(stmts, buildSettings(snapshot["settings"])...)

	return stmts
}

func v(row map[string]interface{}, key string) interface{} {
	return row[key]
}

func buildSites(rows []map[string]interface{}) []insertStmt {
	// Column list mirrors store.buildSitesDDL + additive columns exactly;
	// verifyBuilderColumnsMatchTarget enforces this at migration time.
	cols := []string{
		"id", "name", "url", "external_checkin_url", "platform", "proxy_url",
		"use_system_proxy", "custom_headers", "custom_headers_override_request_headers",
		"status", "is_pinned", "sort_order", "global_weight", "api_key",
		"max_concurrency",
		"post_refresh_probe_enabled", "post_refresh_probe_model", "post_refresh_probe_scope",
		"post_refresh_probe_latency_threshold_ms",
		"created_at", "updated_at", "tags", "browser_ua", "cf_clearance",
		"resin_enabled", "use_utls",
	}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "sites", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNullableString(v(row, "name")),
				asNullableString(v(row, "url")),
				asNullableString(v(row, "external_checkin_url")),
				asNullableString(v(row, "platform")),
				asNullableString(v(row, "proxy_url")),
				asBoolean(v(row, "use_system_proxy"), false),
				serializeColumnValue("sites", "custom_headers", v(row, "custom_headers")),
				asBoolean(v(row, "custom_headers_override_request_headers"), false),
				coalesceNullString(asNullableString(v(row, "status")), "active"),
				asBoolean(v(row, "is_pinned"), false),
				asNumber(v(row, "sort_order"), float64(0)),
				asNumber(v(row, "global_weight"), float64(1)),
				asNullableString(v(row, "api_key")),
				asNumber(v(row, "max_concurrency"), float64(0)),
				asBoolean(v(row, "post_refresh_probe_enabled"), false),
				coalesceNullString(asNullableString(v(row, "post_refresh_probe_model")), ""),
				coalesceNullString(asNullableString(v(row, "post_refresh_probe_scope")), "single"),
				asNumber(v(row, "post_refresh_probe_latency_threshold_ms"), float64(0)),
				asNullableString(v(row, "created_at")),
				asNullableString(v(row, "updated_at")),
				asNullableString(v(row, "tags")),
				asNullableString(v(row, "browser_ua")),
				asNullableString(v(row, "cf_clearance")),
				// resin_enabled is a nullable boolean override; preserve source
				// value verbatim (NULL stays NULL so per-site inherits global).
				asNullableBool(v(row, "resin_enabled")),
				// use_utls is a nullable boolean override; preserve source value
				// verbatim (NULL stays NULL so per-site inherits global UTLS_ENABLED).
				asNullableBool(v(row, "use_utls")),
			},
		})
	}
	return stmts
}

func coalesceNullString(v interface{}, fallback string) interface{} {
	if v == nil {
		return fallback
	}
	return v
}

func buildSiteAPIEndpoints(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "site_id", "url", "enabled", "sort_order", "cooldown_until", "last_selected_at", "last_failed_at", "last_failure_reason", "created_at", "updated_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "site_api_endpoints", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNumber(v(row, "site_id"), float64(0)),
				asNullableString(v(row, "url")),
				asBoolean(v(row, "enabled"), true),
				asNumber(v(row, "sort_order"), float64(0)),
				asNullableString(v(row, "cooldown_until")),
				asNullableString(v(row, "last_selected_at")),
				asNullableString(v(row, "last_failed_at")),
				asNullableString(v(row, "last_failure_reason")),
				asNullableString(v(row, "created_at")),
				asNullableString(v(row, "updated_at")),
			},
		})
	}
	return stmts
}

func buildSiteDisabledModels(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "site_id", "model_name", "created_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "site_disabled_models", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNumber(v(row, "site_id"), float64(0)),
				asNullableString(v(row, "model_name")),
				asNullableString(v(row, "created_at")),
			},
		})
	}
	return stmts
}

func buildSiteAnnouncements(rows []map[string]interface{}) []insertStmt {
	// The store DDL for site_announcements has no created_at/updated_at columns.
	cols := []string{"id", "site_id", "platform", "source_key", "title", "content", "level", "source_url", "starts_at", "ends_at", "upstream_created_at", "upstream_updated_at", "first_seen_at", "last_seen_at", "read_at", "dismissed_at", "raw_payload"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "site_announcements", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNumber(v(row, "site_id"), float64(0)),
				asNullableString(v(row, "platform")),
				asNullableString(v(row, "source_key")),
				asNullableString(v(row, "title")),
				asNullableString(v(row, "content")),
				coalesceNullString(asNullableString(v(row, "level")), "info"),
				asNullableString(v(row, "source_url")),
				asNullableString(v(row, "starts_at")),
				asNullableString(v(row, "ends_at")),
				asNullableString(v(row, "upstream_created_at")),
				asNullableString(v(row, "upstream_updated_at")),
				asNullableString(v(row, "first_seen_at")),
				asNullableString(v(row, "last_seen_at")),
				asNullableString(v(row, "read_at")),
				asNullableString(v(row, "dismissed_at")),
				asNullableString(v(row, "raw_payload")),
			},
		})
	}
	return stmts
}

func buildAccounts(rows []map[string]interface{}) []insertStmt {
	cols := []string{
		"id", "site_id", "username", "access_token", "api_token", "balance", "balance_used",
		"quota", "unit_cost", "value_score", "status", "is_pinned", "sort_order",
		"checkin_enabled", "last_checkin_at", "last_balance_refresh",
		"oauth_provider", "oauth_account_key", "oauth_project_id",
		"extra_config", "created_at", "updated_at", "tags",
	}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "accounts", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNumber(v(row, "site_id"), float64(0)),
				asNullableString(v(row, "username")),
				asNullableString(v(row, "access_token")),
				asNullableString(v(row, "api_token")),
				asNumber(v(row, "balance"), float64(0)),
				asNumber(v(row, "balance_used"), float64(0)),
				asNumber(v(row, "quota"), float64(0)),
				asNumber(v(row, "unit_cost"), nil),
				asNumber(v(row, "value_score"), float64(0)),
				coalesceNullString(asNullableString(v(row, "status")), "active"),
				asBoolean(v(row, "is_pinned"), false),
				asNumber(v(row, "sort_order"), float64(0)),
				asBoolean(v(row, "checkin_enabled"), true),
				asNullableString(v(row, "last_checkin_at")),
				asNullableString(v(row, "last_balance_refresh")),
				asNullableString(v(row, "oauth_provider")),
				asNullableString(v(row, "oauth_account_key")),
				asNullableString(v(row, "oauth_project_id")),
				serializeColumnValue("accounts", "extra_config", v(row, "extra_config")),
				asNullableString(v(row, "created_at")),
				asNullableString(v(row, "updated_at")),
				asNullableString(v(row, "tags")),
			},
		})
	}
	return stmts
}

func buildAccountTokens(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "account_id", "name", "token", "token_group", "value_status", "source", "enabled", "is_default", "created_at", "updated_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "account_tokens", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNumber(v(row, "account_id"), float64(0)),
				asNullableString(v(row, "name")),
				asNullableString(v(row, "token")),
				asNullableString(v(row, "token_group")),
				coalesceNullString(asNullableString(v(row, "value_status")), "ready"),
				coalesceNullString(asNullableString(v(row, "source")), "manual"),
				asBoolean(v(row, "enabled"), true),
				asBoolean(v(row, "is_default"), false),
				asNullableString(v(row, "created_at")),
				asNullableString(v(row, "updated_at")),
			},
		})
	}
	return stmts
}

func buildCheckinLogs(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "account_id", "status", "message", "reward", "failure_reason", "created_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "checkin_logs", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNumber(v(row, "account_id"), float64(0)),
				coalesceNullString(asNullableString(v(row, "status")), "success"),
				asNullableString(v(row, "message")),
				asNullableString(v(row, "reward")),
				asNullableString(v(row, "failure_reason")),
				asNullableString(v(row, "created_at")),
			},
		})
	}
	return stmts
}

func buildModelAvailability(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "account_id", "model_name", "available", "is_manual", "latency_ms", "checked_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "model_availability", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNumber(v(row, "account_id"), float64(0)),
				asNullableString(v(row, "model_name")),
				asBoolean(v(row, "available"), false),
				asBoolean(v(row, "is_manual"), false),
				asNumber(v(row, "latency_ms"), nil),
				asNullableString(v(row, "checked_at")),
			},
		})
	}
	return stmts
}

func buildTokenModelAvailability(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "token_id", "model_name", "available", "latency_ms", "checked_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "token_model_availability", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNumber(v(row, "token_id"), float64(0)),
				asNullableString(v(row, "model_name")),
				asBoolean(v(row, "available"), false),
				asNumber(v(row, "latency_ms"), nil),
				asNullableString(v(row, "checked_at")),
			},
		})
	}
	return stmts
}

func buildTokenRoutes(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "model_pattern", "display_name", "display_icon", "model_mapping", "route_mode", "decision_snapshot", "decision_refreshed_at", "routing_strategy", "context_length", "sort_order", "enabled", "created_at", "updated_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "token_routes", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNullableString(v(row, "model_pattern")),
				asNullableString(v(row, "display_name")),
				asNullableString(v(row, "display_icon")),
				serializeColumnValue("token_routes", "model_mapping", v(row, "model_mapping")),
				coalesceNullString(asNullableString(v(row, "route_mode")), "pattern"),
				serializeColumnValue("token_routes", "decision_snapshot", v(row, "decision_snapshot")),
				asNullableString(v(row, "decision_refreshed_at")),
				asNullableString(v(row, "routing_strategy")),
				asNumber(v(row, "context_length"), nil),
				asNumber(v(row, "sort_order"), float64(0)),
				asBoolean(v(row, "enabled"), true),
				asNullableString(v(row, "created_at")),
				asNullableString(v(row, "updated_at")),
			},
		})
	}
	return stmts
}

func buildRouteChannels(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "route_id", "account_id", "token_id", "oauth_route_unit_id", "source_model", "priority", "weight", "enabled", "manual_override", "success_count", "fail_count", "total_latency_ms", "total_cost", "last_used_at", "last_selected_at", "last_fail_at", "consecutive_fail_count", "cooldown_level", "cooldown_until"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "route_channels", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNumber(v(row, "route_id"), float64(0)),
				asNumber(v(row, "account_id"), float64(0)),
				asNumber(v(row, "token_id"), nil),
				asNumber(v(row, "oauth_route_unit_id"), nil),
				asNullableString(v(row, "source_model")),
				asNumber(v(row, "priority"), float64(0)),
				asNumber(v(row, "weight"), float64(10)),
				asBoolean(v(row, "enabled"), true),
				asBoolean(v(row, "manual_override"), false),
				asNumber(v(row, "success_count"), float64(0)),
				asNumber(v(row, "fail_count"), float64(0)),
				asNumber(v(row, "total_latency_ms"), float64(0)),
				asNumber(v(row, "total_cost"), float64(0)),
				asNullableString(v(row, "last_used_at")),
				asNullableString(v(row, "last_selected_at")),
				asNullableString(v(row, "last_fail_at")),
				asNumber(v(row, "consecutive_fail_count"), float64(0)),
				asNumber(v(row, "cooldown_level"), float64(0)),
				asNullableString(v(row, "cooldown_until")),
			},
		})
	}
	return stmts
}

func buildRouteGroupSources(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "group_route_id", "source_route_id"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "route_group_sources", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNumber(v(row, "group_route_id"), float64(0)),
				asNumber(v(row, "source_route_id"), float64(0)),
			},
		})
	}
	return stmts
}

func buildProxyLogs(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "route_id", "channel_id", "account_id", "downstream_api_key_id", "model_requested", "model_actual", "status", "http_status", "is_stream", "first_byte_latency_ms", "latency_ms", "prompt_tokens", "completion_tokens", "total_tokens", "estimated_cost", "billing_details", "client_family", "client_app_id", "client_app_name", "client_confidence", "error_message", "retry_count", "request_id", "created_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "proxy_logs", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNumber(v(row, "route_id"), nil),
				asNumber(v(row, "channel_id"), nil),
				asNumber(v(row, "account_id"), nil),
				asNumber(v(row, "downstream_api_key_id"), nil),
				asNullableString(v(row, "model_requested")),
				asNullableString(v(row, "model_actual")),
				asNullableString(v(row, "status")),
				asNumber(v(row, "http_status"), nil),
				asBoolean(v(row, "is_stream"), false),
				asNumber(v(row, "first_byte_latency_ms"), nil),
				asNumber(v(row, "latency_ms"), nil),
				asNumber(v(row, "prompt_tokens"), nil),
				asNumber(v(row, "completion_tokens"), nil),
				asNumber(v(row, "total_tokens"), nil),
				asNumber(v(row, "estimated_cost"), nil),
				serializeColumnValue("proxy_logs", "billing_details", v(row, "billing_details")),
				asNullableString(v(row, "client_family")),
				asNullableString(v(row, "client_app_id")),
				asNullableString(v(row, "client_app_name")),
				asNullableString(v(row, "client_confidence")),
				asNullableString(v(row, "error_message")),
				asNumber(v(row, "retry_count"), float64(0)),
				asNullableString(v(row, "request_id")),
				asNullableString(v(row, "created_at")),
			},
		})
	}
	return stmts
}

func buildProxyVideoTasks(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "public_id", "upstream_video_id", "site_url", "token_value", "requested_model", "actual_model", "channel_id", "account_id", "status_snapshot", "upstream_response_meta", "last_upstream_status", "last_polled_at", "created_at", "updated_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "proxy_video_tasks", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNullableString(v(row, "public_id")),
				asNullableString(v(row, "upstream_video_id")),
				asNullableString(v(row, "site_url")),
				asNullableString(v(row, "token_value")),
				asNullableString(v(row, "requested_model")),
				asNullableString(v(row, "actual_model")),
				asNumber(v(row, "channel_id"), nil),
				asNumber(v(row, "account_id"), nil),
				serializeColumnValue("proxy_video_tasks", "status_snapshot", v(row, "status_snapshot")),
				serializeColumnValue("proxy_video_tasks", "upstream_response_meta", v(row, "upstream_response_meta")),
				asNumber(v(row, "last_upstream_status"), nil),
				asNullableString(v(row, "last_polled_at")),
				asNullableString(v(row, "created_at")),
				asNullableString(v(row, "updated_at")),
			},
		})
	}
	return stmts
}

func buildProxyFiles(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "public_id", "owner_type", "owner_id", "filename", "mime_type", "purpose", "byte_size", "sha256", "content_base64", "created_at", "updated_at", "deleted_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "proxy_files", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNullableString(v(row, "public_id")),
				asNullableString(v(row, "owner_type")),
				asNullableString(v(row, "owner_id")),
				asNullableString(v(row, "filename")),
				asNullableString(v(row, "mime_type")),
				asNullableString(v(row, "purpose")),
				asNumber(v(row, "byte_size"), float64(0)),
				asNullableString(v(row, "sha256")),
				asNullableString(v(row, "content_base64")),
				asNullableString(v(row, "created_at")),
				asNullableString(v(row, "updated_at")),
				asNullableString(v(row, "deleted_at")),
			},
		})
	}
	return stmts
}

func buildDownstreamAPIKeys(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "name", "key", "description", "group_name", "tags", "enabled", "expires_at", "max_cost", "used_cost", "max_requests", "used_requests", "supported_models", "allowed_route_ids", "site_weight_multipliers", "excluded_site_ids", "excluded_credential_refs", "allowed_site_ids", "allowed_credential_refs", "key_weight", "proxy_url", "max_rpm", "max_tpm", "ip_allowlist", "ip_blocklist", "last_used_at", "created_at", "updated_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "downstream_api_keys", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNullableString(v(row, "name")),
				asNullableString(v(row, "key")),
				asNullableString(v(row, "description")),
				asNullableString(v(row, "group_name")),
				asNullableString(v(row, "tags")),
				asBoolean(v(row, "enabled"), true),
				asNullableString(v(row, "expires_at")),
				asNumber(v(row, "max_cost"), nil),
				asNumber(v(row, "used_cost"), float64(0)),
				asNumber(v(row, "max_requests"), nil),
				asNumber(v(row, "used_requests"), float64(0)),
				serializeColumnValue("downstream_api_keys", "supported_models", v(row, "supported_models")),
				serializeColumnValue("downstream_api_keys", "allowed_route_ids", v(row, "allowed_route_ids")),
				serializeColumnValue("downstream_api_keys", "site_weight_multipliers", v(row, "site_weight_multipliers")),
				serializeColumnValue("downstream_api_keys", "excluded_site_ids", v(row, "excluded_site_ids")),
				serializeColumnValue("downstream_api_keys", "excluded_credential_refs", v(row, "excluded_credential_refs")),
				serializeColumnValue("downstream_api_keys", "allowed_site_ids", v(row, "allowed_site_ids")),
				serializeColumnValue("downstream_api_keys", "allowed_credential_refs", v(row, "allowed_credential_refs")),
				asNumber(v(row, "key_weight"), nil),
				asNullableString(v(row, "proxy_url")),
				asNumber(v(row, "max_rpm"), nil),
				asNumber(v(row, "max_tpm"), nil),
				asNullableString(v(row, "ip_allowlist")),
				asNullableString(v(row, "ip_blocklist")),
				asNullableString(v(row, "last_used_at")),
				asNullableString(v(row, "created_at")),
				asNullableString(v(row, "updated_at")),
			},
		})
	}
	return stmts
}

func buildEvents(rows []map[string]interface{}) []insertStmt {
	cols := []string{"id", "type", "title", "message", "level", "read", "related_id", "related_type", "created_at"}
	var stmts []insertStmt
	for _, row := range rows {
		stmts = append(stmts, insertStmt{
			table: "events", columns: cols,
			values: []interface{}{
				asNumber(v(row, "id"), float64(0)),
				asNullableString(v(row, "type")),
				asNullableString(v(row, "title")),
				asNullableString(v(row, "message")),
				coalesceNullString(asNullableString(v(row, "level")), "info"),
				asBoolean(v(row, "read"), false),
				asNumber(v(row, "related_id"), nil),
				asNullableString(v(row, "related_type")),
				asNullableString(v(row, "created_at")),
			},
		})
	}
	return stmts
}

func buildSettings(rows []map[string]interface{}) []insertStmt {
	cols := []string{"key", "value"}
	var stmts []insertStmt
	for _, row := range rows {
		key := asString(v(row, "key"))
		if runtimeDBSettingKeys[key] {
			continue
		}
		stmts = append(stmts, insertStmt{
			table: "settings", columns: cols,
			values: []interface{}{
				key,
				asNullableString(v(row, "value")),
			},
		})
	}
	return stmts
}

// ---- PG INSERT builder (matching TS buildInsertSql for postgres) ----

func buildInsertPG(s insertStmt) (string, []interface{}) {
	table := quoteIdentPG(s.table)
	quotedCols := make([]string, len(s.columns))
	for i, c := range s.columns {
		quotedCols[i] = quoteIdentPG(c)
	}
	placeholders := make([]string, len(s.columns))
	for i := range s.columns {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	sql := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		table,
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "),
	)
	return sql, s.values
}

// ---- Ensure target schema ----
//
// Removed: the previous ensureTargetSchema triplicated schema knowledge in
// cmd/migrate and drifted from store/migrate.go (missing max_concurrency,
// post_refresh_probe_* and tags on sites, etc.), silently dropping data.
// openTargetDB now calls store.Open + store.AutoMigrate so the canonical
// dual-dialect DDL is the single source of truth.

func buildInsertSQLite(s insertStmt) (string, []interface{}) {
	quotedCols := make([]string, len(s.columns))
	for i, c := range s.columns {
		quotedCols[i] = `"` + c + `"`
	}
	placeholders := make([]string, len(s.columns))
	for i := range s.columns {
		placeholders[i] = "?"
	}
	sqlText := fmt.Sprintf("INSERT INTO \"%s\" (%s) VALUES (%s)",
		s.table,
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "),
	)
	return sqlText, s.values
}

// ---- Checksum verification ----

func verifyChecksums(srcDB *sql.DB, tgtDB *DB, snapshot map[string][]map[string]interface{}) error {
	for _, table := range AllTableNames() {
		srcCount := len(snapshot[table])

		var tgtCount int
		if err := tgtDB.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, table)).Scan(&tgtCount); err != nil {
			return fmt.Errorf("count %s: %w", table, err)
		}

		if srcCount != tgtCount {
			return fmt.Errorf("%s: row count mismatch (source=%d, target=%d)", table, srcCount, tgtCount)
		}

		// Compute hash of source and target for this table
		srcHash := hashRows(snapshot[table])
		tgtHash, err := hashPGTable(tgtDB, table)
		if err != nil {
			return fmt.Errorf("hash %s: %w", table, err)
		}

		if !bytes.Equal(srcHash, tgtHash) {
			return fmt.Errorf("%s: checksum mismatch (source=%x, target=%x)", table, srcHash, tgtHash)
		}
	}
	return nil
}

func hashRows(rows []map[string]interface{}) []byte {
	h := sha256.New()
	// Sort keys for deterministic serialization
	for _, row := range rows {
		keys := make([]string, 0, len(row))
		for k := range row {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			h.Write([]byte(k))
			h.Write([]byte(fmt.Sprintf("%v", row[k])))
		}
	}
	return h.Sum(nil)
}

func hashPGTable(db *DB, table string) ([]byte, error) {
	rows, err := db.Query(fmt.Sprintf(`SELECT * FROM "%s" ORDER BY id`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	h := sha256.New()
	for rows.Next() {
		values := make([]interface{}, len(cols))
		valuePtrs := make([]interface{}, len(cols))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, err
		}
		for i, col := range cols {
			h.Write([]byte(col))
			h.Write([]byte(fmt.Sprintf("%v", values[i])))
		}
	}
	return h.Sum(nil), rows.Err()
}

// ---- Drift guard ----

// verifyBuilderColumnsMatchTarget fails the migration when any per-table
// insert builder's column list differs from the target schema created by
// store.AutoMigrate. This turns silent column drift (data loss) into a loud
// failure before any data is written.
func verifyBuilderColumnsMatchTarget(db *DB) error {
	builderColumns := builderColumnSets()
	for _, table := range AllTableNames() {
		actual, err := targetColumns(db, table)
		if err != nil {
			return fmt.Errorf("read target columns for %s: %w", table, err)
		}
		expected := builderColumns[table]
		missing, extra := diffColumnSets(expected, actual)
		if len(missing) > 0 || len(extra) > 0 {
			return fmt.Errorf(
				"cmd/migrate insert builder for table %q is out of sync with store DDL (missing=%v extra=%v); refusing to migrate",
				table, missing, extra,
			)
		}
	}
	return nil
}

// builderColumnSets derives each builder's INSERT column list by running the
// builders against one synthetic empty row per table. No hardcoded table list
// lives here — the table set comes from AllTableNames().
func builderColumnSets() map[string][]string {
	snapshot := make(map[string][]map[string]interface{}, len(AllTableNames()))
	for _, table := range AllTableNames() {
		snapshot[table] = []map[string]interface{}{{}}
	}
	out := make(map[string][]string)
	for _, stmt := range buildStatements(snapshot) {
		out[stmt.table] = stmt.columns
	}
	return out
}

// targetColumns returns the canonical column list of a target table.
func targetColumns(db *DB, table string) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf(`SELECT * FROM "%s" LIMIT 0`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rows.Columns()
}

// diffColumnSets reports names present in exactly one of the two sets.
func diffColumnSets(expected, actual []string) (missing, extra []string) {
	expectedSet := make(map[string]bool, len(expected))
	for _, c := range expected {
		expectedSet[c] = true
	}
	actualSet := make(map[string]bool, len(actual))
	for _, c := range actual {
		actualSet[c] = true
	}
	for _, c := range expected {
		if !actualSet[c] {
			missing = append(missing, c)
		}
	}
	for _, c := range actual {
		if !expectedSet[c] {
			extra = append(extra, c)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	return missing, extra
}

// ---- Summary ----

func buildSummary(snapshot map[string][]map[string]interface{}, toURL string, overwrite bool) *MigrationSummary {
	s := &MigrationSummary{
		Dialect:    "postgres",
		Connection: maskPassword(toURL),
		Overwrite:  overwrite,
		Version:    "live-db-snapshot",
		Timestamp:  time.Now().UnixMilli(),
		Rows:       make(map[string]int),
	}
	for _, table := range AllTableNames() {
		s.Rows[table] = len(snapshot[table])
	}
	return s
}
