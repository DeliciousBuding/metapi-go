package router

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestUserRequestQuotaRouterContract(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newPortalFixture(t, dialect)
			cookie := f.activate(f.issue())
			ctx := context.Background()
			users := store.NewGatewayUserStore(f.db)
			other, err := users.CreateUser(ctx, "bob", "Other", f.group.ID, true)
			if err != nil {
				t.Fatal(err)
			}
			key, err := users.CreateOwnKey(ctx, f.user.ID, "Fixture", "fixture-quota-http", []string{"*"}, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.Exec(`UPDATE downstream_api_keys SET used_requests=31,used_cost=3 WHERE id=?`, key.ID); err != nil {
				t.Fatal(err)
			}
			initial := portalJSON(t, f.call("GET", "/api/user/usage", "", cookie, nil), 200)
			if !reflect.DeepEqual(initial, map[string]any{"usedRequests": float64(31), "usedCost": float64(3), "keyCount": float64(1), "recordedRequests": float64(0)}) {
				t.Fatalf("initial usage: %+v", initial)
			}
			cases := []struct {
				id, state  string
				tokens     any
				charged    any
				mismatched bool
			}{
				{"req_settled", "settled", int64(14), int64(28), false},
				{"req_zero", "settled", int64(0), int64(0), false},
				{"req_pending", "pending", nil, nil, false},
				{"req_released", "released", nil, int64(0), false},
				{"req_unknown", "unknown", nil, nil, false},
				{"req_pending_observed", "pending", int64(0), int64(0), false},
				{"req_legacy", "", nil, nil, false},
				{"req_mismatched", "settled", int64(14), int64(28), true},
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			model, private := "quota-model", "private-upstream-diagnostic"
			want := map[string]any{}
			for _, tc := range cases {
				if err := store.InsertGatewayRequest(ctx, f.db, store.GatewayRequest{ID: tc.id, OwnerUserID: &f.user.ID, DownstreamAPIKeyID: &key.ID, ModelRequested: &model, ReasonCode: &private, ExternalRequestID: &private, Transport: "http", TerminalStatus: "completed", AttemptCount: 2, LatencyMs: 17, StartedAt: now, EndedAt: now}); err != nil {
					t.Fatal(err)
				}
				want[tc.id] = nil
				if tc.state == "" {
					continue
				}
				ledgerOwner := f.user.ID
				if tc.mismatched {
					ledgerOwner = other.ID
				}
				if _, err := f.db.Exec(`INSERT INTO gateway_user_budget_ledger(request_id,user_id,group_id,multiplier_units,reserved_units,charged_units,total_tokens,state,created_at) VALUES(?,?,?,2000000,6000,?,?,?,?)`, tc.id, ledgerOwner, f.group.ID, tc.charged, tc.tokens, tc.state, now); err != nil {
					t.Fatal(err)
				}
				if !tc.mismatched {
					var tokens, charged any
					if tc.tokens != nil {
						tokens = float64(tc.tokens.(int64))
					}
					if tc.charged != nil {
						charged = float64(tc.charged.(int64)) / 1e6
					}
					want[tc.id] = map[string]any{"unit": "token_credits", "state": tc.state, "totalTokens": tokens, "groupMultiplier": float64(2), "reserved": 0.006, "charged": charged}
				}
			}
			// A user-owned ledger alone must not grant access to another user's
			// request, an ownerless record, or a request absent from retained logs.
			for i, owner := range []*int64{&other.ID, nil} {
				id := fmt.Sprintf("req_foreign_%d", i)
				if err := store.InsertGatewayRequest(ctx, f.db, store.GatewayRequest{ID: id, OwnerUserID: owner, Transport: "http", TerminalStatus: "completed", StartedAt: now, EndedAt: now}); err != nil {
					t.Fatal(err)
				}
				if _, err := f.db.Exec(`INSERT INTO gateway_user_budget_ledger(request_id,user_id,group_id,multiplier_units,reserved_units,state,created_at) VALUES(?,?,?,1000000,0,'pending',?)`, id, f.user.ID, f.group.ID, now); err != nil {
					t.Fatal(err)
				}
				portalJSON(t, f.call("GET", "/api/user/requests/"+id, "", cookie, nil), 404)
			}
			if _, err := f.db.Exec(`INSERT INTO gateway_user_budget_ledger(request_id,user_id,group_id,multiplier_units,reserved_units,state,created_at) VALUES('req_ledger_only',?,?,1000000,0,'pending',?)`, f.user.ID, f.group.ID, now); err != nil {
				t.Fatal(err)
			}
			portalJSON(t, f.call("GET", "/api/user/requests/req_ledger_only", "", cookie, nil), 404)
			if _, err := users.SetUserGroupPricing(ctx, f.group.ID, 9); err != nil {
				t.Fatal(err)
			}
			seen := map[string]bool{}
			for page := 1; page <= 4; page++ {
				response := f.call("GET", fmt.Sprintf("/api/user/requests?page=%d&pageSize=3", page), "", cookie, nil)
				list := portalJSON(t, response, 200)
				items := list["items"].([]any)
				if list["total"] != float64(len(cases)) || len(items) > 3 || page == 4 && len(items) != 0 || strings.Contains(response.Body.String(), private) {
					t.Fatalf("unsafe page=%d response=%s", page, response.Body.String())
				}
				for _, value := range items {
					item := value.(map[string]any)
					id := item["id"].(string)
					expected, ok := want[id]
					if !ok || seen[id] || len(item) != 8 || !reflect.DeepEqual(item["quota"], expected) || item["modelRequested"] != model || item["attemptCount"] != float64(2) || item["latencyMs"] != float64(17) || item["terminalStatus"] != "completed" {
						t.Fatalf("request=%+v quota want=%+v", item, expected)
					}
					seen[id] = true
					detail := portalJSON(t, f.call("GET", "/api/user/requests/"+id, "", cookie, nil), 200)
					if !reflect.DeepEqual(detail, item) {
						t.Fatalf("list/detail differ: %+v %+v", item, detail)
					}
				}
			}
			if len(seen) != len(cases) {
				t.Fatalf("saw %d requests, want %d", len(seen), len(cases))
			}
			usage := portalJSON(t, f.call("GET", "/api/user/usage", "", cookie, nil), 200)
			if usage["recordedRequests"] != float64(len(cases)) || usage["usedRequests"] != float64(31) || usage["usedCost"] != float64(3) || usage["keyCount"] != float64(1) {
				t.Fatalf("request/Key counts conflated: %+v", usage)
			}
			portalJSON(t, f.call("DELETE", fmt.Sprintf("/api/user/keys/%d", key.ID), "", cookie, nil), 200)
			after := portalJSON(t, f.call("GET", "/api/user/usage", "", cookie, nil), 200)
			if !reflect.DeepEqual(after, map[string]any{"usedRequests": float64(0), "usedCost": float64(0), "keyCount": float64(0), "recordedRequests": float64(len(cases))}) {
				t.Fatalf("deleted Key erased recorded requests: %+v", after)
			}
			detail := portalJSON(t, f.call("GET", "/api/user/requests/req_settled", "", cookie, nil), 200)
			if !reflect.DeepEqual(detail["quota"], want["req_settled"]) {
				t.Fatalf("deleted Key changed quota: %+v", detail)
			}
		})
	}
}

