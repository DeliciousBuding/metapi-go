// ts_schema_detect.go implements startup-time reverse-drift detection for the
// TS→Go migration: a SQLite database produced by a NEWER TypeScript build may
// carry columns (or __drizzle_migrations journal entries) this Go build has
// never seen. Without detection the old binary boots fine and only crashes
// later, at SELECT time, with "no such column" (the hb0730 failure mode).
//
// Detection is deliberately warn-only. Unknown columns may be harmless, and
// blocking startup would brick the older binary against any future TS
// migration — the Go server must keep serving even when the TS side has moved
// ahead.
package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

// knownLatestTSMigrationWhen is the `when` (epoch ms) of the newest entry in
// the TypeScript drizzle migration journal (metapi-ts/drizzle/meta/_journal.json)
// at the time this Go build was written. A database whose __drizzle_migrations
// max(created_at) exceeds this value was produced by a newer TypeScript build.
// When the TS side adds migrations, bump this constant (and keep an eye on the
// reverse-migration column coverage).
const knownLatestTSMigrationWhen int64 = 1776944000000

// tsSchemaDriftResult describes what a reverse-drift scan found.
type tsSchemaDriftResult struct {
	// TSJournalNewer reports whether __drizzle_migrations carries a
	// created_at newer than knownLatestTSMigrationWhen.
	TSJournalNewer bool
	// TSJournalMaxWhen is max(__drizzle_migrations.created_at); 0 when the
	// journal is absent or empty.
	TSJournalMaxWhen int64
	// UnknownColumns lists table.column entries present in the database but
	// absent from the Go authority schema, sorted.
	UnknownColumns []string
}

// warnTSSchemaDrift runs the SQLite reverse-drift scan and emits Chinese
// startup warnings. It is a no-op for non-SQLite databases and never blocks
// startup: detection errors degrade to a warning of their own.
func warnTSSchemaDrift(db *DB) {
	if db == nil || db.Dialect != DialectSQLite {
		return
	}
	result, err := detectTSSchemaDrift(db)
	if err != nil {
		slog.Warn("store: TS 反向漂移检测失败，跳过", "error", err)
		return
	}
	if result == nil {
		return
	}
	if result.TSJournalNewer {
		slog.Warn("store: 数据库由更新版本的 TypeScript 创建，请升级 Metapi Go",
			"ts_migration_when", result.TSJournalMaxWhen,
			"known_latest_when", knownLatestTSMigrationWhen)
	}
	if len(result.UnknownColumns) > 0 {
		slog.Warn("store: 检测到 Go 不认识的列（可能由更新版本的 TypeScript 写入，旧版本会忽略这些列直到查询报错），建议升级 Metapi Go",
			"unknown_columns", strings.Join(result.UnknownColumns, ", "))
	}
}

// detectTSSchemaDrift inspects a SQLite database for two signs that it was
// created by a newer TypeScript build:
//
//  1. __drizzle_migrations (the TS migration journal marker) carries a
//     created_at newer than knownLatestTSMigrationWhen.
//  2. Known tables carry columns the Go authority schema does not have.
//
// The Go authority is built at scan time from a fresh in-memory SQLite
// database (Open + AutoMigrate + PRAGMA table_info), so future Go DDL changes
// are picked up automatically without a hand-maintained column list.
// Returns (nil, nil) for nil or non-SQLite databases.
func detectTSSchemaDrift(db *DB) (*tsSchemaDriftResult, error) {
	if db == nil || db.Dialect != DialectSQLite {
		return nil, nil
	}
	result := &tsSchemaDriftResult{}

	// 1. TS journal age.
	hasJournal, err := sqliteTableExists(db, "__drizzle_migrations")
	if err != nil {
		return nil, err
	}
	if hasJournal {
		var maxWhen sql.NullFloat64
		if err := db.QueryRow(`SELECT MAX(created_at) FROM "__drizzle_migrations"`).Scan(&maxWhen); err != nil {
			return nil, fmt.Errorf("store: read __drizzle_migrations age: %w", err)
		}
		if maxWhen.Valid {
			result.TSJournalMaxWhen = int64(maxWhen.Float64)
			result.TSJournalNewer = result.TSJournalMaxWhen > knownLatestTSMigrationWhen
		}
	}

	// 2. Unknown-column scan against the in-memory authority schema.
	authority, err := buildAuthorityColumnMap()
	if err != nil {
		return nil, err
	}
	tables := make([]string, 0, len(authority))
	for table := range authority {
		tables = append(tables, table)
	}
	sort.Strings(tables)

	for _, table := range tables {
		actual, err := sqliteTableColumns(db, table)
		if err != nil {
			return nil, err
		}
		expected := authority[table]
		for _, column := range actual {
			if !expected[column] {
				result.UnknownColumns = append(result.UnknownColumns, table+"."+column)
			}
		}
	}
	sort.Strings(result.UnknownColumns)
	return result, nil
}

// buildAuthorityColumnMap builds the "Go knows these columns" authority from
// a throwaway in-memory SQLite database: Open + AutoMigrate, then PRAGMA
// table_info per user table. SQLite internal tables (sqlite_*) and the TS
// journal (__drizzle_migrations) are excluded.
func buildAuthorityColumnMap() (map[string]map[string]bool, error) {
	memDB, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		return nil, fmt.Errorf("store: open authority schema db: %w", err)
	}
	defer memDB.Close()

	if err := AutoMigrate(memDB); err != nil {
		return nil, fmt.Errorf("store: build authority schema: %w", err)
	}

	tables, err := sqliteUserTables(memDB)
	if err != nil {
		return nil, err
	}
	authority := make(map[string]map[string]bool, len(tables))
	for _, table := range tables {
		columns, err := sqliteTableColumns(memDB, table)
		if err != nil {
			return nil, err
		}
		set := make(map[string]bool, len(columns))
		for _, column := range columns {
			set[column] = true
		}
		authority[table] = set
	}
	return authority, nil
}

// sqliteTableExists reports whether a table exists in sqlite_master.
func sqliteTableExists(db *DB, table string) (bool, error) {
	var found int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table,
	).Scan(&found); err != nil {
		return false, fmt.Errorf("store: probe sqlite_master for %s: %w", table, err)
	}
	return found > 0, nil
}

// sqliteUserTables lists non-internal tables: sqlite_* internals and the TS
// journal marker are excluded.
func sqliteUserTables(db *DB) ([]string, error) {
	rows, err := db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list sqlite tables: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("store: scan sqlite table name: %w", err)
		}
		if strings.HasPrefix(name, "sqlite_") || name == "__drizzle_migrations" {
			continue
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate sqlite tables: %w", err)
	}
	return tables, nil
}

// sqliteTableColumns returns a table's column names via PRAGMA table_info.
// Unknown tables yield an empty list (PRAGMA returns no rows).
func sqliteTableColumns(db *DB, table string) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, quoteIdentSQLite(table)))
	if err != nil {
		return nil, fmt.Errorf("store: table_info(%s): %w", table, err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var dflt *string
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return nil, fmt.Errorf("store: scan table_info(%s): %w", table, err)
		}
		columns = append(columns, name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate table_info(%s): %w", table, err)
	}
	return columns, nil
}
