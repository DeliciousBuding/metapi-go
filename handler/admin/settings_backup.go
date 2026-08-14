package admin

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
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

var allTables = backupsvc.AllTables

// reverseAllTables is allTables reversed, for DELETE in FK-safe order.
var reverseAllTables []string

func init() {
	reverseAllTables = make([]string, len(allTables))
	for i, t := range allTables {
		reverseAllTables[len(allTables)-1-i] = t
	}
}

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
func (h *backupHandler) importBackup(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBackupImportBody(r)
	if err != nil {
		status := http.StatusBadRequest
		message := "导入数据格式错误：需要 JSON 对象且包含 tables 字段"
		var tooLarge webdavImportTooLargeError
		if errors.As(err, &tooLarge) {
			status = http.StatusRequestEntityTooLarge
			message = err.Error()
		}
		writeError(w, status, message)
		return
	}

	imported, err := importBackupTables(h.db, body)
	if err != nil {
		writeError(w, backupImportErrorStatus(err), err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":  true,
		"message":  "导入完成",
		"imported": imported,
	})
}

// F1: preview backup import BEFORE committing. Reuses the
// same decode + validate path as importBackup but returns a plan — per-table
// rows to insert, rows that would be skipped (runtime-local settings), and
// rows whose PK (id, or key for settings) already exists in the target DB
// (ON CONFLICT DO NOTHING would drop them). No rows are written.
func (h *backupHandler) previewBackupImport(w http.ResponseWriter, r *http.Request) {
	body, err := decodeBackupImportBody(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "导入数据格式错误：需要 JSON 对象且包含 tables 字段")
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

// decodeBackupImportBody decodes a backup import request body and returns the
// tables map. Accepts both shapes:
// - {"tables": {...}} (backend/webdav canonical)
// - {"data": {"tables": {...}}} (legacy frontend wrapper — api.importBackup
// sends JSON.stringify({data}) around the pasted export which itself is
// {"tables":...})

// Normalizing here fixes the manual JSON-import path that previously always
// 400'd because the top-level key was "data" not "tables".
func decodeBackupImportBody(r *http.Request) (map[string]json.RawMessage, error) {
	var body struct {
		Tables map[string]json.RawMessage `json:"tables"`
		Data   *struct {
			Tables map[string]json.RawMessage `json:"tables"`
		} `json:"data"`
	}
	if err := decodeBackupImportRequest(r, &body); err != nil {
		return nil, err
	}
	if body.Tables != nil {
		return body.Tables, nil
	}
	if body.Data != nil && body.Data.Tables != nil {
		return body.Data.Tables, nil
	}
	return nil, fmt.Errorf("导入数据格式错误：需要 JSON 对象且包含 tables 字段")
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
			return nil, backupImportClientError{message: fmt.Sprintf("导入失败：表 %s 数据格式错误：%v", table, err)}
		}
		if backupImportMaxRowsPerTable > 0 && len(rows) > backupImportMaxRowsPerTable {
			return nil, backupImportClientError{
				message: fmt.Sprintf("导入失败：表 %s 行数超过上限 %d", table, backupImportMaxRowsPerTable),
			}
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

func decodeBackupImportRequest(r *http.Request, dst any) error {
	raw, err := readLimitedWebdavBody(r.Body, backupWebdavImportMaxBytes)
	if err != nil {
		return err
	}
	return decodeBackupPayload(raw, dst)
}

func importBackupTables(db *sqlx.DB, tables map[string]json.RawMessage) (map[string]int64, error) {
	if tables == nil {
		return nil, fmt.Errorf("导入数据格式错误：需要 JSON 对象且包含 tables 字段")
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

	imported, err := importBackupTablesWithConn(tx, tables)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit import tx: %w", err)
	}
	committed = true
	return imported, nil
}

func validateBackupImportTableKeys(tables map[string]json.RawMessage) error {
	for table := range tables {
		if !isKnownTable(table) {
			return backupImportClientError{message: fmt.Sprintf("导入失败：未知表 %s", table)}
		}
	}
	return nil
}

type backupImportConn interface {
	DriverName() string
	Queryx(query string, args ...any) (*sqlx.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

func importBackupTablesWithConn(conn backupImportConn, tables map[string]json.RawMessage) (map[string]int64, error) {
	imported := map[string]int64{}
	for _, table := range allTables {
		raw, ok := tables[table]
		if !ok {
			continue
		}

		var rows []map[string]any
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, backupImportClientError{message: fmt.Sprintf("导入失败：表 %s 数据格式错误：%v", table, err)}
		}
		if backupImportMaxRowsPerTable > 0 && len(rows) > backupImportMaxRowsPerTable {
			return nil, backupImportClientError{
				message: fmt.Sprintf("导入失败：表 %s 行数超过上限 %d", table, backupImportMaxRowsPerTable),
			}
		}
		if len(rows) == 0 {
			continue
		}

		count, err := importTableRowsWithConn(conn, table, rows)
		if err != nil {
			return nil, fmt.Errorf("导入失败：表 %s：%w", table, err)
		}
		imported[table] = count
	}
	return imported, nil
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
	return http.StatusInternalServerError
}

// importTableRowsWithConn inserts rows into a table using a shared connection,
// skipping conflicts on primary key. Backup imports pass a shared transaction.
func importTableRowsWithConn(conn backupImportConn, table string, rows []map[string]any) (int64, error) {
	if !isKnownTable(table) {
		return 0, fmt.Errorf("unknown table: %s", table)
	}
	if len(rows) == 0 {
		return 0, nil
	}
	knownColumns, err := tableColumns(conn, table)
	if err != nil {
		return 0, err
	}

	var imported int64
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		if backupImportMaxColumnsPerRow > 0 && len(row) > backupImportMaxColumnsPerRow {
			return 0, backupImportClientError{
				message: fmt.Sprintf("row has %d columns, exceeds limit %d", len(row), backupImportMaxColumnsPerRow),
			}
		}
		if shouldSkipBackupImportRow(table, row) {
			continue
		}

		columns := make([]string, 0, len(row))
		values := make([]any, 0, len(row))

		for col, val := range row {
			if !knownColumns[col] {
				return 0, fmt.Errorf("unknown column %q for table %s", col, table)
			}
			if err := validateBackupImportCellValue(col, val); err != nil {
				return 0, err
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
			return 0, fmt.Errorf("insert row: %w", err)
		}
		n, _ := result.RowsAffected()
		imported += n
	}

	return imported, nil
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

var runtimeLocalSettingKeys = map[string]bool{
	"auth_token":             true,
	"backup_webdav_state_v1": true,
	"db_ssl":                 true,
	"db_type":                true,
	"db_url":                 true,
}

func shouldSkipBackupImportRow(table string, row map[string]any) bool {
	if table != "settings" {
		return false
	}
	key, ok := row["key"].(string)
	if !ok {
		return false
	}
	return runtimeLocalSettingKeys[key]
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
