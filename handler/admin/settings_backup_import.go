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

	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
)

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
