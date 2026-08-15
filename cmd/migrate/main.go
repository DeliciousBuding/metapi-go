// Command metapi-migrate is a standalone CLI that transfers all 18 application
// tables between a SQLite database and a PostgreSQL database (either direction).
//
// The migration logic lives in store.RunMigration so the same code path is
// callable from the admin migrate handler (queued as a background task). This
// file is a thin flag-parsing + summary-printing wrapper.
//
// Usage:
//
//	metapi-migrate --from sqlite://data/hub.db --to postgres://user:pass@host:5432/db
//	metapi-migrate --from postgres://user:pass@host:5432/db --to sqlite://data/hub.db
//	metapi-migrate --from sqlite://data/hub.db --to postgres://user:pass@host:5432/db --dry-run
//	metapi-migrate --from sqlite://data/hub.db --to postgres://user:pass@host:5432/db --overwrite --progress --verify
//
// The migration matches the TS databaseMigrationService.ts behaviour:
// - Per-column type coercion with fallback defaults
// - JSON column serialization (13 columns across 5 tables)
// - FK-safe DELETE order during overwrite
// - PostgreSQL sequence synchronization after insert (PG targets only;
// SQLite AUTOINCREMENT handles itself)
// - Single-transaction boundary with rollback on error
// - Settings key filtering (skips db_type, db_url, db_ssl)
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/deliciousbuding/metapi-go/store"
)

// ---- CLI flags ----

var (
	flagFrom      = flag.String("from", "", "Source SQLite database (sqlite://path or plain path)")
	flagTo        = flag.String("to", "", "Target PostgreSQL connection string (postgres://user:pass@host:port/db)")
	flagDryRun    = flag.Bool("dry-run", false, "Validate and print migration plan without writing data")
	flagProgress  = flag.Bool("progress", false, "Show per-table progress during transfer")
	flagVerify    = flag.Bool("verify", false, "Compute row-count + hash checksum after migration")
	flagOverwrite = flag.Bool("overwrite", true, "Clear target data before inserting (default true, matches TS)")
	flagBatchSize = flag.Int("batch-size", 1, "Rows per multi-row INSERT batch (1 = row-by-row, matching TS default)")
)

func main() {
	flag.Parse()

	if *flagFrom == "" || *flagTo == "" {
		fmt.Fprintf(os.Stderr, "Usage: metapi-migrate --from <sqlite_path|postgres_url> --to <postgres_url|sqlite_path> [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Directions:\n")
		fmt.Fprintf(os.Stderr, "  SQLite → PostgreSQL : --from sqlite://data/hub.db --to postgres://user:pass@host:5432/db\n")
		fmt.Fprintf(os.Stderr, "  PostgreSQL → SQLite : --from postgres://user:pass@host:5432/db --to sqlite://data/hub.db\n\n")
		fmt.Fprintf(os.Stderr, "Flags:\n")
		flag.PrintDefaults()
		os.Exit(1)
	}

	summary, err := store.RunMigration(store.RunMigrationOptions{
		FromPath:  *flagFrom,
		ToURL:     *flagTo,
		Overwrite: *flagOverwrite,
		DryRun:    *flagDryRun,
		Progress:  *flagProgress,
		Verify:    *flagVerify,
		LogWriter: os.Stderr,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// batchSize is accepted for CLI compatibility but the row-by-row insert
	// path is the only one the store.RunMigration implementation uses today.
	_ = flagBatchSize

	printSummary(summary)
}

func printSummary(s *store.MigrationSummary) {
	fmt.Fprintf(os.Stderr, "\nMigration Summary:\n")
	fmt.Fprintf(os.Stderr, "  dialect:    %s\n", s.Dialect)
	fmt.Fprintf(os.Stderr, "  connection: %s\n", s.Connection)
	fmt.Fprintf(os.Stderr, "  overwrite:  %v\n", s.Overwrite)
	fmt.Fprintf(os.Stderr, "  version:    %s\n", s.Version)
	fmt.Fprintf(os.Stderr, "  timestamp:  %d\n", s.Timestamp)
	fmt.Fprintf(os.Stderr, "  rows:\n")
	for _, table := range store.AllTableNames() {
		fmt.Fprintf(os.Stderr, "    %-28s %d\n", table+":", s.Rows[table])
	}
}
