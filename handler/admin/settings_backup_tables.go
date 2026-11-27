package admin

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/deliciousbuding/metapi-go/service"
	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
)

// allTables is the backup export/import table list (service/backup.AllTables),
// derived from the store schema registry minus store.BackupExcludedTables() —
// the same single source of truth the factory-reset set comes from, so a table
// added to the schema is exported and re-imported without editing a list here.
// An excluded table is refused as unknown on import, which is what makes the
// exclusion enforceable rather than advisory.
var allTables = backupsvc.AllTables

// isKnownTable checks if a table name is in the known list.
func isKnownTable(name string) bool {
	return backupsvc.IsKnownTable(name)
}

// decodeBackupImportBodyFrom decodes a backup import request body and returns
// the tables map. Accepts both shapes:
// - {"tables": {...}} (backend/webdav canonical)
// - {"data": {"tables": {...}}} (legacy frontend wrapper — api.importBackup
// sends JSON.stringify({data}) around the pasted export which itself is
// {"tables":...})

// Normalizing here fixes the manual JSON-import path that previously always
// 400'd because the top-level key was "data" not "tables".
func decodeBackupImportBodyFrom(raw []byte) (map[string]json.RawMessage, error) {
	var body struct {
		Tables map[string]json.RawMessage `json:"tables"`
		Data   *struct {
			Tables map[string]json.RawMessage `json:"tables"`
		} `json:"data"`
	}
	if err := decodeBackupPayload(raw, &body); err != nil {
		return nil, err
	}
	if body.Tables != nil {
		return body.Tables, nil
	}
	if body.Data != nil && body.Data.Tables != nil {
		return body.Data.Tables, nil
	}
	return nil, fmt.Errorf("invalid import data: expected a JSON object with a tables field")
}

// backupProvenance is the metadata block every real export writes
// (service/backup.BuildPayload, itself gated by
// TestBuildPayloadMetadataListsExcludedTables).
type backupProvenance struct {
	ExportedAt string `json:"exported_at"`
	Version    string `json:"version"`
}

func (p backupProvenance) present() bool { return p.ExportedAt != "" || p.Version != "" }

