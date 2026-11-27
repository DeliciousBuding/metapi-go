package backup

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// TSV21ImportResult reports what a v2.1 import actually did.
type TSV21ImportResult struct {
	// Imported maps Go table names to the number of rows inserted. Rows whose
	// primary key already existed are skipped (ON CONFLICT DO NOTHING).
	Imported map[string]int64 `json:"imported"`
	// SkippedSettings counts settings rows excluded by policy.
	SkippedSettings int64 `json:"skippedSettings,omitempty"`
	// NewDownstreamApiKeys lists the names (never the key values) of
	// downstream_api_keys entries actually inserted by this import. Importing
	// an untrusted backup is equivalent to accepting the keys it carries;
	// this list (mirrored into the events audit trail) lets operators see
	// exactly which new downstream keys were admitted.
	NewDownstreamApiKeys []string `json:"newDownstreamApiKeys,omitempty"`
	Warnings             []string `json:"warnings,omitempty"`
}

// ImportTSV21 parses a v2.1 payload and imports it into db inside a single
// transaction. Duplicate handling matches the tables-path import: rows whose
// primary key already exists are skipped, nothing is overwritten.
//
// SECURITY: importing an untrusted backup is equivalent to accepting the
// downstream API keys it carries — newly admitted keys are working
// credentials for the proxy surface. They are listed by name (never by key
// value) in the result so callers can surface them in the response and audit
// trail; runtime-local settings (RuntimeLocalSettingKeys) are always skipped.
func ImportTSV21(db *store.DB, raw []byte) (*TSV21ImportResult, error) {
	parsed, err := ParseTSV21(raw)
	if err != nil {
		return nil, err
	}

	tx, err := db.Beginx()
	if err != nil {
		return nil, fmt.Errorf("begin import tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	result, err := importTSV21Parsed(tx, parsed)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit import tx: %w", err)
	}
	committed = true
	return result, nil
}

