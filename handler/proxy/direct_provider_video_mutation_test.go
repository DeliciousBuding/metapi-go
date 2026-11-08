package proxyhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

const providerVideoMutationModule = `export const meta={apiVersion:2,kind:"provider",key:"video-mutations",name:"Video mutations",version:"1.0.0",protocols:["video"],credentialKinds:["api_key"],taskCapture:"durable",hooks:["submit","query","content","taskResult","deleteTask","remix"],options:[{key:"mode",label:"Mode",type:"string",default:"old"}]};export function submit(i){return {method:"POST",body:i.body,headers:{"X-Driver":i.options.mode}};}export function query(i){return {method:"GET",headers:{"X-Driver":i.options.mode}};}export function content(){return {method:"GET"};}export function deleteTask(i){return {method:"DELETE",headers:{"X-Driver":i.options.mode}};}export function remix(i){return {method:"POST",body:i.body,headers:{"X-Driver":i.options.mode,"X-Source-Task":i.taskId}};}export function taskResult(i){if(!i.body)throw new Error("empty-delete-must-not-execute");return {body:i.body};}`

func TestProviderVideoDeletionRequiresOriginalIdentityBeforeDecoder(t *testing.T) {
	for _, protocol := range []string{"video", "seedanceVideo", "zenmuxVideo"} {
		for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
			for _, tc := range []struct {
				name, originalID string
				observation      bool
				decodedNumber    bool
				wantOK           bool
			}{
				{name: "native-match", originalID: `"id":"source",`, wantOK: true},
				{name: "rewritten-wrong-id", originalID: `"id":"other-task",`},
				{name: "invented-missing-id"},
				{name: "null-id", originalID: `"id":null,`},
				{name: "numeric-id", originalID: `"id":42,`},
				{name: "vendor-match", originalID: `"task":"source",`, observation: true, wantOK: true},
				{name: "vendor-wrong-id", originalID: `"task":"other-task",`, observation: true},
				{name: "vendor-missing-id", observation: true},
				{name: "vendor-conflicts-with-native", originalID: `"id":"other-task","task":"source",`, observation: true},
				{name: "vendor-cannot-replace-null-native", originalID: `"id":null,"task":"source",`, observation: true},
				{name: "decoder-numeric-id", originalID: `"id":"42",`, decodedNumber: true},
			} {
				t.Run(protocol+"/"+dialect+"/"+tc.name, func(t *testing.T) {
					sourceID := "source"
					if tc.decodedNumber {
						sourceID = "42"
					}
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						w.Header().Set("Content-Type", "application/json")
						if r.Method == http.MethodPost && r.URL.Path == "/tenant/videos" {
							fmt.Fprintf(w, `{"id":%q,"object":"video","status":"queued","usage":{"total_tokens":2}}`, sourceID)
							return
						}
						if r.Method != http.MethodDelete || r.URL.Path != "/tenant/videos/"+sourceID {
							t.Errorf("unexpected operation: %s %s", r.Method, r.URL.Path)
							w.WriteHeader(http.StatusInternalServerError)
							return
						}
						// Deliberately omit object:"video": a native deletion envelope
						// still owns its original ID before any executable projection.
						fmt.Fprintf(w, `{%s"deleted":true,"usage":{"total_tokens":4}}`, tc.originalID)
					}))
					defer upstream.Close()
					db, server, _ := directVideoFixture(t, dialect, upstream.URL)
					if protocol != "video" {
						endpoint := &store.DirectEndpoint{URL: upstream.URL + "/tenant/videos", Auth: store.DirectAuthBearer}
						endpoints, bit := store.DirectEndpoints{}, store.DirectProtocolSeedanceVideo
						if protocol == "seedanceVideo" {
							endpoint.Profile, endpoints.SeedanceVideo = "seedance-video", endpoint
						} else {
							endpoint.Profile, endpoints.ZenmuxVideo, bit = "zenmux-video", endpoint, store.DirectProtocolZenmuxVideo
						}
						videoExec(t, db, `UPDATE upstream_channels SET endpoint_config=?`, endpoints)
						videoExec(t, db, `UPDATE upstream_grants SET protocols=?`, bit)
					}
					module := strings.Replace(providerVideoMutationModule, `protocols:["video"]`, `protocols:["`+protocol+`"]`, 1)
					if tc.observation {
						module = strings.Replace(module, `taskCapture:"durable",`, `taskCapture:"durable",observations:{`+protocol+`:{response:{taskIdPath:"/task"}}},`, 1)
					}
					decodedID := "i.taskId"
					if tc.decodedNumber {
						decodedID = "Number(i.taskId)"
					}
					// A malicious/buggy decoder substitutes the expected task ID.
					// Observations must remain independent of that output.
					module = strings.Replace(module, `export function taskResult(i){`, `export function taskResult(i){if(i.operation==="deleteTask")return {body:JSON.stringify({id:`+decodedID+`,deleted:true,usage:JSON.parse(i.body).usage})};`, 1)
					if _, err := channelplugins.Save(t.Context(), db.DB, "video-mutations", module, true, 0); err != nil {
						t.Fatal(err)
					}
					videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "video-mutations", Protocols: []string{protocol}}})
					getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
					id := createdVideoID(t, server)
					status, _, body := videoHTTP(t, server, http.MethodDelete, "/v1/videos/"+id, "", "client-one")
					wantStatus, wantCount := http.StatusBadGateway, 1
					if tc.wantOK {
						wantStatus, wantCount = http.StatusOK, 0
					}
					if status != wantStatus || calls.Load() != 2 {
						t.Fatalf("decoder invented deletion identity or request replayed: status=%d want=%d calls=%d %s", status, wantStatus, calls.Load(), body)
					}
					var count int
					if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM proxy_video_tasks WHERE public_id=?`), id); err != nil || count != wantCount {
						t.Fatalf("incorrect local deletion: count=%d want=%d err=%v", count, wantCount, err)
					}
					if !tc.wantOK {
						var raw string
						var state struct {
							Usage store.VideoTaskUsage `json:"usage"`
						}
						wantTokens := int64(2)
						if tc.decodedNumber {
							// The original identity is correct even though its projection
							// fails; do not erase already-verified supplier usage.
							wantTokens = 4
						}
						if err := db.Get(&raw, db.Rebind(`SELECT accounting_state FROM proxy_video_tasks WHERE public_id=?`), id); err != nil || json.Unmarshal([]byte(raw), &state) != nil || state.Usage.TotalTokens != wantTokens {
							t.Fatalf("deletion identity corrupted original ledger: %s want=%d err=%v", raw, wantTokens, err)
						}
					}
				})
			}
		}
	}
}

func TestProviderVideoMutationsKeepCapturedProgramAndSeparateTaskLedger(t *testing.T) {
	for _, protocol := range []string{"video", "seedanceVideo", "zenmuxVideo"} {
		for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
			for _, emptyDelete := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/empty=%t", protocol, dialect, emptyDelete), func(t *testing.T) {
					const sourceID, remixID = "source/ ?#%", "remix/ ?#%"
					var calls atomic.Int32
					var db *store.DB
					var sourcePublicID, fingerprint string
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if r.Header.Get("X-Driver") != "old" || r.Header.Get("Authorization") != "Bearer fixture-upstream-key" {
							t.Error("operation changed its captured program or current credential")
						}
						id, tokens := sourceID, 2
						switch {
						case r.Method == "POST" && r.URL.EscapedPath() == "/tenant/videos":
						case r.Method == "POST" && r.URL.EscapedPath() == "/tenant/videos/"+url.PathEscape(sourceID)+"/remix":
							if r.Header.Get("X-Source-Task") != sourceID {
								t.Error("remix lost the source identity")
							}
							body, _ := io.ReadAll(r.Body)
							if !strings.Contains(string(body), "9007199254740993") || !strings.Contains(string(body), `"prompt":"change"`) {
								t.Errorf("remix changed opaque input: %s", body)
							}
							var pending string
							if err := db.Get(&pending, `SELECT direct_identity FROM proxy_video_tasks WHERE upstream_video_id=''`); err != nil {
								t.Errorf("remix has no pre-dispatch durable pin: %v", err)
							} else {
								var identity directVideoIdentity
								if json.Unmarshal([]byte(pending), &identity) != nil || identity.SourcePublicID != sourcePublicID || identity.DriverFingerprint != fingerprint {
									t.Errorf("remix capture lost source/program: %s", pending)
								}
							}
							id, tokens = remixID, 5
						case r.Method == "GET" && r.URL.EscapedPath() == "/tenant/videos/"+url.PathEscape(remixID):
							id, tokens = remixID, 8
						case r.Method == "DELETE" && r.URL.EscapedPath() == "/tenant/videos/"+url.PathEscape(remixID):
							if body, _ := io.ReadAll(r.Body); len(body) != 0 {
								t.Errorf("delete carried a body: %s", body)
							}
							if emptyDelete {
								w.WriteHeader(http.StatusNoContent)
								return
							}
							w.Header().Set("Content-Type", "application/json")
							fmt.Fprintf(w, `{"id":%q,"deleted":true}`, remixID)
							return
						default:
							t.Errorf("unexpected operation: %s %s", r.Method, r.URL.EscapedPath())
							w.WriteHeader(404)
							return
						}
						w.Header().Set("Content-Type", "application/json")
						fmt.Fprintf(w, `{"id":%q,"object":"video","status":"queued","remixed_from_video_id":%q,"usage":{"total_tokens":%d},"extension":9007199254740993}`, id, sourceID, tokens)
					}))
					defer upstream.Close()
					var server *httptest.Server
					var reopen func()
					db, server, reopen = directVideoFixture(t, dialect, upstream.URL)
					if protocol != "video" {
						endpoint := &store.DirectEndpoint{URL: upstream.URL + "/tenant/videos", Auth: store.DirectAuthBearer}
						endpoints, bit := store.DirectEndpoints{}, store.DirectProtocolSeedanceVideo
						if protocol == "seedanceVideo" {
							endpoint.Profile, endpoints.SeedanceVideo = "seedance-video", endpoint
						} else {
							endpoint.Profile, endpoints.ZenmuxVideo, bit = "zenmux-video", endpoint, store.DirectProtocolZenmuxVideo
						}
						videoExec(t, db, `UPDATE upstream_channels SET endpoint_config=?`, endpoints)
						videoExec(t, db, `UPDATE upstream_grants SET protocols=?`, bit)
					}
					module := strings.Replace(providerVideoMutationModule, `protocols:["video"]`, `protocols:["`+protocol+`"]`, 1)
					if _, err := channelplugins.Save(t.Context(), db.DB, "video-mutations", module, true, 0); err != nil {
						t.Fatal(err)
					}
					binding := store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "video-mutations", Protocols: []string{protocol}}}
					videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, binding)
					getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
					sourcePublicID = createdVideoID(t, server)
					var identity directVideoIdentity
					var rawIdentity string
					if err := db.Get(&rawIdentity, db.Rebind(`SELECT direct_identity FROM proxy_video_tasks WHERE public_id=?`), sourcePublicID); err != nil || json.Unmarshal([]byte(rawIdentity), &identity) != nil {
						t.Fatalf("source identity: %s %v", rawIdentity, err)
					}
					fingerprint = identity.DriverFingerprint
					if _, err := channelplugins.Save(t.Context(), db.DB, "video-mutations", strings.Replace(module, `default:"old"`, `default:"new"`, 1), true, 1); err != nil {
						t.Fatal(err)
					}
					reopen()
					getUpstreamConfig().ChannelPlugins = channelplugins.New(store.GetDB().DB)
					status, _, body := videoHTTP(t, server, "POST", "/v1/videos/"+sourcePublicID+"/remix", `{"prompt":"change","seed":9007199254740993}`, "client-one")
					var result struct {
						ID     string `json:"id"`
						Source string `json:"remixed_from_video_id"`
					}
					if status != 200 || json.Unmarshal(body, &result) != nil || result.ID == sourcePublicID || result.Source != sourcePublicID || !strings.Contains(string(body), "9007199254740993") {
						t.Fatalf("captured remix failed: %d %s", status, body)
					}
					status, _, body = videoHTTP(t, server, "GET", "/v1/videos/"+result.ID, "", "client-one")
					if status != 200 || !strings.Contains(string(body), sourcePublicID) {
						t.Fatalf("remix source not retained across query: %d %s", status, body)
					}
					for publicID, want := range map[string]int64{sourcePublicID: 2, result.ID: 8} {
						var raw string
						var state struct {
							Usage store.VideoTaskUsage `json:"usage"`
						}
						if err := db.Get(&raw, db.Rebind(`SELECT accounting_state FROM proxy_video_tasks WHERE public_id=?`), publicID); err != nil || json.Unmarshal([]byte(raw), &state) != nil || state.Usage.TotalTokens != want {
							t.Fatalf("source/remix ledgers mixed: want=%d %s %v", want, raw, err)
						}
					}
					for _, method := range []string{"POST", "DELETE"} {
						path := "/v1/videos/" + result.ID
						if method == "POST" {
							path += "/remix"
						}
						status, _, _ = videoHTTP(t, server, method, path, `{"prompt":"foreign"}`, "client-two")
						if status != 404 || calls.Load() != 3 {
							t.Fatalf("mutation crossed owner: status=%d calls=%d", status, calls.Load())
						}
					}
					status, _, body = videoHTTP(t, server, "DELETE", "/v1/videos/"+result.ID, "", "client-one")
					wantStatus := http.StatusOK
					if emptyDelete {
						wantStatus = http.StatusNoContent
					}
					if status != wantStatus || calls.Load() != 4 || emptyDelete && len(body) != 0 || !emptyDelete && !strings.Contains(string(body), result.ID) {
						t.Fatalf("delete changed status/body or replayed: %d calls=%d %s", status, calls.Load(), body)
					}
					var count int
					if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM proxy_video_tasks WHERE public_id=?`), result.ID); err != nil || count != 0 {
						t.Fatalf("deleted task retained: %d %v", count, err)
					}
				})
			}
		}
	}
}

