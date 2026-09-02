package store

import (
	"fmt"
	"log/slog"
	"time"
)

// AutoMigrate creates every table in the schema registry (store/tablesets.go)
// with indexes, unique constraints, foreign keys, and check constraints. Uses
// CREATE TABLE IF NOT EXISTS for idempotency. After the base bootstrap it runs
// the additive enterprise steps (schema_migrations bookkeeping + ordered ALTER
// TABLE upgrades) and logs a one-line summary when legacy-schema convergence
// actually happened. Run on startup after Open().
func AutoMigrate(db *DB) error {
	dialect := db.Dialect
	slog.Info("store: running auto-migration", "dialect", dialect)

	if err := schemaRegistryErr(); err != nil {
		return err
	}

	// Tables first, then indexes: CREATE INDEX needs its table to exist.
	for _, t := range schemaTables {
		if _, err := db.Exec(t.build(dialect)); err != nil {
			return fmt.Errorf("store: migrate %s: %w", t.name, err)
		}
	}

	// Non-UNIQUE indexes are created separately via CREATE INDEX IF NOT EXISTS
	// for both SQLite and PostgreSQL. SQLite inline CREATE TABLE only supports
	// PRIMARY KEY and UNIQUE constraints, not plain indexes.
	for _, m := range buildIndexes() {
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

	// TS-era timestamp sweep (formerly the journal-gated step
	// sc2_029_ts_timestamp_normalization). It runs on EVERY boot, unconditionally:
	// a gate that records "applied" cannot be correct for data that arrives
	// after the gate. Two real paths produce TS-shaped 'YYYY-MM-DD HH:MM:SS'
	// values after the first AutoMigrate — cmd/migrate copies them into a
	// target that was already migrated (see RunMigration's post-verify
	// normalize), and TS-era column defaults (datetime('now')) keep emitting
	// them on a takeover database. The rewrite is LIKE-guarded and idempotent,
	// so a steady-state boot matches zero rows and costs one scan per TEXT
	// *_at/*_until column.
	// Unconditional (not journal-gated): the journal can only record the state
	// of the database at the moment the step ran, so a gated sweep silently
	// leaves behind every TS-shaped value written later — by a takeover copy, a
	// backup import or a hand edit. The cost is one scan per TEXT *_at column
	// on every boot, which is why the elapsed time is reported when it is slow
	// even though it rewrote nothing: a slow start should be attributable.
	sweepStartedAt := time.Now()
	if n, err := normalizeLegacyTimestamps(db); err != nil {
		return fmt.Errorf("store: normalize legacy timestamps: %w", err)
	} else if n > 0 {
		slog.Info("store: normalized legacy TS timestamps",
			"rows", n, "dialect", dialect, "elapsed_ms", time.Since(sweepStartedAt).Milliseconds())
	} else if elapsed := time.Since(sweepStartedAt); elapsed > time.Second {
		slog.Warn("store: legacy TS timestamp sweep rewrote nothing but scanned for a while",
			"dialect", dialect, "elapsed_ms", elapsed.Milliseconds())
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
