package proxyhandler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectVideoCumulativeAccountingHTTP(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var tokens atomic.Int64
			var calls atomic.Int32
			cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "video/mp4")
				_, _ = io.WriteString(w, "video")
			}))
			defer cdn.Close()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "POST" {
					_, _ = io.WriteString(w, `{"id":"task","status":"queued"}`)
					return
				}
				if r.Method == "DELETE" {
					_, _ = io.WriteString(w, `{}`)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "task", "status": "succeeded", "content": map[string]string{"video_url": cdn.URL}, "usage": map[string]int64{"completion_tokens": tokens.Load(), "total_tokens": tokens.Load()}})
			}))
			defer upstream.Close()
			db, server, reopen := directVideoFixture(t, dialect, upstream.URL)
			bit, endpoints := nativeVideoEndpoint("seedance-video", upstream.URL+"/tasks")
			videoExec(t, db, `UPDATE upstream_channels SET endpoint_config=?`, endpoints)
			videoExec(t, db, `UPDATE upstream_grants SET protocols=?`, bit)
			videoExec(t, db, `UPDATE downstream_api_keys SET access_policy='{"quota":{"totalTokens":75,"period":{"type":"all_time"}}}' WHERE key='client-one'`)
			var mu sync.Mutex
			var logs []proxy.ProxyLogEntry
			getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error {
				mu.Lock()
				defer mu.Unlock()
				logs = append(logs, entry)
				return nil
			}
			id := createdVideoID(t, server)
			var initialTokens sql.NullInt64
			var initialCost sql.NullFloat64
			var occurred int64
			if err := db.QueryRow(`SELECT total_tokens,cost,occurred_at FROM downstream_quota_usage WHERE event_key=?`, "video:"+id).Scan(&initialTokens, &initialCost, &occurred); err != nil || initialTokens.Valid || initialCost.Valid {
				t.Fatalf("unknown creation usage was invented: %v %v %v", initialTokens, initialCost, err)
			}
			status, _, body := videoHTTP(t, server, "POST", "/v1/videos", `{"model":"client-alias","prompt":"another"}`, "client-one")
			if status != 403 || !strings.Contains(string(body), "quota_usage_unknown") {
				t.Fatalf("pending task did not retain unknown quota: %d %s", status, body)
			}
			tokens.Store(25)
			status, _, body = videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
			if status != 200 {
				t.Fatalf("pending quota locked task polling: %d %s", status, body)
			}
			tokens.Store(75)
			var wg sync.WaitGroup
			errors := make(chan string, 20)
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					path := "/v1/videos/" + id
					if i%2 == 0 {
						path += "/content"
					}
					req, _ := http.NewRequest("GET", server.URL+path, nil)
					req.Header.Set("Authorization", "Bearer client-one")
					resp, err := server.Client().Do(req)
					if err != nil {
						errors <- err.Error()
						return
					}
					data, _ := io.ReadAll(resp.Body)
					_ = resp.Body.Close()
					if resp.StatusCode != 200 {
						errors <- fmt.Sprintf("%d %s", resp.StatusCode, data)
					}
				}(i)
			}
			wg.Wait()
			close(errors)
			for err := range errors {
				t.Error(err)
			}
			if t.Failed() {
				return
			}
			wantCost := EstimateBillingCostFromUsage("client-alias", "openai", ParsedUsage{Found: true, Source: usageSourceUpstream, CompletionTokens: 75, TotalTokens: 75}).EstimatedCost
			checkAccounting := func() {
				var count, total, requests, when, usedRequests int64
				var quotaCost, usedCost, grantCost float64
				if err := db.QueryRow(`SELECT COUNT(*),COALESCE(SUM(total_tokens),0),COALESCE(SUM(requests),0),COALESCE(SUM(cost),0),MIN(occurred_at) FROM downstream_quota_usage WHERE key_id=(SELECT id FROM downstream_api_keys WHERE key='client-one')`).Scan(&count, &total, &requests, &quotaCost, &when); err != nil {
					t.Fatal(err)
				}
				_ = db.QueryRow(`SELECT used_cost,used_requests FROM downstream_api_keys WHERE key='client-one'`).Scan(&usedCost, &usedRequests)
				_ = db.QueryRow(`SELECT SUM(total_cost) FROM upstream_grants`).Scan(&grantCost)
				if count != 1 || total != 75 || requests != 1 || when != occurred || usedRequests != 1 || math.Abs(quotaCost-wantCost) > 1e-12 || math.Abs(usedCost-wantCost) > 1e-12 || math.Abs(grantCost-wantCost) > 1e-12 {
					t.Fatalf("duplicate or lost task accounting: count=%d tokens=%d requests=%d time=%d/%d usedRequests=%d costs=%g/%g/%g want=%g", count, total, requests, when, occurred, usedRequests, quotaCost, usedCost, grantCost, wantCost)
				}
			}
			checkAccounting()
			tokens.Store(20) // An older replica's smaller cumulative snapshot is not a refund or new charge.
			status, _, _ = videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
			if status != 200 {
				t.Fatalf("out-of-order task read: %d", status)
			}
			checkAccounting()
			tokens.Store(75)
			mu.Lock()
			var loggedTokens int64
			var loggedCost float64
			observed75 := 0
			for _, entry := range logs {
				if entry.TotalTokens != nil {
					loggedTokens += *entry.TotalTokens
				}
				loggedCost += entry.EstimatedCost
				details, ok := entry.BillingDetails.(map[string]any)
				if ok {
					if observed, ok := details["observedUsage"].(map[string]any); ok && observed["totalTokens"] == int64(75) {
						observed75++
					}
				}
			}
			mu.Unlock()
			if loggedTokens != 75 || math.Abs(loggedCost-wantCost) > 1e-12 || observed75 < 20 {
				t.Fatalf("logs lost observed usage or aggregated cumulative usage repeatedly: %d %g observed=%d", loggedTokens, loggedCost, observed75)
			}
			videoExec(t, db, `UPDATE downstream_api_keys SET max_requests=1,max_cost=used_cost WHERE key='client-one'`)
			reopen()
			status, _, _ = videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
			if status != 200 {
				t.Fatalf("exhausted quota locked owned task: %d", status)
			}
			before := calls.Load()
			status, _, _ = videoHTTP(t, server, "POST", "/v1/videos/"+id+"/remix", `{"prompt":"p"}`, "client-one")
			if status != 429 || calls.Load() != before {
				t.Fatal("remix bypassed generation budget")
			}
			status, _, _ = videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-two")
			if status != 404 || calls.Load() != before {
				t.Fatal("other owner accessed task")
			}
			videoExec(t, db, `UPDATE downstream_api_keys SET max_requests=0,max_cost=0 WHERE key='client-two'`)
			status, _, _ = videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-two")
			if status != 429 || calls.Load() != before {
				t.Fatal("another owner's task ID bypassed the caller's budget")
			}
			for _, mutation := range []struct{ query, restore string }{
				{`UPDATE downstream_api_keys SET enabled=FALSE WHERE key='client-one'`, `UPDATE downstream_api_keys SET enabled=TRUE WHERE key='client-one'`},
				{`UPDATE downstream_api_keys SET expires_at='2000-01-01T00:00:00Z' WHERE key='client-one'`, `UPDATE downstream_api_keys SET expires_at=NULL WHERE key='client-one'`},
				{`UPDATE downstream_api_keys SET access_policy='{"blockReason":"source_scope_missing"}' WHERE key='client-one'`, `UPDATE downstream_api_keys SET access_policy=NULL WHERE key='client-one'`},
				{`UPDATE downstream_api_keys SET ip_allowlist='192.0.2.1' WHERE key='client-one'`, `UPDATE downstream_api_keys SET ip_allowlist=NULL WHERE key='client-one'`},
			} {
				videoExec(t, db, mutation.query)
				status, _, _ = videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
				if status != 403 || calls.Load() != before {
					t.Fatalf("task-read exemption bypassed auth: %d (%s)", status, mutation.query)
				}
				videoExec(t, db, mutation.restore)
			}
			status, _, _ = videoHTTP(t, server, "DELETE", "/v1/videos/"+id, "", "client-one")
			if status != 200 {
				t.Fatalf("delete locked by generation budget: %d", status)
			}
			checkAccounting()
			var rows int
			_ = db.QueryRow(`SELECT COUNT(*) FROM proxy_video_tasks WHERE public_id=?`, id).Scan(&rows)
			if rows != 0 {
				t.Fatal("successful delete kept task mapping")
			}
		})
	}
}

