package proxyhandler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func nativeVideoEndpoint(profile, target string) (int, store.DirectEndpoints) {
	endpoint := &store.DirectEndpoint{URL: target, Auth: store.DirectAuthBearer, Profile: profile}
	if profile == "seedance-video" {
		return store.DirectProtocolSeedanceVideo, store.DirectEndpoints{SeedanceVideo: endpoint}
	}
	return store.DirectProtocolZenmuxVideo, store.DirectEndpoints{ZenmuxVideo: endpoint}
}

func TestDirectNativeVideoPinDoesNotMutateCandidate(t *testing.T) {
	bit, endpoints := nativeVideoEndpoint("seedance-video", "https://example.test/tasks")
	endpoints.Video = &store.DirectEndpoint{URL: "https://example.test/videos", Auth: store.DirectAuthBearer}
	selected := &routing.SelectedChannel{Direct: &store.DirectUpstreamCandidate{Endpoints: endpoints,
		Protocols: bit | store.DirectProtocolVideo, ProtocolOrder: store.DirectProtocolOrder{store.DirectProtocolVideo, bit}}}
	ctx := &Ctx{videoTask: &directVideoTask{Identity: directVideoIdentity{Version: 2, Protocol: bit}}}
	pinned, err := pinDirectVideoTaskCandidate(ctx, selected)
	if err != nil || pinned == selected || pinned.Direct == selected.Direct || len(pinned.Direct.ProtocolOrder) != 1 || pinned.Direct.ProtocolOrder[0] != bit || selected.Direct.ProtocolOrder[0] != store.DirectProtocolVideo {
		t.Fatalf("format was not pinned on a local copy: %+v %v", pinned, err)
	}
	selected.Direct.Protocols = store.DirectProtocolVideo
	if _, err := pinDirectVideoTaskCandidate(ctx, selected); err == nil {
		t.Fatal("pinning expanded a revoked grant")
	}
	selected.Direct.Protocols |= bit
	selected.Direct.ProtocolOrder = store.DirectProtocolOrder{store.DirectProtocolVideo}
	if _, err := pinDirectVideoTaskCandidate(ctx, selected); err == nil {
		t.Fatal("pinning expanded revoked member protocols")
	}
}

func TestDirectNativeVideoStatusMapping(t *testing.T) {
	for from, to := range map[string]string{"queued": "queued", "running": "in_progress", "succeeded": "completed", "failed": "failed"} {
		for _, profile := range []string{"seedance-video", "zenmux-video"} {
			payload := map[string]json.RawMessage{"id": json.RawMessage(`"task"`), "usage": json.RawMessage(`{"completion_tokens":9007199254740993}`), "updated_at": json.RawMessage(`123`)}
			payload["status"], _ = json.Marshal(from)
			if err := normalizeNativeVideoResponse(payload, profile); err != nil || mediaString(payload, "status") != to || string(payload["usage"]) != `{"completion_tokens":9007199254740993}` {
				t.Fatalf("status %s: %v %+v", from, err, payload)
			}
			if (from == "failed" || from == "succeeded") && string(payload["completed_at"]) != "123" {
				t.Error("lost task completion timestamp")
			}
		}
	}
}

