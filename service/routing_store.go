// Package service hosts the SQL-backed routing store that feeds the token
// router (moved from app/proxy_upstream.go so the app layer stays pure
// wiring). The channel/account/site join columns used by several loaders are
// kept in the channelAccountSiteSelect fragment so a store schema change is a
// one-line edit here instead of a silent drift across three queries.
package service

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

// Compile-time assertions: the routing store satisfies the interfaces the
// token router / route decision service consume.
var (
	_ routing.ChannelSelectorDB = (*ProxyRoutingStore)(nil)
	_ routing.DecisionDB        = (*ProxyRoutingStore)(nil)
	_ routing.SnapshotDB        = (*ProxyRoutingStore)(nil)
)

// channelAccountSiteSelect is the shared account + site column fragment used
// by the route-channel and oauth-route-unit-member joins. Keep in sync with
// store.Account and store.Site.
const channelAccountSiteSelect = `
		a.id, a.site_id, a.username, a.access_token, a.api_token, a.balance, a.balance_used,
		a.quota, a.unit_cost, a.value_score, a.status, a.is_pinned, a.sort_order,
		a.checkin_enabled, a.last_checkin_at, a.last_balance_refresh, a.oauth_provider,
		a.oauth_account_key, a.oauth_project_id, a.extra_config, a.created_at, a.updated_at,
		s.id, s.name, s.url, s.external_checkin_url, s.platform, s.proxy_url, s.use_system_proxy,
		s.custom_headers, s.status, s.is_pinned, s.sort_order, s.global_weight, s.api_key,
		s.post_refresh_probe_enabled, s.post_refresh_probe_model, s.post_refresh_probe_scope,
		s.post_refresh_probe_latency_threshold_ms, s.created_at, s.updated_at`

type ProxyRoutingStore struct {
	db *store.DB
}

func NewProxyRoutingStore(db *store.DB) *ProxyRoutingStore {
	return &ProxyRoutingStore{db: db}
}

func (s *ProxyRoutingStore) LoadEnabledRoutes(ctx context.Context) ([]store.TokenRoute, error) {
	var routes []store.TokenRoute
	err := s.selectContext(ctx, &routes, `
		SELECT id, model_pattern, display_name, display_icon, route_mode, model_mapping,
		       decision_snapshot, decision_refreshed_at, routing_strategy, context_length,
		       sort_order, enabled, created_at, updated_at
		FROM token_routes
		WHERE enabled = ?
		ORDER BY sort_order ASC, id ASC`, true)
	return routes, err
}

func (s *ProxyRoutingStore) FindAllEnabledRoutes(ctx context.Context) ([]store.TokenRoute, error) {
	return s.LoadEnabledRoutes(ctx)
}

func (s *ProxyRoutingStore) LoadRouteGroupSources(ctx context.Context, groupRouteIDs []int64) (map[int64][]int64, error) {
	result := make(map[int64][]int64)
	if len(groupRouteIDs) == 0 {
		return result, nil
	}
	query, args, err := sqlx.In(`
		SELECT group_route_id, source_route_id
		FROM route_group_sources
		WHERE group_route_id IN (?)
		ORDER BY group_route_id ASC, source_route_id ASC`, groupRouteIDs)
	if err != nil {
		return nil, err
	}
	rows, err := s.queryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var groupID, sourceID int64
		if err := rows.Scan(&groupID, &sourceID); err != nil {
			return nil, err
		}
		result[groupID] = append(result[groupID], sourceID)
	}
	return result, rows.Err()
}

