package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// The complete fixture mirrors the AxonHub v1.4 export shape at
// e863c6fe1942deddd0f6e471fa003c430e5314f0. GCP service-account material is
// generated per test instead of being checked into Git.
const axonHubCompleteGCPPlaceholder = "__SYNTHETIC_GCP_SERVICE_ACCOUNT_JSON__"

func readAxonHubCompleteFixture(t *testing.T) []byte {
	t.Helper()
	raw := readAxonHubFixture(t, "axonhub-complete.json")
	escaped, err := json.Marshal(vertexFixtureSA(t))
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.ReplaceAll(raw, []byte(axonHubCompleteGCPPlaceholder), escaped[1:len(escaped)-1])
	if bytes.Contains(raw, []byte(axonHubCompleteGCPPlaceholder)) {
		t.Fatal("GCP placeholder was not replaced")
	}
	return raw
}

var axonHubCompleteSkipped = map[int][]string{
	17: {"provider_translation_unsupported"},
	18: {"provider_wire_format_unsupported"},
	19: {"provider_wire_format_unsupported"},
	28: {"channel_settings_unsupported:quotaRoutingMode"},
	29: {"channel_policy_unsupported:apiKeyAutoDisableMode"},
	31: {"oauth_custom_endpoint_unsupported"},
	32: {"oauth_custom_endpoint_unsupported"},
	38: {"channel_stream_policy_unsupported"},
	42: {"proxy_configuration_unsupported"},
	43: {"credential_field_unsupported:futureCredential"},
	44: {"websocket_transport_unsupported"},
	47: {"provider_type_unknown"},
	48: {"credential_missing"},
	50: {"base_url_invalid"},
	51: {"param_override_invalid"},
	52: {"source_custom_endpoint_not_constructible:ollama/chat"},
	53: {"api_format_unsupported"},
}

