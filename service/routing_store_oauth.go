package service

import (
	"context"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

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

func (s *ProxyRoutingStore) FindRouteIDsByOAuthRouteUnitID(ctx context.Context, unitID int64) ([]int64, error) {
	var ids []int64
	err := s.selectContext(ctx, &ids, `SELECT route_id FROM route_channels WHERE oauth_route_unit_id = ?`, unitID)
	return ids, err
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
		&site.CustomHeaders, &site.CustomHeadersOverrideRequestHeaders, &site.Status, &site.IsPinned, &site.SortOrder, &site.GlobalWeight, &site.APIKey,
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
