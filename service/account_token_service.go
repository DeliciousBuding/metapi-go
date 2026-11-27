package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

const (
	TokenValueStatusReady         = "ready"
	TokenValueStatusMaskedPending = "masked_pending"
)

// ---- Token value helpers ----

// NormalizeTokenForDisplay normalizes a token for display.
// Mirrors TS normalizeTokenForDisplay().
func NormalizeTokenForDisplay(token, platform string) string {
	value := strings.TrimSpace(token)
	if value == "" {
		return ""
	}
	if !strings.HasPrefix(strings.ToLower(value), "sk-") {
		return "sk-" + value
	}
	return value
}

// MaskToken masks a token value for display.
// Mirrors TS maskToken().
func MaskToken(token, platform string) string {
	value := NormalizeTokenForDisplay(token, platform)
	if value == "" {
		return ""
	}
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "sk-") {
		if len(value) <= 7 {
			return "sk-***"
		}
		visibleMiddle := value[3:min(6, len(value))]
		if len(value) <= 12 {
			return fmt.Sprintf("sk-%s***%s", visibleMiddle, value[len(value)-2:])
		}
		return fmt.Sprintf("sk-%s***%s", visibleMiddle, value[len(value)-4:])
	}
	if len(value) <= 10 {
		return fmt.Sprintf("%s***%s", value[:2], value[len(value)-2:])
	}
	return fmt.Sprintf("%s***%s", value[:4], value[len(value)-4:])
}

// IsMaskedTokenValue checks if a token value contains masking characters.
// Mirrors TS isMaskedTokenValue().
func IsMaskedTokenValue(token string) bool {
	value := strings.TrimSpace(token)
	if value == "" {
		return false
	}
	return strings.Contains(value, "*") || strings.Contains(value, "•") // bullet char
}

// ResolveAccountTokenValueStatus resolves the valueStatus for a token.
func ResolveAccountTokenValueStatus(tokenRow *store.AccountToken) string {
	if tokenRow.ValueStatus == TokenValueStatusMaskedPending {
		return TokenValueStatusMaskedPending
	}
	if IsMaskedTokenValue(tokenRow.Token) {
		return TokenValueStatusMaskedPending
	}
	return TokenValueStatusReady
}

// IsMaskedPendingAccountToken checks if a token is in masked-pending state.
func IsMaskedPendingAccountToken(token *store.AccountToken) bool {
	if token == nil {
		return false
	}
	return ResolveAccountTokenValueStatus(token) == TokenValueStatusMaskedPending
}

// IsUsableAccountToken checks if a token is usable (ready + enabled).
func IsUsableAccountToken(token *store.AccountToken) bool {
	if token == nil {
		return false
	}
	return token.Enabled && ResolveAccountTokenValueStatus(token) == TokenValueStatusReady
}

// ---- Token CRUD ----