func TestUserRequestQuotaRouterReadFailures(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			f := newPortalFixture(t, dialect)
			cookie := f.activate(f.issue())
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if err := store.InsertGatewayRequest(context.Background(), f.db, store.GatewayRequest{ID: "req_errors", OwnerUserID: &f.user.ID, Transport: "http", TerminalStatus: "completed", StartedAt: now, EndedAt: now}); err != nil {
				t.Fatal(err)
			}
			// A missing ledger row is a successful null projection; a missing
			// ledger table is an unavailable service, even for an empty page.
			detail := portalJSON(t, f.call("GET", "/api/user/requests/req_errors", "", cookie, nil), 200)
			if quota, ok := detail["quota"]; !ok || quota != nil {
				t.Fatalf("missing ledger: %+v", detail)
			}
			if _, err := f.db.Exec(`ALTER TABLE gateway_user_budget_ledger RENAME TO unavailable_quota_ledger`); err != nil {
				t.Fatal(err)
			}
			unavailable := func(path string) {
				t.Helper()
				body := portalJSON(t, f.call("GET", path, "", cookie, nil), 503)
				if !reflect.DeepEqual(body, map[string]any{"error": "User service unavailable"}) {
					t.Fatalf("database details leaked: %+v", body)
				}
			}
			for _, path := range []string{"/api/user/requests/req_errors", "/api/user/requests", "/api/user/requests?page=2"} {
				unavailable(path)
			}
			portalJSON(t, f.call("GET", "/api/user/usage", "", cookie, nil), 200)
			if _, err := f.db.Exec(`ALTER TABLE gateway_requests RENAME TO unavailable_requests`); err != nil {
				t.Fatal(err)
			}
			unavailable("/api/user/usage")
			unavailable("/api/user/requests")
		})
	}
}