func (s *ProxyRoutingStore) LoadRouteChannels(ctx context.Context, routeIDs []int64) ([]struct {
	Channel store.RouteChannel
	Account store.Account
	Site    store.Site
	Token   *store.AccountToken
}, error) {
	var result []struct {
		Channel store.RouteChannel
		Account store.Account
		Site    store.Site
		Token   *store.AccountToken
	}
	if len(routeIDs) == 0 {
		return result, nil
	}
	query, args, err := sqlx.In(`
		SELECT
			rc.id, rc.route_id, rc.account_id, rc.token_id, rc.oauth_route_unit_id, rc.source_model,
			rc.priority, rc.weight, rc.enabled, rc.manual_override, rc.success_count, rc.fail_count,
			rc.total_latency_ms, rc.total_cost, rc.last_used_at, rc.last_selected_at, rc.last_fail_at,
			rc.consecutive_fail_count, rc.cooldown_level, rc.cooldown_until,
			rc.cooldown_reason_code, rc.cooldown_reason, rc.cooldown_reason_at,`+channelAccountSiteSelect+`,
			at.id, at.account_id, at.name, at.token, at.token_group, at.value_status, at.source,
			at.enabled, at.is_default, at.created_at, at.updated_at
		FROM route_channels rc
		JOIN accounts a ON a.id = rc.account_id
		JOIN sites s ON s.id = a.site_id
		LEFT JOIN account_tokens at ON at.id = rc.token_id
		WHERE rc.route_id IN (?)
		ORDER BY rc.priority DESC, rc.id ASC`, routeIDs)
	if err != nil {
		return nil, err
	}
	rows, err := s.queryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		row, err := scanRouteChannelJoin(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func (s *ProxyRoutingStore) LoadOAuthRouteUnitSummaries(ctx context.Context, unitIDs []int64) (map[int64]routing.OAuthRouteUnitSummary, error) {
	result := make(map[int64]routing.OAuthRouteUnitSummary)
	if len(unitIDs) == 0 {
		return result, nil
	}
	query, args, err := sqlx.In(`
		SELECT id, site_id, provider, name, strategy, enabled
		FROM oauth_route_units
		WHERE id IN (?)`, unitIDs)
	if err != nil {
		return nil, err
	}
	rows, err := s.queryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var unit routing.OAuthRouteUnitSummary
		if err := rows.Scan(&unit.ID, &unit.SiteID, &unit.Provider, &unit.Name, &unit.Strategy, &unit.Enabled); err != nil {
			return nil, err
		}
		result[unit.ID] = unit
	}
	return result, rows.Err()
}

func (s *ProxyRoutingStore) LoadOAuthRouteUnitMembers(ctx context.Context, unitIDs []int64) (map[int64][]routing.OAuthRouteUnitMemberCandidate, error) {
	result := make(map[int64][]routing.OAuthRouteUnitMemberCandidate)
	if len(unitIDs) == 0 {
		return result, nil
	}
	query, args, err := sqlx.In(`
		SELECT
			m.id, m.unit_id, m.account_id, m.sort_order, m.success_count, m.fail_count,
			m.total_latency_ms, m.total_cost, m.last_used_at, m.last_selected_at, m.last_fail_at,
			m.consecutive_fail_count, m.cooldown_level, m.cooldown_until,
			m.cooldown_reason_code, m.cooldown_reason, m.cooldown_reason_at, m.created_at, m.updated_at,`+channelAccountSiteSelect+`
		FROM oauth_route_unit_members m
		JOIN accounts a ON a.id = m.account_id
		JOIN sites s ON s.id = a.site_id
		WHERE m.unit_id IN (?)
		ORDER BY m.unit_id ASC, m.sort_order ASC, m.id ASC`, unitIDs)
	if err != nil {
		return nil, err
	}
	rows, err := s.queryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		member, account, site, err := scanRouteUnitMemberJoin(rows)
		if err != nil {
			return nil, err
		}
		result[member.UnitID] = append(result[member.UnitID], routing.OAuthRouteUnitMemberCandidate{
			Member:  member,
			Account: account,
			Site:    site,
		})
	}
	return result, rows.Err()
}

func (s *ProxyRoutingStore) UpdateChannelLastSelectedAt(ctx context.Context, channelID int64, lastSelectedAt string) error {
	_, err := s.execContext(ctx, `UPDATE route_channels SET last_selected_at = ? WHERE id = ?`, lastSelectedAt, channelID)
	return err
}

