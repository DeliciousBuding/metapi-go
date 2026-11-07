package proxyhandler

import (
	"encoding/json"
	"fmt"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProviderMediaNativeFailureKeepsRawTaskUsage(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				output := 0
				if r.Method == "GET" {
					output = 2
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"id":"job-one","object":"video","status":"queued","model":"provider-model","usage":{"input_tokens":7,"output_tokens":%d,"total_tokens":%d}}`, output, 7+output)
			}))
			defer upstream.Close()
			db, server, _ := directVideoFixture(t, dialect, upstream.URL)
			const source = `export const meta={apiVersion:2,kind:"provider",key:"raw-task",name:"Raw task",version:"1.0.0",protocols:["video"],credentialKinds:["api_key"],hooks:["submit","query","content","taskResult"]};export function submit(i){return {method:"POST",body:i.body};}export function query(){return {method:"GET"};}export function content(){return {method:"GET"};}export function taskResult(i){if(i.taskId)throw new Error("fixture-private-decoder");return {body:i.body};}`
			if _, err := channelplugins.Save(t.Context(), db.DB, "raw-task", source, true, 0); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "raw-task", Protocols: []string{"video"}}})
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			status, _, body := videoHTTP(t, server, "POST", "/v1/videos", `{"model":"client-alias","prompt":"fixture"}`, "client-one")
			var created struct {
				ID string `json:"id"`
			}
			if status != 200 || json.Unmarshal(body, &created) != nil {
				t.Fatalf("submission failed: %d %s", status, body)
			}
			status, _, body = videoHTTP(t, server, "GET", "/v1/videos/"+created.ID, "", "client-one")
			if status != 502 || calls.Load() != 2 || strings.Contains(string(body), "fixture-private") {
				t.Fatalf("decoder failure replayed/leaked: %d calls=%d %s", status, calls.Load(), body)
			}
			var raw string
			var state struct {
				Usage store.VideoTaskUsage `json:"usage"`
			}
			if err := db.Get(&raw, db.Rebind(`SELECT accounting_state FROM proxy_video_tasks WHERE public_id=?`), created.ID); err != nil || json.Unmarshal([]byte(raw), &state) != nil || state.Usage.TotalTokens != 9 {
				t.Fatalf("raw task usage disappeared after decoder failure: %s %v", raw, err)
			}
		})
	}
}

func TestProviderMediaVendorIdentityAndFailedDecoderAccounting(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, mode := range []string{"normal", "throw", "wrong-raw-id", "wrong-decoded-id", "forged-usage", "original-error", "missing-id", "null-id"} {
			t.Run(dialect+"/"+mode, func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					output := 0
					if r.Method == "GET" {
						output = 2
					}
					native := fmt.Sprintf(`{"id":"job-one","object":"video","status":"queued","model":"provider-model","usage":{"input_tokens":7,"output_tokens":%d,"total_tokens":%d}}`, output, 7+output)
					vendor := map[string]any{"id": "job-one", "native": native, "model": "provider-model", "meter": map[string]int{"input": 7, "output": output, "total": 7 + output}}
					if r.Method == "GET" {
						switch mode {
						case "wrong-raw-id":
							vendor["id"] = "other-job"
						case "missing-id":
							delete(vendor, "id")
						case "null-id":
							vendor["id"] = nil
						case "original-error":
							vendor["error"] = "fixture-private-error"
						}
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"vendor": vendor})
				}))
				defer upstream.Close()
				db, server, _ := directVideoFixture(t, dialect, upstream.URL)
				source := strings.Replace(providerMediaSource, `modelPath:"/vendor/model"`, `modelPath:"/vendor/model",taskIdPath:"/vendor/id"`, 1)
				if mode == "throw" || mode == "missing-id" {
					source = strings.Replace(source, `return {body:JSON.parse(i.body).vendor.native};`, `if(i.taskId)throw new Error("fixture-private-decoder");return {body:JSON.parse(i.body).vendor.native};`, 1)
				}
				if mode == "wrong-decoded-id" {
					source = strings.Replace(source, `return {body:JSON.parse(i.body).vendor.native};`, `const n=JSON.parse(i.body).vendor.native;return {body:i.taskId?n.replace('"job-one"','"other-job"'):n};`, 1)
				}
				if mode == "forged-usage" {
					source = strings.Replace(source, `return {body:JSON.parse(i.body).vendor.native};`, `const n=JSON.parse(i.body).vendor.native;return {body:i.taskId?n.replace('"total_tokens":9','"total_tokens":90'):n};`, 1)
				}
				if _, err := channelplugins.Save(t.Context(), db.DB, "media", source, true, 0); err != nil {
					t.Fatal(err)
				}
				videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "media", Protocols: []string{"video"}}})
				getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
				status, _, body := videoHTTP(t, server, "POST", "/v1/videos", `{"model":"client-alias","prompt":"fixture"}`, "client-one")
				var created struct {
					ID string `json:"id"`
				}
				if status != 200 || json.Unmarshal(body, &created) != nil {
					t.Fatalf("create failed: %d %s", status, body)
				}
				status, _, body = videoHTTP(t, server, "GET", "/v1/videos/"+created.ID, "", "client-one")
				wantStatus := 502
				if mode == "normal" {
					wantStatus = 200
				}
				if status != wantStatus || calls.Load() != 2 || strings.Contains(string(body), "fixture-private") {
					t.Fatalf("task decoder replayed or changed failure: %d calls=%d %s", status, calls.Load(), body)
				}
				wantTokens := int64(9)
				if mode == "wrong-raw-id" || mode == "missing-id" || mode == "null-id" {
					wantTokens = 7
				}
				var raw string
				var state struct {
					Usage store.VideoTaskUsage `json:"usage"`
				}
				if err := db.Get(&raw, db.Rebind(`SELECT accounting_state FROM proxy_video_tasks WHERE public_id=?`), created.ID); err != nil || json.Unmarshal([]byte(raw), &state) != nil || state.Usage.TotalTokens != wantTokens {
					t.Fatalf("wrong raw identity settlement: want=%d %s %v", wantTokens, raw, err)
				}
			})
		}
	}
}

func TestProviderMediaFailedSubmissionRetainsObservedTask(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"id":"job-one","object":"video","status":"queued","usage":{"input_tokens":7,"output_tokens":0,"total_tokens":7}}`)
			}))
			defer upstream.Close()
			db, server, _ := directVideoFixture(t, dialect, upstream.URL)
			const source = `export const meta={apiVersion:2,kind:"provider",key:"raw-task",name:"Raw task",version:"1.0.0",protocols:["video"],credentialKinds:["api_key"],hooks:["submit","query","content","taskResult"]};export function submit(i){return {method:"POST",body:i.body};}export function query(){return {method:"GET"};}export function content(){return {method:"GET"};}export function taskResult(i){if(!i.taskId)throw new Error("fixture-private-create");return {body:i.body};}`
			if _, err := channelplugins.Save(t.Context(), db.DB, "raw-task", source, true, 0); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "raw-task", Protocols: []string{"video"}}})
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			status, _, body := videoHTTP(t, server, "POST", "/v1/videos", `{"model":"client-alias","prompt":"fixture"}`, "client-one")
			if status != 502 || calls.Load() != 1 || strings.Contains(string(body), "fixture-private") {
				t.Fatalf("create decoder failure replayed/succeeded: %d %s", status, body)
			}
			var id, raw string
			if err := db.QueryRow(`SELECT public_id,accounting_state FROM proxy_video_tasks WHERE upstream_video_id='job-one'`).Scan(&id, &raw); err != nil || !strings.HasPrefix(id, directVideoIDPrefix) {
				t.Fatalf("observed task lost after failed create: %v", err)
			}
			status, _, body = videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
			if status != 200 || calls.Load() != 2 {
				t.Fatalf("observed failed task could not be polled: %d %s", status, body)
			}
		})
	}
}
