package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// sequenceExecer is the transaction-scoped executor used by sequence repair.
// Callers must keep the transaction open until this function returns so the
// advisory lock and per-table locks cover the read/repair/commit sequence.
// The SQLite→PostgreSQL migrator and backup import both pass their open tx.
type sequenceExecer interface {
	QueryRow(query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
}

const sequenceResyncAdvisoryLockKey = "metapi:resync-pg-id-sequences"

// ResyncPGIDSequences advances every serial id sequence that is behind the
// largest id now stored, for the tables that have one. It never rewinds a
// sequence that is already ahead of MAX(id), including watermarks left by
// deleted rows or allocated ids.
//
// Bulk writers that insert explicit ids leave PostgreSQL's sequences behind: the
// migrator copying a SQLite database over, a backup import restoring rows with
// the ids they were exported with, and legacy databases whose sequence was
// already below an occupied id before this repair existed. The next ordinary
// INSERT then asks the sequence for a value the restore just occupied and fails
// with a duplicate key — a deployment in that state works until its first write,
// which is the worst moment to discover it, and the error names a constraint
// rather than a sequence so nothing points back here.
//
// SQLite needs no help: AUTOINCREMENT updates sqlite_sequence on an explicit
// insert, and a plain INTEGER PRIMARY KEY derives max+1 by itself.
//
// Table names come from the schema registry (AllTableNames filtered by
// tableHasSerialID), never from a payload, which is why they are interpolated;
// the migrator has always built this statement the same way.
//
// Failures are collected and returned together rather than aborting on the first,
// so a caller running against a possibly-incomplete schema can report every table
// it could not resync. The migrator warns and continues, because its target may
// not have run every migration; a backup import and AutoMigrate treat the same
// error as fatal, because there the schema was just migrated and a silently
// un-advanced sequence is exactly the bug this prevents.
func ResyncPGIDSequences(exec sequenceExecer) error {
	// Serialize repair runs across concurrent transactions. The lock is
	// transaction-scoped, so it is released on commit or rollback.
	if _, err := exec.Exec(`SELECT pg_advisory_xact_lock(hashtext($1::text))`, sequenceResyncAdvisoryLockKey); err != nil {
		return fmt.Errorf("acquire PostgreSQL sequence resync lock: %w", err)
	}

	var errs []error
	for _, table := range sequenceTableNames() {
		quotedTable := quoteIdentPG(table)
		// Probe without a table lock first. AutoMigrate runs on every startup
		// and most databases need no repair; locking every table would turn a
		// healthy boot into a fleet-wide write barrier.
		probeSQL := fmt.Sprintf(
			`SELECT COALESCE(MAX("id"), 0), COALESCE(pg_sequence_last_value(pg_get_serial_sequence('%s', 'id')::regclass), 0), pg_get_serial_sequence('%s', 'id') FROM %s`,
			table, table, quotedTable,
		)
		var maxID, sequenceLast int64
		var sequenceName sql.NullString
		if err := exec.QueryRow(probeSQL).Scan(&maxID, &sequenceLast, &sequenceName); err != nil {
			errs = append(errs, fmt.Errorf("table %s: probe: %w", table, err))
			continue
		}
		if !sequenceName.Valid {
			errs = append(errs, fmt.Errorf("table %s: no serial id sequence", table))
			continue
		}
		if maxID == 0 || sequenceLast >= maxID {
			continue
		}

		// SHARE ROW EXCLUSIVE conflicts with ordinary INSERT/UPDATE/DELETE.
		// The caller's transaction holds it until commit, so the MAX(id) read
		// and setval cannot race an ordinary writer. The anonymous block
		// re-reads the sequence state, so a concurrent repair is harmless.
		lockSQL := fmt.Sprintf(`LOCK TABLE %s IN SHARE ROW EXCLUSIVE MODE`, quotedTable)
		if _, err := exec.Exec(lockSQL); err != nil {
			errs = append(errs, fmt.Errorf("table %s: lock: %w", table, err))
			continue
		}
		if _, err := exec.Exec(pgSequenceRepairSQL(table)); err != nil {
			errs = append(errs, fmt.Errorf("table %s: %w", table, err))
		}
	}
	return errors.Join(errs...)
}

// pgSequenceRepairSQL repairs one table's serial sequence without ever moving
// it backwards. PostgreSQL's pg_sequence_last_value() is NULL until a sequence
// has been called, so reading the sequence relation directly is required to
// preserve a setval(..., false) watermark. The anonymous block runs under the
// caller's table lock and transaction-scoped advisory lock.
func pgSequenceRepairSQL(table string) string {
	quotedTable := quoteIdentPG(table)
	return fmt.Sprintf(`
DO $metapi_sequence_resync$
DECLARE
	seq regclass;
	last_value bigint;
	is_called boolean;
	max_id bigint;
BEGIN
	seq := pg_get_serial_sequence('%s', 'id')::regclass;
	IF seq IS NULL THEN
		RAISE EXCEPTION 'no serial id sequence for table %s';
	END IF;
	EXECUTE 'SELECT last_value, is_called FROM ' || seq::text INTO last_value, is_called;
	SELECT COALESCE(MAX("id"), 0) INTO max_id FROM %s;
	IF max_id > 0 THEN
		IF last_value < max_id THEN
			PERFORM setval(seq, max_id, true);
		ELSIF last_value = max_id AND NOT is_called THEN
			PERFORM setval(seq, last_value, true);
		END IF;
	END IF;
END
$metapi_sequence_resync$`, table, table, quotedTable)
}

// reconcilePGIDSequences is the AutoMigrate entry point. It runs the shared
// repair in one transaction so the advisory lock and table locks are held until
// every sequence has been checked.
func reconcilePGIDSequences(db *DB) error {
	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("store: begin PostgreSQL sequence reconciliation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := ResyncPGIDSequences(tx); err != nil {
		return fmt.Errorf("store: reconcile PostgreSQL id sequences: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit PostgreSQL sequence reconciliation: %w", err)
	}
	return nil
}