func (s *ProxyRoutingStore) UpdateRouteUnitMemberLastSelectedAt(ctx context.Context, unitID, accountID int64, lastSelectedAt string) error {
	_, err := s.execContext(ctx, `UPDATE oauth_route_unit_members SET last_selected_at = ?, updated_at = ? WHERE unit_id = ? AND account_id = ?`, lastSelectedAt, lastSelectedAt, unitID, accountID)
	return err
}

func (s *ProxyRoutingStore) FindRouteIDsByOAuthRouteUnitID(ctx context.Context, unitID int64) ([]int64, error) {
	var ids []int64
	err := s.selectContext(ctx, &ids, `SELECT route_id FROM route_channels WHERE oauth_route_unit_id = ?`, unitID)
	return ids, err
}

func (s *ProxyRoutingStore) LoadCredentialScopedChannelIDs(ctx context.Context, channel store.RouteChannel, accountID int64) ([]int64, error) {
	var ids []int64
	if channel.TokenID != nil && *channel.TokenID > 0 {
		err := s.selectContext(ctx, &ids, `SELECT id FROM route_channels WHERE token_id = ?`, *channel.TokenID)
		return ids, err
	}
	err := s.selectContext(ctx, &ids, `SELECT id FROM route_channels WHERE account_id = ? AND token_id IS NULL`, accountID)
	return ids, err
}

func (s *ProxyRoutingStore) LoadChannelWithAccount(ctx context.Context, channelID int64) (*struct {
	Channel store.RouteChannel
	Account store.Account
}, error) {
	row, err := s.loadChannelAccountRoute(ctx, channelID, false)
	if err != nil || row == nil {
		return nil, err
	}
	return &struct {
		Channel store.RouteChannel
		Account store.Account
	}{Channel: row.Channel, Account: row.Account}, nil
}

func (s *ProxyRoutingStore) LoadChannelWithAccountAndRoute(ctx context.Context, channelID int64) (*struct {
	Channel store.RouteChannel
	Account store.Account
	Route   store.TokenRoute
}, error) {
	return s.loadChannelAccountRoute(ctx, channelID, true)
}

func (s *ProxyRoutingStore) UpdateChannelCooldownFields(ctx context.Context, channelIDs []int64, updates map[string]interface{}) error {
	if len(channelIDs) == 0 || len(updates) == 0 {
		return nil
	}
	setClause, args, err := buildAllowedUpdateSet(updates, routeChannelUpdateColumns)
	if err != nil {
		return err
	}
	query, inArgs, err := sqlx.In(`UPDATE route_channels SET `+setClause+` WHERE id IN (?)`, append(args, channelIDs)...)
	if err != nil {
		return err
	}
	_, err = s.execContext(ctx, query, inArgs...)
	return err
}

func (s *ProxyRoutingStore) UpdateChannelSuccessFields(ctx context.Context, channelID int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	setClause, args, err := buildAllowedUpdateSet(updates, routeChannelUpdateColumns)
	if err != nil {
		return err
	}
	args = append(args, channelID)
	_, err = s.execContext(ctx, `UPDATE route_channels SET `+setClause+` WHERE id = ?`, args...)
	return err
}

func (s *ProxyRoutingStore) UpdateRouteUnitMemberCooldownFields(ctx context.Context, memberID int64, updates map[string]interface{}) error {
	return s.updateRouteUnitMemberFields(ctx, memberID, updates)
}

func (s *ProxyRoutingStore) UpdateRouteUnitMemberSuccessFields(ctx context.Context, memberID int64, updates map[string]interface{}) error {
	return s.updateRouteUnitMemberFields(ctx, memberID, updates)
}

func (s *ProxyRoutingStore) updateRouteUnitMemberFields(ctx context.Context, memberID int64, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}
	setClause, args, err := buildAllowedUpdateSet(updates, routeUnitMemberUpdateColumns)
	if err != nil {
		return err
	}
	args = append(args, memberID)
	_, err = s.execContext(ctx, `UPDATE oauth_route_unit_members SET `+setClause+` WHERE id = ?`, args...)
	return err
}

