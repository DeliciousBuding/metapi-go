package backup

type tsV21FieldKind int

const (
	tsV21Text     tsV21FieldKind = iota
	tsV21JSONText                // JSON-encoded TEXT column: non-string values are re-marshaled
	tsV21Int
	tsV21Bool
	tsV21Real
)

type tsV21Field struct {
	tsName   string
	goColumn string
	kind     tsV21FieldKind
	// nullDefault is the value an explicit JSON null converts to when set.
	// Used for nullable numeric/boolean columns whose DDL carries a DEFAULT
	// (route_channels.weight, sites.global_weight, ...): importing explicit
	// null as the DDL default instead of NULL keeps #933-family NULL rows out
	// of the routing tables (a single NULL used to break the routing loads).
	// nil keeps NULL — the right semantics for text and truly-nullable
	// columns (accounts.balance stays unknown/nil per #933).
	nullDefault any
}

type tsV21TableSpec struct {
	goTable string
	section string
	// required lists Go columns that must be present in every row (NOT NULL
	// without a default in the Go schema).
	required []string
	fields   []tsV21Field
	fieldSet map[string]bool
}

func newTSV21TableSpec(goTable, section string, required []string, fields ...tsV21Field) tsV21TableSpec {
	spec := tsV21TableSpec{
		goTable:  goTable,
		section:  section,
		required: required,
		fields:   fields,
		fieldSet: map[string]bool{},
	}
	for _, field := range fields {
		spec.fieldSet[field.tsName] = true
	}
	return spec
}

