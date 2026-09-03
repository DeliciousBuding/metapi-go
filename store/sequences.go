package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// sequenceExecer is the slice of a transaction or connection that a sequence
// resync needs. Both *sqlx.Tx (the SQLite→PostgreSQL migrator) and the backup
// import's conn satisfy it, so this SQL has one owner instead of two copies that
// can drift apart the next time a table is added.
type sequenceExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// ResyncPGIDSequences advances every serial id sequence past the largest id now
// stored, for the tables that have one.
//
// Bulk writers that insert explicit ids leave PostgreSQL's sequences behind: the
// migrator copying a SQLite database over, and a backup import restoring rows
// with the ids they were exported with. The next ordinary INSERT then asks the
// sequence for a value the restore just occupied and fails with a duplicate key —
// a deployment in that state works until its first write, which is the worst
// moment to discover it, and the error names a constraint rather than a sequence
// so nothing points back here.
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
// not have run every migration; a backup import treats the same error as fatal,
// because there the schema was just migrated and a silently un-advanced sequence
// is exactly the bug this prevents.
func ResyncPGIDSequences(exec sequenceExecer) error {
	var errs []error
	for _, table := range sequenceTableNames() {
		query := fmt.Sprintf(
			`SELECT setval(pg_get_serial_sequence('%s', 'id'), COALESCE((SELECT MAX("id") FROM "%s"), 1), TRUE)`,
			table, table,
		)
		if _, err := exec.Exec(query); err != nil {
			errs = append(errs, fmt.Errorf("table %s: %w", table, err))
		}
	}
	return errors.Join(errs...)
}
