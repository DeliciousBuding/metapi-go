package admin

import (
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func seedGatewayRequest(t *testing.T, db *store.DB, id, status, at string) {
	t.Helper()
	model, path, external := "request-model", "/v1/responses", "same-external-id"
	key := int64(42)
	httpStatus := 200
	entry := store.GatewayRequest{ID: id, ExternalRequestID: &external, Transport: "http", TerminalStatus: status, ModelRequested: &model, DownstreamPath: &path, DownstreamAPIKeyID: &key, HTTPStatus: &httpStatus, OutputStarted: status == "output_interrupted", LatencyMs: 101, StartedAt: at, EndedAt: at}
	if err := store.InsertGatewayRequest(context.Background(), db, entry); err != nil {
		t.Fatal(err)
	}
}

func TestGatewayRequestsQueries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*testing.T) (*store.DB, chi.Router)
	}{{"sqlite", setupStatsSQLiteTest}, {"postgres", setupStatsPostgresTest}} {
		t.Run(tc.name, func(t *testing.T) {
			db, r := tc.setup(t)
			at := "2026-10-16T00:00:00.5Z"
			statuses := []string{"completed", "upstream_failed", "output_interrupted", "client_cancelled", "rejected_no_route", "rejected_policy", "gateway_failed"}
			for i, status := range statuses {
				seedGatewayRequest(t, db, "internal-"+strconv.Itoa(i), status, at)
			}
			if _, err := db.Exec("UPDATE gateway_requests SET reason_code = 'protocol_interrupted' WHERE id = 'internal-2'"); err != nil {
				t.Fatal(err)
			}
			for _, data := range []struct {
				id      string
				ordinal int
				status  string
			}{{"internal-0", 0, "failed"}, {"internal-0", 1, "success"}, {"internal-2", 0, "failed"}} {
				if _, err := db.Exec(`INSERT INTO proxy_logs (gateway_request_id,request_id,retry_count,status,created_at) VALUES (?,'same-external-id',?,?,?)`, data.id, data.ordinal, data.status, at); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := db.Exec("UPDATE gateway_requests SET attempt_count = CASE WHEN id = 'internal-0' THEN 2 WHEN id = 'internal-2' THEN 1 ELSE 0 END"); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec("INSERT INTO proxy_logs (request_id,status,created_at) VALUES ('same-external-id','success',?)", at); err != nil {
				t.Fatal(err)
			}
			response := doGet(t, r, "/api/stats/requests?limit=2&offset=1")
			if response.Code != 200 {
				t.Fatal(response.Body.String())
			}
			var body struct {
				Items             []gatewayRequestListItem `json:"items"`
				Total             int
				Summary           gatewayRequestSummary
				CoverageStartedAt *string
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Total != 7 || len(body.Items) != 2 || body.Summary.CompletedCount != 1 || body.Summary.FailedCount != 3 || body.Summary.CancelledCount != 1 || body.Summary.RejectedCount != 2 || body.CoverageStartedAt == nil {
				t.Fatalf("wrong filtered/terminal summary: %s", response.Body.String())
			}
			response = doGet(t, r, "/api/stats/requests/internal-0")
			if response.Code != 200 {
				t.Fatal(response.Body.String())
			}
			var detail struct {
				store.GatewayRequest
				Attempts []map[string]any
			}
			if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
				t.Fatal(err)
			}
			if detail.AttemptCount != 2 || detail.Attempts[0]["retryCount"] != float64(0) || detail.Attempts[1]["status"] != "success" {
				t.Fatalf("external IDs conflated requests: %s", response.Body.String())
			}
			// The compatible attempt-detail API must not merge independent turns
			// either, even though all four seeded attempts share one external ID.
			logID := int64(detail.Attempts[0]["id"].(float64))
			legacyDetail := getRequestAttempts(t, r, logID)
			if attempts := legacyDetail["attempts"].([]any); len(attempts) != 2 {
				t.Fatalf("legacy detail conflated internal requests: %+v", legacyDetail)
			}
			response = doGet(t, r, "/api/stats/requests/internal-4")
			if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
				t.Fatal(err)
			}
			if response.Code != 200 || detail.AttemptCount != 0 || detail.TerminalStatus != "rejected_no_route" || len(detail.Attempts) != 0 {
				t.Fatalf("zero-dispatch invisible: %s", response.Body.String())
			}
			for _, query := range []string{
				"status=output_interrupted",
				"search=internal-2",
				"search=PROTOCOL_INTERRUPTED",
				"search=%5F",
				"search=request-model&status=output_interrupted&downstreamKeyId=42",
				"from=" + url.QueryEscape("2026-10-16T08:00:00+08:00") + "&to=2026-10-16T00:00:00.6Z&status=output_interrupted",
			} {
				response = doGet(t, r, "/api/stats/requests?"+query)
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if response.Code != 200 || body.Total != 1 || body.Summary.FailedCount != 1 || len(body.Items) != 1 || body.Items[0].HTTPStatus == nil || *body.Items[0].HTTPStatus != 200 || !body.Items[0].OutputStarted {
					t.Fatalf("query %s: %s", query, response.Body.String())
				}
			}
			for _, query := range []string{"search=%25", "downstreamKeyId=43", "from=2026-10-16T00:00:00.6Z"} {
				response = doGet(t, r, "/api/stats/requests?"+query)
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if response.Code != 200 || body.Total != 0 || body.Summary.TotalCount != 0 || len(body.Items) != 0 {
					t.Fatalf("literal/empty filter %s: %s", query, response.Body.String())
				}
			}
			if response := doGet(t, r, "/api/stats/requests/absent"); response.Code != 404 {
				t.Fatalf("missing request: %d", response.Code)
			}
			for _, query := range []string{"limit=0", "limit=101", "limit=x", "offset=-1", "status=success", "downstreamKeyId=0", "downstreamKeyId=bad", "from=bad", "to=bad", "from=2026-10-17T00:00:00Z&to=2026-10-16T00:00:00Z"} {
				if response := doGet(t, r, "/api/stats/requests?"+query); response.Code != 400 {
					t.Fatalf("invalid query %s: %d %s", query, response.Code, response.Body.String())
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{"/api/stats/requests", "/api/stats/requests/internal-0"} {
				if response := doGet(t, r, path); response.Code != 500 {
					t.Fatalf("read failure hidden: %d", response.Code)
				}
			}
		})
	}
}

func TestGatewayRequestsOverviewWindowsAndEmpty(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*testing.T) (*store.DB, chi.Router)
	}{{"sqlite", setupStatsSQLiteTest}, {"postgres", setupStatsPostgresTest}} {
		t.Run(tc.name, func(t *testing.T) {
			db, r := tc.setup(t)
			response := doGet(t, r, "/api/stats/requests")
			var empty map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &empty); err != nil {
				t.Fatal(err)
			}
			if empty["coverageStartedAt"] != nil || len(empty["items"].([]any)) != 0 {
				t.Fatalf("invented coverage: %v", empty)
			}
			now := time.Now().UTC()
			for i, days := range []int{0, 3, 20, 120} {
				seedGatewayRequest(t, db, "window-"+strconv.Itoa(i), "rejected_no_route", now.AddDate(0, 0, -days).Format(time.RFC3339Nano))
			}
			seedGatewayRequest(t, db, "future", "completed", now.Add(time.Hour).Format(time.RFC3339Nano))
			for _, tc := range []struct {
				period string
				count  float64
			}{{"24h", 1}, {"7d", 2}, {"30d", 3}, {"all", 4}} {
				response := doGet(t, r, "/api/stats/overview?period="+tc.period)
				var body map[string]any
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if response.Code != 200 {
					t.Fatal(response.Body.String())
				}
				summary := body["gatewayRequestSummary"].(map[string]any)
				if summary["totalCount"] != tc.count || summary["rejectedCount"] != tc.count || summary["coverageStartedAt"] == nil {
					t.Fatalf("wrong real request window: %s", response.Body.String())
				}
				if body["requestSummary"].(map[string]any)["totalCount"] != float64(0) {
					t.Fatal("new terminals mixed with legacy last-attempt inference")
				}
				var total float64
				for _, raw := range body["gatewayRequestPoints"].([]any) {
					point := raw.(map[string]any)
					total += point["requests"].(float64)
					if point["rejectedCount"] != point["requests"] {
						t.Fatal("point outcome mismatch")
					}
				}
				if total != tc.count {
					t.Fatal("request summary/trend window mismatch")
				}
			}
		})
	}
}

