package backup

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
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

// RuntimeLocalSettingKeys are settings whose values are bound to the current
// deployment (admin credentials, DB wiring, webdav sync state). Backup imports
// must never overwrite them. Shared with the tables-path import in
// handler/admin so both paths apply the same policy.
var RuntimeLocalSettingKeys = map[string]bool{
	"auth_token":             true,
	"backup_webdav_state_v1": true,
	"db_ssl":                 true,
	"db_type":                true,
	"db_url":                 true,
}

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

// ---- field mapping ----

type tsV21FieldKind int

const (
	tsV21Text     tsV21FieldKind = iota
	tsV21JSONText                // JSON-encoded TEXT column: non-string values are re-marshaled
	tsV21Int
	tsV21Bool
	tsV21Real
)

type tsV21Field struct {
	tsName   string
	goColumn string
	kind     tsV21FieldKind
	// nullDefault is the value an explicit JSON null converts to when set.
	// Used for nullable numeric/boolean columns whose DDL carries a DEFAULT
	// (route_channels.weight, sites.global_weight, ...): importing explicit
	// null as the DDL default instead of NULL keeps #933-family NULL rows out
	// of the routing tables (a single NULL used to break the routing loads).
	// nil keeps NULL — the right semantics for text and truly-nullable
	// columns (accounts.balance stays unknown/nil per #933).
	nullDefault any
}

type tsV21TableSpec struct {
	goTable string
	section string
	// required lists Go columns that must be present in every row (NOT NULL
	// without a default in the Go schema).
	required []string
	fields   []tsV21Field
	fieldSet map[string]bool
}

func newTSV21TableSpec(goTable, section string, required []string, fields ...tsV21Field) tsV21TableSpec {
	spec := tsV21TableSpec{
		goTable:  goTable,
		section:  section,
		required: required,
		fields:   fields,
		fieldSet: map[string]bool{},
	}
	for _, field := range fields {
		spec.fieldSet[field.tsName] = true
	}
	return spec
}

