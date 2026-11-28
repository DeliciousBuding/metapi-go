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
)

// RegisterBackupRoutes registers all /api/settings/backup routes.
func RegisterBackupRoutes(r chi.Router, db *store.DB) {
	handler := &backupHandler{db: db}

	r.Get("/api/settings/backup/export", handler.exportBackup)
	r.Post("/api/settings/backup/import", handler.importBackup)
	// Import plan preview before commit.
	r.Post("/api/settings/backup/import/preview", handler.previewBackupImport)
	r.Get("/api/settings/backup/webdav", handler.getWebdavConfig)
	r.Put("/api/settings/backup/webdav", handler.saveWebdavConfig)
	r.Post("/api/settings/backup/webdav/export", handler.exportToWebdav)
	r.Post("/api/settings/backup/webdav/import", handler.importFromWebdav)
}

type backupHandler struct {
	db *store.DB
}

const (
	backupWebdavConfigSettingKey    = "backup_webdav_config_v1"
	backupWebdavStateSettingKey     = "backup_webdav_state_v1"
	backupWebdavDefaultAutoSyncCron = "0 */6 * * *"
	backupWebdavFetchTimeout        = 15 * time.Second
)

var backupWebdavImportMaxBytes int64 = 20 << 20

var allowPrivateWebdavTargets bool

type webdavBackupConfig struct {
	Enabled         bool   `json:"enabled"`
	FileURL         string `json:"fileUrl"`
	Username        string `json:"username"`
	Password        string `json:"password"`
	ExportType      string `json:"exportType"`
	AutoSyncEnabled bool   `json:"autoSyncEnabled"`
	AutoSyncCron    string `json:"autoSyncCron"`
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

func buildBackupPayload(db *store.DB, exportType string) (map[string]any, error) {
	return backupsvc.BuildPayload(db.DB, exportType)
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
	raw, passphrase, magpie, err := unwrapMagpieImportBody(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if magpie {
		h.importMagpieBackup(w, r, raw, passphrase)
		return
	}
	if backupsvc.IsOctopusV5Payload(raw) {
		h.importOctopusV5Backup(w, r, raw)
		return
	}
	if backupsvc.IsAxonHubV14Payload(raw) {
		h.importAxonHubV14Backup(w, r, raw)
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

	result, err := importBackupTables(h.db, body, backupPayloadHasProvenance(raw))
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
	if len(result.ignoredTables) > 0 {
		response["ignoredTables"] = result.ignoredTables
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

// Preview a backup import BEFORE committing. Reuses the
// same decode + validate path as importBackup but returns a plan — per-table
// rows to insert, rows that would be skipped (runtime-local settings), and
// rows whose PK (id, or key for settings) already exists in the target DB
// (ON CONFLICT DO NOTHING would drop them). No rows are written.
func (h *backupHandler) previewBackupImport(w http.ResponseWriter, r *http.Request) {
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
	raw, passphrase, magpie, err := unwrapMagpieImportBody(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if magpie {
		originKey := magpieOriginKey(r)
		preview, err := backupsvc.PreviewMagpieBackup(h.db, raw, passphrase, originKey)
		if err != nil {
			writeError(w, http.StatusBadRequest, magpieImportErrorMessage(err))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "plan": preview})
		return
	}
	if backupsvc.IsOctopusV5Payload(raw) {
		originKey := strings.TrimSpace(r.Header.Get("X-External-Origin-Key"))
		preview, err := backupsvc.PreviewOctopusV5(h.db, raw, originKey)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "plan": preview})
		return
	}
	if backupsvc.IsAxonHubV14Payload(raw) {
		originKey := strings.TrimSpace(r.Header.Get("X-External-Origin-Key"))
		preview, err := backupsvc.PreviewAxonHubV14(h.db, raw, originKey)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "plan": preview})
		return
	}

	// TS (cita-777/metapi) backup v2.1 payloads take the dedicated parser.
	if backupsvc.IsTSV21Payload(raw) {
		if err := rejectDuplicateJSONKeys(raw); err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid import data: %v", err))
			return
		}
		preview, err := backupsvc.PreviewTSV21(h.db, raw)
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
	ignoredTables, err := validateBackupImportTableKeys(body, backupPayloadHasProvenance(raw))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	plan, err := previewBackupImportTables(h.db, body)
	if err != nil {
		writeError(w, backupImportErrorStatus(err), err.Error())
		return
	}

	response := map[string]any{
		"success": true,
		"plan":    plan,
	}
	if len(ignoredTables) > 0 {
		response["ignoredTables"] = ignoredTables
	}
	writeJSON(w, http.StatusOK, response)
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
func previewBackupImportTables(db *store.DB, tables map[string]json.RawMessage) (map[string]backupImportPreview, error) {
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
func queryExistingPKs(db *store.DB, table, pkCol string, values []string) map[string]bool {
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
		rows, err := db.Query(query, args...)
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
	// ignoredTables names payload tables this build no longer has, i.e. tables
	// a backup written by an older build carried that have since been deleted,
	// plus the transient quota-reservation table older exports still carry.
	// Their rows are dropped; surfacing the names tells the operator the
	// restore was partial instead of silently losing data.
	ignoredTables []string
	// droppedForbiddenURLs counts site/endpoint rows rejected by the
	// SSRF target guard (cloud metadata / link-local URLs).
	droppedForbiddenURLs int64
	// newDownstreamApiKeys lists the names (never the key values) of
	// downstream_api_keys entries actually inserted by this import. Importing
	// an untrusted backup is equivalent to accepting the keys it carries; the
	// list is surfaced in the response and the events audit trail.
	newDownstreamApiKeys []string
}

func importBackupTables(db *store.DB, tables map[string]json.RawMessage, fromExport bool) (*backupImportResult, error) {
	if tables == nil {
		return nil, fmt.Errorf("invalid import data: expected a JSON object with a tables field")
	}
	ignoredTables, err := validateBackupImportTableKeys(tables, fromExport)
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

	result, err := importBackupTablesWithConn(tx, tables)
	if err != nil {
		return nil, err
	}
	result.ignoredTables = ignoredTables
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit import tx: %w", err)
	}
	committed = true
	return result, nil
}

type backupImportConn interface {
	DriverName() string
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	Exec(query string, args ...any) (sql.Result, error)
}

// logImportedDownstreamKeys records a warning event listing the downstream
// API keys a backup import admitted. Names only — key values are never
// logged. Importing an untrusted backup is equivalent to accepting the keys
// it carries; this event gives operators a durable trail. Best-effort: a
// failed insert never blocks the import response.
func logImportedDownstreamKeys(db *store.DB, names []string) {
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
	const query = `INSERT INTO events (type, title, message, level, related_type, created_at, "read")
		VALUES (?, ?, ?, ?, 'backup', ?, ?)`
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
	if errors.Is(err, backupsvc.ErrImportedEntityConflict) {
		return http.StatusConflict
	}
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
