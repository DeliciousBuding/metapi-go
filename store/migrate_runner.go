package store

import (
	"fmt"
	"io"
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

	if err := writeMigrationRows(tgtDB, inserts, opts, toSQLite, logw); err != nil {
		return nil, err
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
