package backup

import (
	"encoding/json"
	"testing"
)

func axonHubConditionPayload(t *testing.T, when string, inherited bool) []byte {
	t.Helper()
	var payload map[string]any
	if err := json.Unmarshal(axonHubSettingsPayload(`{}`), &payload); err != nil {
		t.Fatal(err)
	}
	var association map[string]any
	if err := json.Unmarshal([]byte(`{"type":"channel_model","channelModel":{"channelId":1,"modelId":"gpt-6"},"when":`+when+`}`), &association); err != nil {
		t.Fatal(err)
	}
	model := payload["models"].([]any)[0].(map[string]any)
	if inherited {
		model["developer"] = "fixture-developer"
		delete(model, "settings")
		settings, err := json.Marshal(map[string]any{"developer_settings": []any{map[string]any{
			"developer": "fixture-developer", "associations": []any{association},
		}}})
		if err != nil {
			t.Fatal(err)
		}
		payload["system_configs"] = []any{map[string]any{"key": "system_model_settings", "value": string(settings)}}
	} else {
		model["settings"] = map[string]any{"associations": []any{association}}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestAxonHubConditionContract(t *testing.T) {
	for _, inherited := range []bool{false, true} {
		for _, tc := range []struct {
			name, when, residual string
			wantRoute            bool
		}{
			{"absent", `null`, "", true},
			{"empty", `{}`, "", true},
			{"disabled", `{"enabled":false,"condition":{"type":"field","field":"stream","operator":"eq","value":true}}`, "", true},
			{"enabled without predicate", `{"enabled":true}`, "", true},
			{"enabled null predicate", `{"enabled":true,"condition":null}`, "", true},
			{"active predicate", `{"enabled":true,"condition":{"type":"field","field":"stream","operator":"eq","value":true}}`, "conditional_model_association_not_imported", false},
			{"unknown predicate field", `{"enabled":false,"futureGuard":{"token":"fixture-private-marker"}}`, "model_association_field_unsupported:when.futureGuard", false},
		} {
			name := "local/"
			if inherited {
				name = "inherited/"
			}
			t.Run(name+tc.name, func(t *testing.T) {
				raw := axonHubConditionPayload(t, tc.when, inherited)
				src, err := ParseAxonHubSource(raw)
				if err != nil {
					t.Fatal(err)
				}
				plan, err := CompileAxonHubPlan(src)
				if err != nil {
					t.Fatal(err)
				}
				if got := len(plan.routes) == 1; got != tc.wantRoute {
					t.Fatalf("route present=%t, want %t; residuals=%v", got, tc.wantRoute, plan.residuals)
				}
				if tc.residual != "" && !residualContains(plan.residuals, tc.residual) {
					t.Fatalf("missing residual %s: %v", tc.residual, plan.residuals)
				}
				preview, err := PreviewAxonHubV14(nil, raw, "condition-fixture")
				if err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(preview)
				if err != nil {
					t.Fatal(err)
				}
				if residualContains([]string{string(encoded)}, "fixture-private-marker") {
					t.Fatal("condition payload leaked into preview")
				}
			})
		}
	}
}

func TestAxonHubConditionImportPreservesUnconditionalGraph(t *testing.T) {
	for _, inherited := range []bool{false, true} {
		t.Run(map[bool]string{false: "local", true: "inherited"}[inherited], func(t *testing.T) {
			db := openAxonHubTestDB(t)
			raw := axonHubConditionPayload(t, `{"enabled":true,"condition":null}`, inherited)
			for i := 0; i < 2; i++ {
				counts, err := ImportAxonHubV14(db, raw, "condition-fixture", false)
				if err != nil {
					t.Fatal(err)
				}
				if counts["routes"] != 1 || counts["grants"] != 1 {
					t.Fatalf("unconditional import counts=%v", counts)
				}
			}
			for _, table := range []string{"upstream_channels", "upstream_credentials", "upstream_models", "upstream_grants", "token_routes"} {
				if countRows(t, db, "SELECT COUNT(*) FROM "+table) != 1 {
					t.Fatalf("reimport changed %s cardinality", table)
				}
			}
			guarded := axonHubConditionPayload(t, `{"futureGuard":true}`, inherited)
			if _, err := ImportAxonHubV14(db, guarded, "condition-fixture", false); err == nil {
				t.Fatal("reimport removed the existing route without replacement confirmation")
			}
			if got := countRows(t, db, "SELECT COUNT(*) FROM token_routes"); got != 1 {
				t.Fatalf("failed origin import changed the existing route: %d", got)
			}
		})
	}
}
