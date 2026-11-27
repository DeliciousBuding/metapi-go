package backup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// TS backup v2.1 (cita-777/metapi) import support.
//
// The TS original exports backups in the shape produced by
// src/server/services/backupService.ts exportBackup:
//
//	{
//	  "version": "2.1",
//	  "timestamp": 1755678900000,
//	  "type": "all" | "accounts" | "preferences",   // omitted when "all"
//	  "accounts": {
//	    "sites": [...],
//	    "siteApiEndpoints": [...],
//	    "siteDisabledModels": [{"siteId", "modelName"}],
//	    "accounts": [...],
//	    "accountTokens": [...],
//	    "tokenRoutes": [...],
//	    "routeChannels": [...],
//	    "routeGroupSources": [...],
//	    "manualModels": [{"accountId", "modelName"}],
//	    "downstreamApiKeys": [...]
//	  },
//	  "preferences": {
//	    "settings": [{"key": "...", "value": <parsed JSON value>}]
//	  }
//	}
//
// Drizzle emits rows keyed by the TS camelCase property names while the Go
// schema uses snake_case columns, so every TS field is mapped explicitly here.
// Unknown top-level keys, unknown account sections and unknown row fields are
// ignored and reported as warnings instead of failing the import, so future
// TS releases with extra fields remain importable. Duplicate handling matches
// the existing tables-path import: INSERT ... ON CONFLICT DO NOTHING (rows
// whose primary key already exists are skipped, nothing is overwritten).

// TSBackupVersionV21 is the backup version string emitted by the TS original.
const TSBackupVersionV21 = "2.1"

var (
	// TSV21MaxRowsPerTable, TSV21MaxColumnsPerRow and TSV21MaxCellBytes mirror
	// the tables-path import limits in handler/admin (50k rows per table,
	// 128 columns per row, 4MB per cell).
	TSV21MaxRowsPerTable  = 50_000
	TSV21MaxColumnsPerRow = 128
	TSV21MaxCellBytes     = 4 << 20
)

// TSV21ClientError reports a payload problem that is the caller's fault
// (malformed JSON, missing required fields, limit violations). Handlers map it
// to HTTP 400.
type TSV21ClientError struct {
	Message string
}

func (e TSV21ClientError) Error() string { return e.Message }

// IsTSV21Payload reports whether raw looks like a TS backup JSON carrying
// version "2.1".
func IsTSV21Payload(raw []byte) bool {
	var probe struct {
		Version json.RawMessage `json:"version"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}
	var version string
	if err := json.Unmarshal(probe.Version, &version); err != nil {
		return false
	}
	return version == TSBackupVersionV21
}

// TSV21Parsed is a v2.1 payload normalized to canonical snake_case table rows
// in FK-safe import order.
type TSV21Parsed struct {
	// Type is the optional top-level type ("all" | "accounts" | "preferences");
	// empty when the payload has no type field.
	Type string
	// TableOrder lists Go tables in FK-safe import order.
	TableOrder []string
	// Tables maps Go table names to converted rows.
	Tables map[string][]map[string]any
	// Warnings collects everything that was ignored (unknown fields/sections,
	// malformed rows). Imports must never fail because of them.
	Warnings []string
	// SkippedSettings counts settings rows excluded by policy (runtime-local
	// keys, empty keys).
	SkippedSettings int64
}

// ParseTSV21 converts a v2.1 payload into canonical table rows. It performs
// all structural validation but does not touch the database.
func ParseTSV21(raw []byte) (*TSV21Parsed, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, TSV21ClientError{Message: fmt.Sprintf("invalid import data: unable to parse JSON: %v", err)}
	}
	if top == nil {
		return nil, TSV21ClientError{Message: "invalid import data: must be a JSON object"}
	}

	parsed := &TSV21Parsed{Tables: map[string][]map[string]any{}}

	// The TS importer refuses payloads without a timestamp; mirror that so
	// truncated files fail with a clear message instead of importing half data.
	if rawTimestamp, ok := top["timestamp"]; !ok || isRawJSONNull(rawTimestamp) {
		return nil, TSV21ClientError{Message: "invalid import data: missing timestamp"}
	}

	if rawType, ok := top["type"]; ok && !isRawJSONNull(rawType) {
		_ = json.Unmarshal(rawType, &parsed.Type)
	}

	for key := range top {
		switch key {
		case "version", "timestamp", "type", "accounts", "preferences":
		default:
			parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("ignored unknown top-level field %s", key))
		}
	}

	accountsRaw, hasAccounts := top["accounts"]
	preferencesRaw, hasPreferences := top["preferences"]
	if hasAccounts && isRawJSONNull(accountsRaw) {
		hasAccounts = false
	}
	if hasPreferences && isRawJSONNull(preferencesRaw) {
		hasPreferences = false
	}

	// Mirror TS importBackup gating: the type field forces a section, sections
	// that are present are always imported.
	accountsRequested := parsed.Type == "accounts" || hasAccounts
	preferencesRequested := parsed.Type == "preferences" || hasPreferences
	if !accountsRequested && !preferencesRequested {
		return nil, TSV21ClientError{Message: "no recognizable account or settings data in import payload"}
	}
	if parsed.Type == "accounts" && !hasAccounts {
		return nil, TSV21ClientError{Message: "invalid import data: accounts section structure is incorrect"}
	}
	if parsed.Type == "preferences" && !hasPreferences {
		return nil, TSV21ClientError{Message: "invalid import data: preferences section structure is incorrect"}
	}

	if hasAccounts {
		if err := parseTSV21AccountsSectionInto(accountsRaw, parsed); err != nil {
			return nil, err
		}
	}
	if hasPreferences {
		if err := parseTSV21PreferencesSectionInto(preferencesRaw, parsed); err != nil {
			return nil, err
		}
	}

	sort.Strings(parsed.Warnings)
	return parsed, nil
}

func isRawJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func checkTSV21CellBytes(field, value string) error {
	if TSV21MaxCellBytes > 0 && len(value) > TSV21MaxCellBytes {
		return TSV21ClientError{Message: fmt.Sprintf("import failed: field %s exceeds the max cell size of %d bytes", field, TSV21MaxCellBytes)}
	}
	return nil
}
