package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// GetAccountByID fetches an account by its ID.
func GetAccountByID(db *sqlx.DB, id int64) (*store.Account, error) {
	var account store.Account
	err := db.Get(&account, db.Rebind("SELECT * FROM accounts WHERE id = ?"), id)
	if err != nil {
		return nil, err
	}
	return &account, nil
}

// AccountWithSite holds the account and site columns selected by the join.
type AccountWithSite struct {
	Account store.Account `db:"accounts"`
	Site    store.Site    `db:"sites"`
}

// accountQueryExecer is the minimal executor surface shared by *sqlx.DB and
// *sqlx.Tx, sufficient for the account read/write helpers below.
type accountQueryExecer interface {
	Get(dest interface{}, query string, args ...interface{}) error
	Exec(query string, args ...interface{}) (sql.Result, error)
	Rebind(query string) string
	DriverName() string
}

// GetAccountWithSiteByID reads an account joined with its site (no row lock).
func GetAccountWithSiteByID(db *sqlx.DB, id int64) (*AccountWithSite, error) {
	return getAccountWithSiteBy(db, id, false)
}

// GetAccountWithSiteByIDForUpdate reads the account joined with its site while
// locking the accounts row (PostgreSQL only: FOR UPDATE OF a; SQLite's
// single-connection pool already serializes transactions). Call it inside a
// transaction to make read-merge-write of mutable columns — notably
// extra_config — safe against concurrent lost updates.
func GetAccountWithSiteByIDForUpdate(q accountQueryExecer, id int64) (*AccountWithSite, error) {
	return getAccountWithSiteBy(q, id, true)
}

func getAccountWithSiteBy(q accountQueryExecer, id int64, forUpdate bool) (*AccountWithSite, error) {
	query := `SELECT a.id AS "accounts.id", a.site_id AS "accounts.site_id", a.username AS "accounts.username",
		a.access_token AS "accounts.access_token", a.api_token AS "accounts.api_token",
		a.balance AS "accounts.balance", a.balance_used AS "accounts.balance_used",
		a.quota AS "accounts.quota", a.unit_cost AS "accounts.unit_cost",
		a.value_score AS "accounts.value_score", a.status AS "accounts.status",
		a.is_pinned AS "accounts.is_pinned", a.sort_order AS "accounts.sort_order",
		a.checkin_enabled AS "accounts.checkin_enabled", a.last_checkin_at AS "accounts.last_checkin_at",
		a.last_balance_refresh AS "accounts.last_balance_refresh",
		a.oauth_provider AS "accounts.oauth_provider", a.oauth_account_key AS "accounts.oauth_account_key",
		a.oauth_project_id AS "accounts.oauth_project_id", a.extra_config AS "accounts.extra_config",
		a.created_at AS "accounts.created_at", a.updated_at AS "accounts.updated_at",
		a.remark AS "accounts.remark",
		s.id AS "sites.id", s.name AS "sites.name", s.url AS "sites.url",
		s.platform AS "sites.platform", s.proxy_url AS "sites.proxy_url",
		s.use_system_proxy AS "sites.use_system_proxy",
		s.custom_headers AS "sites.custom_headers",
			s.custom_headers_override_request_headers AS "sites.custom_headers_override_request_headers",
			s.status AS "sites.status"
			FROM accounts a INNER JOIN sites s ON a.site_id = s.id WHERE a.id = ?`
	if forUpdate && q.DriverName() == "pgx" {
		// Lock only the accounts row (the join also reads sites). A concurrent
		// writer blocks here until this transaction commits, so its merge reads
		// the committed extra_config instead of overwriting it.
		query += " FOR UPDATE OF a"
	}

	var row AccountWithSite
	if err := q.Get(&row, q.Rebind(query), id); err != nil {
		return nil, err
	}
	return &row, nil
}

// DeleteAccount deletes an account by ID.
func DeleteAccount(db *sqlx.DB, id int64) error {
	_, err := db.Exec(db.Rebind("DELETE FROM accounts WHERE id = ?"), id)
	return err
}

// GetNextAccountSortOrder returns the next sortOrder value for new accounts.
func GetNextAccountSortOrder(db *sqlx.DB) (int64, error) {
	var maxOrder int64
	err := db.Get(&maxOrder, db.Rebind("SELECT COALESCE(MAX(sort_order), -1) FROM accounts"))
	if err != nil {
		return 0, err
	}
	return maxOrder + 1, nil
}

