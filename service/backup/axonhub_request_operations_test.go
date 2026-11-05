package backup

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func requestOperationsPayload(settings string) []byte {
	raw := axonHubSettingsPayload(settings)
	// Qiniu's audited default is Chat-only, so every declared endpoint uses the
	// same single JSON request that these operations govern.
	return []byte(strings.Replace(string(raw), `"type":"openai"`, `"type":"qiniu"`, 1))
}

func TestAxonHubRequestOperationsPlanPersistenceAndReimport(t *testing.T) {
	raw := requestOperationsPayload(`{
		"bodyOverrideOperations":[{"op":"set","path":"temperature","value":"0.2"},{"op":"set_if_absent","path":"metadata.route","value":"{{.RequestModel}}"}],
		"headerOverrideOperations":[{"op":"set","path":"X-Private-Tag","value":"synthetic-private"}],
		"transformOptions":{"replaceDeveloperRoleWithSystem":true,"reasoningEffortMapping":[{"from":"high","to":"max"}]}
	}`)
	src, err := ParseAxonHubSource(raw)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := CompileAxonHubPlan(src)
	if err != nil || len(plan.channels) != 1 || len(plan.skipped) != 0 {
		t.Fatalf("supported operations skipped: %v %#v", err, plan)
	}
	want := plan.channels[0].RequestConfig
	if len(want.Body) != 2 || len(want.Headers) != 1 || !want.NativeChatMapping || !want.Options.ReplaceDeveloperRoleWithSystem {
		t.Fatalf("compiled config lost fields: %+v", want)
	}
	db := openAxonHubTestDB(t)
	for round := 0; round < 2; round++ {
		if _, err := ImportAxonHubV14(db, raw, "operation-source", false); err != nil {
			t.Fatal(err)
		}
		var actual store.DirectRequestConfig
		if err := db.Get(&actual, db.Rebind(`SELECT request_config FROM upstream_channels WHERE origin_key=?`), "operation-source"); err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(actual)
		expected, _ := json.Marshal(want)
		if string(encoded) != string(expected) {
			t.Fatalf("round %d changed config: %s", round, encoded)
		}
	}
	var routeID int64
	if err := db.Get(&routeID, `SELECT id FROM token_routes LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	candidates, err := service.NewProxyRoutingStore(db).LoadRouteChannels(context.Background(), []int64{routeID})
	if err != nil || len(candidates) != 1 || len(candidates[0].Channel.Direct.RequestConfig.Body) != 2 {
		t.Fatalf("routing lost persisted config: %v %#v", err, candidates)
	}
	preview, err := PreviewAxonHubV14(db, raw, "operation-source")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(preview)
	if strings.Contains(string(encoded), "synthetic-private") {
		t.Fatal("preview exposed operation values")
	}
	other := []byte(strings.ReplaceAll(string(requestOperationsPayload(`{}`)), "gpt-6", "other-model"))
	if _, err := ImportAxonHubV14(db, other, "other-source", false); err != nil {
		t.Fatal(err)
	}
	if countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`) != 2 {
		t.Fatal("origins did not remain isolated")
	}
}

func TestAxonHubLegacyOperationArrayIsCompiled(t *testing.T) {
	raw := requestOperationsPayload(`{"overrideParameters":"[{\"op\":\"set\",\"path\":\"temperature\",\"value\":\"0.3\"}]"}`)
	src, err := ParseAxonHubSource(raw)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := CompileAxonHubPlan(src)
	if err != nil || len(plan.channels) != 1 || len(plan.channels[0].RequestConfig.Body) != 1 || plan.channels[0].ParamOverride != "" {
		t.Fatalf("legacy operation array lost: %v %#v", err, plan)
	}
}

func TestAxonHubRequestOperationsRefusalAndEmptyPrecedence(t *testing.T) {
	for _, tc := range []struct{ name, settings, reason string }{
		{"unknown op", `{"bodyOverrideOperations":[{"op":"future","path":"x"}]}`, "channel_request_operation_unsupported:operation_unsupported"},
		{"unknown field", `{"bodyOverrideOperations":[{"op":"set","path":"x","future":true}]}`, "channel_settings_unsupported:bodyOverrideOperations"},
		{"model", `{"bodyOverrideOperations":[{"op":"rename","from":"metadata.other","to":"model"}]}`, "channel_request_operation_unsupported:operation_path_unsupported"},
		{"cap", `{"bodyOverrideOperations":[{"op":"delete","path":"max_tokens"}]}`, "channel_request_operation_unsupported:operation_destination_unsupported"},
		{"auth copy", `{"headerOverrideOperations":[{"op":"copy","from":"Authorization","to":"X-Leak"}]}`, "channel_request_operation_unsupported:operation_path_unsupported"},
		{"header no source implementation", `{"headerOverrideOperations":[{"op":"set_if_absent","path":"X-A","value":"a"}]}`, "channel_request_operation_unsupported:header_operation_unsupported"},
		{"empty remove match", `{"bodyOverrideOperations":[{"op":"array_remove","path":"messages","match":{"path":"role","eq":""}}]}`, "channel_request_operation_unsupported:array_match_invalid"},
		{"empty overrides legacy", `{"overrideParameters":"broken","overrideHeaders":[{"key":"Authorization","value":"bad"}],"bodyOverrideOperations":[],"headerOverrideOperations":[]}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src, err := ParseAxonHubSource(requestOperationsPayload(tc.settings))
			if err != nil {
				t.Fatal(err)
			}
			plan, err := CompileAxonHubPlan(src)
			if err != nil {
				t.Fatal(err)
			}
			if tc.reason == "" {
				if len(plan.channels) != 1 || plan.channels[0].RequestConfig.Active() || plan.channels[0].ParamOverride != "" {
					t.Fatalf("empty operations did not replace legacy: %#v", plan)
				}
			} else if len(plan.channels) != 0 || len(plan.skipped) != 1 || !residualContains(plan.skipped[0].Reasons, tc.reason) {
				t.Fatalf("missing exact refusal %q: %#v", tc.reason, plan.skipped)
			}
		})
	}
}