func TestAxonHubCompleteFixturePreview(t *testing.T) {
	raw := readAxonHubCompleteFixture(t)
	src, err := ParseAxonHubSource(raw)
	if err != nil {
		t.Fatalf("parse complete fixture: %v", err)
	}
	if len(src.Channels) != 104 || len(src.Models) != 52 || len(src.Projects) != 3 ||
		len(src.APIKeys) != 12 || len(src.ChannelModelPrices) != 2 ||
		len(src.SystemConfigs) != 5 || src.UsageRequests != 4 || src.UsageLogs != 4 {
		t.Fatalf("fixture sections drifted: channels=%d models=%d projects=%d keys=%d prices=%d configs=%d requests=%d logs=%d",
			len(src.Channels), len(src.Models), len(src.Projects), len(src.APIKeys),
			len(src.ChannelModelPrices), len(src.SystemConfigs), src.UsageRequests, src.UsageLogs)
	}

	plan, err := CompileAxonHubPlan(src)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(plan.channels), 86; got != want {
		t.Fatalf("routable channels = %d, want %d (skipped: %+v)", got, want, plan.skipped)
	}
	if got, want := len(plan.keys), 10; got != want {
		t.Fatalf("routable keys = %d, want %d", got, want)
	}
	if got, want := len(plan.credentials), 88; got != want {
		t.Fatalf("credentials = %d, want %d", got, want)
	}
	if len(plan.blocking) != 0 {
		t.Fatalf("fixture must be importable, blocking = %v", plan.blocking)
	}

	skipped := map[int]AxonHubSkippedChannel{}
	for _, entry := range plan.skipped {
		skipped[entry.SourceID] = entry
	}
	for id := range axonHubCompleteSkipped {
		if _, ok := skipped[id]; !ok {
			t.Fatalf("channel %d must be skipped, got %+v", id, plan.skipped)
		}
	}
	if len(skipped) != len(axonHubCompleteSkipped) {
		t.Fatalf("skipped channel count = %d, want %d: %+v", len(skipped), len(axonHubCompleteSkipped), plan.skipped)
	}
	for id, reasons := range axonHubCompleteSkipped {
		entry := skipped[id]
		for _, reason := range reasons {
			if !containsString(entry.Reasons, reason) {
				t.Fatalf("channel %d reasons = %v, missing %q", id, entry.Reasons, reason)
			}
		}
	}

	residuals := strings.Join(plan.residuals, "\n")
	for _, want := range []string{
		"channel-1:default_test_model_not_imported",
		"channel-1:manual_models_not_imported_until_synced",
		"channel-1:model_auto_sync_replaced_by_model_refresh",
		"channel-30:management_api_key_not_imported",
		"channel-35:provider_quota_polling_not_imported",
		"channel-36:source_retry_policy_replaced_by_gateway_retry_policy",
		"channel-37:pass_through_body_not_configurable",
		"channel-37:pass_through_user_agent_not_configurable",
		"conditional_model_association_not_imported:conditional-model",
		"declared_protocol_not_servable:openai/responses_compact",
		"model_has_no_importable_channel:dev-no-inherit-model",
		"model_has_no_importable_channel:disabled-assoc-model",
		"model_has_no_importable_channel:orphan-model",
		"model_strategy_approximated:adaptive:adaptive-model",
		"model_trace_sticky_mode_not_imported:rr-model",
		"model_type_not_servable:audio",
		"source_key_missing_write_requests",
		"source_key_type_not_proxy",
		"source_key_routing_override_unsupported",
		"source_project_unavailable",
		"project_active_profiles_flattened_into_key_channel_boundaries",
	} {
		if !strings.Contains(residuals, want) {
			t.Fatalf("residuals missing %q:\n%s", want, residuals)
		}
	}

	// Developer inheritance is resolved, but a model that opts out is not.
	if !hasRoute(plan, "dev-inherit-model") {
		t.Fatal("developer-level association was not inherited")
	}
	if hasRoute(plan, "dev-no-inherit-model") {
		t.Fatal("disabled developer inheritance still produced a route")
	}
	// Local rules precede inherited rules at equal priority; the lower numeric
	// priority wins over the overlapping candidate.
	if route := findRoute(plan, "priority-model"); route == nil || len(route.Items) != 1 || route.Items[0].Priority != 1 {
		t.Fatalf("priority resolution = %+v", route)
	}
	// Six association kinds all resolve into concrete candidates.
	for _, name := range []string{
		"assoc-channel-model", "assoc-channel-regex", "assoc-regex",
		"assoc-model", "assoc-tags-model", "assoc-tags-regex",
	} {
		if !hasRoute(plan, name) {
			t.Fatalf("association kind did not produce a route: %s", name)
		}
	}
	// Aliases and auto-trimmed names map onto the upstream model name.
	if route := findRoute(plan, "alias-model"); route == nil || route.Items[0].ModelName != "vendor/gpt-complete" {
		t.Fatalf("alias route = %+v", route)
	}
	if route := findRoute(plan, "gpt-complete"); route == nil || route.Items[0].ModelName != "vendor/gpt-complete" {
		t.Fatalf("auto-trimmed route = %+v", route)
	}
	// modelProtocols restrict only the named model; the disabled entry is inert.
	if route := findRoute(plan, "alias-model"); route == nil || !protocolOrderEqual(route.Items[0].ProtocolOrder, store.DirectProtocolOrder{protoMessages}) {
		t.Fatalf("modelProtocols restriction lost: %+v", route)
	}
	if route := findRoute(plan, "gpt-complete"); route == nil || route.Items[0].ProtocolOrder[0] != protoChat {
		t.Fatalf("default protocol order lost: %+v", route)
	}
	// Vertex endpoints derive from the GCP project and publisher kind.
	if channel := findChannel(plan, 9); channel == nil || channel.Endpoints.Gemini == nil || channel.Endpoints.Gemini.Profile != "vertex" {
		t.Fatalf("vertex gemini endpoint = %+v", channel)
	}
	if channel := findChannel(plan, 10); channel == nil || channel.Endpoints.Messages == nil || channel.Endpoints.Messages.Profile != "vertex-anthropic" {
		t.Fatalf("vertex anthropic endpoint = %+v", channel)
	}
	// Custom base/path and proxy survive.
	if channel := findChannel(plan, 8); channel == nil || channel.Endpoints.Chat == nil ||
		channel.Endpoints.Chat.URL != "https://custom-chat.example.invalid/v1/chat/completions" ||
		channel.Endpoints.Messages == nil || channel.Endpoints.Messages.URL != "https://custom-messages.example.invalid/v1/messages" ||
		!strings.Contains(channel.ChannelProxy, "proxy.example.invalid") {
		t.Fatalf("custom endpoint or proxy lost: %+v", channel)
	}
	// OAuth refresh state stays attached to the credential, not the preview.
	if channel := findChannel(plan, 11); channel == nil || channel.CredentialKeys[0] != "" {
		t.Fatalf("copilot credential = %+v", channel)
	}
	var copilot, xai *axonHubPlanCredential
	for i := range plan.credentials {
		switch plan.credentials[i].ChannelSourceID {
		case 11:
			copilot = &plan.credentials[i]
		case 12:
			xai = &plan.credentials[i]
		}
	}
	if copilot == nil || copilot.Kind != store.DirectCredentialOAuth || copilot.Secret != "" ||
		copilot.OAuthState.RefreshToken != "gho_synthetic_copilot_access" {
		t.Fatalf("copilot oauth credential = %+v", copilot)
	}
	if xai == nil || xai.OAuthState.RefreshToken != "xai-synthetic-refresh" || xai.OAuthState.IDToken != "xai-synthetic-id" {
		t.Fatalf("xai oauth credential = %+v", xai)
	}
	// A disabled key on a multi-key channel narrows the grant set.
	if channel := findChannel(plan, 55); channel == nil || len(channel.CredentialKeys) != 2 {
		t.Fatalf("extension channel = %+v", channel)
	}
	var extensionGrant int
	for _, grant := range plan.grants {
		if grant.ChannelSourceID == 55 {
			extensionGrant++
		}
	}
	if extensionGrant != 1 {
		t.Fatalf("disabled credential still produced %d grants", extensionGrant)
	}

	// Preview output must never echo credentials, hosts or operator text.
	preview, err := PreviewAxonHubV14(nil, raw, "complete")
	if err != nil {
		t.Fatal(err)
	}
	for section, want := range map[string]int{
		"deletedChannels": 1, "deletedApiKeys": 1, "apiKeysWithoutSecret": 1,
		"disabledModelRoutes": 1, "modelPrices": 2, "systemConfigs": 5,
		"usageRequests": 4, "usageLogs": 4,
	} {
		if got := preview.NotImported[section]; got != want {
			t.Fatalf("notImported[%s] = %d, want %d", section, got, want)
		}
	}
	if len(preview.SkippedChannels) != len(axonHubCompleteSkipped) {
		t.Fatalf("preview skipped channels = %d, want %d", len(preview.SkippedChannels), len(axonHubCompleteSkipped))
	}
	if len(preview.Blocking) != 0 {
		t.Fatalf("preview blocking = %v", preview.Blocking)
	}
	encoded, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"sk-synthetic", "synthetic-access", "synthetic-refresh", "xai-synthetic-id",
		"gho_synthetic", "fixture-proxy-password", "api.example.invalid",
		"custom-chat.example.invalid", "Synthetic AxonHub Fixture",
	} {
		if bytes.Contains(encoded, []byte(secret)) {
			t.Fatalf("preview leaked %q: %s", secret, encoded)
		}
	}
}

