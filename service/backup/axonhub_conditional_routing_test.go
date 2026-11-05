package backup

import (
	"encoding/json"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestAxonHubActiveConditionCompilesExecutableRoute(t *testing.T) {
	raw := axonHubConditionPayload(t, `{"enabled":true,"condition":{"type":"group","conditions":[{"type":"condition","field":"stream","operator":"eq","value":true}]}}`, false)
	src, err := ParseAxonHubSource(raw)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := CompileAxonHubPlan(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.routes) != 1 || len(plan.routes[0].Items) != 1 {
		t.Fatalf("active condition must remain executable: routes=%d residuals=%v", len(plan.routes), plan.residuals)
	}
}

func TestAxonHubConditionalAlternativesPersistAndReimport(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := axonHubConditionPayload(t, `{"enabled":true,"condition":{"type":"group","conditions":[{"field":"stream","operator":"eq","value":true}]}}`, false)
	var document map[string]any
	if json.Unmarshal(raw, &document) != nil {
		t.Fatal("fixture decode")
	}
	model := document["models"].([]any)[0].(map[string]any)
	settings := model["settings"].(map[string]any)
	conditional := settings["associations"].([]any)[0].(map[string]any)
	conditional["priority"] = 1
	unconditional := map[string]any{"type": "channel_model", "priority": 9, "channelModel": map[string]any{"channelId": 1, "modelId": "gpt-6"}}
	settings["associations"] = []any{unconditional, conditional}
	raw, _ = json.Marshal(document)
	var priorID int64
	for i := 0; i < 2; i++ {
		if _, err := ImportAxonHubV14(db, raw, "conditional-edges", false); err != nil {
			t.Fatal(err)
		}
		var id int64
		var rules store.DirectAssociationConditions
		if err := db.QueryRow(`SELECT id, association_conditions FROM upstream_group_items`).Scan(&id, &rules); err != nil {
			t.Fatal(err)
		}
		if len(rules) != 2 || rules[0].Priority != 1 || rules[0].Condition == nil || rules[1].Priority != 9 || rules[1].Condition != nil {
			t.Fatalf("association alternatives lost: %+v", rules)
		}
		if i > 0 && id != priorID {
			t.Fatal("reimport replaced source identity")
		}
		priorID = id
	}
	payload, err := BuildPayload(db.DB, "all")
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(payload)
	if !json.Valid(encoded) {
		t.Fatal("invalid portable export")
	}
	var exported map[string]any
	_ = json.Unmarshal(encoded, &exported)
	// Portable exports enumerate real table columns; the condition edge must
	// survive just as the grant and source-origin references do.
	if !containsExportedCondition(encoded) {
		t.Fatal("portable export dropped persisted condition")
	}
	settings["associations"] = []any{unconditional}
	raw, _ = json.Marshal(document)
	if _, err := ImportAxonHubV14(db, raw, "conditional-edges", false); err != nil {
		t.Fatal(err)
	}
	var rules store.DirectAssociationConditions
	if err := db.Get(&rules, `SELECT association_conditions FROM upstream_group_items`); err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatal("reimport left a stale condition")
	}
}

func TestAxonHubConditionInvalidEnabledNeverBecomesUnconditional(t *testing.T) {
	raw := axonHubConditionPayload(t, `{"enabled":null,"condition":{"conditions":[{"field":"stream","operator":"eq","value":true}]}}`, false)
	if _, err := ParseAxonHubSource(raw); err == nil {
		t.Fatal("null enablement became an unconditional association")
	}
}

func TestAxonHubConditionalEdgesRemainOriginScoped(t *testing.T) {
	db := openAxonHubTestDB(t)
	condition := `{"enabled":true,"condition":{"conditions":[{"field":"stream","operator":"eq","value":true}]}}`
	first := axonHubConditionPayload(t, condition, false)
	if _, err := ImportAxonHubV14(db, first, "condition-first", false); err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	_ = json.Unmarshal(first, &document)
	model := document["models"].([]any)[0].(map[string]any)
	model["model_id"] = "second-public-model"
	second, _ := json.Marshal(document)
	if _, err := ImportAxonHubV14(db, second, "condition-second", false); err != nil {
		t.Fatal(err)
	}
	without := axonHubConditionPayload(t, `{"enabled":false}`, false)
	if _, err := ImportAxonHubV14(db, without, "condition-first", false); err != nil {
		t.Fatal(err)
	}
	var firstRules, secondRules store.DirectAssociationConditions
	if err := db.Get(&firstRules, db.Rebind(`SELECT association_conditions FROM upstream_group_items WHERE origin_key=?`), "condition-first"); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&secondRules, db.Rebind(`SELECT association_conditions FROM upstream_group_items WHERE origin_key=?`), "condition-second"); err != nil {
		t.Fatal(err)
	}
	if len(firstRules) != 0 || len(secondRules) != 1 || secondRules[0].Condition == nil {
		t.Fatal("reimport crossed the association origin boundary")
	}
}

func containsExportedCondition(raw []byte) bool {
	var data any
	if json.Unmarshal(raw, &data) != nil {
		return false
	}
	var visit func(any) bool
	visit = func(v any) bool {
		switch node := v.(type) {
		case map[string]any:
			if value, ok := node["association_conditions"]; ok {
				if text, ok := value.(string); ok {
					var rules store.DirectAssociationConditions
					return rules.Scan(text) == nil && len(rules) == 2
				}
			}
			for _, value := range node {
				if visit(value) {
					return true
				}
			}
		case []any:
			for _, value := range node {
				if visit(value) {
					return true
				}
			}
		}
		return false
	}
	return visit(data)
}
