package admin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/service"
	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterBackupRoutes registers all /api/settings/backup routes.
func RegisterBackupRoutes(r chi.Router, db *sqlx.DB) {
	handler := &backupHandler{db: db}

	r.Get("/api/settings/backup/export", handler.exportBackup)
	r.Post("/api/settings/backup/import", handler.importBackup)
	// F1: import plan preview before commit.
	r.Post("/api/settings/backup/import/preview", handler.previewBackupImport)
	r.Get("/api/settings/backup/webdav", handler.getWebdavConfig)
	r.Put("/api/settings/backup/webdav", handler.saveWebdavConfig)
	r.Post("/api/settings/backup/webdav/export", handler.exportToWebdav)
	r.Post("/api/settings/backup/webdav/import", handler.importFromWebdav)
}

type backupHandler struct {
	db *sqlx.DB
}

const (
	backupWebdavConfigSettingKey    = "backup_webdav_config_v1"
	backupWebdavStateSettingKey     = "backup_webdav_state_v1"
	backupWebdavDefaultAutoSyncCron = "0 */6 * * *"
	backupWebdavFetchTimeout        = 15 * time.Second
)

var backupWebdavImportMaxBytes int64 = 64 << 20

var allowPrivateWebdavTargets bool

var (
	backupImportMaxRowsPerTable  = 50_000
	backupImportMaxColumnsPerRow = 128
	backupImportMaxCellBytes     = 4 << 20
)