// InsertAccount inserts a new account and returns the ID.
func InsertAccount(db *sqlx.DB, account map[string]any) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	sortOrder := account["sortOrder"].(int64)
	extraConfig := account["extraConfig"]
	var extraConfigStr *string
	if extraConfig != nil {
		b, err := json.Marshal(extraConfig)
		if err == nil {
			s := string(b)
			extraConfigStr = &s
		}
	}

	var id int64
	err := db.QueryRowx(
		db.Rebind(`INSERT INTO accounts (site_id, username, access_token, api_token, balance, balance_used, quota,
		 unit_cost, value_score, status, is_pinned, sort_order, checkin_enabled,
		 last_checkin_at, last_balance_refresh, oauth_provider, oauth_account_key,
		 oauth_project_id, extra_config, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 RETURNING id`),
		account["siteId"], account["username"], account["accessToken"],
		account["apiToken"], account["balance"], account["balanceUsed"],
		account["quota"], account["unitCost"], account["valueScore"],
		account["status"], account["isPinned"], sortOrder,
		account["checkinEnabled"], account["lastCheckinAt"], account["lastBalanceRefresh"],
		account["oauthProvider"], account["oauthAccountKey"], account["oauthProjectID"],
		extraConfigStr, now, now,
	).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, nil
}

// UpdateAccountFields writes a set of account columns in one UPDATE.
func UpdateAccountFields(db *sqlx.DB, accountID int64, updates map[string]any) error {
	return updateAccountFieldsOn(db, accountID, updates)
}

// UpdateAccountFieldsTx is the transaction variant of UpdateAccountFields. Use
// it within the same transaction that read-locked the account row so the
// read-merge-write of extra_config commits atomically (no lost updates).
func UpdateAccountFieldsTx(tx *sqlx.Tx, accountID int64, updates map[string]any) error {
	return updateAccountFieldsOn(tx, accountID, updates)
}

