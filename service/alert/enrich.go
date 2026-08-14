package alert

import (
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/jmoiron/sqlx"
)

// Enrichment presentation limits and the panel deep link appended to
// token-expired / low-balance / proxy-all-failed alert messages.
const (
	enrichMaxRoutesShown = 3
	enrichMaxSitesShown  = 2
	alertPanelLink       = "/observability?section=health"
)

// alertEnrichmentScope selects the lookup key for enrichment queries.
// Account-scoped alerts (token expired / low balance) resolve through
// route_channels.account_id; model-scoped alerts (proxy all failed) resolve
// through token_routes.model_pattern matching.
type alertEnrichmentScope struct {
	accountID *int64
	model     string
}

// siteChannelCount is one alternative site with its usable channel count.
type siteChannelCount struct {
	name  string
	count int
}

// enrichAlertMessage appends up to three context lines to base:
//
//	受影响路由: <patterns, 最多 3 条>
//	替代站点: <站点名(通道数), 最多 2 个站点>
//	面板: /observability?section=health
//
// A nil db or any query failure degrades to the original message so alert
// delivery never depends on enrichment success.
func enrichAlertMessage(db *sqlx.DB, base string, scope alertEnrichmentScope) string {
	if db == nil {
		return base
	}
	lines, err := buildEnrichmentLines(db, scope)
	if err != nil {
		return base
	}
	return base + "\n" + strings.Join(lines, "\n")
}

func buildEnrichmentLines(db *sqlx.DB, scope alertEnrichmentScope) ([]string, error) {
	routes, routeIDs, err := loadAffectedRoutes(db, scope)
	if err != nil {
		return nil, err
	}
	sites, err := loadAlternativeSites(db, scope, routeIDs)
	if err != nil {
		return nil, err
	}
	return []string{
		"受影响路由: " + formatAffectedRoutes(routes),
		"替代站点: " + formatAlternativeSites(sites),
		"面板: " + alertPanelLink,
	}, nil
}

type routeRow struct {
	ID           int64   `db:"id"`
	ModelPattern string  `db:"model_pattern"`
	DisplayName  *string `db:"display_name"`
}

// loadAffectedRoutes returns the routes touched by the failing account (all
// wired routes) or by the failing model (enabled dispatch routes whose
// pattern/display name matches). The second return value carries route ids
// so the alternative-site lookup can reuse them for model-scoped alerts.
func loadAffectedRoutes(db *sqlx.DB, scope alertEnrichmentScope) ([]string, []int64, error) {
	var rows []routeRow
	var err error
	if scope.accountID != nil {
		err = db.Select(&rows, db.Rebind(`
			SELECT DISTINCT tr.id, tr.model_pattern, tr.display_name
			FROM route_channels rc
			JOIN token_routes tr ON tr.id = rc.route_id
			WHERE rc.account_id = ?
			ORDER BY tr.id`), *scope.accountID)
	} else {
		err = db.Select(&rows, db.Rebind(`
			SELECT id, model_pattern, display_name
			FROM token_routes
			WHERE enabled = ?
			ORDER BY id`), true)
	}
	if err != nil {
		return nil, nil, err
	}

	labels := make([]string, 0, len(rows))
	routeIDs := make([]int64, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	model := strings.TrimSpace(scope.model)
	for _, row := range rows {
		if scope.accountID == nil {
			if !routing.MatchesModelPattern(model, row.ModelPattern) &&
				!routing.IsRouteDisplayNameMatch(model, row.DisplayName) {
				continue
			}
		}
		label := routing.GetExposedModelNameForRoute(row.DisplayName, row.ModelPattern)
		routeIDs = append(routeIDs, row.ID)
		if seen[label] {
			continue
		}
		seen[label] = true
		labels = append(labels, label)
	}
	return labels, routeIDs, nil
}

type altSiteRow struct {
	Name         string `db:"name"`
	ChannelCount int    `db:"channel_count"`
}

// loadAlternativeSites lists other enabled sites that can still serve the
// affected routes. Account-scoped alerts exclude the failing account's own
// site; model-scoped alerts use the matched route ids directly. Sites are
// ordered by usable channel count descending.
func loadAlternativeSites(db *sqlx.DB, scope alertEnrichmentScope, routeIDs []int64) ([]siteChannelCount, error) {
	var rows []altSiteRow
	if scope.accountID != nil {
		// The failing account's site is excluded so candidates are truly
		// "other sites"; channels must sit on a route the account serves.
		err := db.Select(&rows, db.Rebind(`
			SELECT s.name, COUNT(rc.id) AS channel_count
			FROM route_channels rc
			JOIN accounts a ON a.id = rc.account_id
			JOIN sites s ON s.id = a.site_id
			WHERE rc.enabled = ?
			  AND a.status = 'active'
			  AND s.status = 'active'
			  AND NOT EXISTS (
			      SELECT 1 FROM accounts a2
			      WHERE a2.id = ? AND a2.site_id = a.site_id
			  )
			  AND EXISTS (
			      SELECT 1 FROM route_channels rc2
			      WHERE rc2.account_id = ? AND rc2.route_id = rc.route_id
			  )
			GROUP BY s.id, s.name
			ORDER BY channel_count DESC, s.name`), true, *scope.accountID, *scope.accountID)
		if err != nil {
			return nil, err
		}
	} else if len(routeIDs) > 0 {
		query, args, err := sqlx.In(`
			SELECT s.name, COUNT(rc.id) AS channel_count
			FROM route_channels rc
			JOIN accounts a ON a.id = rc.account_id
			JOIN sites s ON s.id = a.site_id
			WHERE rc.enabled = ?
			  AND a.status = 'active'
			  AND s.status = 'active'
			  AND rc.route_id IN (?)
			GROUP BY s.id, s.name
			ORDER BY channel_count DESC, s.name`, true, routeIDs)
		if err != nil {
			return nil, err
		}
		if err := db.Select(&rows, db.Rebind(query), args...); err != nil {
			return nil, err
		}
	}

	sites := make([]siteChannelCount, 0, len(rows))
	for _, row := range rows {
		sites = append(sites, siteChannelCount{name: row.Name, count: row.ChannelCount})
	}
	return sites, nil
}

func formatAffectedRoutes(labels []string) string {
	if len(labels) == 0 {
		return "无"
	}
	shown := labels
	if len(shown) > enrichMaxRoutesShown {
		shown = shown[:enrichMaxRoutesShown]
	}
	line := strings.Join(shown, ", ")
	if len(labels) > enrichMaxRoutesShown {
		line += fmt.Sprintf(" 等 %d 条", len(labels))
	}
	return line
}

func formatAlternativeSites(sites []siteChannelCount) string {
	if len(sites) == 0 {
		return "无"
	}
	shown := sites
	if len(shown) > enrichMaxSitesShown {
		shown = shown[:enrichMaxSitesShown]
	}
	parts := make([]string, 0, len(shown))
	for _, site := range shown {
		parts = append(parts, fmt.Sprintf("%s(%d)", site.name, site.count))
	}
	line := strings.Join(parts, ", ")
	if len(sites) > enrichMaxSitesShown {
		line += fmt.Sprintf(" 等 %d 个站点", len(sites))
	}
	return line
}