// tsV21AccountTableSpecs lists the regular account sections in FK-safe import
// order. manualModels is handled separately because it maps into
// model_availability with synthetic columns.
var tsV21AccountTableSpecs = []tsV21TableSpec{
	newTSV21TableSpec("sites", "sites",
		[]string{"name", "url", "platform"},
		tsV21Field{"id", "id", tsV21Int, nil},
		tsV21Field{"name", "name", tsV21Text, nil},
		tsV21Field{"url", "url", tsV21Text, nil},
		tsV21Field{"externalCheckinUrl", "external_checkin_url", tsV21Text, nil},
		tsV21Field{"platform", "platform", tsV21Text, nil},
		tsV21Field{"proxyUrl", "proxy_url", tsV21Text, nil},
		tsV21Field{"useSystemProxy", "use_system_proxy", tsV21Bool, false},
		tsV21Field{"customHeaders", "custom_headers", tsV21JSONText, nil},
		tsV21Field{"status", "status", tsV21Text, nil},
		tsV21Field{"isPinned", "is_pinned", tsV21Bool, false},
		tsV21Field{"sortOrder", "sort_order", tsV21Int, int64(0)},
		tsV21Field{"globalWeight", "global_weight", tsV21Real, float64(1)},
		tsV21Field{"apiKey", "api_key", tsV21Text, nil},
		tsV21Field{"postRefreshProbeEnabled", "post_refresh_probe_enabled", tsV21Bool, false},
		tsV21Field{"postRefreshProbeModel", "post_refresh_probe_model", tsV21Text, nil},
		tsV21Field{"postRefreshProbeScope", "post_refresh_probe_scope", tsV21Text, nil},
		tsV21Field{"postRefreshProbeLatencyThresholdMs", "post_refresh_probe_latency_threshold_ms", tsV21Int, int64(0)},
		tsV21Field{"createdAt", "created_at", tsV21Text, nil},
		tsV21Field{"updatedAt", "updated_at", tsV21Text, nil},
	),
	newTSV21TableSpec("site_api_endpoints", "siteApiEndpoints",
		[]string{"site_id", "url"},
		tsV21Field{"id", "id", tsV21Int, nil},
		tsV21Field{"siteId", "site_id", tsV21Int, nil},
		tsV21Field{"url", "url", tsV21Text, nil},
		tsV21Field{"enabled", "enabled", tsV21Bool, nil},
		tsV21Field{"sortOrder", "sort_order", tsV21Int, nil},
		tsV21Field{"cooldownUntil", "cooldown_until", tsV21Text, nil},
		tsV21Field{"lastSelectedAt", "last_selected_at", tsV21Text, nil},
		tsV21Field{"lastFailedAt", "last_failed_at", tsV21Text, nil},
		tsV21Field{"lastFailureReason", "last_failure_reason", tsV21Text, nil},
		tsV21Field{"createdAt", "created_at", tsV21Text, nil},
		tsV21Field{"updatedAt", "updated_at", tsV21Text, nil},
	),
	newTSV21TableSpec("site_disabled_models", "siteDisabledModels",
		[]string{"site_id", "model_name"},
		tsV21Field{"siteId", "site_id", tsV21Int, nil},
		tsV21Field{"modelName", "model_name", tsV21Text, nil},
	),
	newTSV21TableSpec("accounts", "accounts",
		[]string{"site_id", "access_token"},
		tsV21Field{"id", "id", tsV21Int, nil},
		tsV21Field{"siteId", "site_id", tsV21Int, nil},
		tsV21Field{"username", "username", tsV21Text, nil},
		tsV21Field{"accessToken", "access_token", tsV21Text, nil},
		tsV21Field{"apiToken", "api_token", tsV21Text, nil},
		tsV21Field{"balance", "balance", tsV21Real, nil},
		tsV21Field{"balanceUsed", "balance_used", tsV21Real, nil},
		tsV21Field{"quota", "quota", tsV21Real, nil},
		tsV21Field{"unitCost", "unit_cost", tsV21Real, nil},
		tsV21Field{"valueScore", "value_score", tsV21Real, nil},
		tsV21Field{"status", "status", tsV21Text, nil},
		tsV21Field{"isPinned", "is_pinned", tsV21Bool, nil},
		tsV21Field{"sortOrder", "sort_order", tsV21Int, nil},
		tsV21Field{"checkinEnabled", "checkin_enabled", tsV21Bool, nil},
		tsV21Field{"lastCheckinAt", "last_checkin_at", tsV21Text, nil},
		tsV21Field{"lastBalanceRefresh", "last_balance_refresh", tsV21Text, nil},
		tsV21Field{"oauthProvider", "oauth_provider", tsV21Text, nil},
		tsV21Field{"oauthAccountKey", "oauth_account_key", tsV21Text, nil},
		tsV21Field{"oauthProjectId", "oauth_project_id", tsV21Text, nil},
		tsV21Field{"extraConfig", "extra_config", tsV21JSONText, nil},
		tsV21Field{"createdAt", "created_at", tsV21Text, nil},
		tsV21Field{"updatedAt", "updated_at", tsV21Text, nil},
	),
	newTSV21TableSpec("account_tokens", "accountTokens",
		[]string{"account_id", "name", "token"},
		tsV21Field{"id", "id", tsV21Int, nil},
		tsV21Field{"accountId", "account_id", tsV21Int, nil},
		tsV21Field{"name", "name", tsV21Text, nil},
		tsV21Field{"token", "token", tsV21Text, nil},
		tsV21Field{"tokenGroup", "token_group", tsV21Text, nil},
		tsV21Field{"valueStatus", "value_status", tsV21Text, nil},
		tsV21Field{"source", "source", tsV21Text, nil},
		tsV21Field{"enabled", "enabled", tsV21Bool, nil},
		tsV21Field{"isDefault", "is_default", tsV21Bool, nil},
		tsV21Field{"createdAt", "created_at", tsV21Text, nil},
		tsV21Field{"updatedAt", "updated_at", tsV21Text, nil},
	),
	newTSV21TableSpec("token_routes", "tokenRoutes",
		[]string{"model_pattern"},
		tsV21Field{"id", "id", tsV21Int, nil},
		tsV21Field{"modelPattern", "model_pattern", tsV21Text, nil},
		tsV21Field{"displayName", "display_name", tsV21Text, nil},
		tsV21Field{"displayIcon", "display_icon", tsV21Text, nil},
		tsV21Field{"routeMode", "route_mode", tsV21Text, nil},
		tsV21Field{"modelMapping", "model_mapping", tsV21JSONText, nil},
		tsV21Field{"decisionSnapshot", "decision_snapshot", tsV21JSONText, nil},
		tsV21Field{"decisionRefreshedAt", "decision_refreshed_at", tsV21Text, nil},
		tsV21Field{"routingStrategy", "routing_strategy", tsV21Text, nil},
		tsV21Field{"enabled", "enabled", tsV21Bool, true},
		tsV21Field{"createdAt", "created_at", tsV21Text, nil},
		tsV21Field{"updatedAt", "updated_at", tsV21Text, nil},
	),
	newTSV21TableSpec("route_group_sources", "routeGroupSources",
		[]string{"group_route_id", "source_route_id"},
		tsV21Field{"id", "id", tsV21Int, nil},
		tsV21Field{"groupRouteId", "group_route_id", tsV21Int, nil},
		tsV21Field{"sourceRouteId", "source_route_id", tsV21Int, nil},
	),
	newTSV21TableSpec("route_channels", "routeChannels",
		[]string{"route_id", "account_id"},
		tsV21Field{"id", "id", tsV21Int, nil},
		tsV21Field{"routeId", "route_id", tsV21Int, nil},
		tsV21Field{"accountId", "account_id", tsV21Int, nil},
		tsV21Field{"tokenId", "token_id", tsV21Int, nil},
		tsV21Field{"oauthRouteUnitId", "oauth_route_unit_id", tsV21Int, nil},
		tsV21Field{"sourceModel", "source_model", tsV21Text, nil},
		tsV21Field{"priority", "priority", tsV21Int, int64(0)},
		tsV21Field{"weight", "weight", tsV21Int, int64(10)},
		tsV21Field{"enabled", "enabled", tsV21Bool, true},
		tsV21Field{"manualOverride", "manual_override", tsV21Bool, false},
		tsV21Field{"successCount", "success_count", tsV21Int, int64(0)},
		tsV21Field{"failCount", "fail_count", tsV21Int, int64(0)},
		tsV21Field{"totalLatencyMs", "total_latency_ms", tsV21Int, int64(0)},
		tsV21Field{"totalCost", "total_cost", tsV21Real, float64(0)},
		tsV21Field{"lastUsedAt", "last_used_at", tsV21Text, nil},
		tsV21Field{"lastSelectedAt", "last_selected_at", tsV21Text, nil},
		tsV21Field{"lastFailAt", "last_fail_at", tsV21Text, nil},
		tsV21Field{"consecutiveFailCount", "consecutive_fail_count", tsV21Int, int64(0)},
		tsV21Field{"cooldownLevel", "cooldown_level", tsV21Int, int64(0)},
		tsV21Field{"cooldownUntil", "cooldown_until", tsV21Text, nil},
	),
	newTSV21TableSpec("downstream_api_keys", "downstreamApiKeys",
		[]string{"name", "key"},
		tsV21Field{"name", "name", tsV21Text, nil},
		tsV21Field{"key", "key", tsV21Text, nil},
		tsV21Field{"description", "description", tsV21Text, nil},
		tsV21Field{"groupName", "group_name", tsV21Text, nil},
		tsV21Field{"tags", "tags", tsV21JSONText, nil},
		tsV21Field{"enabled", "enabled", tsV21Bool, nil},
		tsV21Field{"expiresAt", "expires_at", tsV21Text, nil},
		tsV21Field{"maxCost", "max_cost", tsV21Real, nil},
		tsV21Field{"usedCost", "used_cost", tsV21Real, nil},
		tsV21Field{"maxRequests", "max_requests", tsV21Int, nil},
		tsV21Field{"usedRequests", "used_requests", tsV21Int, nil},
		tsV21Field{"supportedModels", "supported_models", tsV21JSONText, nil},
		tsV21Field{"allowedRouteIds", "allowed_route_ids", tsV21JSONText, nil},
		tsV21Field{"siteWeightMultipliers", "site_weight_multipliers", tsV21JSONText, nil},
		tsV21Field{"excludedSiteIds", "excluded_site_ids", tsV21JSONText, nil},
		tsV21Field{"excludedCredentialRefs", "excluded_credential_refs", tsV21JSONText, nil},
		tsV21Field{"lastUsedAt", "last_used_at", tsV21Text, nil},
	),
}

// tsV21KnownAccountSections is every section name the TS v2.1 accounts object
// may carry. Anything else is reported as an unknown section.
var tsV21KnownAccountSections = func() map[string]bool {
	sections := map[string]bool{"manualModels": true}
	for _, spec := range tsV21AccountTableSpecs {
		sections[spec.section] = true
	}
	return sections
}()