func TestDirectNativeVideoPrepare(t *testing.T) {
	for _, profile := range []string{"seedance-video", "zenmux-video"} {
		t.Run(profile, func(t *testing.T) {
			wire, contentType, err := prepareDirectNativeVideoProfile(profile, "POST", "/v1/videos", "application/json", []byte(`{"model":"actual","prompt":"a quiet forest","seconds":"4","size":"1280x720","input_reference":"https://example.test/reference.png","generate_audio":false,"seed":0}`))
			if err != nil || contentType != "application/json" {
				t.Fatalf("prepare: %v %s", err, contentType)
			}
			var payload map[string]json.RawMessage
			_ = json.Unmarshal(wire.Body, &payload)
			if mediaString(payload, "model") != "actual" || mediaString(payload, "ratio") != "16:9" || mediaString(payload, "resolution") != "720p" || string(payload["duration"]) != "4" || string(payload["generate_audio"]) != "false" || string(payload["seed"]) != "0" {
				t.Fatalf("wrong native request: %s", wire.Body)
			}
			for _, key := range []string{"prompt", "seconds", "size", "input_reference"} {
				if _, exists := payload[key]; exists {
					t.Errorf("unconverted field %s", key)
				}
			}
			if !bytes.Contains(payload["content"], []byte(`"first_frame"`)) || !bytes.Contains(payload["content"], []byte(`"a quiet forest"`)) {
				t.Fatalf("missing native content: %s", payload["content"])
			}
			for _, body := range []string{
				`{"model":"m"}`, `{"model":"m","prompt":"p","seconds":"NaN"}`, `{"model":"m","prompt":"p","seconds":"0"}`,
				`{"model":"m","prompt":"p","size":"111x222"}`, `{"model":"m","prompt":"p","stream":true}`,
				`{"model":"m","content":[{"type":"text","text":"p","role":"first_frame"}]}`,
				`{"model":"m","content":[{"type":"video_url","video_url":{"url":"x"}}]}`,
			} {
				if _, _, err := prepareDirectNativeVideoProfile(profile, "POST", "/v1/videos", "application/json", []byte(body)); err == nil {
					t.Errorf("accepted invalid request %s", body)
				}
			}
			if _, _, err := prepareDirectNativeVideoProfile(profile, "POST", "/v1/videos/task/remix", "application/json", []byte(`{"prompt":"p"}`)); err == nil {
				t.Fatal("native remix must be rejected")
			}
		})
	}
	for _, body := range []string{
		`{"model":"m","prompt":"p","service_tier":"flex"}`,
		`{"model":"m","prompt":"p","extra_body":{"execution_expires_after":12}}`,
		`{"model":"m","prompt":"p","seconds":"3.4"}`,
	} {
		if _, _, err := prepareDirectNativeVideoProfile("zenmux-video", "POST", "/v1/videos", "application/json", []byte(body)); err == nil {
			t.Errorf("accepted unsupported ZenMux fields: %s", body)
		}
	}
	if _, _, err := prepareDirectNativeVideoProfile("zenmux-video", "DELETE", "/v1/videos/task", "", nil); err == nil {
		t.Fatal("ZenMux deletion must be rejected")
	}
	wire, _, err := prepareDirectNativeVideoProfile("seedance-video", "POST", "/v1/videos", "application/json", []byte(`{"model":"m","prompt":"p","seconds":"3.4","service_tier":"flex","execution_expires_after":50}`))
	if err != nil || !bytes.Contains(wire.Body, []byte(`"duration":3`)) {
		t.Fatalf("Seedance duration: %v %+v", err, wire)
	}
	wire, _, err = prepareDirectNativeVideoProfile("seedance-video", "POST", "/v1/videos", "application/json", []byte(`{"model":"m","prompt":"p","seconds":"-1"}`))
	if err != nil || !bytes.Contains(wire.Body, []byte(`"duration":-1`)) {
		t.Fatalf("Seedance automatic duration: %v %+v", err, wire)
	}
	wire, _, err = prepareDirectNativeVideoProfile("zenmux-video", "POST", "/v1/videos", "application/json", []byte(`{"model":"mapped","prompt":"p","callback_url":"https://example.test/callback","return_last_frame":true,"tools":[{"type":"web_search"}],"extra_body":{"model":"wrong","custom_flag":true}}`))
	if err != nil || !bytes.Contains(wire.Body, []byte(`"model":"mapped"`)) || !bytes.Contains(wire.Body, []byte(`"custom_flag":true`)) || !bytes.Contains(wire.Body, []byte(`"return_last_frame":true`)) {
		t.Fatalf("ZenMux extension merge: %v %+v", err, wire)
	}
}