// tsV21AccountTableSpecs lists the regular account sections in FK-safe import
// order. manualModels is handled separately because it maps into
// model_availability with synthetic columns.
var tsV21AccountTableSpecs = []tsV21TableSpec{
	newTSV21TableSpec("sites", "sites",
		[]string{"name", "url", "platform"},
		tsV21Field{"id", "id", tsV21Int, nil},
		tsV21Field{"name", "name", tsV21Text, nil},
		tsV21Field{"url", "url", tsV21Text, nil},
		tsV21Field{"externalCheckinUrl", "external_checkin_url", tsV21Text, nil},
		tsV21Field{"platform", "platform", tsV21Text, nil},
		tsV21Field{"proxyUrl", "proxy_url", tsV21Text, nil},
		tsV21Field{"useSystemProxy", "use_system_proxy", tsV21Bool, false},
		tsV21Field{"customHeaders", "custom_headers", tsV21JSONText, nil},
		tsV21Field{"status", "status", tsV21Text, nil},
		tsV21Field{"isPinned", "is_pinned", tsV21Bool, false},
		tsV21Field{"sortOrder", "sort_order", tsV21Int, int64(0)},
		tsV21Field{"globalWeight", "global_weight", tsV21Real, float64(1)},
		tsV21Field{"apiKey", "api_key", tsV21Text, nil},
		tsV21Field{"postRefreshProbeEnabled", "post_refresh_probe_enabled", tsV21Bool, false},
		tsV21Field{"postRefreshProbeModel", "post_refresh_probe_model", tsV21Text, nil},
		tsV21Field{"postRefreshProbeScope", "post_refresh_probe_scope", tsV21Text, nil},
		tsV21Field{"postRefreshProbeLatencyThresholdMs", "post_refresh_probe_latency_threshold_ms", tsV21Int, int64(0)},
		tsV21Field{"createdAt", "created_at", tsV21Text, nil},
		tsV21Field{"updatedAt", "updated_at", tsV21Text, nil},
	),
	newTSV21TableSpec("site_api_endpoints", "siteApiEndpoints",
		[]string{"site_id", "url"},
		tsV21Field{"id", "id", tsV21Int, nil},
		tsV21Field{"siteId", "site_id", tsV21Int, nil},
		tsV21Field{"url", "url", tsV21Text, nil},
		tsV21Field{"enabled", "enabled", tsV21Bool, nil},
		tsV21Field{"sortOrder", "sort_order", tsV21Int, nil},
		tsV21Field{"cooldownUntil", "cooldown_until", tsV21Text, nil},
		tsV21Field{"lastSelectedAt", "last_selected_at", tsV21Text, nil},
		tsV21Field{"lastFailedAt", "last_failed_at", tsV21Text, nil},
		tsV21Field{"lastFailureReason", "last_failure_reason", tsV21Text, nil},
		tsV21Field{"createdAt", "created_at", tsV21Text, nil},
		tsV21Field{"updatedAt", "updated_at", tsV21Text, nil},
	),
	newTSV21TableSpec("site_disabled_models", "siteDisabledModels",
		[]string{"site_id", "model_name"},
		tsV21Field{"siteId", "site_id", tsV21Int, nil},
		tsV21Field{"modelName", "model_name", tsV21Text, nil},
	),
	newTSV21TableSpec("accounts", "accounts",
		[]string{"site_id", "access_token"},
		tsV21Field{"id", "id", tsV21Int, nil},
		tsV21Field{"siteId", "site_id", tsV21Int, nil},
		tsV21Field{"username", "username", tsV21Text, nil},
		tsV21Field{"accessToken", "access_token", tsV21Text, nil},
		tsV21Field{"apiToken", "api_token", tsV21Text, nil},
		tsV21Field{"balance", "balance", tsV21Real, nil},
		tsV21Field{"balanceUsed", "balance_used", tsV21Real, nil},
		tsV21Field{"quota", "quota", tsV21Real, nil},
		tsV21Field{"unitCost", "unit_cost", tsV21Real, nil},
		tsV21Field{"valueScore", "value_score", tsV21Real, nil},
		tsV21Field{"status", "status", tsV21Text, nil},
		tsV21Field{"isPinned", "is_pinned", tsV21Bool, nil},
		tsV21Field{"sortOrder", "sort_order", tsV21Int, nil},
		tsV21Field{"checkinEnabled", "checkin_enabled", tsV21Bool, nil},
		tsV21Field{"lastCheckinAt", "last_checkin_at", tsV21Text, nil},
		tsV21Field{"lastBalanceRefresh", "last_balance_refresh", tsV21Text, nil},
		tsV21Field{"oauthProvider", "oauth_provider", tsV21Text, nil},
		tsV21Field{"oauthAccountKey", "oauth_account_key", tsV21Text, nil},
		tsV21Field{"oauthProjectId", "oauth_project_id", tsV21Text, nil},
		tsV21Field{"extraConfig", "extra_config", tsV21JSONText, nil},
		tsV21Field{"createdAt", "created_at", tsV21Text, nil},
		tsV21Field{"updatedAt", "updated_at", tsV21Text, nil},
	),
	newTSV21TableSpec("account_tokens", "accountTokens",
		[]string{"account_id", "name", "token"},
		tsV21Field{"id", "id", tsV21Int, nil},
		tsV21Field{"accountId", "account_id", tsV21Int, nil},
		tsV21Field{"name", "name", tsV21Text, nil},
		tsV21Field{"token", "token", tsV21Text, nil},
		tsV21Field{"tokenGroup", "token_group", tsV21Text, nil},
		tsV21Field{"valueStatus", "value_status", tsV21Text, nil},
		tsV21Field{"source", "source", tsV21Text, nil},
		tsV21Field{"enabled", "enabled", tsV21Bool, nil},
		tsV21Field{"isDefault", "is_default", tsV21Bool, nil},
		tsV21Field{"createdAt", "created_at", tsV21Text, nil},
		tsV21Field{"updatedAt", "updated_at", tsV21Text, nil},
	),
	newTSV21TableSpec("token_routes", "tokenRoutes",
		[]string{"model_pattern"},
		tsV21Field{"id", "id", tsV21Int, nil},
		tsV21Field{"modelPattern", "model_pattern", tsV21Text, nil},
		tsV21Field{"displayName", "display_name", tsV21Text, nil},
		tsV21Field{"displayIcon", "display_icon", tsV21Text, nil},
		tsV21Field{"routeMode", "route_mode", tsV21Text, nil},
		tsV21Field{"modelMapping", "model_mapping", tsV21JSONText, nil},
		tsV21Field{"decisionSnapshot", "decision_snapshot", tsV21JSONText, nil},
		tsV21Field{"decisionRefreshedAt", "decision_refreshed_at", tsV21Text, nil},
		tsV21Field{"routingStrategy", "routing_strategy", tsV21Text, nil},
		tsV21Field{"enabled", "enabled", tsV21Bool, true},
		tsV21Field{"createdAt", "created_at", tsV21Text, nil},
		tsV21Field{"updatedAt", "updated_at", tsV21Text, nil},
	),
	newTSV21TableSpec("route_group_sources", "routeGroupSources",
		[]string{"group_route_id", "source_route_id"},
		tsV21Field{"id", "id", tsV21Int, nil},
		tsV21Field{"groupRouteId", "group_route_id", tsV21Int, nil},
		tsV21Field{"sourceRouteId", "source_route_id", tsV21Int, nil},
	),
	newTSV21TableSpec("route_channels", "routeChannels",
		[]string{"route_id", "account_id"},
		tsV21Field{"id", "id", tsV21Int, nil},
		tsV21Field{"routeId", "route_id", tsV21Int, nil},
		tsV21Field{"accountId", "account_id", tsV21Int, nil},
		tsV21Field{"tokenId", "token_id", tsV21Int, nil},
		tsV21Field{"oauthRouteUnitId", "oauth_route_unit_id", tsV21Int, nil},
		tsV21Field{"sourceModel", "source_model", tsV21Text, nil},
		tsV21Field{"priority", "priority", tsV21Int, int64(0)},
		tsV21Field{"weight", "weight", tsV21Int, int64(10)},
		tsV21Field{"enabled", "enabled", tsV21Bool, true},
		tsV21Field{"manualOverride", "manual_override", tsV21Bool, false},
		tsV21Field{"successCount", "success_count", tsV21Int, int64(0)},
		tsV21Field{"failCount", "fail_count", tsV21Int, int64(0)},
		tsV21Field{"totalLatencyMs", "total_latency_ms", tsV21Int, int64(0)},
		tsV21Field{"totalCost", "total_cost", tsV21Real, float64(0)},
		tsV21Field{"lastUsedAt", "last_used_at", tsV21Text, nil},
		tsV21Field{"lastSelectedAt", "last_selected_at", tsV21Text, nil},
		tsV21Field{"lastFailAt", "last_fail_at", tsV21Text, nil},
		tsV21Field{"consecutiveFailCount", "consecutive_fail_count", tsV21Int, int64(0)},
		tsV21Field{"cooldownLevel", "cooldown_level", tsV21Int, int64(0)},
		tsV21Field{"cooldownUntil", "cooldown_until", tsV21Text, nil},
	),
	newTSV21TableSpec("downstream_api_keys", "downstreamApiKeys",
		[]string{"name", "key"},
		tsV21Field{"name", "name", tsV21Text, nil},
		tsV21Field{"key", "key", tsV21Text, nil},
		tsV21Field{"description", "description", tsV21Text, nil},
		tsV21Field{"groupName", "group_name", tsV21Text, nil},
		tsV21Field{"tags", "tags", tsV21JSONText, nil},
		tsV21Field{"enabled", "enabled", tsV21Bool, nil},
		tsV21Field{"expiresAt", "expires_at", tsV21Text, nil},
		tsV21Field{"maxCost", "max_cost", tsV21Real, nil},
		tsV21Field{"usedCost", "used_cost", tsV21Real, nil},
		tsV21Field{"maxRequests", "max_requests", tsV21Int, nil},
		tsV21Field{"usedRequests", "used_requests", tsV21Int, nil},
		tsV21Field{"supportedModels", "supported_models", tsV21JSONText, nil},
		tsV21Field{"allowedRouteIds", "allowed_route_ids", tsV21JSONText, nil},
		tsV21Field{"siteWeightMultipliers", "site_weight_multipliers", tsV21JSONText, nil},
		tsV21Field{"excludedSiteIds", "excluded_site_ids", tsV21JSONText, nil},
		tsV21Field{"excludedCredentialRefs", "excluded_credential_refs", tsV21JSONText, nil},
		tsV21Field{"lastUsedAt", "last_used_at", tsV21Text, nil},
	),
}

