package admin

import (
	"fmt"
	"math"
	"strings"
	"time"

	backupsvc "github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/store"
)

var (
	backupImportMaxRowsPerTable  = 50_000
	backupImportMaxColumnsPerRow = 128
	backupImportMaxCellBytes     = 4 << 20
)

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

// validateBackupImportRow applies per-table value checks that the generic
// scalar validator cannot express. A backup is untrusted input: quotas are
// summed into admission decisions (negative ledger rows cancel real usage),
// and rows that omit NOT NULL-ish time columns used to import "successfully"
// and then break the list endpoints with a NULL scan error.
func validateBackupImportRow(table string, row map[string]any) error {
	if backupsvc.IsLocalUserResource(table, row) {
		return backupImportClientError{message: "user-owned files/video tasks require an identity-preserving full database restore"}
	}
	if table == "downstream_api_keys" {
		ownerType, present := row["key_owner_type"]
		if (present && ownerType != "system") || row["owner_user_id"] != nil {
			return backupImportClientError{message: "user-owned Keys require an identity-preserving full database restore"}
		}
	}
	if (table == "gateway_requests" || table == "proxy_logs") && (row["owner_user_id"] != nil || row["user_group_id"] != nil) {
		return backupImportClientError{message: "user-attributed records require an identity-preserving full database restore"}
	}
	switch table {
	case "credential_refresh_families", "credential_refresh_aliases":
		if err := store.NormalizeCredentialRefreshJournalRow(table, row); err != nil {
			return backupImportClientError{message: "invalid credential refresh journal"}
		}
	case "upstream_channels":
		if raw, present := row["request_config"]; present {
			config := store.DirectRequestConfig{}
			if err := config.Scan(raw); err != nil {
				return backupImportClientError{message: "invalid upstream request configuration"}
			}
			if config.Active() {
				var endpoints store.DirectEndpoints
				if err := endpoints.Scan(row["endpoint_config"]); err != nil || config.ValidateEndpoints(endpoints) != nil {
					return backupImportClientError{message: "request configuration is incompatible with upstream endpoints"}
				}
			}
		}
	case "downstream_quota_usage":
		for _, column := range []string{"requests", "total_tokens", "cost"} {
			value, ok := row[column]
			if !ok || value == nil {
				continue
			}
			n, ok := value.(float64)
			if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
				return backupImportClientError{message: fmt.Sprintf("column %q must be a non-negative number", column)}
			}
		}
		if value, ok := row["occurred_at"]; ok && value != nil {
			n, ok := value.(float64)
			// 0 < occurred_at < 2100-01-01 (ms). Anything else is a poisoned
			// ledger row that would land outside every quota window.
			if !ok || n <= 0 || n > 4102444800000 {
				return backupImportClientError{message: `column "occurred_at" must be a plausible millisecond timestamp`}
			}
		}
	case "sites":
		for _, column := range []string{"name", "url", "platform"} {
			if value, ok := row[column].(string); !ok || strings.TrimSpace(value) == "" {
				return backupImportClientError{message: fmt.Sprintf("column %q is required for table sites", column)}
			}
		}
		// created_at/updated_at are scanned into non-nullable Go strings by the
		// sites list. Older exports may omit them; fill them rather than
		// importing a row that returns 500 on the next read.
		now := time.Now().UTC().Format(time.RFC3339)
		for _, column := range []string{"created_at", "updated_at"} {
			if value, ok := row[column].(string); !ok || strings.TrimSpace(value) == "" {
				row[column] = now
			}
		}
	}
	return nil
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

// coerceBackupBoolCellValue normalizes SQLite-exported integer booleans (0/1)
// into native Go bool values when importing into a target BOOLEAN column
// (required for PostgreSQL pgx binary encoding of OID 16).
func coerceBackupBoolCellValue(column string, value any) (any, error) {
	switch v := value.(type) {
	case nil, bool:
		return v, nil
	case float64:
		if v == 0 {
			return false, nil
		}
		if v == 1 {
			return true, nil
		}
		return nil, backupImportClientError{message: fmt.Sprintf("column %q expects a boolean value", column)}
	case int:
		if v == 0 {
			return false, nil
		}
		if v == 1 {
			return true, nil
		}
		return nil, backupImportClientError{message: fmt.Sprintf("column %q expects a boolean value", column)}
	case int64:
		if v == 0 {
			return false, nil
		}
		if v == 1 {
			return true, nil
		}
		return nil, backupImportClientError{message: fmt.Sprintf("column %q expects a boolean value", column)}
	default:
		return nil, backupImportClientError{message: fmt.Sprintf("column %q expects a boolean value", column)}
	}
}
