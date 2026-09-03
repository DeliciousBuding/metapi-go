package backup

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// AllTables is the full backup/export order, with parent tables before
// children so imports can replay it safely. It is derived from the store
// schema registry minus store.BackupExcludedTables() — the same single source
// of truth that drives AutoMigrate, cmd/migrate and the factory reset — so a
// table added to the schema ships in every type=all backup until somebody
// excludes it by name with a recorded reason. This used to be a hand-copied
// list that had drifted to 28 of 37 tables: product_announcements,
// announcement_dismissals, model_name_redirects, balance_history and
// model_verify_history were silently dropped from every export.
var AllTables = store.BackupTableNames()

// accountsExportScope names the tables an export of type=accounts carries. It
// is a scope declaration, not an ordering: AccountsTables below intersects it
// with the registry-derived backup order, so there is one FK-safe order owner.
var accountsExportScope = map[string]bool{
	"sites":                    true,
	"site_api_endpoints":       true,
	"site_disabled_models":     true,
	"accounts":                 true,
	"account_tokens":           true,
	"checkin_logs":             true,
	"model_availability":       true,
	"token_model_availability": true,
	"token_routes":             true,
	"route_group_sources":      true,
	"oauth_route_units":        true,
	"oauth_route_unit_members": true,
	"route_channels":           true,
	"proxy_video_tasks":        true,
	"admin_background_tasks":   true,
	"downstream_api_keys":      true,
	"site_announcements":       true,
}

// AccountsTables is the subset exported when type=accounts, in the same
// FK-safe order as AllTables.
var AccountsTables = filterTables(AllTables, accountsExportScope)

// filterTables keeps the members of scope in the order given by tables.
func filterTables(tables []string, scope map[string]bool) []string {
	out := make([]string, 0, len(scope))
	for _, t := range tables {
		if scope[t] {
			out = append(out, t)
		}
	}
	return out
}

var ErrInvalidExportType = errors.New("invalid export type; only all/accounts/preferences are supported")

var (
	MaxExportRowsPerTable = 50_000
	MaxExportCellBytes    = 4 << 20
	MaxExportPayloadBytes = int64(64 << 20)
)

type ExportLimitError struct {
	message string
}

func (e ExportLimitError) Error() string {
	return e.message
}

func BuildPayload(db *sqlx.DB, exportType string) (map[string]any, error) {
	exportType, tables, err := TablesForExportType(exportType)
	if err != nil {
		return nil, err
	}

	result := map[string]any{}
	estimatedPayloadBytes := int64(512)
	for _, table := range tables {
		if err := addEstimatedPayloadBytes(&estimatedPayloadBytes, int64(len(table)+16)); err != nil {
			return nil, err
		}
		rows, err := queryTableAsJSON(db, table, &estimatedPayloadBytes)
		if err != nil {
			return nil, fmt.Errorf("export failed: unable to read table %s: %w", table, err)
		}
		result[table] = rows
	}

	return map[string]any{
		"metadata": map[string]any{
			"exported_at":     time.Now().UTC().Format(time.RFC3339Nano),
			"version":         "1.0",
			"excluded_tables": excludedTablesFor(exportType, tables),
		},
		"type":   exportType,
		"tables": result,
	}, nil
}

// excludedTablesFor reports every schema-registry table this payload does NOT
// carry, keyed by table name with the reason. Two kinds of gap are surfaced:
// the deliberate backup exclusions (store.BackupExcludedTables) and, for a
// scoped export type, the tables outside that scope.
//
// It lands in metadata.excluded_tables so a backup file states its own gaps
// instead of leaving an operator to discover them after a restore. The field
// is purely additive: exported_at and version keep their shape, and both
// importers (the tables path and the TS v2.1 path) ignore metadata entirely,
// so existing payloads and existing consumers are unaffected.
func excludedTablesFor(exportType string, exported []string) map[string]string {
	inPayload := make(map[string]bool, len(exported))
	for _, t := range exported {
		inPayload[t] = true
	}
	reasons := store.BackupExcludedTables()
	out := map[string]string{}
	for _, t := range store.SchemaTableNames() {
		if inPayload[t] {
			continue
		}
		if reason, ok := reasons[t]; ok {
			out[t] = reason
			continue
		}
		out[t] = fmt.Sprintf("not part of the %q export scope", exportType)
	}
	return out
}

func TablesForExportType(exportType string) (string, []string, error) {
	normalized := strings.TrimSpace(strings.ToLower(exportType))
	if normalized == "" {
		normalized = "all"
	}

	switch normalized {
	case "all":
		return normalized, AllTables, nil
	case "accounts":
		return normalized, AccountsTables, nil
	case "preferences":
		return normalized, []string{"settings"}, nil
	default:
		return "", nil, ErrInvalidExportType
	}
}