func TestDirectVideoAccountingTransactionRollback(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"task","status":"queued"}`)
			}))
			defer upstream.Close()
			db, server, _ := directVideoFixture(t, dialect, upstream.URL)
			id := createdVideoID(t, server)
			task, err := loadDirectVideoTask(db, id)
			if err != nil {
				t.Fatal(err)
			}
			var keyID int64
			_ = db.QueryRow(`SELECT id FROM downstream_api_keys WHERE key='client-one'`).Scan(&keyID)
			var before string
			_ = db.QueryRow(`SELECT accounting_state FROM proxy_video_tasks WHERE public_id=?`, id).Scan(&before)
			videoExec(t, db, `ALTER TABLE upstream_grants RENAME TO unavailable_video_grants`)
			_, err = store.AccountVideoTask(context.Background(), db, id, task.Identity.Owner, task.Identity.GrantID, keyID, store.VideoTaskUsage{Found: true, CompletionTokens: 100, TotalTokens: 100}, 1)
			if err == nil {
				t.Fatal("known missing final accounting sink did not abort transaction")
			}
			videoExec(t, db, `ALTER TABLE unavailable_video_grants RENAME TO upstream_grants`)
			var after string
			var cost float64
			var tokens sql.NullInt64
			_ = db.QueryRow(`SELECT accounting_state FROM proxy_video_tasks WHERE public_id=?`, id).Scan(&after)
			_ = db.QueryRow(`SELECT used_cost FROM downstream_api_keys WHERE id=?`, keyID).Scan(&cost)
			_ = db.QueryRow(`SELECT total_tokens FROM downstream_quota_usage WHERE event_key=?`, "video:"+id).Scan(&tokens)
			if before != after || cost != 0 || tokens.Valid {
				t.Fatalf("partial transaction escaped rollback: %s/%s cost=%g tokens=%v", before, after, cost, tokens)
			}
		})
	}
}

func TestDirectVideoAccountingContentFailure(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel-%v", cancelRequest), func(t *testing.T) {
			started := make(chan struct{})
			cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				if cancelRequest {
					<-r.Context().Done()
					return
				}
				w.WriteHeader(503)
			}))
			defer cdn.Close()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == "POST" {
					_, _ = io.WriteString(w, `{"id":"task","status":"queued"}`)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "task", "status": "succeeded", "content": map[string]string{"video_url": cdn.URL}, "usage": map[string]int{"completion_tokens": 80, "total_tokens": 80}})
			}))
			defer upstream.Close()
			db, server, _ := directVideoFixture(t, store.DialectSQLite, upstream.URL)
			bit, endpoints := nativeVideoEndpoint("seedance-video", upstream.URL+"/tasks")
			videoExec(t, db, `UPDATE upstream_channels SET endpoint_config=?`, endpoints)
			videoExec(t, db, `UPDATE upstream_grants SET protocols=?`, bit)
			id := createdVideoID(t, server)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelRequest {
				go func() { <-started; cancel() }()
			}
			req, _ := http.NewRequestWithContext(ctx, "GET", server.URL+"/v1/videos/"+id+"/content", nil)
			req.Header.Set("Authorization", "Bearer client-one")
			resp, err := server.Client().Do(req)
			if resp != nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode != 502 {
					t.Errorf("download failure status=%d", resp.StatusCode)
				}
			}
			if cancelRequest && err == nil {
				t.Fatal("expected client cancellation")
			}
			var total int64
			var cost float64
			deadline := time.Now().Add(time.Second)
			for {
				_ = db.QueryRow(`SELECT COALESCE(total_tokens,0),COALESCE(cost,0) FROM downstream_quota_usage WHERE event_key=?`, "video:"+id).Scan(&total, &cost)
				if total == 80 || time.Now().After(deadline) {
					break
				}
				time.Sleep(time.Millisecond)
			}
			if total != 80 || cost <= 0 {
				t.Fatalf("observed task consumption disappeared on content failure: %d %g", total, cost)
			}
		})
	}
}

func TestDirectVideoAccountingAcrossConnections(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"id":"task","status":"queued"}`)
			}))
			defer upstream.Close()
			db, server, reopen := directVideoFixture(t, dialect, upstream.URL)
			id := createdVideoID(t, server)
			task, err := loadDirectVideoTask(db, id)
			if err != nil {
				t.Fatal(err)
			}
			reopen()
			peer := store.GetDB()
			var keyID int64
			_ = db.QueryRow(`SELECT id FROM downstream_api_keys WHERE key='client-one'`).Scan(&keyID)
			var wg sync.WaitGroup
			results := make(chan store.VideoTaskAccountingResult, 20)
			failures := make(chan error, 20)
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					conn := db
					if i%2 == 0 {
						conn = peer
					}
					result, err := store.AccountVideoTask(context.Background(), conn, id, task.Identity.Owner, task.Identity.GrantID, keyID, store.VideoTaskUsage{Found: true, CompletionTokens: 100, TotalTokens: 100}, 1)
					if err != nil {
						failures <- err
						return
					}
					results <- result
				}(i)
			}
			wg.Wait()
			close(results)
			close(failures)
			for err := range failures {
				t.Error(err)
			}
			var tokens int64
			var cost float64
			for result := range results {
				tokens += result.Delta.TotalTokens
				cost += result.CostDelta
			}
			var used, grantCost float64
			_ = db.QueryRow(`SELECT used_cost FROM downstream_api_keys WHERE id=?`, keyID).Scan(&used)
			_ = db.QueryRow(`SELECT total_cost FROM upstream_grants WHERE id=?`, task.Identity.GrantID).Scan(&grantCost)
			if tokens != 100 || cost != 1 || used != 1 || grantCost != 1 {
				t.Fatalf("cross-worker task was charged multiple times: %d %g %g %g", tokens, cost, used, grantCost)
			}
		})
	}
}