func TestProviderVideoMutationFailuresDoNotReplayOrDeleteOwnedTask(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, hook := range []string{"deleteTask", "remix"} {
			for _, mode := range []string{"not-declared", "wrong-id", "decoder-throws", "upstream-error", "disabled", "wrong-method", "wrong-url", "empty-200", "negative-delete"} {
				if hook == "remix" && (mode == "empty-200" || mode == "negative-delete") {
					continue
				}
				t.Run(dialect+"/"+hook+"/"+mode, func(t *testing.T) {
					var calls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						w.Header().Set("Content-Type", "application/json")
						if r.Method == "POST" && r.URL.Path == "/tenant/videos" {
							fmt.Fprint(w, `{"id":"source","object":"video","status":"queued","usage":{"total_tokens":2}}`)
							return
						}
						if mode == "upstream-error" {
							w.WriteHeader(http.StatusInternalServerError)
							fmt.Fprint(w, `{"error":{"message":"synthetic supplier failure"}}`)
							return
						}
						if hook == "remix" {
							id := "remix"
							if mode == "wrong-id" {
								id = "source"
							}
							fmt.Fprintf(w, `{"id":%q,"object":"video","status":"queued","usage":{"total_tokens":5}}`, id)
						} else {
							if mode == "empty-200" {
								return
							}
							id, deleted := "source", true
							if mode == "wrong-id" {
								id = "other"
							}
							if mode == "negative-delete" {
								deleted = false
							}
							fmt.Fprintf(w, `{"id":%q,"deleted":%t}`, id, deleted)
						}
					}))
					defer upstream.Close()
					db, server, _ := directVideoFixture(t, dialect, upstream.URL)
					module := providerVideoMutationModule
					switch mode {
					case "not-declared":
						module = strings.Replace(module, `,"deleteTask","remix"`, ``, 1)
					case "decoder-throws":
						module = strings.Replace(module, `export function taskResult(i){`, `export function taskResult(i){if(i.operation==="`+hook+`")throw new Error("fixture-private-mutation");`, 1)
					case "wrong-method":
						module = strings.Replace(module, `export function `+hook+`(i){return {method:`, `export function `+hook+`(i){return {method:"GET",discarded:`, 1)
					case "wrong-url":
						module = strings.Replace(module, `export function `+hook+`(i){return {method:`, `export function `+hook+`(i){return {path:"/other",method:`, 1)
					}
					if _, err := channelplugins.Save(t.Context(), db.DB, "video-mutations", module, true, 0); err != nil {
						t.Fatal(err)
					}
					videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "video-mutations", Protocols: []string{"video"}}})
					getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
					id := createdVideoID(t, server)
					if mode == "disabled" {
						if _, err := channelplugins.Save(t.Context(), db.DB, "video-mutations", module, false, 1); err != nil {
							t.Fatal(err)
						}
					}
					method, path := http.MethodDelete, "/v1/videos/"+id
					if hook == "remix" {
						method, path = http.MethodPost, path+"/remix"
					}
					status, _, body := videoHTTP(t, server, method, path, `{"prompt":"change"}`, "client-one")
					wantStatus, wantCalls := http.StatusBadGateway, int32(2)
					if mode == "upstream-error" {
						wantStatus = http.StatusInternalServerError
					}
					if mode == "disabled" {
						wantStatus, wantCalls = http.StatusServiceUnavailable, 1
					}
					if mode == "not-declared" || mode == "wrong-method" || mode == "wrong-url" {
						wantCalls = 1
					}
					if status != wantStatus || calls.Load() != wantCalls || strings.Contains(string(body), "fixture-private") {
						t.Fatalf("mutation changed failure/replayed/leaked: status=%d calls=%d %s", status, calls.Load(), body)
					}
					var count int
					if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM proxy_video_tasks WHERE public_id=?`), id); err != nil || count != 1 {
						t.Fatalf("failed mutation deleted source: %d %v", count, err)
					}
					if err := db.Get(&count, `SELECT COUNT(*) FROM proxy_video_tasks WHERE upstream_video_id=''`); err != nil || count != 0 {
						t.Fatalf("terminal mutation left pending capture: %d %v", count, err)
					}
					var raw string
					var state struct {
						Usage store.VideoTaskUsage `json:"usage"`
					}
					if err := db.Get(&raw, db.Rebind(`SELECT accounting_state FROM proxy_video_tasks WHERE public_id=?`), id); err != nil || json.Unmarshal([]byte(raw), &state) != nil || state.Usage.TotalTokens != 2 {
						t.Fatalf("mutation rewrote source ledger: %s %v", raw, err)
					}
					if hook == "remix" && mode == "decoder-throws" {
						if err := db.Get(&raw, `SELECT accounting_state FROM proxy_video_tasks WHERE upstream_video_id='remix'`); err != nil || json.Unmarshal([]byte(raw), &state) != nil || state.Usage.TotalTokens != 5 {
							t.Fatalf("failed decoder lost observed new task/usage: %s %v", raw, err)
						}
					}
				})
			}
		}
	}
}

func TestProviderVideoRemixCancellationStopsWriteAndReleasesPendingPin(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			started, stopped := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path == "/tenant/videos" {
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"id":"source","object":"video","status":"queued","usage":{"total_tokens":2}}`)
					return
				}
				// Finish reading the submitted body so net/http can observe a
				// disconnect while this handler waits for supplier completion.
				_, _ = io.Copy(io.Discard, r.Body)
				close(started)
				select {
				case <-r.Context().Done():
					close(stopped)
				case <-time.After(5 * time.Second):
					t.Error("cancelled remix retained supplier connection")
				}
			}))
			defer upstream.Close()
			db, server, _ := directVideoFixture(t, dialect, upstream.URL)
			if _, err := channelplugins.Save(t.Context(), db.DB, "video-mutations", providerVideoMutationModule, true, 0); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "video-mutations", Protocols: []string{"video"}}})
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			id := createdVideoID(t, server)
			requestCtx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, server.URL+"/v1/videos/"+id+"/remix", strings.NewReader(`{"prompt":"change"}`))
				if err != nil {
					done <- err
					return
				}
				req.Header.Set("Authorization", "Bearer client-one")
				req.Header.Set("Content-Type", "application/json")
				resp, err := server.Client().Do(req)
				if resp != nil {
					resp.Body.Close()
				}
				done <- err
			}()
			select {
			case <-started:
			case err := <-done:
				t.Fatalf("remix never started: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("remix did not reach supplier")
			}
			var pins int
			if err := db.Get(&pins, `SELECT COUNT(*) FROM proxy_video_tasks WHERE upstream_video_id=''`); err != nil || pins != 1 {
				t.Errorf("in-flight remix has no durable capture: %d %v", pins, err)
			}
			cancel()
			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("supplier did not observe cancellation")
			}
			select {
			case err := <-done:
				if err == nil {
					t.Error("cancelled request succeeded")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("client request did not stop")
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				if err := db.Get(&pins, `SELECT COUNT(*) FROM proxy_video_tasks WHERE upstream_video_id=''`); err != nil {
					t.Fatal(err)
				}
				if pins == 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("terminal cancellation retained pending capture")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if calls.Load() != 2 {
				t.Fatalf("cancelled write replayed: %d", calls.Load())
			}
			var count int
			if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM proxy_video_tasks WHERE public_id=?`), id); err != nil || count != 1 {
				t.Fatalf("cancellation removed source: %d %v", count, err)
			}
		})
	}
}
