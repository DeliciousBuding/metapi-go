package store

import (
	"fmt"
	"log/slog"

	"github.com/jmoiron/sqlx"

	// Register pure-Go SQLite driver (no CGO).
	_ "modernc.org/sqlite"

	// Register PostgreSQL driver.
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Dialect name constants.
const (
	DialectSQLite   = "sqlite"
	DialectPostgres = "postgres"
)

// Open opens a database connection pool for the given dialect and DSN.
// - "sqlite" uses modernc.org/sqlite (pure Go, no CGO).
// - "postgres" uses jackc/pgx/v5.
//
// SQLite connections are configured with WAL journal mode and foreign_keys=ON.
func Open(dialect string, dsn string, sslMode bool) (*DB, error) {
	mode := ""
	if sslMode {
		mode = "require"
	}
	return OpenWithPostgresSSLMode(dialect, dsn, mode)
}

// OpenWithPostgresSSLMode opens a database connection pool and, for PostgreSQL,
// applies an explicit sslmode without duplicating existing DSN parameters.
func OpenWithPostgresSSLMode(dialect string, dsn string, sslMode string) (*DB, error) {
	return OpenWithPostgresSSLModeAndPool(dialect, dsn, sslMode, DefaultPostgresPoolConfig())
}

// OpenWithPostgresSSLModeAndPool opens a database connection pool and applies
// an explicit PostgreSQL budget. SQLite ignores the PostgreSQL pool settings.
func OpenWithPostgresSSLModeAndPool(
	dialect string,
	dsn string,
	sslMode string,
	pool PostgresPoolConfig,
) (*DB, error) {
	var driverName string
	var connStr string

	switch dialect {
	case DialectSQLite:
		driverName = "sqlite"
		connStr = dsn
	case DialectPostgres:
		driverName = "pgx"
		normalizedSSLMode := normalizePostgresSSLMode(sslMode)
		if sslMode != "" && normalizedSSLMode == "" {
			return nil, fmt.Errorf("store: unsupported postgres sslmode %q", sslMode)
		}
		connStr = applyPostgresSSLMode(dsn, normalizedSSLMode)
		connStr = applyPostgresApplicationName(connStr, pool.ApplicationName)
	default:
		return nil, fmt.Errorf("store: unsupported dialect %q (expected %q or %q)", dialect, DialectSQLite, DialectPostgres)
	}

	// Bind the pgx driver to DOLLAR ($1, $2,...) so that all "?"
	// placeholders are automatically rebound for PostgreSQL.
	// Must be called BEFORE sqlx.Open so the DB's BindType is set correctly.
	if driverName == "pgx" {
		sqlx.BindDriver("pgx", sqlx.DOLLAR)
	}

	sqldb, err := sqlx.Open(driverName, connStr)
	if err != nil {
		return nil, fmt.Errorf("store: failed to open %s database: %w", dialect, err)
	}

	db := &DB{DB: sqldb, Dialect: dialect}

	// Configure driver-specific settings.
	switch dialect {
	case DialectSQLite:
		db.SetMaxOpenConns(1) // SQLite :memory: databases are per-connection
		if err := applySQLitePragmas(db); err != nil {
			db.Close()
			return nil, fmt.Errorf("store: failed to apply SQLite pragmas: %w", err)
		}
	case DialectPostgres:
		if err := configurePostgresPool(db, pool); err != nil {
			db.Close()
			return nil, fmt.Errorf("store: failed to configure Postgres pool: %w", err)
		}
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: failed to ping %s database: %w", dialect, err)
	}

	slog.Info("store: database opened", "dialect", dialect)
	return db, nil
}