// tsV21KnownAccountSections is every section name the TS v2.1 accounts object
// may carry. Anything else is reported as an unknown section.
var tsV21KnownAccountSections = map[string]bool{
	"sites":              true,
	"siteApiEndpoints":   true,
	"siteDisabledModels": true,
	"accounts":           true,
	"accountTokens":      true,
	"tokenRoutes":        true,
	"routeChannels":      true,
	"routeGroupSources":  true,
	"manualModels":       true,
	"downstreamApiKeys":  true,
}

func parseTSV21AccountsSectionInto(raw json.RawMessage, parsed *TSV21Parsed) error {
	var section map[string]json.RawMessage
	if err := json.Unmarshal(raw, &section); err != nil || section == nil {
		return TSV21ClientError{Message: "invalid import data: accounts must be a JSON object"}
	}

	for key := range section {
		if !tsV21KnownAccountSections[key] {
			parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("ignored unknown section accounts.%s", key))
		}
	}

	// Parent tables first, manualModels in the middle (model_availability
	// references accounts), children afterwards.
	for _, spec := range tsV21AccountTableSpecs[:5] {
		if err := appendTSV21SectionRows(section, spec, parsed); err != nil {
			return err
		}
	}
	if err := appendTSV21ManualModels(section, parsed); err != nil {
		return err
	}
	for _, spec := range tsV21AccountTableSpecs[5:] {
		if err := appendTSV21SectionRows(section, spec, parsed); err != nil {
			return err
		}
	}
	return nil
}

