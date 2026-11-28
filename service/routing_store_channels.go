package service

import (
	"context"
	"database/sql"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
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
		s.custom_headers, s.custom_headers_override_request_headers, s.status, s.is_pinned, s.sort_order, s.global_weight, s.api_key,
		s.post_refresh_probe_enabled, s.post_refresh_probe_model, s.post_refresh_probe_scope,
		s.post_refresh_probe_latency_threshold_ms, s.created_at, s.updated_at`

type routeChannelRow = struct {
	Channel store.RouteChannel
	Account store.Account
	Site    store.Site
	Token   *store.AccountToken
}

func (s *ProxyRoutingStore) LoadRouteChannels(ctx context.Context, routeIDs []int64) ([]struct {
	Channel store.RouteChannel
	Account store.Account
	Site    store.Site
	Token   *store.AccountToken
}, error) {
	var result []routeChannelRow
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.appendDirectRouteChannels(ctx, routeIDs, result)
}

func scanRouteChannelJoin(rows *sqlx.Rows) (routeChannelRow, error) {
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
		&site.CustomHeaders, &site.CustomHeadersOverrideRequestHeaders, &site.Status, &site.IsPinned, &site.SortOrder, &site.GlobalWeight, &site.APIKey,
		&site.PostRefreshProbeEnabled, &site.PostRefreshProbeModel, &site.PostRefreshProbeScope,
		&site.PostRefreshProbeLatencyThresholdMs, &site.CreatedAt, &site.UpdatedAt,
	}
	dests = append(dests, tokenDests...)
	err := rows.Scan(dests...)
	if err != nil {
		return routeChannelRow{}, err
	}
	return routeChannelRow{Channel: channel, Account: account, Site: site, Token: token.value()}, nil
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
			tr.decision_snapshot, tr.decision_refreshed_at, tr.routing_strategy, tr.trace_sticky_mode, tr.enabled, tr.created_at, tr.updated_at
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
		&route.DecisionSnapshot, &route.DecisionRefreshedAt, &route.RoutingStrategy, &route.TraceStickyMode, &route.Enabled, &route.CreatedAt, &route.UpdatedAt,
	)
	return channel, account, route, err
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

const routeChannelSelectSQL = `
	SELECT id, route_id, account_id, token_id, oauth_route_unit_id, source_model,
	       priority, weight, enabled, manual_override, success_count, fail_count,
	       total_latency_ms, total_cost, last_used_at, last_selected_at, last_fail_at,
	       consecutive_fail_count, cooldown_level, cooldown_until,
	       cooldown_reason_code, cooldown_reason, cooldown_reason_at
	FROM route_channels`