func TestDirectNativeVideoMultipart(t *testing.T) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	for key, value := range map[string]string{"model": "m", "prompt": "p", "seconds": "4", "generate_audio": "false"} {
		_ = w.WriteField(key, value)
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="input_reference"; filename="reference.png"`)
	header.Set("Content-Type", "image/png")
	file, _ := w.CreatePart(header)
	_, _ = file.Write([]byte{137, 80, 78, 71, 13, 10, 26, 10})
	_ = w.Close()
	wire, contentType, err := prepareDirectNativeVideoProfile("seedance-video", "POST", "/v1/videos", w.FormDataContentType(), body.Bytes())
	if err != nil || contentType != "application/json" || !bytes.Contains(wire.Body, []byte("data:image/png;base64,iVBORw0KGgo=")) || !bytes.Contains(wire.Body, []byte(`"generate_audio":false`)) {
		t.Fatalf("multipart conversion: %v %+v", err, wire)
	}
}

func TestDirectNativeVideoLifecycleHTTP(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, profile := range []string{"seedance-video", "zenmux-video"} {
			t.Run(dialect+"/"+profile, func(t *testing.T) {
				var calls, posts, downloads, deletes atomic.Int32
				var rotated, failDelete atomic.Bool
				const upstreamID = "task/id ?#%"
				cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					downloads.Add(1)
					if r.Header.Get("Authorization") != "" || r.Header.Get("X-Tenant-Private") != "" || r.Header.Get("Cookie") != "" {
						t.Error("CDN request leaked private headers")
					}
					if r.Header.Get("Range") != "bytes=0-3" {
						t.Error("download lost Range")
					}
					w.Header().Set("Content-Type", "video/webm")
					w.Header().Set("Content-Range", "bytes 0-3/10")
					w.WriteHeader(206)
					_, _ = w.Write([]byte{0, 1, 255, 0})
				}))
				defer cdn.Close()
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					key := "fixture-upstream-key"
					if rotated.Load() {
						key = "rotated-upstream-key"
					}
					if r.Header.Get("Authorization") != "Bearer "+key {
						t.Error("upstream credential did not rotate")
					}
					if r.Header.Get("X-Tenant-Private") != "fixture-private" {
						t.Error("upstream private header was not applied")
					}
					w.Header().Set("Content-Type", "application/json")
					if r.Method == "POST" && r.URL.Path == "/native/tasks" {
						posts.Add(1)
						var payload map[string]json.RawMessage
						_ = json.NewDecoder(r.Body).Decode(&payload)
						if mediaString(payload, "model") != "provider-model" || string(payload["duration"]) != "4" || len(payload["content"]) == 0 || len(payload["prompt"]) != 0 {
							t.Errorf("bad create body: %v", payload)
						}
						_ = json.NewEncoder(w).Encode(map[string]any{"id": upstreamID, "status": "queued"})
						return
					}
					if r.URL.EscapedPath() != "/native/tasks/"+url.PathEscape(upstreamID) {
						t.Errorf("task switched endpoint or ID: %s %s", r.Method, r.URL)
						w.WriteHeader(400)
						return
					}
					if r.Method == "DELETE" {
						deletes.Add(1)
						if failDelete.Load() {
							_, _ = io.WriteString(w, `{"error":{"code":"busy","message":"try later"}}`)
						} else {
							_, _ = io.WriteString(w, `{}`)
						}
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"id": upstreamID, "status": "succeeded", "duration": 4,
						"content": map[string]string{"video_url": cdn.URL + "/movie", "last_frame_url": cdn.URL + "/frame"},
						"error":   nil, "usage": map[string]int{"completion_tokens": 42, "total_tokens": 42}})
				}))
				defer upstream.Close()
				db, server, reopen := directVideoFixture(t, dialect, upstream.URL)
				bit, endpoints := nativeVideoEndpoint(profile, upstream.URL+"/native/tasks")
				endpoints.Video = &store.DirectEndpoint{URL: upstream.URL + "/wrong/videos", Auth: store.DirectAuthBearer}
				videoExec(t, db, `UPDATE upstream_channels SET endpoint_config = ?, custom_header = ?`, endpoints, `[{"header_key":"X-Tenant-Private","header_value":"fixture-private"}]`)
				videoExec(t, db, `UPDATE upstream_grants SET protocols = ?`, bit|store.DirectProtocolVideo)
				videoExec(t, db, `UPDATE upstream_group_items SET protocol_order = ?`, store.DirectProtocolOrder{bit, store.DirectProtocolVideo})
				id := createdVideoID(t, server)
				var identity, token, site string
				if err := db.QueryRow(`SELECT direct_identity, token_value, site_url FROM proxy_video_tasks WHERE public_id = ?`, id).Scan(&identity, &token, &site); err != nil {
					t.Fatal(err)
				}
				var saved directVideoIdentity
				if json.Unmarshal([]byte(identity), &saved) != nil || saved.Version != 2 || saved.Protocol != bit || token != "" || site != "" || strings.Contains(identity, "fixture-private") || strings.Contains(identity, "fixture-upstream-key") {
					t.Fatalf("bad identity: %s", identity)
				}
				videoExec(t, db, `UPDATE upstream_group_items SET protocol_order = ?`, store.DirectProtocolOrder{store.DirectProtocolVideo, bit})
				videoExec(t, db, `UPDATE upstream_credentials SET secret = ?`, "rotated-upstream-key")
				rotated.Store(true)
				reopen()
				status, _, body := videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
				if status != 200 || !bytes.Contains(body, []byte(`"status":"completed"`)) || !bytes.Contains(body, []byte(`"seconds":"4"`)) || !bytes.Contains(body, []byte(`"completion_tokens":42`)) {
					t.Fatalf("poll: %d %s", status, body)
				}
				before := calls.Load()
				status, _, _ = videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-two")
				if status != 404 || before != calls.Load() {
					t.Fatal("another owner accessed native task")
				}
				status, _, _ = videoHTTP(t, server, "POST", "/v1/videos/"+id+"/remix", `{"prompt":"p"}`, "client-one")
				if status != 400 || before != calls.Load() {
					t.Fatal("unsupported remix performed I/O")
				}
				status, _, _ = videoHTTP(t, server, "GET", "/v1/videos/"+id+"/content?variant=thumbnail", "", "client-one")
				if status != 400 || before != calls.Load() {
					t.Fatal("invalid native content variant was not rejected before I/O")
				}
				req, _ := http.NewRequest("GET", server.URL+"/v1/videos/"+id+"/content", nil)
				req.Header.Set("Authorization", "Bearer client-one")
				req.Header.Set("Range", "bytes=0-3")
				resp, err := server.Client().Do(req)
				if err != nil {
					t.Fatal(err)
				}
				data, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode != 206 || resp.Header.Get("Content-Type") != "video/webm" || !bytes.Equal(data, []byte{0, 1, 255, 0}) || downloads.Load() != 1 {
					t.Fatalf("content: %d %v %q", resp.StatusCode, resp.Header, data)
				}
				// Do not invalidate the warm route cache: permission changes must
				// be checked against the current graph before any upstream I/O.
				videoExec(t, db, `UPDATE upstream_group_items SET protocol_order = ?`, store.DirectProtocolOrder{store.DirectProtocolVideo})
				before = calls.Load()
				status, _, _ = videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
				if status < 400 || before != calls.Load() || posts.Load() != 1 {
					t.Fatal("native task permission removal was bypassed or POST replayed")
				}
				videoExec(t, db, `UPDATE upstream_group_items SET protocol_order = ?`, store.DirectProtocolOrder{store.DirectProtocolVideo, bit})
				routing.InvalidateCache()
				failDelete.Store(true)
				status, _, _ = videoHTTP(t, server, "DELETE", "/v1/videos/"+id, "", "client-one")
				if profile == "zenmux-video" {
					if status != 400 || deletes.Load() != 0 {
						t.Fatal("ZenMux delete was not rejected before I/O")
					}
				} else {
					if status != 502 {
						t.Fatalf("delete app error: %d", status)
					}
					if _, err := loadDirectVideoTask(db, id); err != nil {
						t.Fatal("failed native delete lost durable task")
					}
					failDelete.Store(false)
					videoExec(t, db, `UPDATE upstream_grants SET cooldown_until = NULL`)
					routing.InvalidateCache()
					status, _, body = videoHTTP(t, server, "DELETE", "/v1/videos/"+id, "", "client-one")
					if status != 200 || !bytes.Contains(body, []byte(`"deleted":true`)) {
						t.Fatalf("delete: %d %s", status, body)
					}
				}
				videoExec(t, db, `UPDATE upstream_group_items SET protocol_order = ?`, store.DirectProtocolOrder{store.DirectProtocolVideo})
				before = calls.Load()
				status, _, _ = videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
				if status < 400 || before != calls.Load() || posts.Load() != 1 {
					t.Fatal("task permission removal was bypassed or POST replayed")
				}
			})
		}
	}
}

func TestDirectNativeVideoTransportFailures(t *testing.T) {
	for _, test := range []struct{ name, response string }{
		{"wrong-id", `{"id":"other","status":"succeeded","usage":{"completion_tokens":7}}`},
		{"application-error", `{"error":{"code":"invalid","message":"bad request"},"usage":{"completion_tokens":7}}`},
		{"bad-status", `{"id":"expected","status":"mystery","usage":{"completion_tokens":7}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, test.response) }))
			defer upstream.Close()
			req, _ := http.NewRequest("GET", upstream.URL+"/expected", nil)
			resp, err := sendDirectNativeVideoRequest(&UpstreamConfig{}, req, nil, 1000, "zenmux-video", "expected")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != 502 || !bytes.Contains(body, []byte(`"completion_tokens":7`)) {
				t.Fatalf("failure lost status/usage: %d %s", resp.StatusCode, body)
			}
		})
	}
	for _, cancel := range []bool{false, true} {
		t.Run(fmt.Sprintf("download-cancel-%v", cancel), func(t *testing.T) {
			started := make(chan struct{})
			cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				<-r.Context().Done()
			}))
			defer cdn.Close()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"id": "expected", "status": "succeeded", "content": map[string]string{"video_url": cdn.URL}, "usage": map[string]int{"completion_tokens": 7}})
			}))
			defer upstream.Close()
			ctx, stop := context.WithCancel(context.Background())
			defer stop()
			if cancel {
				go func() { <-started; stop() }()
			}
			req, _ := http.NewRequestWithContext(ctx, "GET", upstream.URL+"/expected/content", nil)
			_, err := sendDirectNativeVideoRequest(&UpstreamConfig{}, req, &platform.ProxyConfig{CustomHeaders: map[string]string{"X-Private": "hidden"}}, 50, "seedance-video", "expected")
			if err == nil || !directMediaFailureUsage(err).Found || directMediaFailureUsage(err).CompletionTokens != 7 {
				t.Fatalf("download failure lost usage: %v %+v", err, directMediaFailureUsage(err))
			}
		})
	}
}

