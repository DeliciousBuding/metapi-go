package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/service"
	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
)

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
