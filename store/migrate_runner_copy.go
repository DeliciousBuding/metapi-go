package store

import (
	"fmt"
	"io"
	"time"
)

func writeMigrationRows(tgtDB *DB, inserts []insertStmt, opts RunMigrationOptions, toSQLite bool, logw io.Writer) error {
	// 7. Begin transaction
	tx, err := tgtDB.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // safe no-op after commit

	// 8. Clear target data in FK-safe order (if overwrite)
	if opts.Overwrite {
		fmt.Fprintf(logw, "\nClearing target data (FK-safe order)...\n")
		for _, table := range ClearTableNames() {
			if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM "%s"`, table)); err != nil {
				return fmt.Errorf("clear %s: %w", table, err)
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
			return fmt.Errorf("insert into %s: %w", group[0].table, err)
		}
		inserted += len(group)
		if opts.Progress && (inserted%100 == 0 || inserted == len(inserts)) {
			elapsed := time.Since(start)
			fmt.Fprintf(logw, "  %d/%d rows inserted (%s elapsed)\n", inserted, len(inserts), elapsed.Round(time.Millisecond))
		}
	}

	if opts.Progress {
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
		return fmt.Errorf("commit: %w", err)
	}

	return nil
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