func appendTSV21SectionRows(section map[string]json.RawMessage, spec tsV21TableSpec, parsed *TSV21Parsed) error {
	rawRows, present := section[spec.section]
	if !present || isRawJSONNull(rawRows) {
		return nil
	}
	rows, err := parseTSV21SectionRows(spec, rawRows, &parsed.Warnings)
	if err != nil {
		return err
	}
	if len(rows) > 0 {
		parsed.Tables[spec.goTable] = rows
		parsed.TableOrder = append(parsed.TableOrder, spec.goTable)
	}
	return nil
}

func parseTSV21SectionRows(spec tsV21TableSpec, raw json.RawMessage, warnings *[]string) ([]map[string]any, error) {
	var rawRows []json.RawMessage
	if err := json.Unmarshal(raw, &rawRows); err != nil {
		return nil, TSV21ClientError{Message: fmt.Sprintf("invalid import data: accounts.%s must be an array", spec.section)}
	}
	if TSV21MaxRowsPerTable > 0 && len(rawRows) > TSV21MaxRowsPerTable {
		return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: accounts.%s exceeds the max rows of %d", spec.section, TSV21MaxRowsPerTable)}
	}

	unknownFieldCounts := map[string]int{}
	rows := make([]map[string]any, 0, len(rawRows))
	for i, rawRow := range rawRows {
		var row map[string]any
		if err := json.Unmarshal(rawRow, &row); err != nil || row == nil {
			*warnings = append(*warnings, fmt.Sprintf("ignored account row %d in %s: not a JSON object", i+1, spec.section))
			continue
		}

		converted, err := convertTSV21Row(spec, row, i+1)
		if err != nil {
			return nil, err
		}
		if TSV21MaxColumnsPerRow > 0 && len(converted) > TSV21MaxColumnsPerRow {
			return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: account row %d in %s exceeds the max columns of %d", i+1, spec.section, TSV21MaxColumnsPerRow)}
		}
		for field := range row {
			if !spec.fieldSet[field] {
				unknownFieldCounts[field]++
			}
		}
		if len(converted) > 0 {
			rows = append(rows, converted)
		}
	}

	unknownFields := make([]string, 0, len(unknownFieldCounts))
	for field := range unknownFieldCounts {
		unknownFields = append(unknownFields, field)
	}
	sort.Strings(unknownFields)
	for _, field := range unknownFields {
		count := unknownFieldCounts[field]
		if count > 1 {
			*warnings = append(*warnings, fmt.Sprintf("ignored unknown field %s.%s (%d rows)", spec.section, field, count))
		} else {
			*warnings = append(*warnings, fmt.Sprintf("ignored unknown field %s.%s", spec.section, field))
		}
	}
	return rows, nil
}