// backupPayloadHasProvenance reports whether a payload came from an actual
// export rather than being written by hand. Both accepted request shapes are
// probed: {"tables":...} and the legacy frontend wrapper {"data":{"tables":...}}.
func backupPayloadHasProvenance(raw []byte) bool {
	var probe struct {
		Metadata backupProvenance `json:"metadata"`
		Data     struct {
			Metadata backupProvenance `json:"metadata"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &probe) != nil {
		return false
	}
	return probe.Metadata.present() || probe.Data.Metadata.present()
}

// validateBackupImportTableKeys decides which payload keys this build refuses,
// and returns the ones it skips.
//
// An unrecognized key has two very different causes, told apart by where the
// payload came from:
//
//   - The table exists but is deliberately excluded from backups
//     (store.BackupExcludedTables: admin_sessions, admin_audit_logs, ...).
//     Refusing it loudly is what stops a hand-written payload from smuggling
//     session credentials or another deployment's audit trail past the
//     exclusion, so that rejection is a 400 whatever the provenance. The one
//     exception is downstream_quota_reservations in a genuine export: backups
//     written before it was excluded still carry this transient state and are
//     skipped (and reported) instead of becoming unrestorable.
//   - The payload is JSON somebody typed or pasted (no export metadata). An
//     unknown key is then most likely a typo -- "settings_typo" instead of
//     "settings" -- and answering 200 with nothing imported would hide it, so
//     that stays a 400 as well.
//   - The payload is a genuine export from an older build, so an unrecognized
//     key names a table that has since been deleted. Refusing those would make
//     every backup taken before a table deletion unrestorable after the
//     upgrade, so they are skipped and reported instead.
//
// Skipping is safe because both the import and the preview loop iterate the
// registry and look payload keys up in it -- an unrecognized key is never read,
// let alone written. The returned names are sorted so the response and the
// tests are deterministic.
func validateBackupImportTableKeys(tables map[string]json.RawMessage, fromExport bool) ([]string, error) {
	excluded := store.BackupExcludedTables()
	var ignored []string
	for table := range tables {
		if isKnownTable(table) {
			continue
		}
		_, isExcluded := excluded[table]
		// A genuine export written before downstream_quota_reservations was
		// excluded still carries its in-flight admission holds. Skipping and
		// reporting that one transient table keeps the backup restorable: the
		// holds are TTL-bound state owned by the source process and are never
		// meaningful on the target. A hand-written payload still fails, and
		// every other excluded table (session credentials, audit history, ...)
		// keeps the reject-never-skip rule below.
		if table == "downstream_quota_reservations" && fromExport {
			ignored = append(ignored, table)
			continue
		}
		if isExcluded || !fromExport {
			return nil, backupImportClientError{message: fmt.Sprintf("import failed: unknown table %s", table)}
		}
		ignored = append(ignored, table)
	}
	sort.Strings(ignored)
	return ignored, nil
}

func importBackupTablesWithConn(conn backupImportConn, tables map[string]json.RawMessage) (*backupImportResult, error) {
	result := &backupImportResult{imported: map[string]int64{}}
	for _, table := range allTables {
		raw, ok := tables[table]
		if !ok {
			continue
		}

		var rows []map[string]any
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, backupImportClientError{message: fmt.Sprintf("import failed: table %s has invalid data: %v", table, err)}
		}
		if backupImportMaxRowsPerTable > 0 && len(rows) > backupImportMaxRowsPerTable {
			return nil, backupImportClientError{
				message: fmt.Sprintf("import failed: table %s exceeds the max rows of %d", table, backupImportMaxRowsPerTable),
			}
		}
		// SECURITY: site/endpoint rows carry server-outbound target URLs.
		// Enforce the same metadata/link-local guard the single-row create
		// paths apply, so a crafted backup cannot plant SSRF targets.
		if kept, dropped := service.SanitizeImportedSiteRows(table, rows); dropped > 0 {
			rows = kept
			result.droppedForbiddenURLs += int64(dropped)
		}
		if len(rows) == 0 {
			continue
		}

		// SECURITY: downstream_api_keys rows are usable credentials. Snapshot
		// which payload keys already exist before inserting so the entries
		// this import actually admits can be reported (by name only) in the
		// response and the events audit trail.
		var downstreamCandidates []string
		var downstreamBefore map[string]bool
		if table == "downstream_api_keys" {
			downstreamCandidates = downstreamKeyValues(rows)
			downstreamBefore = queryExistingDownstreamKeysOnConn(conn, downstreamCandidates)
		}

		count, skipped, err := importTableRowsWithConn(conn, table, rows)
		if err != nil {
			return nil, fmt.Errorf("import failed: table %s: %w", table, err)
		}
		result.imported[table] = count
		result.skippedSettings += skipped

		if table == "downstream_api_keys" && len(downstreamCandidates) > 0 {
			downstreamAfter := queryExistingDownstreamKeysOnConn(conn, downstreamCandidates)
			result.newDownstreamApiKeys = downstreamNamesForNewKeys(rows, downstreamCandidates, downstreamBefore, downstreamAfter)
		}
	}

	// Rows were inserted with the ids they were exported with, which leaves
	// PostgreSQL's serial sequences pointing below them: the next ordinary INSERT
	// asks for an id the restore just occupied and fails on a unique constraint,
	// naming a key rather than a sequence so nothing points back here. A restored
	// deployment that cannot perform its first write is not a working restore, so
	// this is fatal rather than a warning -- the schema was just migrated, which
	// is the one case where a missing table cannot explain the failure. The
	// migrator resyncs through the same helper for the same reason; SQLite needs
	// nothing (see store.ResyncPGIDSequences).
	if conn.DriverName() == "pgx" {
		if err := store.ResyncPGIDSequences(conn); err != nil {
			return nil, fmt.Errorf("import failed: resync id sequences: %w", err)
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

// queryExistingDownstreamKeysOnConn returns which of keys already exist in
// downstream_api_keys, chunked to stay under SQL variable limits.
func queryExistingDownstreamKeysOnConn(conn backupImportConn, keys []string) map[string]bool {
	out := map[string]bool{}
	if len(keys) == 0 {
		return out
	}
	pgStyle := false
	switch conn.DriverName() {
	case "pgx", "postgres":
		pgStyle = true
	}
	const chunkSize = 500
	for start := 0; start < len(keys); start += chunkSize {
		end := start + chunkSize
		if end > len(keys) {
			end = len(keys)
		}
		chunk := keys[start:end]
		placeholders := make([]string, len(chunk))
		args := make([]any, len(chunk))
		for i, value := range chunk {
			if pgStyle {
				placeholders[i] = fmt.Sprintf("$%d", i+1)
			} else {
				placeholders[i] = "?"
			}
			args[i] = value
		}
		query := fmt.Sprintf(`SELECT "key" FROM "downstream_api_keys" WHERE "key" IN (%s)`,
			strings.Join(placeholders, ","))
		rows, err := conn.Queryx(query, args...)
		if err != nil {
			// Best-effort reporting: a read failure degrades to "no existing
			// keys known" rather than blocking the import itself.
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

// importTableRowsWithConn inserts rows into a table using a shared connection,
// skipping conflicts on primary key. Backup imports pass a shared transaction.
// It returns the number of rows inserted and the number of rows skipped by
// policy (runtime-local settings keys).
func importTableRowsWithConn(conn backupImportConn, table string, rows []map[string]any) (int64, int64, error) {
	if !isKnownTable(table) {
		return 0, 0, fmt.Errorf("unknown table: %s", table)
	}
	if len(rows) == 0 {
		return 0, 0, nil
	}
	knownColumns, boolColumns, err := tableColumns(conn, table)
	if err != nil {
		return 0, 0, err
	}

	var imported int64
	var skipped int64
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		if backupImportMaxColumnsPerRow > 0 && len(row) > backupImportMaxColumnsPerRow {
			return 0, 0, backupImportClientError{
				message: fmt.Sprintf("row has %d columns, exceeds limit %d", len(row), backupImportMaxColumnsPerRow),
			}
		}
		if shouldSkipBackupImportRow(table, row) {
			skipped++
			continue
		}
		if err := validateBackupImportRow(table, row); err != nil {
			return 0, 0, err
		}

		columns := make([]string, 0, len(row))
		values := make([]any, 0, len(row))

		for col, val := range row {
			if !knownColumns[col] {
				return 0, 0, fmt.Errorf("unknown column %q for table %s", col, table)
			}
			if err := validateBackupImportCellValue(col, val); err != nil {
				return 0, 0, err
			}
			if boolColumns[col] {
				coerced, err := coerceBackupBoolCellValue(col, val)
				if err != nil {
					return 0, 0, err
				}
				val = coerced
			}
			columns = append(columns, col)
			values = append(values, val)
		}

		// Build dialect-aware INSERT with correct placeholders.
		var query string
		driverName := conn.DriverName()
		switch driverName {
		case "pgx", "postgres":
			placeholders := make([]string, 0, len(columns))
			for i := range columns {
				placeholders = append(placeholders, fmt.Sprintf("$%d", i+1))
			}
			query = fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) ON CONFLICT DO NOTHING",
				quoteIdentifier(table),
				strings.Join(quoteIdentifiers(columns), ", "),
				strings.Join(placeholders, ", "),
			)
		default: // sqlite, sqlite3
			placeholders := make([]string, 0, len(columns))
			for i := 0; i < len(columns); i++ {
				placeholders = append(placeholders, "?")
			}
			query = fmt.Sprintf("INSERT OR IGNORE INTO %s (%s) VALUES (%s)",
				quoteIdentifier(table),
				strings.Join(quoteIdentifiers(columns), ", "),
				strings.Join(placeholders, ", "),
			)
		}

		result, err := conn.Exec(query, values...)
		if err != nil {
			return 0, 0, fmt.Errorf("insert row: %w", err)
		}
		n, _ := result.RowsAffected()
		imported += n
	}

	return imported, skipped, nil
}

func tableColumns(conn backupImportConn, table string) (map[string]bool, map[string]bool, error) {
	if !isKnownTable(table) {
		return nil, nil, fmt.Errorf("unknown table: %s", table)
	}

	query := fmt.Sprintf("SELECT * FROM %s LIMIT 0", quoteIdentifier(table))
	rows, err := conn.Queryx(query)
	if err != nil {
		return nil, nil, fmt.Errorf("read table columns: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, nil, fmt.Errorf("read table columns: %w", err)
	}

	allowed := make(map[string]bool, len(cols))
	for _, col := range cols {
		allowed[col] = true
	}

	boolCols := make(map[string]bool)
	if colTypes, err := rows.ColumnTypes(); err == nil {
		for _, ct := range colTypes {
			dbType := strings.ToUpper(strings.TrimSpace(ct.DatabaseTypeName()))
			if dbType == "BOOL" || dbType == "BOOLEAN" {
				boolCols[ct.Name()] = true
			}
		}
	}
	return allowed, boolCols, nil
}

func quoteIdentifiers(columns []string) []string {
	quoted := make([]string, 0, len(columns))
	for _, col := range columns {
		quoted = append(quoted, quoteIdentifier(col))
	}
	return quoted
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
