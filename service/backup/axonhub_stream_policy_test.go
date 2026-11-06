package backup

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service"
)

func TestAxonHubStreamPoliciesCompile(t *testing.T) {
	for _, policy := range []string{"unlimited", "require", "forbid"} {
		t.Run(policy, func(t *testing.T) {
			raw := axonHubConditionPayload(t, `{"enabled":false}`, false)
			var document map[string]json.RawMessage
			if err := json.Unmarshal(raw, &document); err != nil {
				t.Fatal(err)
			}
			var channels []map[string]json.RawMessage
			if err := json.Unmarshal(document["channels"], &channels); err != nil {
				t.Fatal(err)
			}
			channels[0]["policies"], _ = json.Marshal(map[string]string{"stream": policy})
			document["channels"], _ = json.Marshal(channels)
			raw, _ = json.Marshal(document)
			source, err := ParseAxonHubSource(raw)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := CompileAxonHubPlan(source)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.channels) != 1 || len(plan.routes) != 1 {
				t.Fatalf("stream policy %s is not executable: skipped=%+v residuals=%v", policy, plan.skipped, plan.residuals)
			}
		})
	}
}

func TestAxonHubStreamPolicyReimportUpdatesPersistedCandidate(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := axonHubConditionPayload(t, `{"enabled":false}`, false)
	var document map[string]json.RawMessage
	_ = json.Unmarshal(raw, &document)
	var channels []map[string]json.RawMessage
	_ = json.Unmarshal(document["channels"], &channels)
	var originalID int64
	for _, policy := range []string{"require", "forbid", "unlimited"} {
		channels[0]["policies"], _ = json.Marshal(map[string]string{"stream": policy})
		document["channels"], _ = json.Marshal(channels)
		raw, _ = json.Marshal(document)
		if _, err := ImportAxonHubV14(db, raw, "stream-source", false); err != nil {
			t.Fatal(err)
		}
		var id int64
		var stored string
		if err := db.QueryRow(`SELECT id,stream_policy FROM upstream_channels`).Scan(&id, &stored); err != nil {
			t.Fatal(err)
		}
		if stored != policy || originalID != 0 && id != originalID {
			t.Fatalf("reimport identity/policy=%d/%s", id, stored)
		}
		originalID = id
		var routeID int64
		if err := db.Get(&routeID, `SELECT id FROM token_routes LIMIT 1`); err != nil {
			t.Fatal(err)
		}
		rows, err := service.NewProxyRoutingStore(db).LoadRouteChannels(context.Background(), []int64{routeID})
		if err != nil || len(rows) != 1 || rows[0].Channel.Direct.StreamPolicy != policy {
			t.Fatalf("routing did not load policy=%s err=%v", policy, err)
		}
		payload, err := BuildPayload(db.DB, "all")
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(payload)
		if !strings.Contains(string(encoded), `"stream_policy":"`+policy+`"`) {
			t.Fatal("portable export lost policy")
		}
	}
}