func convertTSV21Row(spec tsV21TableSpec, row map[string]any, rowIndex int) (map[string]any, error) {
	converted := make(map[string]any, len(spec.fields))
	for _, field := range spec.fields {
		value, present := row[field.tsName]
		if !present {
			continue
		}
		normalized, err := convertTSV21FieldValue(field, value)
		if err != nil {
			return nil, err
		}
		converted[field.goColumn] = normalized
	}
	for _, required := range spec.required {
		if _, ok := converted[required]; !ok {
			return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: account row %d in %s is missing required field %s", rowIndex, spec.section, required)}
		}
	}
	return converted, nil
}

func convertTSV21FieldValue(field tsV21Field, value any) (any, error) {
	switch field.kind {
	case tsV21Text:
		switch v := value.(type) {
		case nil:
			return nil, nil
		case string:
			if err := checkTSV21CellBytes(field.tsName, v); err != nil {
				return nil, err
			}
			return v, nil
		default:
			return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: field %s must be a string or null", field.tsName)}
		}
	case tsV21JSONText:
		switch v := value.(type) {
		case nil:
			return nil, nil
		case string:
			if err := checkTSV21CellBytes(field.tsName, v); err != nil {
				return nil, err
			}
			return v, nil
		default:
			raw, err := json.Marshal(v)
			if err != nil {
				return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: field %s cannot be serialized: %v", field.tsName, err)}
			}
			if err := checkTSV21CellBytes(field.tsName, string(raw)); err != nil {
				return nil, err
			}
			return string(raw), nil
		}
	case tsV21Int:
		switch v := value.(type) {
		case nil:
			if field.nullDefault != nil {
				return field.nullDefault, nil
			}
			return nil, nil
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v {
				return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: field %s must be an integer", field.tsName)}
			}
			return int64(v), nil
		case int64:
			return v, nil
		default:
			return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: field %s must be an integer", field.tsName)}
		}
	case tsV21Bool:
		switch v := value.(type) {
		case nil:
			if field.nullDefault != nil {
				return field.nullDefault, nil
			}
			return nil, nil
		case bool:
			return v, nil
		case float64:
			if v == 0 {
				return false, nil
			}
			if v == 1 {
				return true, nil
			}
		}
		return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: field %s must be a boolean", field.tsName)}
	case tsV21Real:
		switch v := value.(type) {
		case nil:
			if field.nullDefault != nil {
				return field.nullDefault, nil
			}
			return nil, nil
		case float64:
			return v, nil
		case int64:
			return float64(v), nil
		}
		return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: field %s must be a number", field.tsName)}
	default:
		return nil, TSV21ClientError{Message: fmt.Sprintf("import failed: field %s has an unrecognized type", field.tsName)}
	}
}

func checkTSV21CellBytes(field, value string) error {
	if TSV21MaxCellBytes > 0 && len(value) > TSV21MaxCellBytes {
		return TSV21ClientError{Message: fmt.Sprintf("import failed: field %s exceeds the max cell size of %d bytes", field, TSV21MaxCellBytes)}
	}
	return nil
}

