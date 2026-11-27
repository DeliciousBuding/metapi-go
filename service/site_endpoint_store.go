package service

import (
	"fmt"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// UpsertSiteAPIEndpoints replaces all apiEndpoints for a site within a transaction.
func UpsertSiteAPIEndpoints(tx *sqlx.Tx, siteID int64, endpoints []store.SiteAPIEndpoint) error {
	now := time.Now().UTC().Format(time.RFC3339)

	// Delete existing endpoints
	_, err := tx.Exec(tx.Rebind("DELETE FROM site_api_endpoints WHERE site_id = ?"), siteID)
	if err != nil {
		return fmt.Errorf("delete site_api_endpoints: %w", err)
	}

	// Insert new endpoints
	for i := range endpoints {
		ep := &endpoints[i]
		normalizedURL := NormalizeSiteAPIEndpointBaseUrl(ep.URL)
		if normalizedURL == "" {
			continue
		}
		if IsForbiddenSiteTargetURL(normalizedURL) {
			return fmt.Errorf("site api endpoint url rejects cloud metadata / link-local targets")
		}
		enabled := true
		if !ep.Enabled {
			enabled = false
		}
		sortOrder := ep.SortOrder
		if sortOrder == 0 && i > 0 {
			sortOrder = int64(i)
		}
		_, err := tx.Exec(
			tx.Rebind(`INSERT INTO site_api_endpoints (site_id, url, enabled, sort_order, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?)`),
			siteID, normalizedURL, enabled, sortOrder, now, now,
		)
		if err != nil {
			return fmt.Errorf("insert site_api_endpoint: %w", err)
		}
	}
	return nil
}

// LoadSiteAPIEndpoints loads all api endpoints for a list of site IDs.
func LoadSiteAPIEndpoints(db *sqlx.DB, siteIDs []int64) (map[int64][]store.SiteAPIEndpoint, error) {
	result := make(map[int64][]store.SiteAPIEndpoint)
	if len(siteIDs) == 0 {
		return result, nil
	}

	query, args, err := sqlx.In(
		`SELECT id, site_id, url, enabled, sort_order, cooldown_until,
		        last_selected_at, last_failed_at, last_failure_reason, created_at, updated_at
		 FROM site_api_endpoints
		 WHERE site_id IN (?)
		 ORDER BY site_id, sort_order, id`,
		siteIDs,
	)
	if err != nil {
		return nil, err
	}
	query = db.Rebind(query)

	var rows []store.SiteAPIEndpoint
	if err := db.Select(&rows, query, args...); err != nil {
		return nil, err
	}

	for _, row := range rows {
		result[row.SiteID] = append(result[row.SiteID], row)
	}
	return result, nil
}