func TestAxonHubCompleteFixtureImport(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := readAxonHubCompleteFixture(t)

	if _, err := ImportAxonHubV14(db, raw, "complete-origin", false); err != nil {
		t.Fatalf("import complete fixture: %v", err)
	}
	if got, want := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`), 86; got != want {
		t.Fatalf("upstream_channels = %d, want %d", got, want)
	}
	if got, want := countRows(t, db, `SELECT COUNT(*) FROM upstream_credentials`), 88; got != want {
		t.Fatalf("upstream_credentials = %d, want %d", got, want)
	}
	if got, want := countRows(t, db, `SELECT COUNT(*) FROM upstream_models`), 48; got != want {
		t.Fatalf("upstream_models = %d, want %d", got, want)
	}
	if got, want := countRows(t, db, `SELECT COUNT(*) FROM upstream_grants`), 49; got != want {
		t.Fatalf("upstream_grants = %d, want %d", got, want)
	}
	// Two grants are shared by two route items each (a regex route can select a
	// model that also owns its own route model record).
	if got, want := countRows(t, db, `SELECT COUNT(*) FROM upstream_group_items`), 51; got != want {
		t.Fatalf("upstream_group_items = %d, want %d", got, want)
	}
	if got, want := countRows(t, db, `SELECT COUNT(*) FROM token_routes`), 47; got != want {
		t.Fatalf("token_routes = %d, want %d", got, want)
	}
	if got, want := countRows(t, db, `SELECT COUNT(*) FROM downstream_api_keys`), 10; got != want {
		t.Fatalf("downstream_api_keys = %d, want %d", got, want)
	}
	// Every imported entity type keeps an origin-scoped source-id mapping so a
	// re-import updates the same rows instead of duplicating them.
	for entity, want := range map[string]int{
		"upstream_channels": 86, "upstream_credentials": 88,
		"upstream_models": 48, "upstream_grants": 49,
		"upstream_groups": 47, "upstream_group_items": 51,
		"token_routes": 47, "downstream_api_keys": 10,
	} {
		if got := countRows(t, db, `SELECT COUNT(*) FROM external_source_ids WHERE origin_key = ? AND entity_type = ?`, "complete-origin", entity); got != want {
			t.Fatalf("source-id mappings for %s = %d, want %d", entity, got, want)
		}
	}
	// Remapped foreign keys must point at rows this import wrote.
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_grants g
		LEFT JOIN upstream_models m ON m.id = g.model_id
		LEFT JOIN upstream_credentials c ON c.id = g.credential_id
		WHERE m.id IS NULL OR c.id IS NULL`); got != 0 {
		t.Fatalf("grants with dangling remapped foreign keys = %d", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_group_items gi
		LEFT JOIN upstream_grants g ON g.id = gi.grant_id
		LEFT JOIN upstream_groups gr ON gr.id = gi.group_id
		WHERE g.id IS NULL OR gr.id IS NULL`); got != 0 {
		t.Fatalf("group items with dangling remapped foreign keys = %d", got)
	}
	if got, want := countRows(t, db, `SELECT COUNT(*) FROM downstream_quota_usage`), 4; got != want {
		t.Fatalf("downstream_quota_usage = %d, want %d", got, want)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels WHERE enabled = ?`, false); got != 1 {
		t.Fatalf("disabled channels = %d, want the archived channel only", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels WHERE name = 'Complete duplicate name'`); got != 1 {
		t.Fatalf("duplicate-name channel rows = %d", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels WHERE name = 'Complete duplicate name#149'`); got != 1 {
		t.Fatalf("disambiguated duplicate-name channel rows = %d", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels WHERE name = 'Skip deleted channel'`); got != 0 {
		t.Fatal("deleted source channel was imported")
	}

	// Credential kinds and private material.
	kinds := map[string]int{}
	var kindRows []string
	if err := db.Select(&kindRows, db.Rebind(`SELECT kind FROM upstream_credentials`)); err != nil {
		t.Fatal(err)
	}
	for _, kind := range kindRows {
		kinds[kind]++
	}
	if kinds[store.DirectCredentialOAuth] != 5 {
		t.Fatalf("oauth credentials = %d, want 5", kinds[store.DirectCredentialOAuth])
	}
	if kinds[store.DirectCredentialGCPServiceAccount] != 2 {
		t.Fatalf("gcp credentials = %d, want 2", kinds[store.DirectCredentialGCPServiceAccount])
	}
	var copilotState store.DirectOAuthState
	if err := db.Get(&copilotState, db.Rebind(`SELECT oauth_state FROM upstream_credentials c JOIN upstream_channels ch ON ch.id = c.channel_id WHERE ch.name = 'Complete GitHub Copilot'`)); err != nil {
		t.Fatal(err)
	}
	if copilotState.RefreshToken != "gho_synthetic_copilot_access" {
		t.Fatalf("copilot refresh token = %+v", copilotState)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_credentials WHERE kind = ? AND gcp_service_account <> ''`, store.DirectCredentialGCPServiceAccount); got != 2 {
		t.Fatalf("stored GCP material rows = %d", got)
	}

	// The custom channel keeps its exact endpoint URLs, proxy and per-model
	// protocol restriction.
	var endpoints store.DirectEndpoints
	if err := db.Get(&endpoints, db.Rebind(`SELECT endpoint_config FROM upstream_channels WHERE name = 'Complete custom endpoints'`)); err != nil {
		t.Fatal(err)
	}
	if endpoints.Chat == nil || endpoints.Chat.URL != "https://custom-chat.example.invalid/v1/chat/completions" ||
		endpoints.Messages == nil || endpoints.Messages.URL != "https://custom-messages.example.invalid/v1/messages" {
		t.Fatalf("custom endpoint config = %+v", endpoints)
	}
	var proxy string
	if err := db.Get(&proxy, db.Rebind(`SELECT channel_proxy FROM upstream_channels WHERE name = 'Complete custom endpoints'`)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(proxy, "fixture-proxy-user") || !strings.Contains(proxy, "proxy.example.invalid") {
		t.Fatalf("channel proxy = %q", proxy)
	}
	var aliasOrder store.DirectProtocolOrder
	if err := db.Get(&aliasOrder, db.Rebind(`SELECT gi.protocol_order FROM upstream_group_items gi
		JOIN upstream_groups g ON g.id = gi.group_id
		WHERE g.name = 'alias-model'`)); err != nil {
		t.Fatal(err)
	}
	if !protocolOrderEqual(aliasOrder, store.DirectProtocolOrder{protoMessages}) {
		t.Fatalf("alias model protocol order = %v", aliasOrder)
	}
	var priority int
	if err := db.Get(&priority, db.Rebind(`SELECT gi.priority FROM upstream_group_items gi
		JOIN upstream_groups g ON g.id = gi.group_id WHERE g.name = 'priority-model'`)); err != nil {
		t.Fatal(err)
	}
	if priority != 1 {
		t.Fatalf("highest-priority candidate priority = %d", priority)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM token_routes WHERE model_pattern = 'disabled-model' AND enabled = ?`, false); got != 1 {
		t.Fatalf("disabled source model route rows = %d", got)
	}
	// Route strategies.
	if got := countRows(t, db, `SELECT COUNT(*) FROM token_routes WHERE model_pattern = 'rr-model' AND routing_strategy = 'round_robin'`); got != 1 {
		t.Fatalf("round-robin route rows = %d", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM token_routes WHERE model_pattern = 'adaptive-model' AND routing_strategy = 'weighted'`); got != 1 {
		t.Fatalf("adaptive route approximation rows = %d", got)
	}
	for _, name := range []string{"conditional-model", "dev-no-inherit-model", "disabled-assoc-model", "orphan-model"} {
		if got := countRows(t, db, `SELECT COUNT(*) FROM token_routes WHERE model_pattern = ?`, name); got != 0 {
			t.Fatalf("route %q should not exist: %d rows", name, got)
		}
	}

	// Downstream keys: project and key active-profile intersection, quota,
	// IP allowlist, scopes and status.
	key := loadImportedAccessPolicy(t, db, "sk-synthetic-complete-key-501")
	if key.AllowedUpstreamChannelIDs == nil || len(*key.AllowedUpstreamChannelIDs) != 1 {
		t.Fatalf("key channel intersection = %+v", key.AllowedUpstreamChannelIDs)
	}
	var sourceChannelID int64
	if err := db.Get(&sourceChannelID, db.Rebind(`SELECT id FROM upstream_channels WHERE name = 'Complete OpenAI primary'`)); err != nil {
		t.Fatal(err)
	}
	if (*key.AllowedUpstreamChannelIDs)[0] != sourceChannelID {
		t.Fatalf("key channel intersection = %v, want %d", *key.AllowedUpstreamChannelIDs, sourceChannelID)
	}
	if len(key.ModelIDs) != 2 || len(key.ModelMappings) != 1 || key.ModelMappings[0].From != "public-alias" {
		t.Fatalf("key model scope = %+v %+v", key.ModelIDs, key.ModelMappings)
	}
	if key.Quota == nil || key.Quota.Requests == nil || *key.Quota.Requests != 1000 ||
		key.Quota.TotalTokens == nil || *key.Quota.TotalTokens != 200000 ||
		key.Quota.Cost == nil || *key.Quota.Cost != 12.5 ||
		key.Quota.Period.Type != "all_time" || key.Quota.Timezone != "Asia/Shanghai" {
		t.Fatalf("key quota = %+v", key.Quota)
	}
	var ipAllowlist string
	if err := db.Get(&ipAllowlist, db.Rebind(`SELECT ip_allowlist FROM downstream_api_keys WHERE key = ?`), "sk-synthetic-complete-key-501"); err != nil {
		t.Fatal(err)
	}
	if ipAllowlist != "192.0.2.7/32\n198.51.100.23" {
		t.Fatalf("ip allowlist = %q", ipAllowlist)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM downstream_quota_usage WHERE key_id = (SELECT id FROM downstream_api_keys WHERE key = ?)`, "sk-synthetic-complete-key-501"); got != 3 {
		t.Fatalf("key quota usage rows = %d, want 3", got)
	}
	rolling := loadImportedAccessPolicy(t, db, "sk-synthetic-complete-key-507")
	if rolling.Quota == nil || rolling.Quota.Period.Type != "past_duration" ||
		rolling.Quota.Period.PastDuration == nil || rolling.Quota.Period.PastDuration.Value != 24 ||
		rolling.Quota.Period.PastDuration.Unit != "hour" {
		t.Fatalf("rolling quota = %+v", rolling.Quota)
	}
	calendar := loadImportedAccessPolicy(t, db, "sk-synthetic-complete-key-508")
	if calendar.Quota == nil || calendar.Quota.Period.Type != "calendar_duration" ||
		calendar.Quota.Period.CalendarDuration == nil || calendar.Quota.Period.CalendarDuration.Unit != "month" {
		t.Fatalf("calendar quota = %+v", calendar.Quota)
	}
	// Tag match modes: "all" intersects with the project profile, "none"
	// excludes every channel carrying the tag.
	if ids := loadImportedAccessPolicy(t, db, "sk-synthetic-complete-key-508").AllowedUpstreamChannelIDs; ids == nil || len(*ids) != 2 {
		t.Fatalf("all-tags key channels = %+v, want 2", ids)
	}
	if ids := loadImportedAccessPolicy(t, db, "sk-synthetic-complete-key-507").AllowedUpstreamChannelIDs; ids == nil || len(*ids) != 2 {
		t.Fatalf("rolling key channels = %+v, want 2", ids)
	}
	noneIDs := loadImportedAccessPolicy(t, db, "sk-synthetic-complete-key-511").AllowedUpstreamChannelIDs
	if noneIDs == nil || len(*noneIDs) != 83 {
		t.Fatalf("none-tags key channels = %+v, want 83", noneIDs)
	}
	for _, id := range *noneIDs {
		var name string
		if err := db.Get(&name, db.Rebind(`SELECT name FROM upstream_channels WHERE id = ?`), id); err != nil {
			t.Fatal(err)
		}
		if name == "Complete OpenAI primary" || name == "Complete Anthropic relay" || name == "Complete Gemini native" {
			t.Fatalf("none-tags boundary kept prod channel %q", name)
		}
	}
	for value, want := range map[string]string{
		"sk-synthetic-complete-key-502": "source_key_missing_write_requests",
		"sk-synthetic-complete-key-503": "source_key_type_not_proxy",
		"sk-synthetic-complete-key-509": "source_key_routing_override_unsupported",
		"sk-synthetic-complete-key-511": "source_project_unavailable",
		"sk-synthetic-complete-key-512": "source_project_unavailable",
	} {
		policy := loadImportedAccessPolicy(t, db, value)
		if policy.BlockReason != want {
			t.Fatalf("%s block reason = %q, want %q", value, policy.BlockReason, want)
		}
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM downstream_api_keys WHERE key = ? AND enabled = ?`, "sk-synthetic-complete-key-504", false); got != 1 {
		t.Fatal("disabled source key was imported as enabled")
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM downstream_api_keys WHERE key = ?`, "sk-synthetic-complete-key-505"); got != 0 {
		t.Fatal("deleted source key was imported")
	}

	// The imported graph must be loadable by the real routing store.
	routingStore := service.NewProxyRoutingStore(db)
	var routeIDs []int64
	if err := db.Select(&routeIDs, db.Rebind(`SELECT id FROM token_routes ORDER BY id`)); err != nil {
		t.Fatal(err)
	}
	candidates, err := routingStore.LoadRouteChannels(context.Background(), routeIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) == 0 {
		t.Fatal("routing store loaded no candidates")
	}

	// Re-import updates rows and keeps health counters.
	if _, err := db.Exec(`UPDATE upstream_grants SET success_count = 7, total_latency_ms = 420`); err != nil {
		t.Fatal(err)
	}
	second, err := ImportAxonHubV14(db, raw, "complete-origin", false)
	if err != nil {
		t.Fatalf("re-import: %v", err)
	}
	if second["channels"] != 86 || second["routes"] != 47 || second["apiKeys"] != 10 {
		t.Fatalf("re-import counts drifted: %#v", second)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`); got != 86 {
		t.Fatalf("re-import duplicated channels: %d", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_grants WHERE success_count = 7`); got != 49 {
		t.Fatalf("re-import replaced rows instead of updating them: %d", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM downstream_api_keys`); got != 10 {
		t.Fatalf("re-import duplicated downstream keys: %d", got)
	}
}

func TestAxonHubCompleteFixtureRollsBackOnKeyCollision(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := readAxonHubCompleteFixture(t)
	if _, err := db.Exec(`INSERT INTO downstream_api_keys (name,key,group_name,enabled,supported_models,access_policy,ip_allowlist,created_at,updated_at)
		VALUES ('native','sk-synthetic-complete-key-501','native',true,'["*"]','{}','',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportAxonHubV14(db, raw, "collision-origin", false); err == nil {
		t.Fatal("import accepted a downstream key that already belongs to another origin")
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`); got != 0 {
		t.Fatalf("failed import persisted %d channels", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM token_routes`); got != 0 {
		t.Fatalf("failed import persisted %d routes", got)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM downstream_api_keys`); got != 1 {
		t.Fatalf("failed import changed downstream keys: %d", got)
	}
}

func TestAxonHubCompleteFixtureReplacement(t *testing.T) {
	db := openAxonHubTestDB(t)
	full := readAxonHubCompleteFixture(t)
	if _, err := ImportAxonHubV14(db, full, "complete-origin", false); err != nil {
		t.Fatal(err)
	}
	reduced := removeAxonHubChannelAndItsModels(t, full, 1)
	preview, err := PreviewAxonHubV14(db, reduced, "complete-origin")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Removals) == 0 || preview.RemovalImpact == nil {
		t.Fatalf("reduced snapshot preview removals = %+v", preview)
	}
	if _, err := ImportAxonHubV14(db, reduced, "complete-origin", false); !errors.Is(err, ErrAxonHubReplacementRequired) {
		t.Fatalf("unconfirmed replacement = %v", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`); got != 86 {
		t.Fatalf("refused replacement changed channels: %d", got)
	}
	if _, err := ImportAxonHubV14(db, reduced, "complete-origin", true); err != nil {
		t.Fatalf("confirmed replacement: %v", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels WHERE name = 'Complete OpenAI primary'`); got != 0 {
		t.Fatal("replacement kept the removed channel")
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`); got >= 86 {
		t.Fatalf("replacement did not remove channels: %d", got)
	}
}

func loadImportedAccessPolicy(t *testing.T, db *store.DB, key string) store.DownstreamAccessPolicy {
	t.Helper()
	var raw string
	if err := db.Get(&raw, db.Rebind(`SELECT access_policy FROM downstream_api_keys WHERE key = ?`), key); err != nil {
		t.Fatalf("load policy %s: %v", key, err)
	}
	policy, err := store.ParseDownstreamAccessPolicy(raw)
	if err != nil {
		t.Fatalf("parse policy %s: %v", key, err)
	}
	if policy == nil {
		t.Fatalf("policy %s is empty", key)
	}
	return *policy
}

func findChannel(plan *axonHubPlan, sourceID int) *axonHubPlanChannel {
	for i := range plan.channels {
		if plan.channels[i].SourceID == sourceID {
			return &plan.channels[i]
		}
	}
	return nil
}

func findRoute(plan *axonHubPlan, pattern string) *axonHubPlanRoute {
	for i := range plan.routes {
		if plan.routes[i].Pattern == pattern {
			return &plan.routes[i]
		}
	}
	return nil
}

func hasRoute(plan *axonHubPlan, pattern string) bool { return findRoute(plan, pattern) != nil }

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func protocolOrderEqual(got, want store.DirectProtocolOrder) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