func (s *ProxyRoutingStore) LoadRouteUnitMemberWithAccount(ctx context.Context, unitID, accountID int64) (*struct {
	Member  store.OAuthRouteUnitMember
	Account store.Account
	Unit    store.OAuthRouteUnit
}, error) {
	rows, err := s.queryxContext(ctx, `
		SELECT
			m.id, m.unit_id, m.account_id, m.sort_order, m.success_count, m.fail_count,
			m.total_latency_ms, m.total_cost, m.last_used_at, m.last_selected_at, m.last_fail_at,
			m.consecutive_fail_count, m.cooldown_level, m.cooldown_until,
			m.cooldown_reason_code, m.cooldown_reason, m.cooldown_reason_at, m.created_at, m.updated_at,
			a.id, a.site_id, a.username, a.access_token, a.api_token, a.balance, a.balance_used,
			a.quota, a.unit_cost, a.value_score, a.status, a.is_pinned, a.sort_order,
			a.checkin_enabled, a.last_checkin_at, a.last_balance_refresh, a.oauth_provider,
			a.oauth_account_key, a.oauth_project_id, a.extra_config, a.created_at, a.updated_at,
			u.id, u.site_id, u.provider, u.name, u.strategy, u.enabled, u.created_at, u.updated_at
		FROM oauth_route_unit_members m
		JOIN accounts a ON a.id = m.account_id
		JOIN oauth_route_units u ON u.id = m.unit_id
		WHERE m.unit_id = ? AND m.account_id = ?`, unitID, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	member, account, unit, err := scanRouteUnitMemberAccountUnit(rows)
	if err != nil {
		return nil, err
	}
	return &struct {
		Member  store.OAuthRouteUnitMember
		Account store.Account
		Unit    store.OAuthRouteUnit
	}{Member: member, Account: account, Unit: unit}, rows.Err()
}

func (s *ProxyRoutingStore) LoadChannelsByTokenID(ctx context.Context, tokenID int64) ([]store.RouteChannel, error) {
	var channels []store.RouteChannel
	err := s.selectContext(ctx, &channels, routeChannelSelectSQL+` WHERE token_id = ?`, tokenID)
	return channels, err
}

func (s *ProxyRoutingStore) LoadChannelsByAccountIDWithoutToken(ctx context.Context, accountID int64) ([]store.RouteChannel, error) {
	var channels []store.RouteChannel
	err := s.selectContext(ctx, &channels, routeChannelSelectSQL+` WHERE account_id = ? AND token_id IS NULL`, accountID)
	return channels, err
}

func (s *ProxyRoutingStore) LoadRuntimeHealthChannelRows(ctx context.Context, channelIDs []int64) ([]struct {
	SiteID            int64
	SourceModel       *string
	RouteModelPattern string
}, error) {
	var result []struct {
		SiteID            int64
		SourceModel       *string
		RouteModelPattern string
	}
	if len(channelIDs) == 0 {
		return result, nil
	}
	query, args, err := sqlx.In(`
		SELECT a.site_id, rc.source_model, tr.model_pattern
		FROM route_channels rc
		JOIN accounts a ON a.id = rc.account_id
		JOIN token_routes tr ON tr.id = rc.route_id
		WHERE rc.id IN (?)`, channelIDs)
	if err != nil {
		return nil, err
	}
	rows, err := s.queryxContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var row struct {
			SiteID            int64
			SourceModel       *string
			RouteModelPattern string
		}
		if err := rows.Scan(&row.SiteID, &row.SourceModel, &row.RouteModelPattern); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// UpdateRouteDecisionSnapshot persists a route decision explanation snapshot.
func (s *ProxyRoutingStore) UpdateRouteDecisionSnapshot(ctx context.Context, routeID int64, snapshot string, refreshedAt string) error {
	if strings.TrimSpace(refreshedAt) == "" {
		refreshedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := s.execContext(ctx, `
		UPDATE token_routes
		SET decision_snapshot = ?, decision_refreshed_at = ?, updated_at = ?
		WHERE id = ?`, snapshot, refreshedAt, refreshedAt, routeID)
	return err
}

// ClearRouteDecisionSnapshot clears one route decision snapshot.
func (s *ProxyRoutingStore) ClearRouteDecisionSnapshot(ctx context.Context, routeID int64) error {
	_, err := s.execContext(ctx, `
		UPDATE token_routes
		SET decision_snapshot = NULL, decision_refreshed_at = NULL
		WHERE id = ?`, routeID)
	return err
}

// ClearRouteDecisionSnapshots clears decision snapshots for the given route IDs.
func (s *ProxyRoutingStore) ClearRouteDecisionSnapshots(ctx context.Context, routeIDs []int64) error {
	if len(routeIDs) == 0 {
		return nil
	}
	query, args, err := sqlx.In(`
		UPDATE token_routes
		SET decision_snapshot = NULL, decision_refreshed_at = NULL
		WHERE id IN (?)`, routeIDs)
	if err != nil {
		return err
	}
	_, err = s.execContext(ctx, query, args...)
	return err
}

// ClearAllRouteDecisionSnapshots clears every route decision snapshot.
func (s *ProxyRoutingStore) ClearAllRouteDecisionSnapshots(ctx context.Context) error {
	_, err := s.execContext(ctx, `
		UPDATE token_routes
		SET decision_snapshot = NULL, decision_refreshed_at = NULL
		WHERE decision_snapshot IS NOT NULL OR decision_refreshed_at IS NOT NULL`)
	return err
}

func (s *ProxyRoutingStore) loadChannelAccountRoute(ctx context.Context, channelID int64, includeRoute bool) (*struct {
	Channel store.RouteChannel
	Account store.Account
	Route   store.TokenRoute
}, error) {
	query := `
		SELECT
			rc.id, rc.route_id, rc.account_id, rc.token_id, rc.oauth_route_unit_id, rc.source_model,
			rc.priority, rc.weight, rc.enabled, rc.manual_override, rc.success_count, rc.fail_count,
			rc.total_latency_ms, rc.total_cost, rc.last_used_at, rc.last_selected_at, rc.last_fail_at,
			rc.consecutive_fail_count, rc.cooldown_level, rc.cooldown_until,
			rc.cooldown_reason_code, rc.cooldown_reason, rc.cooldown_reason_at,
			a.id, a.site_id, a.username, a.access_token, a.api_token, a.balance, a.balance_used,
			a.quota, a.unit_cost, a.value_score, a.status, a.is_pinned, a.sort_order,
			a.checkin_enabled, a.last_checkin_at, a.last_balance_refresh, a.oauth_provider,
			a.oauth_account_key, a.oauth_project_id, a.extra_config, a.created_at, a.updated_at,
			tr.id, tr.model_pattern, tr.display_name, tr.display_icon, tr.route_mode, tr.model_mapping,
			tr.decision_snapshot, tr.decision_refreshed_at, tr.routing_strategy, tr.enabled, tr.created_at, tr.updated_at
		FROM route_channels rc
		JOIN accounts a ON a.id = rc.account_id
		JOIN token_routes tr ON tr.id = rc.route_id
		WHERE rc.id = ?`
	rows, err := s.queryxContext(ctx, query, channelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	channel, account, route, err := scanChannelAccountRoute(rows)
	if err != nil {
		return nil, err
	}
	if !includeRoute {
		route = store.TokenRoute{}
	}
	return &struct {
		Channel store.RouteChannel
		Account store.Account
		Route   store.TokenRoute
	}{Channel: channel, Account: account, Route: route}, rows.Err()
}

// queryxContext/selectContext/execContext delegate to store.DB's Context
// helpers, which rebind ? to $N for PostgreSQL internally. Keeping the dialect
// branch out of the routing store means callers never branch on dialect here.

func (s *ProxyRoutingStore) queryxContext(ctx context.Context, query string, args ...interface{}) (*sqlx.Rows, error) {
	return s.db.QueryxContext(ctx, query, args...)
}

func (s *ProxyRoutingStore) selectContext(ctx context.Context, dest interface{}, query string, args ...interface{}) error {
	return s.db.SelectContext(ctx, dest, query, args...)
}

func (s *ProxyRoutingStore) execContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return s.db.ExecContext(ctx, query, args...)
}

func scanRouteChannelJoin(rows *sqlx.Rows) (struct {
	Channel store.RouteChannel
	Account store.Account
	Site    store.Site
	Token   *store.AccountToken
}, error) {
	var channel store.RouteChannel
	var account store.Account
	var site store.Site
	token, tokenDests := nullableAccountTokenScanTargets()
	dests := []interface{}{
		&channel.ID, &channel.RouteID, &channel.AccountID, &channel.TokenID, &channel.OAuthRouteUnitID, &channel.SourceModel,
		&channel.Priority, &channel.Weight, &channel.Enabled, &channel.ManualOverride, &channel.SuccessCount, &channel.FailCount,
		&channel.TotalLatencyMs, &channel.TotalCost, &channel.LastUsedAt, &channel.LastSelectedAt, &channel.LastFailAt,
		&channel.ConsecutiveFailCount, &channel.CooldownLevel, &channel.CooldownUntil,
		&channel.CooldownReasonCode, &channel.CooldownReason, &channel.CooldownReasonAt,
		&account.ID, &account.SiteID, &account.Username, &account.AccessToken, &account.APIToken, &account.Balance, &account.BalanceUsed,
		&account.Quota, &account.UnitCost, &account.ValueScore, &account.Status, &account.IsPinned, &account.SortOrder,
		&account.CheckinEnabled, &account.LastCheckinAt, &account.LastBalanceRefresh, &account.OAuthProvider,
		&account.OAuthAccountKey, &account.OAuthProjectID, &account.ExtraConfig, &account.CreatedAt, &account.UpdatedAt,
		&site.ID, &site.Name, &site.URL, &site.ExternalCheckinURL, &site.Platform, &site.ProxyURL, &site.UseSystemProxy,
		&site.CustomHeaders, &site.Status, &site.IsPinned, &site.SortOrder, &site.GlobalWeight, &site.APIKey,
		&site.PostRefreshProbeEnabled, &site.PostRefreshProbeModel, &site.PostRefreshProbeScope,
		&site.PostRefreshProbeLatencyThresholdMs, &site.CreatedAt, &site.UpdatedAt,
	}
	dests = append(dests, tokenDests...)
	err := rows.Scan(dests...)
	if err != nil {
		return struct {
			Channel store.RouteChannel
			Account store.Account
			Site    store.Site
			Token   *store.AccountToken
		}{}, err
	}
	return struct {
		Channel store.RouteChannel
		Account store.Account
		Site    store.Site
		Token   *store.AccountToken
	}{Channel: channel, Account: account, Site: site, Token: token.value()}, nil
}

func scanRouteUnitMemberJoin(rows *sqlx.Rows) (store.OAuthRouteUnitMember, store.Account, store.Site, error) {
	var member store.OAuthRouteUnitMember
	var account store.Account
	var site store.Site
	err := rows.Scan(
		&member.ID, &member.UnitID, &member.AccountID, &member.SortOrder, &member.SuccessCount, &member.FailCount,
		&member.TotalLatencyMs, &member.TotalCost, &member.LastUsedAt, &member.LastSelectedAt, &member.LastFailAt,
		&member.ConsecutiveFailCount, &member.CooldownLevel, &member.CooldownUntil,
		&member.CooldownReasonCode, &member.CooldownReason, &member.CooldownReasonAt,
		&member.CreatedAt, &member.UpdatedAt,
		&account.ID, &account.SiteID, &account.Username, &account.AccessToken, &account.APIToken, &account.Balance, &account.BalanceUsed,
		&account.Quota, &account.UnitCost, &account.ValueScore, &account.Status, &account.IsPinned, &account.SortOrder,
		&account.CheckinEnabled, &account.LastCheckinAt, &account.LastBalanceRefresh, &account.OAuthProvider,
		&account.OAuthAccountKey, &account.OAuthProjectID, &account.ExtraConfig, &account.CreatedAt, &account.UpdatedAt,
		&site.ID, &site.Name, &site.URL, &site.ExternalCheckinURL, &site.Platform, &site.ProxyURL, &site.UseSystemProxy,
		&site.CustomHeaders, &site.Status, &site.IsPinned, &site.SortOrder, &site.GlobalWeight, &site.APIKey,
		&site.PostRefreshProbeEnabled, &site.PostRefreshProbeModel, &site.PostRefreshProbeScope,
		&site.PostRefreshProbeLatencyThresholdMs, &site.CreatedAt, &site.UpdatedAt,
	)
	return member, account, site, err
}

func scanRouteUnitMemberAccountUnit(rows *sqlx.Rows) (store.OAuthRouteUnitMember, store.Account, store.OAuthRouteUnit, error) {
	var member store.OAuthRouteUnitMember
	var account store.Account
	var unit store.OAuthRouteUnit
	err := rows.Scan(
		&member.ID, &member.UnitID, &member.AccountID, &member.SortOrder, &member.SuccessCount, &member.FailCount,
		&member.TotalLatencyMs, &member.TotalCost, &member.LastUsedAt, &member.LastSelectedAt, &member.LastFailAt,
		&member.ConsecutiveFailCount, &member.CooldownLevel, &member.CooldownUntil,
		&member.CooldownReasonCode, &member.CooldownReason, &member.CooldownReasonAt,
		&member.CreatedAt, &member.UpdatedAt,
		&account.ID, &account.SiteID, &account.Username, &account.AccessToken, &account.APIToken, &account.Balance, &account.BalanceUsed,
		&account.Quota, &account.UnitCost, &account.ValueScore, &account.Status, &account.IsPinned, &account.SortOrder,
		&account.CheckinEnabled, &account.LastCheckinAt, &account.LastBalanceRefresh, &account.OAuthProvider,
		&account.OAuthAccountKey, &account.OAuthProjectID, &account.ExtraConfig, &account.CreatedAt, &account.UpdatedAt,
		&unit.ID, &unit.SiteID, &unit.Provider, &unit.Name, &unit.Strategy, &unit.Enabled, &unit.CreatedAt, &unit.UpdatedAt,
	)
	return member, account, unit, err
}

func scanChannelAccountRoute(rows *sqlx.Rows) (store.RouteChannel, store.Account, store.TokenRoute, error) {
	var channel store.RouteChannel
	var account store.Account
	var route store.TokenRoute
	err := rows.Scan(
		&channel.ID, &channel.RouteID, &channel.AccountID, &channel.TokenID, &channel.OAuthRouteUnitID, &channel.SourceModel,
		&channel.Priority, &channel.Weight, &channel.Enabled, &channel.ManualOverride, &channel.SuccessCount, &channel.FailCount,
		&channel.TotalLatencyMs, &channel.TotalCost, &channel.LastUsedAt, &channel.LastSelectedAt, &channel.LastFailAt,
		&channel.ConsecutiveFailCount, &channel.CooldownLevel, &channel.CooldownUntil,
		&channel.CooldownReasonCode, &channel.CooldownReason, &channel.CooldownReasonAt,
		&account.ID, &account.SiteID, &account.Username, &account.AccessToken, &account.APIToken, &account.Balance, &account.BalanceUsed,
		&account.Quota, &account.UnitCost, &account.ValueScore, &account.Status, &account.IsPinned, &account.SortOrder,
		&account.CheckinEnabled, &account.LastCheckinAt, &account.LastBalanceRefresh, &account.OAuthProvider,
		&account.OAuthAccountKey, &account.OAuthProjectID, &account.ExtraConfig, &account.CreatedAt, &account.UpdatedAt,
		&route.ID, &route.ModelPattern, &route.DisplayName, &route.DisplayIcon, &route.RouteMode, &route.ModelMapping,
		&route.DecisionSnapshot, &route.DecisionRefreshedAt, &route.RoutingStrategy, &route.Enabled, &route.CreatedAt, &route.UpdatedAt,
	)
	return channel, account, route, err
}

type nullableAccountToken struct {
	id          sql.NullInt64
	accountID   sql.NullInt64
	name        sql.NullString
	token       sql.NullString
	tokenGroup  sql.NullString
	valueStatus sql.NullString
	source      sql.NullString
	enabled     sql.NullBool
	isDefault   sql.NullBool
	createdAt   sql.NullString
	updatedAt   sql.NullString
}

func nullableAccountTokenScanTargets() (*nullableAccountToken, []interface{}) {
	token := &nullableAccountToken{}
	return token, []interface{}{
		&token.id, &token.accountID, &token.name, &token.token, &token.tokenGroup, &token.valueStatus, &token.source,
		&token.enabled, &token.isDefault, &token.createdAt, &token.updatedAt,
	}
}

func (t *nullableAccountToken) value() *store.AccountToken {
	if t == nil || !t.id.Valid {
		return nil
	}
	token := &store.AccountToken{
		ID:          t.id.Int64,
		AccountID:   t.accountID.Int64,
		Name:        t.name.String,
		Token:       t.token.String,
		ValueStatus: t.valueStatus.String,
		Source:      t.source.String,
		Enabled:     t.enabled.Bool,
		IsDefault:   t.isDefault.Bool,
		CreatedAt:   t.createdAt.String,
		UpdatedAt:   t.updatedAt.String,
	}
	if t.tokenGroup.Valid {
		token.TokenGroup = &t.tokenGroup.String
	}
	return token
}

var routeChannelUpdateColumns = map[string]string{
	"successCount":         "success_count",
	"failCount":            "fail_count",
	"totalLatencyMs":       "total_latency_ms",
	"totalCost":            "total_cost",
	"lastUsedAt":           "last_used_at",
	"lastFailAt":           "last_fail_at",
	"consecutiveFailCount": "consecutive_fail_count",
	"cooldownLevel":        "cooldown_level",
	"cooldownUntil":        "cooldown_until",
	"cooldownReasonCode":   "cooldown_reason_code",
	"cooldownReason":       "cooldown_reason",
	"cooldownReasonAt":     "cooldown_reason_at",
	// P1-2: the failure-recording path may disable a channel outright when
	// its failing status falls in the operator-configured disable ranges.
	"enabled":        "enabled",
	"manualOverride": "manual_override",
}

var routeUnitMemberUpdateColumns = map[string]string{
	"successCount":         "success_count",
	"failCount":            "fail_count",
	"totalLatencyMs":       "total_latency_ms",
	"totalCost":            "total_cost",
	"lastUsedAt":           "last_used_at",
	"lastFailAt":           "last_fail_at",
	"lastSelectedAt":       "last_selected_at",
	"consecutiveFailCount": "consecutive_fail_count",
	"cooldownLevel":        "cooldown_level",
	"cooldownUntil":        "cooldown_until",
	"cooldownReasonCode":   "cooldown_reason_code",
	"cooldownReason":       "cooldown_reason",
	"cooldownReasonAt":     "cooldown_reason_at",
	"updatedAt":            "updated_at",
}

func buildAllowedUpdateSet(updates map[string]interface{}, allowed map[string]string) (string, []interface{}, error) {
	parts := make([]string, 0, len(updates))
	args := make([]interface{}, 0, len(updates))
	keys := make([]string, 0, len(updates))
	for key := range updates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := updates[key]
		column, ok := allowed[key]
		if !ok {
			return "", nil, fmt.Errorf("unsupported routing update field %q", key)
		}
		parts = append(parts, column+" = ?")
		args = append(args, value)
	}
	return strings.Join(parts, ", "), args, nil
}

const routeChannelSelectSQL = `
	SELECT id, route_id, account_id, token_id, oauth_route_unit_id, source_model,
	       priority, weight, enabled, manual_override, success_count, fail_count,
	       total_latency_ms, total_cost, last_used_at, last_selected_at, last_fail_at,
	       consecutive_fail_count, cooldown_level, cooldown_until,
	       cooldown_reason_code, cooldown_reason, cooldown_reason_at
	FROM route_channels`
