package oauth

import (
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// OauthConnectionItem represents an OAuth connection in list responses.
type OauthConnectionItem struct {
	AccountID          int64               `json:"accountId"`
	SiteID             int64               `json:"siteId"`
	Provider           string              `json:"provider"`
	Username           string              `json:"username"`
	Email              string              `json:"email"`
	AccountKey         string              `json:"accountKey"`
	PlanType           string              `json:"planType,omitempty"`
	ProjectID          string              `json:"projectId,omitempty"`
	ModelCount         int                 `json:"modelCount"`
	ModelsPreview      []string            `json:"modelsPreview"`
	Quota              *OauthQuotaSnapshot `json:"quota,omitempty"`
	Status             string              `json:"status"`
	RouteChannelCount  int                 `json:"routeChannelCount"`
	LastModelSyncAt    string              `json:"lastModelSyncAt,omitempty"`
	LastModelSyncError string              `json:"lastModelSyncError,omitempty"`
	ProxyURL           string              `json:"proxyUrl,omitempty"`
	UseSystemProxy     bool                `json:"useSystemProxy"`
	RouteParticipation *RouteParticipation `json:"routeParticipation,omitempty"`
	Site               *ConnectionSite     `json:"site,omitempty"`
}

// RouteParticipation represents a route unit participation.
type RouteParticipation struct {
	Kind        string `json:"kind"`
	ID          int64  `json:"id"`
	RouteUnitID int64  `json:"routeUnitId"`
	Name        string `json:"name"`
	Strategy    string `json:"strategy"`
	MemberCount int64  `json:"memberCount"`
}

// ConnectionSite holds site info for a connection.
type ConnectionSite struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	Platform string `json:"platform"`
}

// ListConnectionsInput holds input for listing OAuth connections.
type ListConnectionsInput struct {
	Limit  int
	Offset int
}

// ListConnectionsResult holds the result of listing connections.
type ListConnectionsResult struct {
	Items  []OauthConnectionItem `json:"items"`
	Total  int64                 `json:"total"`
	Limit  int                   `json:"limit"`
	Offset int                   `json:"offset"`
}

// ListOauthConnections lists OAuth connections with pagination.
func ListOauthConnections(db *store.DB, input ListConnectionsInput) (*ListConnectionsResult, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	limit := input.Limit
	if limit <= 0 {
		limit = 50
	}
	limit = config.ClampInt(limit, 1, 200)
	offset := config.MaxInt(input.Offset, 0)

	// OAuth identity backfill is a one-time startup migration (see
	// RunOauthIdentityBackfillOnce, invoked from app startup). It must not
	// run on every paginated list request: the original implementation did a
	// full-table scan + N+1 UPDATE per page load on this hot path.

	// `IS NOT NULL AND != ''`: session-imported accounts carry an empty-string
	// provider, which GetOauthInfoFromAccount rejects — counting them would
	// report a fleet total the page can never list ("total: 6" over an empty
	// table).
	var total int64
	if err := db.Get(&total, "SELECT COUNT(*) FROM accounts WHERE oauth_provider IS NOT NULL AND oauth_provider != ''"); err != nil {
		return nil, fmt.Errorf("count oauth accounts: %w", err)
	}

	rows, err := selectOAuthAccountSiteRows(db, "ORDER BY a.id DESC LIMIT ? OFFSET ?", limit, offset)
	if err != nil {
		return nil, err
	}

	accountIDs := make([]int64, len(rows))
	for i, row := range rows {
		accountIDs[i] = row.Account.ID
	}

	if len(accountIDs) == 0 {
		return &ListConnectionsResult{Items: []OauthConnectionItem{}, Total: total, Limit: limit, Offset: offset}, nil
	}

	// Get route unit participation.
	routeUnits := ListOauthRouteUnitsByAccountIDs(db, accountIDs)

	items := make([]OauthConnectionItem, 0, len(rows))
	for _, row := range rows {
		oauth := GetOauthInfoFromAccount(&row.Account)
		if oauth == nil {
			continue
		}

		status := "healthy"
		if oauth.ModelDiscoveryStatus == OauthModelDiscoveryAbnormal ||
			row.Account.Status != "active" ||
			row.SiteStatus != "active" {
			status = "abnormal"
		}

		item := OauthConnectionItem{
			AccountID:          row.Account.ID,
			SiteID:             row.Account.SiteID,
			Provider:           oauth.Provider,
			Username:           strPtr(row.Account.Username),
			Email:              oauth.Email,
			AccountKey:         oauth.AccountKey,
			PlanType:           oauth.PlanType,
			ProjectID:          oauth.ProjectID,
			Quota:              oauth.Quota,
			Status:             status,
			LastModelSyncAt:    oauth.LastModelSyncAt,
			LastModelSyncError: oauth.LastModelSyncError,
			ProxyURL:           GetProxyURLFromExtraConfig(row.Account.ExtraConfig),
			UseSystemProxy:     GetUseSystemProxyFromExtraConfig(row.Account.ExtraConfig),
			Site: &ConnectionSite{
				ID:       row.Account.SiteID,
				Name:     row.SiteName,
				URL:      row.SiteURL,
				Platform: row.SitePlatform,
			},
		}

		if ru, ok := routeUnits[row.Account.ID]; ok {
			item.RouteParticipation = &RouteParticipation{
				Kind:        "route_unit",
				ID:          ru.ID,
				RouteUnitID: ru.ID,
				Name:        ru.Name,
				Strategy:    string(ru.Strategy),
				MemberCount: ru.MemberCount,
			}
		}

		items = append(items, item)
	}

	return &ListConnectionsResult{
		Items:  items,
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func strPtr(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}
