package service

import (
	"context"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

func (s *ProxyRoutingStore) appendDirectRouteChannels(ctx context.Context, routeIDs []int64, result []routeChannelRow) ([]routeChannelRow, error) {
	directQuery, directArgs, err := sqlx.In(`
		SELECT i.id, i.priority, i.weight, i.protocol_order, i.association_conditions, i.last_selected_at, grt.id, rg.route_id, ug.id, ug.mode, ug.active_item_id,
			c.id, c.name, c.base_url, c.dialect, c.provider, c.openai_chat_completion_path, c.openai_response_path,
			c.anthropic_message_path, c.endpoint_config, c.channel_proxy, c.proxy, c.custom_header, c.param_override, c.request_config, c.rate_limit, c.stream_policy,
			m.id, m.name, k.id, k.name, k.kind, k.secret, c.enabled, m.enabled, k.enabled, grt.enabled, grt.protocols,
      grt.cooldown_until,grt.success_count,grt.fail_count,grt.total_latency_ms,grt.total_cost
		FROM upstream_route_groups rg
		JOIN upstream_groups ug ON ug.id = rg.group_id
		JOIN upstream_group_items i ON i.group_id = ug.id
		JOIN upstream_grants grt ON grt.id = i.grant_id
		JOIN upstream_models m ON m.id = grt.model_id
		JOIN upstream_credentials k ON k.id = grt.credential_id
		JOIN upstream_channels c ON c.id = m.channel_id
		WHERE rg.route_id IN (?) AND ug.enabled = ?
			AND (ug.mode <> 'manual' OR i.id = ug.active_item_id)
		ORDER BY i.priority ASC, i.id ASC`, routeIDs, true)
	if err != nil {
		return nil, err
	}
	directRows, err := s.queryxContext(ctx, directQuery, directArgs...)
	if err != nil {
		return nil, err
	}
	defer directRows.Close()
	for directRows.Next() {
		var itemID, priority, weight int64
		var direct store.DirectUpstreamCandidate
		var cooldown, lastSelectedAt *string
		var successes, failures, totalLatency int64
		var totalCost float64
		if err := directRows.Scan(&itemID, &priority, &weight, &direct.ProtocolOrder, &direct.AssociationConditions, &lastSelectedAt, &direct.GrantID, &direct.RouteID, &direct.GroupID, &direct.GroupMode, &direct.ActiveItemID,
			&direct.ChannelID, &direct.ChannelName, &direct.BaseURL, &direct.Dialect, &direct.Provider, &direct.ChatPath, &direct.ResponsesPath,
			&direct.AnthropicPath, &direct.Endpoints, &direct.ChannelProxy, &direct.UseSystemProxy, &direct.CustomHeader, &direct.ParamOverride, &direct.RequestConfig, &direct.RateLimit, &direct.StreamPolicy,
			&direct.ModelID, &direct.ModelName, &direct.CredentialID, &direct.CredentialName, &direct.CredentialKind, &direct.Credential,
			&direct.ChannelEnabled, &direct.ModelEnabled, &direct.CredentialEnabled, &direct.GrantEnabled, &direct.Protocols, &cooldown, &successes, &failures, &totalLatency, &totalCost); err != nil {
			return nil, err
		}
		direct.ItemID = itemID
		result = append(result, routeChannelRow{Channel: store.RouteChannel{
			ID: -itemID, RouteID: direct.RouteID, Direct: &direct,
			Priority: &priority, Weight: &weight, Enabled: true,
			LastSelectedAt: lastSelectedAt, CooldownUntil: cooldown,
			SuccessCount: &successes, FailCount: &failures,
			TotalLatencyMs: &totalLatency, TotalCost: &totalCost,
		}})
	}
	return result, directRows.Err()
}
