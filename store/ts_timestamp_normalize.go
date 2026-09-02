package store

import (
	"fmt"
	"strings"
)

// TS-era datetime normalization (sc2_029).
//
// The TypeScript implementation (drizzle datetime('now')) wrote timestamps as
// 'YYYY-MM-DD HH:MM:SS' (space separator, no zone marker) while the Go
// implementation writes RFC3339 UTC ('YYYY-MM-DDTHH:MM:SSZ'). A takeover
// database carries both shapes in the same TEXT columns, and every
// lexicographic comparison the Go code makes is skewed by the mix: a space
// (0x20) sorts before 'T' (0x54), so TS-era rows always compare as older
// than Go-era rows regardless of their real dates. That distorts
// ORDER BY created_at listings and created_at >= ? range filters, and the
// checkin sweep (last_checkin_at < cutoff) treats every TS-era account as
// expired on first Go boot — a burst of re-checkins against upstreams.
//
// normalizeLegacyTimestamps rewrites TS-shaped values in place:
//
//	'YYYY-MM-DD HH:MM:SS[.fff]'  ->  'YYYY-MM-DDTHH:MM:SS[.fff]Z'
//
// drizzle wrote these values in UTC, so appending 'Z' is a faithful
// conversion. Values already containing 'T' (Go rows), NULLs, and
// non-matching shapes are untouched. Re-runs match zero rows (idempotent).
//
// Candidate columns are discovered by schema introspection — every TEXT
// column whose name follows the *_at / *_until convention, in every user
// table — so TS-takeover schemas get the same treatment for columns the Go
// DDL never declares. On Postgres, columns with a native timestamp type are
// skipped: they compare chronologically already.

// legacyTimestampPattern matches 'YYYY-MM-DD HH:MM:SS' with any fractional
// part; the _ wildcards pin the shape without constraining digits.
const legacyTimestampPattern = "____-__-__ __:__:__%"

type tableColumn struct {
	table  string
	column string
}

// isTimestampColumnName reports whether a column name follows the repo-wide
// timestamp convention (created_at, last_checkin_at, cooldown_until, ...).
func isTimestampColumnName(name string) bool {
	return strings.HasSuffix(name, "_at") || strings.HasSuffix(name, "_until")
}

// quoteIdentTSNorm double-quotes an identifier after validating it is
// alphanumeric/underscore. Names come from schema introspection, never from
// user input, but the validation keeps the fmt-built SQL injection-proof.
func quoteIdentTSNorm(name string) (string, bool) {
	if name == "" {
		return "", false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return "", false
	}
	return `"` + name + `"`, true
}

// legacyTimestampColumns lists (table, column) candidates for normalization.
func legacyTimestampColumns(db *DB) ([]tableColumn, error) {
	var out []tableColumn

	if db.Dialect == DialectPostgres {
		rows, err := db.Query(`
			SELECT table_name, column_name
			FROM information_schema.columns
			WHERE table_schema = current_schema()
			  AND data_type = 'text'
			  AND table_name <> 'schema_migrations'
			ORDER BY table_name, ordinal_position`)
		if err != nil {
			return nil, fmt.Errorf("store: list pg columns: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var table, column string
			if err := rows.Scan(&table, &column); err != nil {
				return nil, fmt.Errorf("store: scan pg column: %w", err)
			}
			if isTimestampColumnName(column) {
				out = append(out, tableColumn{table: table, column: column})
			}
		}
		return out, rows.Err()
	}

	// SQLite: every user table, then PRAGMA table_info for TEXT-affinity
	// columns. Declared types in the wild include TEXT, '', 'datetime'
	// (drizzle) — all TEXT affinity. INTEGER/REAL/BLOB/NUMERIC are skipped.
	tableRows, err := db.Query(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name <> 'schema_migrations'`)
	if err != nil {
		return nil, fmt.Errorf("store: list sqlite tables: %w", err)
	}
	var tables []string
	for tableRows.Next() {
		var name string
		if err := tableRows.Scan(&name); err != nil {
			tableRows.Close()
			return nil, fmt.Errorf("store: scan sqlite table: %w", err)
		}
		tables = append(tables, name)
	}
	if err := tableRows.Err(); err != nil {
		tableRows.Close()
		return nil, err
	}
	tableRows.Close()

	for _, table := range tables {
		quoted, ok := quoteIdentTSNorm(table)
		if !ok {
			continue
		}
		colRows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, quoted))
		if err != nil {
			return nil, fmt.Errorf("store: table_info(%s): %w", table, err)
		}
		for colRows.Next() {
			var cid, notNull, pk int
			var name, colType string
			var dflt *string
			if err := colRows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
				colRows.Close()
				return nil, fmt.Errorf("store: table_info(%s) scan: %w", table, err)
			}
			if !isTimestampColumnName(name) {
				continue
			}
			upper := strings.ToUpper(strings.TrimSpace(colType))
			switch {
			case upper == "", upper == "TEXT",
				strings.Contains(upper, "CHAR"),
				strings.Contains(upper, "TEXT"),
				strings.Contains(upper, "DATE"),
				strings.Contains(upper, "TIME"):
				out = append(out, tableColumn{table: table, column: name})
			}
		}
		if err := colRows.Err(); err != nil {
			colRows.Close()
			return nil, err
		}
		colRows.Close()
	}
	return out, nil
}

// normalizeLegacyTimestamps rewrites every TS-shaped timestamp value and
// returns the number of rows changed.
func normalizeLegacyTimestamps(db *DB) (int64, error) {
	candidates, err := legacyTimestampColumns(db)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, tc := range candidates {
		quotedTable, ok := quoteIdentTSNorm(tc.table)
		if !ok {
			continue
		}
		quotedColumn, ok := quoteIdentTSNorm(tc.column)
		if !ok {
			continue
		}
		// Both dialects implement replace() and || with the same semantics.
		query := fmt.Sprintf(
			`UPDATE %s SET %s = replace(%s, ' ', 'T') || 'Z' WHERE %s LIKE ?`,
			quotedTable, quotedColumn, quotedColumn, quotedColumn)
		result, err := db.Exec(db.Rebind(query), legacyTimestampPattern)
		if err != nil {
			return total, fmt.Errorf("store: normalize %s.%s: %w", tc.table, tc.column, err)
		}
		if n, err := result.RowsAffected(); err == nil {
			total += n
		}
	}
	return total, nil
}
