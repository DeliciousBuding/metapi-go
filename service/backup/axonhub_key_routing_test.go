package backup

import (
	"encoding/json"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestAxonHubActiveKeyRoutingCompiles(t *testing.T) {
	payload := accessPayload(t)
	key := payload["api_keys"].([]any)[0].(map[string]any)
	profile := key["profiles"].(map[string]any)["profiles"].([]any)[0].(map[string]any)
	profile["loadBalanceStrategy"] = "round-robin"
	profile["traceStickyMode"] = "prefer_previous_channel"
	source, err := ParseAxonHubSource(encodeAccessPayload(t, payload))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := CompileAxonHubPlan(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.keys) != 1 || plan.keys[0].Policy.BlockReason != "" {
		t.Fatalf("supported key routing still blocked: %+v", plan.keys)
	}
}

func TestAxonHubKeyRoutingPersistenceAndReimport(t *testing.T) {
	db := openAxonHubTestDB(t)
	payload := accessPayload(t)
	key := payload["api_keys"].([]any)[0].(map[string]any)
	profile := key["profiles"].(map[string]any)["profiles"].([]any)[0].(map[string]any)
	profile["loadBalanceStrategy"], profile["traceStickyMode"] = "round-robin", "prefer_previous_channel"
	for range 2 {
		if _, err := ImportAxonHubV14(db, encodeAccessPayload(t, payload), "key-routing", false); err != nil {
			t.Fatal(err)
		}
		var raw string
		if err := db.Get(&raw, db.Rebind(`SELECT access_policy FROM downstream_api_keys WHERE key=?`), "sk-fixture-access"); err != nil {
			t.Fatal(err)
		}
		var policy store.DownstreamAccessPolicy
		if json.Unmarshal([]byte(raw), &policy) != nil || policy.RoutingStrategy != "round_robin" || policy.TraceStickyMode != "prefer_previous_channel" || policy.AllowedUpstreamChannelIDs == nil || len(*policy.AllowedUpstreamChannelIDs) != 1 || len(policy.ModelMappings) != 1 {
			t.Fatalf("routing import lost policy boundaries: %+v", policy)
		}
	}
	profile["loadBalanceStrategy"], profile["traceStickyMode"] = "default", "system_default"
	if _, err := ImportAxonHubV14(db, encodeAccessPayload(t, payload), "key-routing", false); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := db.Get(&raw, db.Rebind(`SELECT access_policy FROM downstream_api_keys WHERE key=?`), "sk-fixture-access"); err != nil {
		t.Fatal(err)
	}
	policy, err := store.ParseDownstreamAccessPolicy(raw)
	if err != nil || policy.RoutingStrategy != "" || policy.TraceStickyMode != "" {
		t.Fatalf("default reimport kept override: %+v %v", policy, err)
	}
}

func TestAxonHubKeyRoutingSourcePrecedenceAndResiduals(t *testing.T) {
	source := &AxonHubSource{SystemConfigs: []AxonHubSourceSystemConfig{{Key: "retry_policy", Value: `{"load_balancer_strategy":"round-robin","trace_sticky_mode":"disabled"}`}}}
	model := AxonHubSourceModel{}
	var residuals []string
	strategy, sticky := axonHubModelRouting(source, model, &residuals, "model")
	if strategy != "round_robin" || sticky != "disabled" {
		t.Fatalf("system default lost: %s %s", strategy, sticky)
	}
	model.Settings.LoadBalancerStrategy, model.Settings.TraceStickyMode = "adaptive", "prefer_previous_channel"
	strategy, sticky = axonHubModelRouting(source, model, &residuals, "model")
	if strategy != "weighted" || sticky != "prefer_previous_channel" {
		t.Fatal("model did not override system")
	}
	for _, unsupported := range []string{"adaptive", "failover", "circuit-breaker", "unknown"} {
		policy := store.DownstreamAccessPolicy{}
		compileAxonHubKeyRouting(&AxonHubAccessProfile{LoadBalanceStrategy: &unsupported}, &policy, &residuals)
		if policy.BlockReason == "" || policy.RoutingStrategy != "" {
			t.Fatalf("non-equivalent source strategy silently approximated: %s %+v", unsupported, policy)
		}
	}
	for _, fallback := range []string{"", "default", "system_default"} {
		policy := store.DownstreamAccessPolicy{}
		compileAxonHubKeyRouting(&AxonHubAccessProfile{LoadBalanceStrategy: &fallback, TraceStickyMode: &fallback}, &policy, &residuals)
		if policy.RoutingStrategy != "" || policy.TraceStickyMode != "" || policy.BlockReason != "" {
			t.Fatal("source default became an active Key override")
		}
	}
}

func TestAxonHubUnsupportedKeyRegexRemainsBlockedWithOtherBoundaries(t *testing.T) {
	payload := accessPayload(t)
	key := payload["api_keys"].([]any)[0].(map[string]any)
	profile := key["profiles"].(map[string]any)["profiles"].([]any)[0].(map[string]any)
	profile["modelMappings"] = []any{map[string]any{"from": "(?=private).*", "to": "fixture-model"}}
	profile["traceStickyMode"] = "prefer_previous_channel"
	source, err := ParseAxonHubSource(encodeAccessPayload(t, payload))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := CompileAxonHubPlan(source)
	if err != nil {
		t.Fatal(err)
	}
	policy := plan.keys[0].Policy
	if policy.BlockReason != "source_key_model_mapping_unsupported" || len(policy.ModelIDs) != 1 || len(plan.keys[0].ChannelSourceIDs) != 1 || policy.TraceStickyMode != "prefer_previous_channel" {
		t.Fatalf("unsupported regex broadened or erased key boundary: %+v", policy)
	}
	if policy.AllowsModel("fixture-model") || policy.AllowsUpstreamChannel(4) {
		t.Fatal("blocked regex key became executable")
	}
}
