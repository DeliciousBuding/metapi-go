package service

import (
	"encoding/json"
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

// GetAccountWithSiteByID reads an account joined with its site (no row lock).
func GetAccountWithSiteByID(db *sqlx.DB, id int64) (*AccountWithSite, error) {
	return getAccountWithSiteBy(db, id, false)
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
