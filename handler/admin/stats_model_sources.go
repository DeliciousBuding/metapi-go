package admin

import "strings"

// Marketplace sources describe stored supply, not selector eligibility. In
// particular enabled configuration and historical successes do not promise that
// a downstream key can dispatch a request now. Never select credential values.
type marketplaceModelSource struct {
	Kind           string                    `json:"kind"`
	ID             int64                     `json:"id"`
	Name           string                    `json:"name"`
	ConnectionName string                    `json:"connectionName"`
	UpstreamModel  string                    `json:"upstreamModel"`
	Evidence       string                    `json:"evidence"`
	Enabled        bool                      `json:"enabled"`
	SiteID         int64                     `json:"siteId,omitempty"`
	ChannelID      int64                     `json:"channelId,omitempty"`
	CredentialID   int64                     `json:"credentialId,omitempty"`
	CredentialKind string                    `json:"credentialKind,omitempty"`
	SuccessCount   *int64                    `json:"successCount,omitempty"`
	FailCount      *int64                    `json:"failCount,omitempty"`
	Bindings       []marketplaceRouteBinding `json:"bindings"`
}

type marketplaceRouteBinding struct {
	ItemID       int64  `json:"itemId"`
	RouteID      int64  `json:"routeId"`
	PublicModel  string `json:"publicModel"`
	RouteEnabled bool   `json:"routeEnabled"`
	GroupEnabled bool   `json:"groupEnabled"`
	Selected     bool   `json:"selected"`
}

func marketplaceExactModel(pattern string) bool {
	return pattern != "" && !strings.ContainsAny(pattern, "*?^$[]()|\\+")
}

func (h *statsHandler) marketplaceDirectSources() (map[string][]marketplaceModelSource, error) {
	// Two fleet-wide batches, independent of model/page size; no per-model or
	// per-credential queries and no secret-bearing routing-store structs.
	rows, err := queryRowsErr(h.db, `SELECT g.id, m.name AS model_name, c.id AS channel_id,
		c.name AS connection_name, k.id AS credential_id, k.name AS credential_name, k.kind AS credential_kind,
		c.enabled AS channel_enabled, m.enabled AS model_enabled, k.enabled AS credential_enabled,
		g.enabled AS grant_enabled, g.success_count, g.fail_count
		FROM upstream_grants g JOIN upstream_models m ON m.id = g.model_id
		JOIN upstream_channels c ON c.id = m.channel_id
		JOIN upstream_credentials k ON k.id = g.credential_id
		ORDER BY c.id, g.id`)
	if err != nil {
		return nil, err
	}
	bindings, err := queryRowsErr(h.db, `SELECT i.id AS item_id, i.grant_id, r.id AS route_id,
		r.model_pattern AS public_model, r.enabled AS route_enabled, u.enabled AS group_enabled,
		u.mode, u.active_item_id, r.route_mode
		FROM upstream_group_items i JOIN upstream_groups u ON u.id = i.group_id
		JOIN upstream_route_groups rg ON rg.group_id = u.id
		JOIN token_routes r ON r.id = rg.route_id ORDER BY r.id, i.id`)
	if err != nil {
		return nil, err
	}
	byGrant := map[int64][]marketplaceRouteBinding{}
	for _, row := range bindings {
		name := strings.TrimSpace(coerceString(row["publicModel"]))
		if !marketplaceExactModel(name) || coerceString(row["routeMode"]) == "explicit_group" {
			continue
		}
		binding := marketplaceRouteBinding{
			ItemID: coerceInt64(row["itemId"]), RouteID: coerceInt64(row["routeId"]), PublicModel: name,
			RouteEnabled: coerceBool(row["routeEnabled"]), GroupEnabled: coerceBool(row["groupEnabled"]),
			Selected: coerceString(row["mode"]) != "manual" || coerceInt64(row["activeItemId"]) == coerceInt64(row["itemId"]),
		}
		id := coerceInt64(row["grantId"])
		byGrant[id] = append(byGrant[id], binding)
	}
	byModel := map[string][]marketplaceModelSource{}
	for _, row := range rows {
		name := strings.TrimSpace(coerceString(row["modelName"]))
		if name == "" {
			continue
		}
		id := coerceInt64(row["id"])
		successes, failures := coerceInt64(row["successCount"]), coerceInt64(row["failCount"])
		source := marketplaceModelSource{
			Kind: "direct", ID: id, Name: coerceString(row["credentialName"]),
			ConnectionName: coerceString(row["connectionName"]), UpstreamModel: name, Evidence: "configured",
			ChannelID: coerceInt64(row["channelId"]), CredentialID: coerceInt64(row["credentialId"]),
			CredentialKind: coerceString(row["credentialKind"]), SuccessCount: &successes, FailCount: &failures,
			Enabled:  coerceBool(row["channelEnabled"]) && coerceBool(row["modelEnabled"]) && coerceBool(row["credentialEnabled"]) && coerceBool(row["grantEnabled"]),
			Bindings: byGrant[id],
		}
		if source.Bindings == nil {
			source.Bindings = []marketplaceRouteBinding{}
		}
		byModel[name] = append(byModel[name], source)
		seen := map[string]bool{name: true}
		for _, binding := range source.Bindings {
			// Retain disabled candidates for explanation, but keep the existing
			// model-list contract: disabled public routes do not add aliases.
			if binding.RouteEnabled && !seen[binding.PublicModel] {
				seen[binding.PublicModel] = true
				byModel[binding.PublicModel] = append(byModel[binding.PublicModel], source)
			}
		}
	}
	return byModel, nil
}
