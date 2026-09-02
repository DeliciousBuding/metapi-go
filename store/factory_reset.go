package store

import (
	"database/sql"
	"fmt"
	"log/slog"
)

// FactoryReset wipes every business table and restarts the auto-increment
// sequences, which is what the admin "restore factory settings" operation
// promises. The table set is FactoryResetTableNames(): derived from the schema
// registry (the same single source of truth AutoMigrate and cmd/migrate use)
// minus an explicit exclusion map, so a table added to the schema is wiped by
// a factory reset until someone deliberately excludes it with a reason.
//
// admin_sessions is part of the set on purpose. Session validation reads the
// table on every authenticated request, so wiping it invalidates every cookie
// issued before the reset — without it, an operator who reset the system left
// pre-reset admin sessions able to write to an otherwise empty database.
//
// It returns the per-table row count deleted, for the API response and audit
// log. Deletion happens in one transaction in FK-safe order (children before
// parents) so a failure leaves the database untouched rather than half wiped.
func FactoryReset(db *DB) (map[string]int64, error) {
	if db == nil {
		return nil, fmt.Errorf("store: FactoryReset: nil database")
	}
	if err := schemaRegistryErr(); err != nil {
		return nil, err
	}

	tables := FactoryResetTableNames()
	tx, err := db.Begin()
	if err != nil {
		return nil, fmt.Errorf("store: factory reset begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	deleted := make(map[string]int64, len(tables))
	for _, table := range tables {
		var count int64
		if err := tx.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM "%s"`, table)).Scan(&count); err != nil {
			return nil, fmt.Errorf("store: factory reset count %s: %w", table, err)
		}
		if _, err := tx.Exec(fmt.Sprintf(`DELETE FROM "%s"`, table)); err != nil {
			return nil, fmt.Errorf("store: factory reset delete %s: %w", table, err)
		}
		deleted[table] = count
	}

	if err := resetSequences(db, tx); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("store: factory reset commit: %w", err)
	}
	slog.Info("store: factory reset complete", "tables", len(deleted), "dialect", db.Dialect)
	return deleted, nil
}

// resetSequences restarts auto-increment bookkeeping so the first post-reset
// insert gets id 1 again. Everything runs through the caller's transaction:
// on SQLite the pool holds a single connection, so touching the *DB handle
// while a transaction is open would wait for a connection the transaction
// itself owns.
//
// SQLite keeps the counters in sqlite_sequence, which only exists once an
// AUTOINCREMENT table has taken a row — hence the existence probe (a factory
// reset on a never-used database must not fail). PostgreSQL counters are
// per-table and ALTER SEQUENCE IF EXISTS is a no-op for the text-PK tables
// that have none.
func resetSequences(db *DB, tx *sql.Tx) error {
	if db.Dialect == DialectPostgres {
		for _, table := range FactoryResetTableNames() {
			if !tableHasSerialID(table) {
				continue
			}
			q := fmt.Sprintf(`ALTER SEQUENCE IF EXISTS %s_id_seq RESTART WITH 1`, table)
			if _, err := tx.Exec(q); err != nil {
				return fmt.Errorf("store: factory reset sequence %s: %w", table, err)
			}
		}
		return nil
	}

	var exists int
	if err := tx.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'sqlite_sequence'`,
	).Scan(&exists); err != nil {
		return fmt.Errorf("store: factory reset probe sqlite_sequence: %w", err)
	}
	if exists == 0 {
		return nil
	}
	if _, err := tx.Exec(`DELETE FROM sqlite_sequence`); err != nil {
		return fmt.Errorf("store: factory reset sqlite_sequence: %w", err)
	}
	return nil
}
