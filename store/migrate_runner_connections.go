package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"strings"
)

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