// tsV21Conn is satisfied by both *store.DB and *sqlx.Tx; every statement goes
// through Rebind so ? placeholders work on PostgreSQL as well as SQLite.
type tsV21Conn interface {
	Rebind(query string) string
	Queryx(query string, args ...any) (*sqlx.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

func importTSV21Parsed(conn tsV21Conn, parsed *TSV21Parsed) (*TSV21ImportResult, error) {
	result := &TSV21ImportResult{
		Imported:        map[string]int64{},
		SkippedSettings: parsed.SkippedSettings,
		Warnings:        parsed.Warnings,
	}
	for _, table := range parsed.TableOrder {
		// SECURITY: downstream_api_keys rows are usable credentials. Snapshot
		// which payload keys already exist before inserting so the entries
		// this import actually admits can be reported (by name only) to the
		// caller and the audit trail.
		var downstreamCandidates []string
		var downstreamBefore map[string]bool
		if table == "downstream_api_keys" {
			downstreamCandidates = downstreamKeyValues(parsed.Tables[table])
			downstreamBefore = tsV21QueryExistingPKs(conn, table, "key", downstreamCandidates)
		}

		// SECURITY: site/endpoint rows carry server-outbound target URLs.
		// Enforce the same metadata/link-local guard the single-row create
		// paths apply, so a crafted backup cannot plant SSRF targets.
		tableRows := parsed.Tables[table]
		if kept, dropped := service.SanitizeImportedSiteRows(table, tableRows); dropped > 0 {
			tableRows = kept
			result.Warnings = append(result.Warnings, fmt.Sprintf(
				"ignored %d %s row(s): forbidden target URL (cloud metadata / link-local)", dropped, table))
		}

		count, err := importTSV21TableRows(conn, table, tableRows)
		if err != nil {
			return nil, fmt.Errorf("import failed: table %s: %w", table, err)
		}
		result.Imported[table] = count

		if table == "downstream_api_keys" && len(downstreamCandidates) > 0 {
			downstreamAfter := tsV21QueryExistingPKs(conn, table, "key", downstreamCandidates)
			result.NewDownstreamApiKeys = downstreamNamesForNewKeys(parsed.Tables[table], downstreamCandidates, downstreamBefore, downstreamAfter)
		}
	}
	return result, nil
}

// downstreamKeyValues extracts the unique, payload-ordered key values from
// downstream_api_keys rows.
func downstreamKeyValues(rows []map[string]any) []string {
	seen := map[string]bool{}
	var out []string
	for _, row := range rows {
		key, ok := row["key"].(string)
		if !ok || key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

// downstreamNamesForNewKeys returns the names of the downstream_api_keys
// entries present after the import but not before, in payload order. Only
// names are returned: key values must never surface in import responses or
// audit events. The name comes from the first payload row carrying each key,
// which is the row ON CONFLICT DO NOTHING admits.
func downstreamNamesForNewKeys(rows []map[string]any, candidates []string, before, after map[string]bool) []string {
	if len(candidates) == 0 {
		return nil
	}
	namesByKey := make(map[string]string, len(candidates))
	for _, row := range rows {
		key, ok := row["key"].(string)
		if !ok || key == "" {
			continue
		}
		if _, exists := namesByKey[key]; exists {
			continue
		}
		name, _ := row["name"].(string)
		namesByKey[key] = name
	}
	var out []string
	for _, key := range candidates {
		if after[key] && !before[key] {
			out = append(out, namesByKey[key])
		}
	}
	return out
}

func importTSV21TableRows(conn tsV21Conn, table string, rows []map[string]any) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	knownColumns, err := tsV21TableColumns(conn, table)
	if err != nil {
		return 0, err
	}

	var imported int64
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		columns := make([]string, 0, len(row))
		values := make([]any, 0, len(row))
		for column, value := range row {
			if !knownColumns[column] {
				return 0, fmt.Errorf("target table %s has no column %q", table, column)
			}
			columns = append(columns, column)
			values = append(values, value)
		}

		quotedColumns := make([]string, len(columns))
		placeholders := make([]string, len(columns))
		for i, column := range columns {
			quotedColumns[i] = quoteIdentifier(column)
			placeholders[i] = "?"
		}
		query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING",
			quoteIdentifier(table),
			strings.Join(quotedColumns, ", "),
			strings.Join(placeholders, ", "),
		)
		result, err := conn.Exec(conn.Rebind(query), values...)
		if err != nil {
			return 0, fmt.Errorf("insert row: %w", err)
		}
		affected, _ := result.RowsAffected()
		imported += affected
	}
	return imported, nil
}

func tsV21TableColumns(conn tsV21Conn, table string) (map[string]bool, error) {
	query := fmt.Sprintf("SELECT * FROM %s LIMIT 0", quoteIdentifier(table))
	rows, err := conn.Queryx(conn.Rebind(query))
	if err != nil {
		return nil, fmt.Errorf("read table columns: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("read table columns: %w", err)
	}
	allowed := make(map[string]bool, len(cols))
	for _, col := range cols {
		allowed[col] = true
	}
	return allowed, nil
}

// tsV21QueryExistingPKs returns the set of PK values already present in the
// table, chunked to stay under SQL variable limits (SQLite 999 / PG 65535).
// Preview is best-effort: a read failure degrades to "no existing PKs known".
func tsV21QueryExistingPKs(conn tsV21Conn, table, pkColumn string, values []string) map[string]bool {
	out := map[string]bool{}
	if len(values) == 0 {
		return out
	}
	const chunkSize = 500
	for start := 0; start < len(values); start += chunkSize {
		end := start + chunkSize
		if end > len(values) {
			end = len(values)
		}
		chunk := values[start:end]
		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, value := range chunk {
			placeholders[i] = "?"
			args[i] = value
		}
		query := fmt.Sprintf("SELECT %s FROM %s WHERE %s IN (%s)",
			quoteIdentifier(pkColumn), quoteIdentifier(table), quoteIdentifier(pkColumn), strings.Join(placeholders, ","))
		rows, err := conn.Queryx(conn.Rebind(query), args...)
		if err != nil {
			return out
		}
		for rows.Next() {
			var value string
			if err := rows.Scan(&value); err == nil {
				out[value] = true
			}
		}
		_ = rows.Close()
	}
	return out
}