func TestDirectVideoLegacyAccountingDoesNotGuess(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "POST" {
			_, _ = io.WriteString(w, `{"id":"task","status":"queued"}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"task","status":"completed","usage":{"completion_tokens":100,"total_tokens":100}}`)
	}))
	defer upstream.Close()
	db, server, _ := directVideoFixture(t, store.DialectSQLite, upstream.URL)
	id := createdVideoID(t, server)
	videoExec(t, db, `UPDATE proxy_video_tasks SET accounting_state=NULL WHERE public_id=?`, id)
	videoExec(t, db, `DELETE FROM downstream_quota_usage`)
	videoExec(t, db, `UPDATE downstream_api_keys SET used_cost=2 WHERE key='client-one'`)
	videoExec(t, db, `UPDATE upstream_grants SET total_cost=2`)
	var observedMode string
	getUpstreamConfig().LogProxy = func(_ context.Context, entry proxy.ProxyLogEntry) error {
		details, _ := entry.BillingDetails.(map[string]any)
		observedMode, _ = details["accountingMode"].(string)
		if entry.TotalTokens != nil || entry.EstimatedCost != 0 {
			t.Error("legacy baseline was guessed")
		}
		return nil
	}
	status, _, body := videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
	if status != 200 || !strings.Contains(string(body), `"total_tokens":100`) || observedMode != "legacy_task_untracked" {
		t.Fatalf("legacy read: %d %s mode=%s", status, body, observedMode)
	}
	var rows int
	var cost float64
	_ = db.QueryRow(`SELECT COUNT(*) FROM downstream_quota_usage`).Scan(&rows)
	_ = db.QueryRow(`SELECT used_cost FROM downstream_api_keys WHERE key='client-one'`).Scan(&cost)
	if rows != 0 || cost != 2 {
		t.Fatalf("legacy task rebilled: rows=%d cost=%g", rows, cost)
	}
}

