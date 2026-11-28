package service

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

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