func QueryTableAsJSON(db *sqlx.DB, table string) ([]map[string]any, error) {
	return queryTableAsJSON(db, table, nil)
}

func queryTableAsJSON(db *sqlx.DB, table string, estimatedPayloadBytes *int64) ([]map[string]any, error) {
	if !IsKnownTable(table) {
		return nil, fmt.Errorf("unknown table: %s", table)
	}

	rows, err := db.Queryx(fmt.Sprintf("SELECT * FROM %s", quoteIdentifier(table)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		if MaxExportRowsPerTable > 0 && len(result) >= MaxExportRowsPerTable {
			return nil, ExportLimitError{
				message: fmt.Sprintf("table %s exceeds max export rows of %d", table, MaxExportRowsPerTable),
			}
		}
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			return nil, err
		}
		for k, v := range row {
			if b, ok := v.([]byte); ok {
				row[k] = string(b)
			}
			if err := validateExportCell(table, k, row[k]); err != nil {
				return nil, err
			}
		}
		if estimatedPayloadBytes != nil {
			if err := addEstimatedPayloadBytes(estimatedPayloadBytes, estimateJSONRowBytes(row)+1); err != nil {
				return nil, err
			}
		}
		result = append(result, row)
	}
	if result == nil {
		result = []map[string]any{}
	}
	return result, rows.Err()
}

func validateExportCell(table string, column string, value any) error {
	if MaxExportCellBytes <= 0 {
		return nil
	}
	switch v := value.(type) {
	case string:
		if len(v) > MaxExportCellBytes {
			return ExportLimitError{
				message: fmt.Sprintf("table %s column %s exceeds max export cell size of %d bytes", table, column, MaxExportCellBytes),
			}
		}
	case []byte:
		if len(v) > MaxExportCellBytes {
			return ExportLimitError{
				message: fmt.Sprintf("table %s column %s exceeds max export cell size of %d bytes", table, column, MaxExportCellBytes),
			}
		}
	}
	return nil
}

func estimateJSONRowBytes(row map[string]any) int64 {
	if len(row) == 0 {
		return 2
	}
	var total int64 = 2 // object braces
	first := true
	for key, value := range row {
		if !first {
			total++ // comma
		}
		first = false
		total += estimateJSONStringBytes(key)
		total++ // colon
		total += estimateJSONValueBytes(value)
	}
	return total
}

func estimateJSONValueBytes(value any) int64 {
	switch v := value.(type) {
	case nil:
		return 4
	case bool:
		if v {
			return 4
		}
		return 5
	case string:
		return estimateJSONStringBytes(v)
	case []byte:
		return estimateJSONStringBytes(string(v))
	case int:
		return estimateSignedDecimalBytes(int64(v))
	case int8:
		return estimateSignedDecimalBytes(int64(v))
	case int16:
		return estimateSignedDecimalBytes(int64(v))
	case int32:
		return estimateSignedDecimalBytes(int64(v))
	case int64:
		return estimateSignedDecimalBytes(v)
	case uint:
		return estimateUnsignedDecimalBytes(uint64(v))
	case uint8:
		return estimateUnsignedDecimalBytes(uint64(v))
	case uint16:
		return estimateUnsignedDecimalBytes(uint64(v))
	case uint32:
		return estimateUnsignedDecimalBytes(uint64(v))
	case uint64:
		return estimateUnsignedDecimalBytes(v)
	default:
		return 128
	}
}

func estimateJSONStringBytes(value string) int64 {
	var total int64 = 2 // quotes
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '"', '\\':
			total += 2
		case '\b', '\f', '\n', '\r', '\t':
			total += 2
		case '<', '>', '&':
			total += 6
		default:
			if value[i] < 0x20 {
				total += 6
			} else {
				total++
			}
		}
	}
	return total
}

func estimateSignedDecimalBytes(value int64) int64 {
	if value < 0 {
		if value == -1<<63 {
			return 20
		}
		return 1 + estimateUnsignedDecimalBytes(uint64(-value))
	}
	return estimateUnsignedDecimalBytes(uint64(value))
}

func estimateUnsignedDecimalBytes(value uint64) int64 {
	var digits int64 = 1
	for value >= 10 {
		value /= 10
		digits++
	}
	return digits
}

func addEstimatedPayloadBytes(total *int64, delta int64) error {
	if total == nil {
		return nil
	}
	*total += delta
	if MaxExportPayloadBytes > 0 && *total > MaxExportPayloadBytes {
		return ExportLimitError{
			message: fmt.Sprintf("backup export exceeds max payload of %d bytes", MaxExportPayloadBytes),
		}
	}
	return nil
}

func IsKnownTable(name string) bool {
	for _, table := range AllTables {
		if table == name {
			return true
		}
	}
	return false
}

func quoteIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}