type webdavBackupConfig struct {
	Enabled         bool   `json:"enabled"`
	FileURL         string `json:"fileUrl"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	ExportType      string `json:"exportType"`
	AutoSyncEnabled bool   `json:"autoSyncEnabled"`
	AutoSyncCron    string `json:"autoSyncCron"`
}

// allTables is the backup export/import table list (service/backup.AllTables),
// derived from the store schema registry minus store.BackupExcludedTables() —
// the same single source of truth the factory-reset set comes from, so a table
// added to the schema is exported and re-imported without editing a list here.
// An excluded table is refused as unknown on import, which is what makes the
// exclusion enforceable rather than advisory.
var allTables = backupsvc.AllTables

// GET /api/settings/backup/export?type=
func (h *backupHandler) exportBackup(w http.ResponseWriter, r *http.Request) {
	exportType := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("type")))
	if exportType == "" {
		exportType = "all"
	}
	backup, err := buildBackupPayload(h.db, exportType)
	if err != nil {
		status := backupExportErrorStatus(err)
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, backup)
}

var errInvalidBackupExportType = backupsvc.ErrInvalidExportType

func backupExportErrorStatus(err error) int {
	if errors.Is(err, errInvalidBackupExportType) {
		return http.StatusBadRequest
	}
	var limitErr backupsvc.ExportLimitError
	if errors.As(err, &limitErr) {
		return http.StatusRequestEntityTooLarge
	}
	return http.StatusInternalServerError
}

func buildBackupPayload(db *sqlx.DB, exportType string) (map[string]any, error) {
	return backupsvc.BuildPayload(db, exportType)
}

// isKnownTable checks if a table name is in the known list.
func isKnownTable(name string) bool {
	return backupsvc.IsKnownTable(name)
}

// POST /api/settings/backup/import
//
// SECURITY: a backup import is semi-trusted input. Settings rows whose keys
// are bound to the current deployment (credentials, DB wiring, scheduler
// state — see backupsvc.RuntimeLocalSettingKeys) are always skipped and
// reported via skippedSettings. Importing an untrusted backup is equivalent
// to accepting the downstream API keys it carries: newly admitted keys are
// working proxy credentials, so they are listed by name (never by key value)
// in the response and in the events audit trail (newDownstreamApiKeys).
func (h *backupHandler) importBackup(w http.ResponseWriter, r *http.Request) {
	raw, err := readLimitedWebdavBody(r.Body, backupWebdavImportMaxBytes)
	if err != nil {
		status := http.StatusBadRequest
		message := "invalid import data: expected a JSON object with a tables field"
		var tooLarge webdavImportTooLargeError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
			message = err.Error()
		}
		writeError(w, status, message)
		return
	}

	// TS (cita-777/metapi) backup v2.1 payloads take the dedicated parser.
	if backupsvc.IsTSV21Payload(raw) {
		h.importTSV21Backup(w, raw)
		return
	}

	body, err := decodeBackupImportBodyFrom(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid import data: expected a JSON object with a tables field")
		return
	}

	result, err := importBackupTables(h.db, body)
	if err != nil {
		writeError(w, backupImportErrorStatus(err), err.Error())
		return
	}

	response := map[string]any{
		"success":  true,
		"message":  "import completed",
		"imported": result.imported,
	}
	if result.skippedSettings > 0 {
		response["skippedSettings"] = result.skippedSettings
	}
	if result.droppedForbiddenURLs > 0 {
		response["droppedForbiddenSiteRows"] = result.droppedForbiddenURLs
	}
	if len(result.newDownstreamApiKeys) > 0 {
		response["newDownstreamApiKeys"] = result.newDownstreamApiKeys
	}
	logImportedDownstreamKeys(h.db, result.newDownstreamApiKeys)
	writeJSON(w, http.StatusOK, response)
}

// importTSV21Backup handles the TS backup v2.1 branch of the import endpoint.
func (h *backupHandler) importTSV21Backup(w http.ResponseWriter, raw []byte) {
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid import data: %v", err))
		return
	}
	result, err := backupsvc.ImportTSV21(backupStoreDB(h.db), raw)
	if err != nil {
		writeError(w, backupImportErrorStatus(err), err.Error())
		return
	}

	response := map[string]any{
		"success":  true,
		"message":  "import completed",
		"imported": result.Imported,
	}
	if result.SkippedSettings > 0 {
		response["skippedSettings"] = result.SkippedSettings
	}
	if len(result.NewDownstreamApiKeys) > 0 {
		response["newDownstreamApiKeys"] = result.NewDownstreamApiKeys
	}
	if len(result.Warnings) > 0 {
		response["warnings"] = result.Warnings
	}
	logImportedDownstreamKeys(h.db, result.NewDownstreamApiKeys)
	writeJSON(w, http.StatusOK, response)
}

// F1: preview backup import BEFORE committing. Reuses the
// same decode + validate path as importBackup but returns a plan — per-table
// rows to insert, rows that would be skipped (runtime-local settings), and
// rows whose PK (id, or key for settings) already exists in the target DB
// (ON CONFLICT DO NOTHING would drop them). No rows are written.
func (h *backupHandler) previewBackupImport(w http.ResponseWriter, r *http.Request) {
	raw, err := readLimitedWebdavBody(r.Body, backupWebdavImportMaxBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid import data: expected a JSON object with a tables field")
		return
	}

	// TS (cita-777/metapi) backup v2.1 payloads take the dedicated parser.
	if backupsvc.IsTSV21Payload(raw) {
		if err := rejectDuplicateJSONKeys(raw); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid import data: %v", err))
			return
		}
		preview, err := backupsvc.PreviewTSV21(backupStoreDB(h.db), raw)
		if err != nil {
			writeError(w, backupImportErrorStatus(err), err.Error())
			return
		}
		response := map[string]any{
			"success": true,
			"plan":    preview.Plan,
		}
		if len(preview.Warnings) > 0 {
			response["warnings"] = preview.Warnings
		}
		writeJSON(w, http.StatusOK, response)
		return
	}

	body, err := decodeBackupImportBodyFrom(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid import data: expected a JSON object with a tables field")
		return
	}
	if err := validateBackupImportTableKeys(body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	plan, err := previewBackupImportTables(h.db, body)
	if err != nil {
		writeError(w, backupImportErrorStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"plan":    plan,
	})
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

// backupStoreDB wraps the handler's *sqlx.DB in a store.DB so v2.1 import
// helpers get automatic ? → $N rebinding on PostgreSQL.
func backupStoreDB(db *sqlx.DB) *store.DB {
	dialect := store.DialectSQLite
	if db.DriverName() == "pgx" {
		dialect = store.DialectPostgres
	}
	return &store.DB{DB: db, Dialect: dialect}
}

// backupImportPreview is the per-table preview summary.
type backupImportPreview struct {
	Rows        int64 `json:"rows"`        // total rows present in the backup for this table
	ToInsert    int64 `json:"toInsert"`    // rows that would actually be inserted
	Duplicates  int64 `json:"duplicates"`  // rows whose PK already exists (ON CONFLICT DO NOTHING drops them)
	SkippedRows int64 `json:"skippedRows"` // rows skipped by policy (runtime-local settings)
}

// previewBackupImportTables computes the import plan without writing anything.
// PK detection: "id" for every table except "settings" which uses "key".
func previewBackupImportTables(db *sqlx.DB, tables map[string]json.RawMessage) (map[string]backupImportPreview, error) {
	plan := make(map[string]backupImportPreview, len(tables))
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

		// SECURITY: mirror the import path's SSRF target guard so the plan
		// preview matches what the import will actually admit.
		if kept, dropped := service.SanitizeImportedSiteRows(table, rows); dropped > 0 {
			rows = kept
		}

		preview := backupImportPreview{Rows: int64(len(rows))}
		pkCol := "id"
		if table == "settings" {
			pkCol = "key"
		}

		// Collect candidate PK values for the exists-check. Rows without a PK
		// value can't be deduped — count them as insert immediately. Duplicate
		// PKs within the backup itself are dropped by ON CONFLICT DO NOTHING.
		existing := map[string]bool{}
		var pkVals []string
		hasPKCol := false
		for _, row := range rows {
			if shouldSkipBackupImportRow(table, row) {
				preview.SkippedRows++
				continue
			}
			rawVal, present := row[pkCol]
			if !present {
				preview.ToInsert++
				continue
			}
			hasPKCol = true
			key := fmt.Sprintf("%v", rawVal)
			if _, dup := existing[key]; dup {
				preview.Duplicates++
				continue
			}
			existing[key] = true
			pkVals = append(pkVals, key)
		}

		// Query which PKs already exist in the target table.
		if hasPKCol && len(pkVals) > 0 {
			existingInDB := queryExistingPKs(db, table, pkCol, pkVals)
			for _, key := range pkVals {
				if existingInDB[key] {
					preview.Duplicates++
				} else {
					preview.ToInsert++
				}
			}
		}
		plan[table] = preview
	}
	return plan, nil
}

// queryExistingPKs returns the set of PK values already present in the table,
// chunked to stay under SQL variable limits (SQLite 999 / PG 65535).
func queryExistingPKs(db *sqlx.DB, table, pkCol string, values []string) map[string]bool {
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
		for i, v := range chunk {
			placeholders[i] = "?"
			args[i] = v
		}
		query := fmt.Sprintf("SELECT %s FROM %s WHERE %s IN (%s)",
			quoteIdentifier(pkCol), quoteIdentifier(table), quoteIdentifier(pkCol), strings.Join(placeholders, ","))
		rows, err := db.Queryx(db.Rebind(query), args...)
		if err != nil {
			// Preview is best-effort: a read failure degrades to "no existing
			// PKs known" rather than blocking the import flow.
			return out
		}
		for rows.Next() {
			var val string
			if err := rows.Scan(&val); err == nil {
				out[val] = true
			}
		}
		_ = rows.Close()
	}
	return out
}

// backupImportResult reports what a tables-format import actually did.
type backupImportResult struct {
	// imported maps Go table names to the number of rows inserted. Rows whose
	// primary key already existed are skipped (ON CONFLICT DO NOTHING).
	imported map[string]int64
	// skippedSettings counts settings rows excluded by policy (runtime-local
	// keys from backupsvc.RuntimeLocalSettingKeys).
	skippedSettings int64
	// droppedForbiddenURLs counts site/endpoint rows rejected by the
	// SSRF target guard (cloud metadata / link-local URLs).
	droppedForbiddenURLs int64
	// newDownstreamApiKeys lists the names (never the key values) of
	// downstream_api_keys entries actually inserted by this import. Importing
	// an untrusted backup is equivalent to accepting the keys it carries; the
	// list is surfaced in the response and the events audit trail.
	newDownstreamApiKeys []string
}

func importBackupTables(db *sqlx.DB, tables map[string]json.RawMessage) (*backupImportResult, error) {
	if tables == nil {
		return nil, fmt.Errorf("invalid import data: expected a JSON object with a tables field")
	}
	if err := validateBackupImportTableKeys(tables); err != nil {
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

	result, err := importBackupTablesWithConn(tx, tables)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit import tx: %w", err)
	}
	committed = true
	return result, nil
}

func validateBackupImportTableKeys(tables map[string]json.RawMessage) error {
	for table := range tables {
		if !isKnownTable(table) {
			return backupImportClientError{message: fmt.Sprintf("import failed: unknown table %s", table)}
		}
	}
	return nil
}

type backupImportConn interface {
	DriverName() string
	Queryx(query string, args ...any) (*sqlx.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
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

// logImportedDownstreamKeys records a warning event listing the downstream
// API keys a backup import admitted. Names only — key values are never
// logged. Importing an untrusted backup is equivalent to accepting the keys
// it carries; this event gives operators a durable trail. Best-effort: a
// failed insert never blocks the import response.
func logImportedDownstreamKeys(db *sqlx.DB, names []string) {
	if len(names) == 0 {
		return
	}
	listed := names
	suffix := ""
	const maxListed = 20
	if len(listed) > maxListed {
		suffix = fmt.Sprintf(" (%d in total)", len(names))
		listed = listed[:maxListed]
	}
	message := fmt.Sprintf("New downstream API keys: %s%s", strings.Join(listed, ", "), suffix)
	now := time.Now().UTC().Format(time.RFC3339)
	// "read" is bound as a boolean parameter: a literal 0 is rejected by
	// PostgreSQL's boolean column type (matches service.CreateEvent).
	query := db.Rebind(`INSERT INTO events (type, title, message, level, related_type, created_at, "read")
		VALUES (?, ?, ?, ?, 'backup', ?, ?)`)
	if _, err := db.Exec(query, "backup", "Backup import added downstream API keys", message, "warning", now, false); err != nil {
		slog.Warn("backup import: failed to log downstream key event", "error", err)
	}
}

type backupImportClientError struct {
	message string
}

func (e backupImportClientError) Error() string {
	return e.message
}

func backupImportErrorStatus(err error) int {
	var clientErr backupImportClientError
	if errors.As(err, &clientErr) {
		return http.StatusBadRequest
	}
	var tsClientErr backupsvc.TSV21ClientError
	if errors.As(err, &tsClientErr) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
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
	knownColumns, err := tableColumns(conn, table)
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

		columns := make([]string, 0, len(row))
		values := make([]any, 0, len(row))

		for col, val := range row {
			if !knownColumns[col] {
				return 0, 0, fmt.Errorf("unknown column %q for table %s", col, table)
			}
			if err := validateBackupImportCellValue(col, val); err != nil {
				return 0, 0, err
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

func validateBackupImportCellValue(column string, value any) error {
	switch v := value.(type) {
	case nil, bool, float64:
		return nil
	case string:
		if backupImportMaxCellBytes > 0 && len(v) > backupImportMaxCellBytes {
			return backupImportClientError{
				message: fmt.Sprintf("column %q value exceeds limit %d bytes", column, backupImportMaxCellBytes),
			}
		}
		return nil
	default:
		return backupImportClientError{message: fmt.Sprintf("column %q must be a scalar JSON value", column)}
	}
}

// shouldSkipBackupImportRow reports whether a row must be dropped by policy.
// Only settings rows are filtered: keys bound to the current deployment
// (credentials, DB wiring, scheduler state) may never be planted by an
// import. The deny-list lives in backupsvc.RuntimeLocalSettingKeys so the
// tables path and the TS v2.1 path enforce the same policy.
func shouldSkipBackupImportRow(table string, row map[string]any) bool {
	if table != "settings" {
		return false
	}
	key, ok := row["key"].(string)
	if !ok {
		return false
	}
	return backupsvc.RuntimeLocalSettingKeys[key]
}

func tableColumns(conn backupImportConn, table string) (map[string]bool, error) {
	if !isKnownTable(table) {
		return nil, fmt.Errorf("unknown table: %s", table)
	}

	query := fmt.Sprintf("SELECT * FROM %s LIMIT 0", quoteIdentifier(table))
	rows, err := conn.Queryx(query)
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

// GET /api/settings/backup/webdav