func updateAccountFieldsOn(q accountQueryExecer, accountID int64, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	var setClauses []string
	var args []any

	colMap := map[string]string{
		"accessToken":        "access_token",
		"apiToken":           "api_token",
		"username":           "username",
		"status":             "status",
		"checkinEnabled":     "checkin_enabled",
		"unitCost":           "unit_cost",
		"extraConfig":        "extra_config",
		"isPinned":           "is_pinned",
		"sortOrder":          "sort_order",
		"balance":            "balance",
		"balanceUsed":        "balance_used",
		"quota":              "quota",
		"valueScore":         "value_score",
		"lastBalanceRefresh": "last_balance_refresh",
		"remark":             "remark",
		"tags":               "tags",
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
	args = append(args, accountID)

	query := fmt.Sprintf("UPDATE accounts SET %s WHERE id = ?", strings.Join(setClauses, ", "))
	_, err := q.Exec(q.Rebind(query), args...)
	return err
}

// ListAccountsWithSites returns all accounts joined with their sites.
// Each account is enriched with nested site, capabilities, credentialMode, and
// effective runtimeHealth (expired status never reports healthy).
func ListAccountsWithSites(db *sqlx.DB) ([]map[string]any, error) {
	rows, err := db.Queryx(
		`SELECT a.*, s.id as site_id_val, s.name as site_name, s.url as site_url,
		        s.platform as site_platform, s.status as site_status
		 FROM accounts a INNER JOIN sites s ON a.site_id = s.id
		 ORDER BY a.sort_order, a.id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanAccountOverviewRows(rows)
}

// AccountListFilter narrows the paginated accounts list server-side so the
// admin UI filters the whole fleet, not just the loaded page (issue #1108:
// client-side filters over a server-paginated table could never match rows
// on other pages).
type AccountListFilter struct {
	// Statuses is a whitelisted subset of the account status domain
	// (active / disabled / expired). Empty means "no status filter".
	Statuses []string
	// SiteIDs narrows to the given sites. Empty means "all sites".
	SiteIDs []int64
	// Query is a case-insensitive substring match over username and site
	// name/url/platform (mirrors the frontend's old client-side haystack).
	// Empty means "no search filter".
	Query string
}

// ListAccountsWithSitesPaginated returns a single page of accounts joined with
// their sites, plus the total row count matching the same join (and the same
// optional filter, so `total` is the true filtered count). Enrichment is
// identical to ListAccountsWithSites so a paginated page is byte-compatible
// with a slice of the unpaginated snapshot. Used by GET /api/accounts?page=...
// to bound the admin list response when the fleet grows (defensive pagination,
// same envelope as /api/channels and /api/checkin/logs).
func ListAccountsWithSitesPaginated(db *sqlx.DB, limit, offset int, filter AccountListFilter) ([]map[string]any, int64, error) {
	where, args := buildAccountListFilter(filter)

	rows, err := db.Queryx(db.Rebind(
		`SELECT a.*, s.id as site_id_val, s.name as site_name, s.url as site_url,
		        s.platform as site_platform, s.status as site_status
		 FROM accounts a INNER JOIN sites s ON a.site_id = s.id`+where+`
		 ORDER BY a.sort_order, a.id
		 LIMIT ? OFFSET ?`), append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	result, scanErr := scanAccountOverviewRows(rows)

	var total int64
	if err := db.Get(&total, db.Rebind(
		`SELECT COUNT(*) FROM accounts a INNER JOIN sites s ON a.site_id = s.id`+where), args...); err != nil {
		return result, 0, err
	}
	return result, total, scanErr
}

func scanAccountOverviewRows(rows *sqlx.Rows) ([]map[string]any, error) {
	var result []map[string]any
	var scanErr error
	for rows.Next() {
		row := make(map[string]any)
		if err := rows.MapScan(row); err != nil {
			if scanErr == nil {
				scanErr = err
			}
			continue
		}
		normalizeMapScanValues(row)
		account := mapKeysToCamel(row)
		result = append(result, enrichAccountOverviewRow(account))
	}
	return result, scanErr
}

// buildAccountListFilter assembles the WHERE clause + args for the account
// list filter. All values are parameterized (never interpolated), and the
// query/status/site conditions are mutually ANDed.
func buildAccountListFilter(filter AccountListFilter) (string, []any) {
	var clauses []string
	var args []any
	if q := strings.TrimSpace(filter.Query); q != "" {
		pattern := "%" + escapeLikePattern(q) + "%"
		// Same haystack the frontend's client-side search used to match:
		// username + site name/platform/url. Keep the LIKE ESCAPE so a user
		// search containing % or _ matches literally.
		clauses = append(clauses, `(LOWER(a.username) LIKE LOWER(?) ESCAPE '\' OR
		 LOWER(s.name) LIKE LOWER(?) ESCAPE '\' OR
		 LOWER(s.platform) LIKE LOWER(?) ESCAPE '\' OR
		 LOWER(s.url) LIKE LOWER(?) ESCAPE '\')`)
		args = append(args, pattern, pattern, pattern, pattern)
	}
	if len(filter.Statuses) > 0 {
		query, inArgs, err := sqlx.In(`a.status IN (?)`, filter.Statuses)
		if err == nil && len(inArgs) > 0 {
			clauses = append(clauses, query)
			args = append(args, inArgs...)
		}
	}
	if len(filter.SiteIDs) > 0 {
		query, inArgs, err := sqlx.In(`a.site_id IN (?)`, filter.SiteIDs)
		if err == nil && len(inArgs) > 0 {
			clauses = append(clauses, query)
			args = append(args, inArgs...)
		}
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

// escapeLikePattern escapes LIKE wildcards (% and _) so operator search
// terms match literally.
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// enrichAccountOverviewRow attaches admin-list overview fields used by the UI.
func enrichAccountOverviewRow(account map[string]any) map[string]any {
	if account == nil {
		return account
	}

	status := asString(account["status"])
	siteStatus := asString(account["siteStatus"])
	extraConfig := asStringPtr(account["extraConfig"])
	accessToken := asString(account["accessToken"])
	oauthProvider := asStringPtr(account["oauthProvider"])

	acct := &store.Account{
		AccessToken:   accessToken,
		APIToken:      asStringPtr(account["apiToken"]),
		ExtraConfig:   extraConfig,
		OAuthProvider: oauthProvider,
	}
	credentialMode := ResolveStoredCredentialMode(acct)
	capabilities := BuildCapabilitiesFromCredentialMode(credentialMode, HasSessionToken(accessToken))
	sessionCapable := capabilities.CanRefreshBalance

	account["credentialMode"] = string(credentialMode)
	account["capabilities"] = capabilities
	account["runtimeHealth"] = BuildRuntimeHealthForAccount(RuntimeHealthInput{
		AccountStatus:  status,
		SiteStatus:     siteStatus,
		ExtraConfig:    extraConfig,
		SessionCapable: &sessionCapable,
		OAuthProvider:  oauthProvider,
	})
	account["site"] = map[string]any{
		"id":       account["siteIdVal"],
		"name":     account["siteName"],
		"url":      account["siteUrl"],
		"platform": account["sitePlatform"],
		"status":   siteStatus,
	}
	// Prefer nested site; drop flat join aliases that are not account columns.
	delete(account, "siteIdVal")
	delete(account, "siteName")
	delete(account, "siteUrl")
	delete(account, "sitePlatform")
	delete(account, "siteStatus")
	// One credential policy for every admin surface (see RedactAccountSecrets):
	// plaintext secrets never leave the process on a read or a write response.
	RedactAccountSecrets(account)
	return account
}

func normalizeMapScanValues(m map[string]any) {
	for k, v := range m {
		switch typed := v.(type) {
		case []byte:
			m[k] = string(typed)
		}
	}
}

func asString(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	case []byte:
		return string(typed)
	case fmt.Stringer:
		return typed.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
}

func asStringPtr(v any) *string {
	if v == nil {
		return nil
	}
	s := strings.TrimSpace(asString(v))
	if s == "" {
		return nil
	}
	return &s
}

// snakeToCamel converts snake_case to camelCase.
func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// mapKeysToCamel returns a new map with all keys converted from snake_case to camelCase.
func mapKeysToCamel(m map[string]any) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[snakeToCamel(k)] = v
	}
	return result
}
