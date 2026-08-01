package oauth

import (
	"fmt"

	"github.com/tokendancelab/metapi-go/store"
)

// oauthAccountColumns is the explicit account projection shared by OAuth
// connection listing and refresh scans. Keeping SELECT * out of this path
// prevents schema additions or duplicate join column names from breaking
// sqlx struct scans.
const oauthAccountColumns = `a.id, a.site_id, a.username, a.access_token, a.api_token,
	a.balance, a.balance_used, a.quota, a.unit_cost, a.value_score, a.status,
	a.is_pinned, a.sort_order, a.checkin_enabled, a.last_checkin_at,
	a.last_balance_refresh, a.oauth_provider, a.oauth_account_key,
	a.oauth_project_id, a.extra_config, a.created_at, a.updated_at, a.tags`

type oauthAccountSiteRow struct {
	store.Account
	SiteName     string `db:"site_name"`
	SiteURL      string `db:"site_url"`
	SitePlatform string `db:"site_platform"`
	SiteStatus   string `db:"site_status"`
}

func selectOAuthAccountSiteRows(db *store.DB, suffix string, args ...any) ([]oauthAccountSiteRow, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	query := `SELECT ` + oauthAccountColumns + `,
			s.name AS site_name, s.url AS site_url,
			s.platform AS site_platform, s.status AS site_status
		FROM accounts a
		INNER JOIN sites s ON a.site_id = s.id
		WHERE a.oauth_provider IS NOT NULL`
	if suffix != "" {
		query += " " + suffix
	}
	var rows []oauthAccountSiteRow
	if err := db.Select(&rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}

// OAuthRefreshCandidate contains only the state the scheduler needs while
// keeping SQL ownership inside the OAuth service.
type OAuthRefreshCandidate struct {
	Account    store.Account
	SiteStatus string
}

// ListOAuthRefreshCandidates returns all OAuth accounts and their parent-site
// status using the same explicit projection as the admin connection list.
func ListOAuthRefreshCandidates() ([]OAuthRefreshCandidate, error) {
	rows, err := selectOAuthAccountSiteRows(store.GetDB(), "")
	if err != nil {
		return nil, err
	}
	candidates := make([]OAuthRefreshCandidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, OAuthRefreshCandidate{
			Account:    row.Account,
			SiteStatus: row.SiteStatus,
		})
	}
	return candidates, nil
}