func TestDirectNativeVideoLegacyIdentity(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"legacy","status":"queued"}`)
	}))
	defer upstream.Close()
	db, server, reopen := directVideoFixture(t, store.DialectSQLite, upstream.URL)
	id := createdVideoID(t, server)
	task, err := loadDirectVideoTask(db, id)
	if err != nil {
		t.Fatal(err)
	}
	if task.Identity.Version != 1 || task.Identity.Protocol != 0 {
		t.Fatal("ordinary Video task unnecessarily changed its durable format")
	}
	task.Identity.Version, task.Identity.Protocol = 1, 0
	identity, _ := json.Marshal(task.Identity)
	videoExec(t, db, `UPDATE proxy_video_tasks SET direct_identity = ? WHERE public_id = ?`, string(identity), id)
	reopen()
	status, _, body := videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
	if status != 200 {
		t.Fatalf("v1 task broke: %d %s", status, body)
	}
	videoExec(t, db, `UPDATE proxy_video_tasks SET created_at = ? WHERE public_id = ?`, time.Now().Add(-8*24*time.Hour).UTC().Format(time.RFC3339), id)
	status, _, _ = videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
	if status != 404 {
		t.Fatalf("expired legacy task status=%d", status)
	}
}

func TestDirectNativeVideoTaskIDNamedOperationHTTP(t *testing.T) {
	for _, taskID := range []string{"content", "remix"} {
		for _, profile := range []string{"seedance-video", "zenmux-video"} {
			t.Run(profile+"/"+taskID, func(t *testing.T) {
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == "GET" && r.URL.Path != "/native/tasks/"+taskID {
						t.Errorf("poll was mistaken for content: %s", r.URL)
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]string{"id": taskID, "status": "queued"})
				}))
				defer upstream.Close()
				db, server, _ := directVideoFixture(t, store.DialectSQLite, upstream.URL)
				bit, endpoints := nativeVideoEndpoint(profile, upstream.URL+"/native/tasks")
				videoExec(t, db, `UPDATE upstream_channels SET endpoint_config = ?`, endpoints)
				videoExec(t, db, `UPDATE upstream_grants SET protocols = ?`, bit)
				id := createdVideoID(t, server)
				status, _, body := videoHTTP(t, server, "GET", "/v1/videos/"+id, "", "client-one")
				var payload map[string]json.RawMessage
				_ = json.Unmarshal(body, &payload)
				if status != 200 || mediaString(payload, "id") != id {
					t.Fatalf("operation-named ID broke poll: status=%d body=%s", status, body)
				}
			})
		}
	}
}

func TestDirectNativeVideoPostNeverReplayedHTTP(t *testing.T) {
	for _, profile := range []string{"seedance-video", "zenmux-video"} {
		for _, failure := range []string{"application", "invalid-json", "upstream-503", "short-body", "persistence"} {
			t.Run(profile+"/"+failure, func(t *testing.T) {
				var posts atomic.Int32
				var db *store.DB
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					posts.Add(1)
					w.Header().Set("Content-Type", "application/json")
					switch failure {
					case "application":
						_, _ = io.WriteString(w, `{"error":{"code":"busy","message":"busy"},"usage":{"completion_tokens":11}}`)
					case "invalid-json":
						_, _ = io.WriteString(w, `{`)
					case "upstream-503":
						w.WriteHeader(503)
					case "short-body":
						w.Header().Set("Content-Length", "200")
						_, _ = io.WriteString(w, `{`)
					case "persistence":
						if _, err := db.Exec(`DROP TABLE proxy_video_tasks`); err != nil {
							t.Error(err)
						}
						_, _ = io.WriteString(w, `{"id":"created-once","status":"queued"}`)
					}
				}))
				defer upstream.Close()
				var server *httptest.Server
				db, server, _ = directVideoFixture(t, store.DialectSQLite, upstream.URL)
				bit, endpoints := nativeVideoEndpoint(profile, upstream.URL+"/tasks")
				videoExec(t, db, `UPDATE upstream_channels SET endpoint_config = ?`, endpoints)
				videoExec(t, db, `UPDATE upstream_grants SET protocols = ?`, bit)
				// A second independently authorized member makes an accidental
				// channel retry observable; one candidate would mask the bug.
				for _, query := range []string{
					`INSERT INTO upstream_channels (id,origin_key,source_id,name,dialect,provider,enabled,base_url,endpoint_config,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex) SELECT id+1000,origin_key,source_id+1000,name||' second',dialect,provider,enabled,base_url,endpoint_config,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex FROM upstream_channels`,
					`INSERT INTO upstream_credentials (id,origin_key,channel_id,source_id,name,secret,enabled) SELECT id+1000,origin_key,channel_id+1000,source_id+1000,name,secret,enabled FROM upstream_credentials`,
					`INSERT INTO upstream_models (id,origin_key,channel_id,source_id,name,enabled) SELECT id+1000,origin_key,channel_id+1000,source_id+1000,name,enabled FROM upstream_models`,
					`INSERT INTO upstream_grants (id,origin_key,source_id,model_id,credential_id,protocols,enabled) SELECT id+1000,origin_key,source_id+1000,model_id+1000,credential_id+1000,protocols,enabled FROM upstream_grants`,
					`INSERT INTO upstream_group_items (origin_key,group_id,source_id,grant_id,priority,weight,protocol_order) SELECT origin_key,group_id,source_id+1000,grant_id+1000,priority,weight,protocol_order FROM upstream_group_items`,
				} {
					videoExec(t, db, query)
				}
				copy := *config.Get()
				copy.ProxyMaxChannelAttempts = 4
				config.Set(&copy)
				status, _, body := videoHTTP(t, server, "POST", "/v1/videos", `{"model":"client-alias","prompt":"p"}`, "client-one")
				if status < 400 || posts.Load() != 1 {
					t.Fatalf("create replayed or failure hidden: status=%d posts=%d body=%s", status, posts.Load(), body)
				}
			})
		}
	}
}
