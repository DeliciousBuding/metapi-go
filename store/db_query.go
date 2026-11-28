package store

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"
)

// DB wraps a *sqlx.DB connection pool and provides convenience methods.
// Query helpers override sqlx.DB methods to rebind ? placeholders to $N for
// PostgreSQL before queries reach pgx.
type DB struct {
	*sqlx.DB
	Dialect string
}

// Exec executes a query that does not return rows. For PostgreSQL, the query
// string is rebound from ? to $N placeholders before execution.
func (db *DB) Exec(query string, args ...any) (sql.Result, error) {
	if db.Dialect == DialectPostgres {
		query = db.Rebind(query)
	}
	return db.DB.Exec(query, args...)
}

// Query executes a query that returns rows. For PostgreSQL, the query string
// is rebound from ? to $N placeholders before execution.
func (db *DB) Query(query string, args ...any) (*sql.Rows, error) {
	if db.Dialect == DialectPostgres {
		query = db.Rebind(query)
	}
	return db.DB.Query(query, args...)
}

// QueryRow executes a query that returns at most one row. For PostgreSQL, the
// query string is rebound from ? to $N placeholders before execution.
func (db *DB) QueryRow(query string, args ...any) *sql.Row {
	if db.Dialect == DialectPostgres {
		query = db.Rebind(query)
	}
	return db.DB.QueryRow(query, args...)
}

// Queryx executes a query and returns sqlx rows. For PostgreSQL, the query
// string is rebound from ? to $N placeholders before execution.
func (db *DB) Queryx(query string, args ...any) (*sqlx.Rows, error) {
	if db.Dialect == DialectPostgres {
		query = db.Rebind(query)
	}
	return db.DB.Queryx(query, args...)
}

// QueryRowx executes a query and returns an sqlx row. For PostgreSQL, the query
// string is rebound from ? to $N placeholders before execution.
func (db *DB) QueryRowx(query string, args ...any) *sqlx.Row {
	if db.Dialect == DialectPostgres {
		query = db.Rebind(query)
	}
	return db.DB.QueryRowx(query, args...)
}

// Get loads one row into dest. For PostgreSQL, the query string is rebound
// from ? to $N placeholders before execution.
func (db *DB) Get(dest any, query string, args ...any) error {
	if db.Dialect == DialectPostgres {
		query = db.Rebind(query)
	}
	return db.DB.Get(dest, query, args...)
}

// Select loads all rows into dest. For PostgreSQL, the query string is rebound
// from ? to $N placeholders before execution.
func (db *DB) Select(dest any, query string, args ...any) error {
	if db.Dialect == DialectPostgres {
		query = db.Rebind(query)
	}
	return db.DB.Select(dest, query, args...)
}

// ExecContext executes a query that does not return rows, honoring the request
// context. For PostgreSQL, the query string is rebound from ? to $N
// placeholders before execution so callers never branch on dialect.
func (db *DB) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	if db.Dialect == DialectPostgres {
		query = db.Rebind(query)
	}
	return db.DB.ExecContext(ctx, query, args...)
}

// QueryxContext executes a query and returns sqlx rows, honoring the request
// context. For PostgreSQL, the query string is rebound from ? to $N
// placeholders before execution.
func (db *DB) QueryxContext(ctx context.Context, query string, args ...any) (*sqlx.Rows, error) {
	if db.Dialect == DialectPostgres {
		query = db.Rebind(query)
	}
	return db.DB.QueryxContext(ctx, query, args...)
}

// QueryRowxContext executes a query and returns an sqlx row, honoring the
// request context. For PostgreSQL, the query string is rebound from ? to $N
// placeholders before execution.
func (db *DB) QueryRowxContext(ctx context.Context, query string, args ...any) *sqlx.Row {
	if db.Dialect == DialectPostgres {
		query = db.Rebind(query)
	}
	return db.DB.QueryRowxContext(ctx, query, args...)
}

// GetContext loads one row into dest, honoring the request context. For
// PostgreSQL, the query string is rebound from ? to $N placeholders before
// execution so callers never branch on dialect.
func (db *DB) GetContext(ctx context.Context, dest any, query string, args ...any) error {
	if db.Dialect == DialectPostgres {
		query = db.Rebind(query)
	}
	return db.DB.GetContext(ctx, dest, query, args...)
}

// SelectContext loads all rows into dest, honoring the request context. For
// PostgreSQL, the query string is rebound from ? to $N placeholders before
// execution so callers never branch on dialect.
func (db *DB) SelectContext(ctx context.Context, dest any, query string, args ...any) error {
	if db.Dialect == DialectPostgres {
		query = db.Rebind(query)
	}
	return db.DB.SelectContext(ctx, dest, query, args...)
}
