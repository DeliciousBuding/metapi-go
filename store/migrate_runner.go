package store

import (
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"
)

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
	// BatchSize groups consecutive same-table rows into multi-row INSERT
	// statements of at most BatchSize rows. <= 0 keeps the row-by-row path
	// (the TS default); the CLI --batch-size flag maps here.
	BatchSize int
	// LogWriter receives structural + progress log lines. nil discards output;
	// the CLI passes os.Stderr, the admin handler passes a task-log-backed writer
	// so /api/tasks/{id} polling surfaces meaningful migration progress.
	LogWriter io.Writer
}

type MigrationSummary struct {
	Dialect    string         `json:"dialect"`
	Connection string         `json:"connection"`
	Overwrite  bool           `json:"overwrite"`
	Version    string         `json:"version"`
	Timestamp  int64          `json:"timestamp"`
	Rows       map[string]int `json:"rows"`
}

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
	// The table set, the copy order and every generic column list are parsed
	// from the schema registry; a malformed registry must fail before any
	// source data is read.
	if err := schemaRegistryErr(); err != nil {
		return nil, err
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

	// 2. Read all application tables into memory (matching TS toBackupSnapshot)
	fmt.Fprintf(logw, "Reading source database...\n")
	snapshot, err := readAllTables(srcDB)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}

	// Print per-table row counts
	for _, t := range AllTableNames() {
		fmt.Fprintf(logw, "  %-28s %d rows\n", t+":", len(snapshot[t]))
	}

	reportUncopiedSourceColumns(logw, snapshot)

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

	// 9. Insert all rows (dialect-specific placeholders). BatchSize > 0 groups
	// consecutive same-table rows into multi-row INSERT statements of at most
	// BatchSize rows; BatchSize <= 0 keeps the row-by-row path (TS default).
	fmt.Fprintf(logw, "\nInserting %d rows...\n", len(inserts))
	inserted := 0
	start := time.Now()

	for _, group := range groupInsertStatements(inserts, opts.BatchSize) {
		var sqlText string
		var args []interface{}
		if toSQLite {
			sqlText, args = buildInsertSQLite(group...)
		} else {
			sqlText, args = buildInsertPG(group...)
		}
		if _, err := tx.Exec(sqlText, args...); err != nil {
			return nil, fmt.Errorf("insert into %s: %w", group[0].table, err)
		}
		inserted += len(group)
		if progress && (inserted%100 == 0 || inserted == len(inserts)) {
			elapsed := time.Since(start)
			fmt.Fprintf(logw, "  %d/%d rows inserted (%s elapsed)\n", inserted, len(inserts), elapsed.Round(time.Millisecond))
		}
	}

	if progress {
		elapsed := time.Since(start)
		fmt.Fprintf(logw, "  Done: %d rows in %s\n", inserted, elapsed.Round(time.Millisecond))
	}

	// 10. Sync sequences (PostgreSQL only; SQLite AUTOINCREMENT handles itself).
	// Shared with the backup import, which has the same problem: rows arrive with
	// explicit ids and the sequences stay where they were.
	if !toSQLite {
		fmt.Fprintf(logw, "\nSyncing PostgreSQL sequences...\n")
		if err := ResyncPGIDSequences(tx); err != nil {
			// A target whose migrations have not all run can legitimately be missing
			// a table; warn and continue, and the joined error names every table
			// that could not be resynced.
			fmt.Fprintf(logw, "  Warning: sync sequences: %v\n", err)
		}
	}

	// 11. Commit
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	summary := buildSummary(snapshot, toURL, overwrite)

	// 12. Verify checksums (if requested). A verification failure is a hard
	// error: the copy demonstrably diverged from the source, so the CLI must
	// exit non-zero instead of printing a "warning" and claiming success.
	if verify {
		fmt.Fprintf(logw, "\nVerifying checksums...\n")
		if err := verifyChecksums(srcDB, tgtDB, snapshot); err != nil {
			fmt.Fprintf(logw, "  Verification failed: %v\n", err)
			return nil, fmt.Errorf("verification failed: %w", err)
		}
		fmt.Fprintf(logw, "  All checksums match.\n")
	}

	// 13. Normalize TS-era timestamps — the "normalize" leg of
	// copy -> verify -> normalize. It must come after verification: checksums
	// compare the target against the source's own values, so rewriting
	// 'YYYY-MM-DD HH:MM:SS' first would fail the very check that proves the
	// copy is faithful. AutoMigrate also sweeps on every server boot, but a
	// migrated database has to be correct the moment the CLI exits.
	if n, err := normalizeLegacyTimestamps(tgtDB); err != nil {
		return nil, fmt.Errorf("normalize legacy timestamps: %w", err)
	} else if n > 0 {
		fmt.Fprintf(logw, "\nNormalized %d legacy TS timestamp value(s) to RFC3339 UTC.\n", n)
	}

	return summary, nil
}

// reportUncopiedSourceColumns logs every source column the copy will drop
// because the Go schema does not declare it. The target has nowhere to put
// such a value, so dropping is unavoidable — but it must never be silent.
func reportUncopiedSourceColumns(logw io.Writer, snapshot map[string][]map[string]interface{}) {
	for _, table := range AllTableNames() {
		spec, err := schemaColumns(table)
		if err != nil {
			continue
		}
		declared := make(map[string]bool, len(spec))
		for _, c := range spec {
			declared[c.name] = true
		}
		seen := make(map[string]bool)
		var dropped []string
		for _, row := range snapshot[table] {
			for col := range row {
				if declared[col] || seen[col] {
					continue
				}
				seen[col] = true
				dropped = append(dropped, col)
			}
		}
		if len(dropped) > 0 {
			sort.Strings(dropped)
			fmt.Fprintf(logw, "  Warning: %s: source column(s) %v are not in the target schema and are NOT copied\n", table, dropped)
		}
	}
}

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
	// Open the source without touching its journal mode. Forcing
	// `_journal_mode=WAL` would contradict the documented read-only promise for
	// the source (docs/migration.md) and fails on a read-only source; a WAL
	// source (the TypeScript version's default) is read correctly regardless.
	return sql.Open("sqlite", sourcePath)
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
		// Surface a writable data directory instead of a cryptic SQLite error
		// (same probe the server bootstrap uses for its SQLite database).
		if sqlitePath != ":memory:" {
			if probeErr := probeSQLiteWritable(sqlitePath); probeErr != nil {
				return nil, probeErr
			}
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

// readAllTables reads all migration tables from the source into memory.
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

// sequenceTableNames returns every migrated table with an "id" column, so
// PostgreSQL sequence syncing skips the text-PK tables (settings: TS parity;
// admin_sessions: hashed token keys, #1034; analytics_projection_checkpoints)
// without a hardcoded list that a new table could drift out of.
func sequenceTableNames() []string {
	var out []string
	for _, table := range AllTableNames() {
		if !tableHasSerialID(table) {
			continue
		}
		out = append(out, table)
	}
	return out
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

func buildSummary(snapshot map[string][]map[string]interface{}, toURL string, overwrite bool) *MigrationSummary {
	// Report the TARGET's real dialect (previously hardcoded "postgres",
	// which mislabeled SQLite→SQLite copies in the CLI summary).
	dialect := "postgres"
	if isSQLiteTarget(toURL) {
		dialect = "sqlite"
	}
	s := &MigrationSummary{
		Dialect:    dialect,
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