// appendTSV21ManualModels maps TS manualModels rows into model_availability
// rows with is_manual=true / available=true (mirroring the TS importer).
func appendTSV21ManualModels(section map[string]json.RawMessage, parsed *TSV21Parsed) error {
	rawRows, present := section["manualModels"]
	if !present || isRawJSONNull(rawRows) {
		return nil
	}
	var rawList []json.RawMessage
	if err := json.Unmarshal(rawRows, &rawList); err != nil {
		return TSV21ClientError{Message: "invalid import data: accounts.manualModels must be an array"}
	}
	if TSV21MaxRowsPerTable > 0 && len(rawList) > TSV21MaxRowsPerTable {
		return TSV21ClientError{Message: fmt.Sprintf("import failed: accounts.manualModels exceeds the max rows of %d", TSV21MaxRowsPerTable)}
	}

	checkedAt := time.Now().UTC().Format(time.RFC3339)
	rows := make([]map[string]any, 0, len(rawList))
	for i, rawRow := range rawList {
		var row map[string]any
		if err := json.Unmarshal(rawRow, &row); err != nil || row == nil {
			parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("ignored accounts.manualModels row %d: not a JSON object", i+1))
			continue
		}
		accountID, err := convertTSV21FieldValue(tsV21Field{tsName: "accountId", goColumn: "account_id", kind: tsV21Int}, row["accountId"])
		if err != nil {
			return err
		}
		if accountID == nil {
			return TSV21ClientError{Message: fmt.Sprintf("import failed: accounts.manualModels row %d is missing required field accountId", i+1)}
		}
		modelName, ok := row["modelName"].(string)
		modelName = strings.TrimSpace(modelName)
		if !ok || modelName == "" {
			return TSV21ClientError{Message: fmt.Sprintf("import failed: accounts.manualModels row %d is missing required field modelName", i+1)}
		}
		for field := range row {
			if field != "accountId" && field != "modelName" {
				parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("ignored unknown field manualModels.%s", field))
			}
		}
		rows = append(rows, map[string]any{
			"account_id": accountID,
			"model_name": modelName,
			"available":  true,
			"is_manual":  true,
			"latency_ms": nil,
			"checked_at": checkedAt,
		})
	}
	if len(rows) > 0 {
		parsed.Tables["model_availability"] = rows
		parsed.TableOrder = append(parsed.TableOrder, "model_availability")
	}
	return nil
}

func parseTSV21PreferencesSectionInto(raw json.RawMessage, parsed *TSV21Parsed) error {
	var section map[string]json.RawMessage
	if err := json.Unmarshal(raw, &section); err != nil || section == nil {
		return TSV21ClientError{Message: "invalid import data: preferences must be a JSON object"}
	}
	for key := range section {
		if key != "settings" {
			parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("ignored unknown section preferences.%s", key))
		}
	}

	rawSettings, present := section["settings"]
	if !present || isRawJSONNull(rawSettings) {
		return TSV21ClientError{Message: "invalid import data: preferences.settings must be an array"}
	}
	var rawRows []json.RawMessage
	if err := json.Unmarshal(rawSettings, &rawRows); err != nil {
		return TSV21ClientError{Message: "invalid import data: preferences.settings must be an array"}
	}
	if TSV21MaxRowsPerTable > 0 && len(rawRows) > TSV21MaxRowsPerTable {
		return TSV21ClientError{Message: fmt.Sprintf("import failed: preferences.settings exceeds the max rows of %d", TSV21MaxRowsPerTable)}
	}

	rows := make([]map[string]any, 0, len(rawRows))
	for i, rawRow := range rawRows {
		var row map[string]any
		if err := json.Unmarshal(rawRow, &row); err != nil || row == nil {
			parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("ignored preferences.settings row %d: not a JSON object", i+1))
			continue
		}

		key, _ := row["key"].(string)
		key = strings.TrimSpace(key)
		if key == "" {
			parsed.SkippedSettings++
			parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("ignored preferences.settings row %d: key missing or empty", i+1))
			continue
		}
		if RuntimeLocalSettingKeys[key] {
			parsed.SkippedSettings++
			continue
		}

		for field := range row {
			if field != "key" && field != "value" {
				parsed.Warnings = append(parsed.Warnings, fmt.Sprintf("ignored unknown field settings.%s", field))
			}
		}

		value, hasValue := row["value"]
		if !hasValue {
			value = nil
		}
		// The TS importer stores JSON.stringify(value); the Go settings table
		// also holds JSON-encoded text, so re-marshal the parsed value.
		rawValue, err := json.Marshal(value)
		if err != nil {
			return TSV21ClientError{Message: fmt.Sprintf("import failed: preferences.settings row %d value cannot be serialized: %v", i+1, err)}
		}
		if err := checkTSV21CellBytes("value", string(rawValue)); err != nil {
			return err
		}
		rows = append(rows, map[string]any{"key": key, "value": string(rawValue)})
	}
	if len(rows) > 0 {
		parsed.Tables["settings"] = rows
		parsed.TableOrder = append(parsed.TableOrder, "settings")
	}
	return nil
}