// GetTokenByID fetches a token by its ID.
func GetTokenByID(db *sqlx.DB, id int64) (*store.AccountToken, error) {
	var token store.AccountToken
	err := db.Get(&token, db.Rebind("SELECT * FROM account_tokens WHERE id = ?"), id)
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// ListTokensWithRelations returns tokens with account and site relations.
// Mirrors TS listTokensWithRelations().
func ListTokensWithRelations(db *sqlx.DB, accountID *int64) ([]map[string]any, error) {
	var query string
	var args []any

	baseQuery := `SELECT t.id, t.account_id, t.name, t.token, t.token_group, t.value_status,
		t.source, t.enabled, t.is_default, t.created_at, t.updated_at,
		a.id as account_id_val, a.username, a.status as account_status,
		a.access_token, a.extra_config,
		s.id as site_id, s.name as site_name, s.url as site_url, s.platform as site_platform
		FROM account_tokens t
		INNER JOIN accounts a ON t.account_id = a.id
		INNER JOIN sites s ON a.site_id = s.id`

	if accountID != nil {
		query = baseQuery + " WHERE t.account_id = ? ORDER BY t.id"
		args = append(args, *accountID)
	} else {
		query = baseQuery + " ORDER BY t.id"
	}

	rows, err := db.Queryx(db.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []map[string]any
	for rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			continue
		}
		// Filter out API key connections
		if isAPIKeyConnFromRow(row) {
			continue
		}
		// MapScan exposes raw snake_case DB column names; the wire contract
		// (web/src/features/accounts/types.ts accountTokenSchema) expects
		// camelCase fields and no internal join columns. Rebuild a clean map
		// so `accountId` / `tokenGroup` / `isDefault` / `createdAt` /
		// `updatedAt` are present and `account_id`, `token_group`,
		// `is_default`, `value_status`, `account_id_val`, `site_id`, ... do
		// not leak into the response.
		tokenVal, _ := row["token"].(string)
		platform, _ := row["site_platform"].(string)
		valueStatus, _ := row["value_status"].(string)
		if IsMaskedTokenValue(tokenVal) && valueStatus != TokenValueStatusMaskedPending {
			valueStatus = TokenValueStatusMaskedPending
		}
		item := map[string]any{
			"id":          row["id"],
			"accountId":   row["account_id"],
			"name":        row["name"],
			"tokenGroup":  row["token_group"],
			"valueStatus": valueStatus,
			"source":      row["source"],
			"enabled":     rowBoolValue(row["enabled"]),
			"isDefault":   rowBoolValue(row["is_default"]),
			"createdAt":   row["created_at"],
			"updatedAt":   row["updated_at"],
			// No plain `token` key: masked form only (secret never crosses
			// the list response; same as the pre-contract-fix behavior).
			"tokenMasked": MaskToken(tokenVal, platform),
			"account": map[string]any{
				"id":       row["account_id_val"],
				"username": row["username"],
				"status":   row["account_status"],
			},
			"site": map[string]any{
				"id":       row["site_id"],
				"name":     row["site_name"],
				"url":      row["site_url"],
				"platform": row["site_platform"],
			},
		}
		result = append(result, item)
	}
	return result, nil
}

// rowBoolValue coerces a scanned boolean-ish column value (SQLite integer,
// Postgres bool, possible string/bytes) into a JSON boolean so both dialects
// emit the same wire shape for enabled/isDefault.
func rowBoolValue(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case int64:
		return b != 0
	case int:
		return b != 0
	case float64:
		return b != 0
	case string:
		return strings.EqualFold(strings.TrimSpace(b), "true") || b == "1"
	case []byte:
		return rowBoolValue(string(b))
	default:
		return false
	}
}

func isAPIKeyConnFromRow(row map[string]any) bool {
	accessToken, _ := row["access_token"].(string)
	if strings.TrimSpace(accessToken) != "" {
		return false
	}
	extraConfig, _ := row["extra_config"].(string)
	return GetCredentialModeFromExtraConfig(&extraConfig) == CredentialModeAPIKey
}

// DeleteTokenByID deletes a token by ID with upstream-first strategy.
// The caller must handle the upstream deletion before calling this.
func DeleteTokenByID(db *sqlx.DB, tokenID int64) error {
	token, err := GetTokenByID(db, tokenID)
	if err != nil {
		return fmt.Errorf("token not found")
	}

	_, err = db.Exec(db.Rebind("DELETE FROM account_tokens WHERE id = ?"), tokenID)
	if err != nil {
		return err
	}

	// If this was the default, repair
	if token.IsDefault {
		if _, err := RepairDefaultToken(db, token.AccountID); err != nil {
			return err
		}
	}

	return nil
}

// UpdateTokenFields updates specific fields on a token.
func UpdateTokenFields(db *sqlx.DB, tokenID int64, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var setClauses []string
	var args []any

	colMap := map[string]string{
		"name":        "name",
		"token":       "token",
		"tokenGroup":  "token_group",
		"valueStatus": "value_status",
		"source":      "source",
		"enabled":     "enabled",
		"isDefault":   "is_default",
	}

	for key, val := range updates {
		if col, ok := colMap[key]; ok {
			setClauses = append(setClauses, col+" = ?")
			args = append(args, val)
		}
	}

	if len(setClauses) == 0 {
		return nil
	}

	setClauses = append(setClauses, "updated_at = ?")
	args = append(args, now)
	args = append(args, tokenID)

	query := fmt.Sprintf("UPDATE account_tokens SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	_, err := db.Exec(db.Rebind(query), args...)
	return err
}
