package upstream

import (
	"fmt"
	"net/url"

	"github.com/deliciousbuding/metapi-go/store"
)

func DisplayURL(value string) string {
	u, err := url.Parse(value)
	if err != nil {
		return ""
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// ListImported projects channel configuration and effective membership without credentials.
func ListImported(db *store.DB) ([]map[string]any, []map[string]any, error) {
	channels, err := store.SelectMaps(db.DB, `SELECT c.id, c.name, c.origin_key AS "originKey", c.dialect, c.provider, c.base_url AS "baseUrl", c.enabled,
  c.openai_chat_completion_path AS "openaiChatCompletionPath", c.openai_response_path AS "openaiResponsePath", c.anthropic_message_path AS "anthropicMessagePath", c.endpoint_config AS "endpointConfig", c.rate_limit AS "rateLimit", c.stream_policy AS "streamPolicy",
  (SELECT COUNT(*) FROM upstream_credentials k WHERE k.channel_id=c.id) AS "credentialCount",
  (SELECT COUNT(*) FROM upstream_models m WHERE m.channel_id=c.id) AS "modelCount"
  FROM upstream_channels c ORDER BY c.name, c.id`)
	if err != nil {
		return nil, nil, &Error{Status: 500, Message: "Failed to load imported upstreams"}
	}
	members, err := store.SelectMaps(db.DB, `SELECT i.id, i.group_id AS "groupId", i.priority, i.weight, i.protocol_order AS "protocolOrder", i.association_conditions AS "associationConditions", g.name AS "groupName",
  g.mode, g.active_item_id AS "activeItemId", rg.route_id AS "routeId", m.channel_id AS "channelId", m.name AS "modelName",
	  k.id AS "credentialId", k.name AS "credentialName", k.kind AS "credentialKind", k.enabled AS "credentialEnabled", grt.protocols,
    grt.id AS "grantId", grt.enabled AS "grantEnabled", m.enabled AS "modelEnabled", g.enabled AS "groupEnabled",
    c.enabled AS "channelEnabled", tr.enabled AS "routeEnabled",
    grt.cooldown_until AS "cooldownUntil",grt.cooldown_reason_code AS "cooldownReasonCode",grt.success_count AS "successCount",grt.fail_count AS "failCount"
  FROM upstream_group_items i JOIN upstream_groups g ON g.id=i.group_id
  JOIN upstream_route_groups rg ON rg.group_id=g.id JOIN upstream_grants grt ON grt.id=i.grant_id
  JOIN upstream_models m ON m.id=grt.model_id JOIN upstream_credentials k ON k.id=grt.credential_id
  JOIN upstream_channels c ON c.id=m.channel_id JOIN token_routes tr ON tr.id=rg.route_id
  ORDER BY g.name, i.priority, i.id`)
	if err != nil {
		return nil, nil, &Error{Status: 500, Message: "Failed to load imported routing members"}
	}
	for _, row := range channels {
		row["ownership"] = Ownership(fmt.Sprint(row["originKey"]))
		var endpoints store.DirectEndpoints
		if err := endpoints.Scan(row["endpointConfig"]); err != nil {
			return nil, nil, &Error{Status: 500, Message: "Invalid imported upstream endpoints"}
		}
		row["endpointConfig"] = endpoints
		var limits store.DirectRateLimit
		if err := limits.Scan(row["rateLimit"]); err != nil {
			return nil, nil, &Error{Status: 500, Message: "Invalid imported channel rate limits"}
		}
		row["rateLimit"] = limits
		row["baseUrl"] = DisplayURL(fmt.Sprint(row["baseUrl"]))
		row["enabled"] = store.RowBool(row["enabled"])
	}
	for _, row := range members {
		var conditions store.DirectAssociationConditions
		if err := conditions.Scan(row["associationConditions"]); err != nil {
			return nil, nil, &Error{Status: 500, Message: "Invalid imported association conditions"}
		}
		delete(row, "associationConditions")
		row["associationRuleCount"] = len(conditions)
		var order store.DirectProtocolOrder
		if err := order.Scan(row["protocolOrder"]); err != nil {
			return nil, nil, &Error{Status: 500, Message: "Invalid imported protocol priority"}
		}
		if order == nil {
			order = store.DirectProtocolOrder{}
		}
		row["protocolOrder"] = order
		effective := true
		for _, key := range []string{"channelEnabled", "modelEnabled", "credentialEnabled", "grantEnabled", "groupEnabled", "routeEnabled"} {
			row[key] = store.RowBool(row[key])
			effective = effective && row[key].(bool)
		}
		selected := fmt.Sprint(row["mode"]) != "manual" || fmt.Sprint(row["activeItemId"]) == fmt.Sprint(row["id"])
		row["selectedByGroup"] = selected
		row["effectiveEnabled"] = effective && selected
	}
	return channels, members, nil
}