func TestDirectVideoAccountingDeletedSinkRollsBack(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, missing := range []string{"grant", "key"} {
			t.Run(dialect+"/"+missing, func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"id":"task","status":"queued"}`)
				}))
				defer upstream.Close()
				db, server, _ := directVideoFixture(t, dialect, upstream.URL)
				id := createdVideoID(t, server)
				task, err := loadDirectVideoTask(db, id)
				if err != nil {
					t.Fatal(err)
				}
				var keyID int64
				_ = db.QueryRow(`SELECT id FROM downstream_api_keys WHERE key='client-one'`).Scan(&keyID)
				var before string
				_ = db.QueryRow(`SELECT accounting_state FROM proxy_video_tasks WHERE public_id=?`, id).Scan(&before)
				if missing == "grant" {
					videoExec(t, db, `DELETE FROM upstream_grants WHERE id=?`, task.Identity.GrantID)
				} else {
					videoExec(t, db, `DELETE FROM downstream_api_keys WHERE id=?`, keyID)
				}
				_, err = store.AccountVideoTask(context.Background(), db, id, task.Identity.Owner, task.Identity.GrantID, keyID, store.VideoTaskUsage{Found: true, CompletionTokens: 100, TotalTokens: 100}, 1)
				if err == nil {
					t.Fatal("deleted billing sink silently committed a partial ledger")
				}
				var after string
				_ = db.QueryRow(`SELECT accounting_state FROM proxy_video_tasks WHERE public_id=?`, id).Scan(&after)
				if after != before {
					t.Fatal("task high-water mark advanced without its billing sink")
				}
				var cost float64
				if missing == "grant" {
					var total sql.NullInt64
					_ = db.QueryRow(`SELECT total_tokens FROM downstream_quota_usage WHERE event_key=?`, "video:"+id).Scan(&total)
					_ = db.QueryRow(`SELECT used_cost FROM downstream_api_keys WHERE id=?`, keyID).Scan(&cost)
					if total.Valid || cost != 0 {
						t.Fatal("missing grant left key charges")
					}
				} else {
					var rows int
					_ = db.QueryRow(`SELECT COUNT(*) FROM downstream_quota_usage WHERE event_key=?`, "video:"+id).Scan(&rows)
					_ = db.QueryRow(`SELECT total_cost FROM upstream_grants WHERE id=?`, task.Identity.GrantID).Scan(&cost)
					if rows != 0 || cost != 0 {
						t.Fatal("missing key left grant charges")
					}
				}
			})
		}
	}
}