func TestGatewayRequestAttemptReadFailureIsNotEmptySuccess(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedGatewayRequest(t, db, "request", "completed", "2026-10-16T00:00:00Z")
	if _, err := db.Exec("DROP TABLE proxy_logs"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/stats/requests/request"} {
		response := doGet(t, r, path)
		if response.Code != 500 {
			t.Fatalf("attempt query error hidden as empty: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestGatewayRequestPendingOrLostAttemptIsPartial(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*testing.T) (*store.DB, chi.Router)
	}{{"sqlite", setupStatsSQLiteTest}, {"postgres", setupStatsPostgresTest}} {
		t.Run(tc.name, func(t *testing.T) {
			db, r := tc.setup(t)
			seedGatewayRequest(t, db, "pending", "completed", "2026-10-16T00:00:00Z")
			if _, err := db.Exec("UPDATE gateway_requests SET attempt_count = 2 WHERE id = 'pending'"); err != nil {
				t.Fatal(err)
			}
			var detail struct {
				store.GatewayRequest
				Partial  bool
				Attempts []map[string]any
			}
			for persisted := 0; persisted <= 2; persisted++ {
				response := doGet(t, r, "/api/stats/requests/pending")
				if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
					t.Fatal(err)
				}
				if response.Code != 200 || detail.AttemptCount != 2 || len(detail.Attempts) != persisted || detail.Partial != (persisted != 2) {
					t.Fatalf("async/lost attempts presented as zero dispatch: %s", response.Body.String())
				}
				if persisted < 2 {
					if _, err := db.Exec("INSERT INTO proxy_logs (gateway_request_id,retry_count) VALUES ('pending',?)", persisted); err != nil {
						t.Fatal(err)
					}
				}
			}
		})
	}
}
