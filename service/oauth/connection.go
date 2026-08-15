package oauth

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
func ListOauthConnections(input ListConnectionsInput) (*ListConnectionsResult, error) {
	db := store.GetDB()
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

	var total int64
	if err := db.Get(&total, "SELECT COUNT(*) FROM accounts WHERE oauth_provider IS NOT NULL"); err != nil {
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
	routeUnits := ListOauthRouteUnitsByAccountIDs(accountIDs)

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

// DeleteOauthConnection deletes an OAuth connection.
func DeleteOauthConnection(accountID int64) error {
	db := store.GetDB()
	if db == nil {
		return fmt.Errorf("database not initialized")
	}

	var account store.Account
	if err := db.Get(&account, "SELECT * FROM accounts WHERE id = ?", accountID); err != nil {
		return fmt.Errorf("oauth account not found")
	}

	oauth := GetOauthInfoFromAccount(&account)
	if oauth == nil {
		return fmt.Errorf("account is not managed by oauth")
	}

	_, err := db.Exec("DELETE FROM accounts WHERE id = ?", accountID)
	if err != nil {
		return err
	}

	// Rebuild routes after deletion.
	hooks := getWorkflowHooks()
	if hooks != nil {
		if rebuildErr := hooks.RebuildRoutesOnly(context.Background()); rebuildErr != nil {
			slog.Warn("route rebuild failed after deleting connection", "accountID", accountID, "error", rebuildErr)
		}
		hooks.InvalidateTokenRouterCache()
	}

	return nil
}

// UpdateOauthConnectionProxySettings updates proxy settings for an OAuth connection.
func UpdateOauthConnectionProxySettings(accountID int64, proxyURL *string, useSystemProxy *bool) (*UpdateProxyResult, error) {
	db := store.GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var account store.Account
	if err := db.Get(&account, "SELECT * FROM accounts WHERE id = ?", accountID); err != nil {
		return nil, fmt.Errorf("oauth account not found")
	}

	oauth := GetOauthInfoFromAccount(&account)
	if oauth == nil {
		return nil, fmt.Errorf("account is not managed by oauth")
	}

	patch := make(map[string]interface{})
	if proxyURL != nil {
		if *proxyURL != "" {
			patch["proxyUrl"] = *proxyURL
		} else {
			patch["proxyUrl"] = nil
		}
	}
	if useSystemProxy != nil {
		patch["useSystemProxy"] = *useSystemProxy
	}

	extraConfig := MergeAccountExtraConfig(account.ExtraConfig, patch)
	now := time.Now().Format(time.RFC3339)

	_, err := db.Exec("UPDATE accounts SET extra_config = ?, updated_at = ? WHERE id = ?",
		extraConfig, now, accountID)
	if err != nil {
		return nil, err
	}

	// Refresh models for account (allowInactive=true) and rebuild routes.
	modelRefreshStatus := "success"
	var modelRefreshErrMsg string
	hooks := getWorkflowHooks()
	if hooks != nil {
		if refreshErr := hooks.RefreshModelsForAccount(context.Background(), accountID, true); refreshErr != nil {
			modelRefreshStatus = "error"
			modelRefreshErrMsg = refreshErr.Error()
			slog.Warn("model refresh failed in UpdateProxySettings", "accountID", accountID, "error", refreshErr)
		}
		if rebuildErr := hooks.RebuildRoutesOnly(context.Background()); rebuildErr != nil {
			slog.Warn("route rebuild failed in UpdateProxySettings", "accountID", accountID, "error", rebuildErr)
		}
		hooks.InvalidateTokenRouterCache()
	}

	return &UpdateProxyResult{
		Success:         true,
		AccountID:       accountID,
		ProxyURL:        GetProxyURLFromExtraConfig(extraConfig),
		UseSystemProxy:  GetUseSystemProxyFromExtraConfig(extraConfig),
		RefreshedRoutes: true,
		ModelRefresh: ModelRefreshResult{
			Success:      modelRefreshStatus == "success",
			Status:       modelRefreshStatus,
			ErrorMessage: modelRefreshErrMsg,
		},
	}, nil
}

// UpdateProxyResult holds the result of updating proxy settings.
type UpdateProxyResult struct {
	Success         bool               `json:"success"`
	AccountID       int64              `json:"accountId"`
	ProxyURL        string             `json:"proxyUrl"`
	UseSystemProxy  bool               `json:"useSystemProxy"`
	RefreshedRoutes bool               `json:"refreshedRoutes"`
	ModelRefresh    ModelRefreshResult `json:"modelRefresh"`
}

// ModelRefreshResult holds model refresh status.
type ModelRefreshResult struct {
	Success      bool   `json:"success"`
	Status       string `json:"status"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// StartOauthRebindFlow starts a rebind flow for an existing OAuth account.
func StartOauthRebindFlow(accountID int64, requestOrigin string, proxyURL *string, useSystemProxy *bool) (*FlowStartResult, error) {
	db := store.GetDB()
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var account store.Account
	if err := db.Get(&account, "SELECT * FROM accounts WHERE id = ?", accountID); err != nil {
		return nil, fmt.Errorf("oauth account not found")
	}

	oauth := GetOauthInfoFromAccount(&account)
	if oauth == nil {
		return nil, fmt.Errorf("account is not managed by oauth")
	}

	resolvedProxyURL := ""
	if proxyURL != nil {
		resolvedProxyURL = *proxyURL
	} else {
		resolvedProxyURL = GetProxyURLFromExtraConfig(account.ExtraConfig)
	}

	resolvedUseSystemProxy := false
	if useSystemProxy != nil {
		resolvedUseSystemProxy = *useSystemProxy
	} else {
		resolvedUseSystemProxy = GetUseSystemProxyFromExtraConfig(account.ExtraConfig)
	}

	return StartFlow(StartFlowInput{
		Provider:        oauth.Provider,
		RebindAccountID: accountID,
		ProjectID:       oauth.ProjectID,
		ProxyURL:        resolvedProxyURL,
		UseSystemProxy:  resolvedUseSystemProxy,
		RequestOrigin:   requestOrigin,
	})
}

// ---- Helpers ----

func strPtr(s *string) string {
	if s == nil {
		return ""
	}
	return strings.TrimSpace(*s)
}

// oauthIdentityBackfillMarkerKey is the settings KV flag that marks the
// one-time OAuth identity column backfill as complete. Once set, a healthy
// instance never re-scans the accounts table for backfill candidates.
const oauthIdentityBackfillMarkerKey = "oauth.identity_backfill_complete"

// RunOauthIdentityBackfillOnce is a one-time startup migration that backfills
// the oauth_provider, oauth_account_key, and oauth_project_id columns from
// extraConfig.oauth for accounts that still carry the data only in extra_config.
//
// It is bounded (LIMIT-batched) and gated behind a settings marker so a healthy
// instance never re-scans accounts after the first successful run. A partial
// run (batch error) leaves the marker unset and retries on the next boot.
//
// This was previously invoked from ListOauthConnections on every page load,
// which forced a full-table scan + N+1 UPDATEs per request on the admin list
// hot path. Moving it to startup keeps the list endpoint O(limit).
func RunOauthIdentityBackfillOnce(db *store.DB) {
	settings := store.NewSettingsStore(db)
	if done, _ := settings.Get(oauthIdentityBackfillMarkerKey); strings.TrimSpace(done) == "1" {
		return
	}

	for {
		backfilled, err := backfillOauthIdentityBatch(db, 100)
		if err != nil {
			slog.Warn("oauth identity backfill batch failed; will retry on next boot", "error", err)
			return
		}
		if backfilled == 0 {
			break
		}
	}

	if err := settings.Set(oauthIdentityBackfillMarkerKey, "1"); err != nil {
		slog.Warn("oauth identity backfill marker could not be set; will re-run on next boot", "error", err)
		return
	}
	slog.Info("oauth identity backfill complete; settings marker set")
}

// backfillOauthIdentityBatch scans up to limit accounts whose oauth identity
// columns are still NULL/empty while extra_config carries oauth data, and
// updates them in place. Returns the number of rows updated. Row-level scan
// and update failures are logged and skipped so a single bad row cannot
// abort the whole migration; a query/scan failure (the whole batch) is
// returned so the caller retries on the next boot.
func backfillOauthIdentityBatch(db *store.DB, limit int) (int, error) {
	rows, err := db.Queryx(
		`SELECT id, oauth_provider, oauth_account_key, oauth_project_id, extra_config
		 FROM accounts
		 WHERE extra_config IS NOT NULL AND extra_config != ''
		   AND (oauth_provider IS NULL OR oauth_provider = ''
		        OR oauth_account_key IS NULL OR oauth_account_key = ''
		        OR oauth_project_id IS NULL OR oauth_project_id = '')
		 LIMIT ?`, limit)
	if err != nil {
		return 0, fmt.Errorf("scan oauth identity backfill candidates: %w", err)
	}
	defer rows.Close()

	type backfillRow struct {
		ID              int64   `db:"id"`
		OAuthProvider   *string `db:"oauth_provider"`
		OAuthAccountKey *string `db:"oauth_account_key"`
		OAuthProjectID  *string `db:"oauth_project_id"`
		ExtraConfig     *string `db:"extra_config"`
	}

	updated := 0
	for rows.Next() {
		var row backfillRow
		if err := rows.StructScan(&row); err != nil {
			slog.Warn("oauth identity backfill: row scan failed; skipping", "accountID", row.ID, "error", err)
			continue
		}

		oauth := GetOauthInfoFromExtraConfig(row.ExtraConfig)
		if oauth == nil {
			continue
		}

		provider := ""
		accountKey := ""
		projectID := ""
		needsUpdate := false
		if (row.OAuthProvider == nil || *row.OAuthProvider == "") && oauth.Provider != "" {
			provider = oauth.Provider
			needsUpdate = true
		}
		if (row.OAuthAccountKey == nil || *row.OAuthAccountKey == "") && oauth.AccountKey != "" {
			accountKey = oauth.AccountKey
			needsUpdate = true
		}
		if (row.OAuthProjectID == nil || *row.OAuthProjectID == "") && oauth.ProjectID != "" {
			projectID = oauth.ProjectID
			needsUpdate = true
		}
		if !needsUpdate {
			continue
		}

		now := time.Now().Format(time.RFC3339)
		if _, err := db.Exec(
			`UPDATE accounts SET oauth_provider = COALESCE(NULLIF(?, ''), oauth_provider),
			 oauth_account_key = COALESCE(NULLIF(?, ''), oauth_account_key),
			 oauth_project_id = COALESCE(NULLIF(?, ''), oauth_project_id),
			 updated_at = ?
			 WHERE id = ?`,
			provider, accountKey, projectID, now, row.ID); err != nil {
			slog.Warn("oauth identity backfill: update failed for account", "accountID", row.ID, "error", err)
			continue
		}
		updated++
	}
	return updated, nil
}
