package admin

import (
	"reflect"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestStatsFlowEdges(t *testing.T) {
	bothStatsDatabases(t, func(t *testing.T, db *store.DB, r chi.Router) {
		get := func(query string) map[string]any { return catalogCall(t, r, "GET", "/api/stats/flow"+query, nil, 200) }
		empty := get("")
		if empty["period"] != "7d" || empty["totalRequests"] != float64(0) || !reflect.DeepEqual(empty["edges"], []any{}) {
			t.Fatalf("empty=%v", empty)
		}
		if rec := doGet(t, r, "/api/stats/flow?period=week"); rec.Code != 400 {
			t.Fatalf("invalid period: %d", rec.Code)
		}
		mustExec(t, db, `INSERT INTO sites (id,name,url,platform) VALUES (1,'site','https://site.example','openai')`)
		mustExec(t, db, `INSERT INTO accounts (id,site_id,access_token) VALUES (1,1,'fixture')`)
		mustExec(t, db, `INSERT INTO downstream_api_keys (id,name,key) VALUES (9,'fixture-key-name','fixture-private')`)
		now := time.Now().UTC().Add(-time.Minute)
		stamp := now.Format(time.RFC3339)
		for _, entry := range []struct{ id, path string }{{"invocation", "/v1/messages"}, {"audit", "/v1/models"}} {
			mustExec(t, db, `INSERT INTO gateway_requests (id,model_requested,downstream_path,transport,terminal_status,attempt_count,output_started,latency_ms,started_at,ended_at) VALUES (?,'alias',?,'http','completed',1,FALSE,1,?,?)`, entry.id, entry.path, stamp, stamp)
		}
		for _, row := range []struct {
			gateway, status string
			latency         int
		}{{"invocation", "success", 100}, {"", "failed", 200}, {"audit", "failed", 400}} {
			mustExec(t, db, `INSERT INTO proxy_logs (account_id,channel_id,downstream_api_key_id,model_requested,model_actual,status,prompt_tokens,completion_tokens,estimated_cost,latency_ms,created_at,gateway_request_id) VALUES (1,8,9,'alias','served',?,5,2,0.25,?,?,?)`, row.status, row.latency, stamp, row.gateway)
		}
		for i := 0; i < 55; i++ {
			mustExec(t, db, `INSERT INTO proxy_logs (channel_id,model_requested,status,total_tokens,created_at) VALUES (?,'legacy','success',10,?)`, i, stamp)
		}
		mustExec(t, db, `INSERT INTO proxy_logs (created_at) VALUES (?),(?)`, now.AddDate(0, 0, -10).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339))
		for _, tc := range []struct {
			query string
			edges int
		}{{"", 20}, {"?topN=0", 1}, {"?topN=99", 50}} {
			out := get(tc.query)
			edges := out["edges"].([]any)
			if len(edges) != tc.edges || out["totalRequests"] != float64(57) || out["totalFailed"] != float64(1) || out["totalTokens"] != float64(564) || out["totalCost"] != 0.5 {
				t.Fatalf("flow counts: %v", out)
			}
			want := map[string]any{"downstreamKeyId": float64(9), "downstreamKeyName": "fixture-key-name", "model": "served", "siteId": float64(1), "siteName": "site", "channelId": float64(8), "requests": float64(2), "failedCount": float64(1), "totalTokens": float64(14), "totalCost": 0.5, "averageLatencyMs": float64(150)}
			if !reflect.DeepEqual(edges[0], want) {
				t.Errorf("top edge=%v", edges[0])
			}
			if len(edges) > 1 {
				edge := edges[1].(map[string]any)
				if edge["siteId"] != nil || edge["downstreamKeyName"] != nil || edge["averageLatencyMs"] != nil {
					t.Errorf("unassigned=%v", edge)
				}
			}
		}
		all := get("?period=all")
		if _, ok := all["window"].(map[string]any)["from"]; ok || all["totalRequests"] != float64(58) {
			t.Fatalf("all=%v", all)
		}
		db.Close()
		if rec := doGet(t, r, "/api/stats/flow"); rec.Code != 500 {
			t.Fatalf("closed database: %d", rec.Code)
		}
	})
}