// ---- import ----

// TSV21ImportResult reports what a v2.1 import actually did.
type TSV21ImportResult struct {
	// Imported maps Go table names to the number of rows inserted. Rows whose
	// primary key already existed are skipped (ON CONFLICT DO NOTHING).
	Imported map[string]int64 `json:"imported"`
	// SkippedSettings counts settings rows excluded by policy.
	SkippedSettings int64    `json:"skippedSettings,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
}

// ImportTSV21 parses a v2.1 payload and imports it into db inside a single
// transaction. Duplicate handling matches the tables-path import: rows whose
// primary key already exists are skipped, nothing is overwritten.
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
		count, err := importTSV21TableRows(conn, table, parsed.Tables[table])
		if err != nil {
			return nil, fmt.Errorf("import failed: table %s: %w", table, err)
		}
		result.Imported[table] = count
	}
	return result, nil
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

// ---- preview ----

// TSV21TablePreview is the per-table preview plan for a v2.1 import.
type TSV21TablePreview struct {
	Rows        int64 `json:"rows"`
	ToInsert    int64 `json:"toInsert"`
	Duplicates  int64 `json:"duplicates"`
	SkippedRows int64 `json:"skippedRows"`
}

// TSV21PreviewResult is the full v2.1 import plan without writes.
type TSV21PreviewResult struct {
	Plan     map[string]TSV21TablePreview `json:"plan"`
	Warnings []string                     `json:"warnings,omitempty"`
}

// PreviewTSV21 computes the import plan for a v2.1 payload without writing
// anything. PK detection mirrors the tables-path preview: "id" for every table
// except "settings" which uses "key".
func PreviewTSV21(db *store.DB, raw []byte) (*TSV21PreviewResult, error) {
	parsed, err := ParseTSV21(raw)
	if err != nil {
		return nil, err
	}

	preview := &TSV21PreviewResult{
		Plan:     map[string]TSV21TablePreview{},
		Warnings: parsed.Warnings,
	}
	for _, table := range parsed.TableOrder {
		rows := parsed.Tables[table]
		plan := TSV21TablePreview{Rows: int64(len(rows))}
		if table == "settings" {
			plan.SkippedRows = parsed.SkippedSettings
		}
		pkColumn := "id"
		if table == "settings" {
			pkColumn = "key"
		}

		existing := map[string]bool{}
		var pkValues []string
		hasPKColumn := false
		for _, row := range rows {
			rawValue, present := row[pkColumn]
			if !present {
				plan.ToInsert++
				continue
			}
			hasPKColumn = true
			pk := fmt.Sprintf("%v", rawValue)
			if _, duplicate := existing[pk]; duplicate {
				plan.Duplicates++
				continue
			}
			existing[pk] = true
			pkValues = append(pkValues, pk)
		}
		if hasPKColumn && len(pkValues) > 0 {
			existingInDB := tsV21QueryExistingPKs(db, table, pkColumn, pkValues)
			for _, pk := range pkValues {
				if existingInDB[pk] {
					plan.Duplicates++
				} else {
					plan.ToInsert++
				}
			}
		}
		preview.Plan[table] = plan
	}
	return preview, nil
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
