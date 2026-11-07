package proxyhandler

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
)

const providerJWTDeclaration = `authentication:{type:"jwt_hs256",header:"Authorization",prefix:"Bearer ",jwt:{keySeparator:"|",issuerClaim:"iss",ttlSeconds:1800,timeUnit:"seconds",notBeforeSkewSeconds:5}}`

func assertProviderJWT(t *testing.T, r *http.Request, id, secret string) {
	t.Helper()
	parts := strings.Split(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "), ".")
	if len(parts) != 3 || r.Header.Get("X-Api-Key") != "" || r.Header.Get("X-Goog-Api-Key") != "" {
		t.Error("provider did not send only a host JWT")
		return
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		t.Error("provider JWT was not signed with the current credential")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	var claims struct {
		Issuer string `json:"iss"`
		Expiry int64  `json:"exp"`
	}
	if err != nil || json.Unmarshal(body, &claims) != nil || claims.Issuer != id || claims.Expiry <= time.Now().Unix() {
		t.Error("provider JWT has invalid claims")
	}
}

func TestProviderCustomAuthenticationOverridesHTTPAndCompactHeaders(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, jwt := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/jwt=%v", dialect, jwt), func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if jwt {
						assertProviderJWT(t, r, "fixture-id", "fixture-signing-key")
					} else if r.Header.Get("X-Vendor-Key") != "Key fixture-current-key" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "" || r.Header.Get("X-Goog-Api-Key") != "" {
						t.Error("provider custom authentication was shadowed")
					}
					w.Header().Set("Content-Type", "application/json")
					if jwt {
						fmt.Fprint(w, `{"id":"compact-auth","object":"response.compaction","output":[{"type":"compaction","encrypted_content":"opaque"}],"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}`)
					} else {
						fmt.Fprint(w, `{"id":"chat-auth","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"receipt"},"finish_reason":"stop"}],"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}}`)
					}
				}))
				t.Cleanup(upstream.Close)
				protocol, path, secret := "chat", "/v1/chat/completions", "fixture-current-key"
				declaration := `authentication:{type:"api_key",header:"X-Vendor-Key",prefix:"Key "}`
				endpoints := store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: upstream.URL + "/generate", Auth: store.DirectAuthBearer}}
				bit := store.DirectProtocolChat
				body := `{"model":"client-alias","messages":[{"role":"user","content":"fixture"}]}`
				if jwt {
					protocol, path, secret, declaration = "responsesCompact", "/v1/responses/compact", "fixture-id|fixture-signing-key", providerJWTDeclaration
					endpoints = store.DirectEndpoints{ResponsesCompact: &store.DirectEndpoint{URL: upstream.URL + "/compact", Auth: store.DirectAuthBearer}}
					bit = store.DirectProtocolResponsesCompact
					body = `{"model":"client-alias","input":[{"role":"user","content":"fixture"}]}`
				}
				db, logs := installPlatformHTTPRuntime(t, dialect, "custom", "provider-model", endpoints, bit, "")
				videoExec(t, db, `UPDATE upstream_credentials SET secret=?`, secret)
				source := fmt.Sprintf(`export const meta={apiVersion:2,kind:"provider",key:"auth",name:"Auth",version:"1.0.0",protocols:[%q],credentialKinds:["api_key"],hooks:["request","response"],%s};export function request(i){if(JSON.stringify(i).includes("fixture-signing-key")||JSON.stringify(i).includes("fixture-current-key"))throw new Error("secret entered JS");return {method:"POST",body:i.body,headers:{"X-Vendor-Key":"driver-spoof"}};}export function response(i){return {body:i.body};}`, protocol, declaration)
				if _, err := channelplugins.Save(t.Context(), db.DB, "auth", source, true, 0); err != nil {
					t.Fatal(err)
				}
				requestConfig := store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "auth", Protocols: []string{protocol}}}
				requestConfig.Headers = []requestoverride.Operation{{Op: "set", Path: "X-Vendor-Key", Value: "private-rule-spoof"}}
				videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, requestConfig)
				getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
				request := makeProxyReq("POST", path, body)
				request.Header.Set("X-Vendor-Key", "client-spoof")
				out := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(out, request)
				if out.Code != 200 || calls.Load() != 1 {
					t.Fatalf("host-authenticated request failed: status=%d calls=%d", out.Code, calls.Load())
				}
				platformUsage(t, logs, "success", 2, 1)
				videoExec(t, db, `UPDATE upstream_credentials SET secret=?`, "malformed-fixture")
				if jwt {
					out = httptest.NewRecorder()
					directMediaRouter().ServeHTTP(out, makeProxyReq("POST", path, body))
					if out.Code != 503 || calls.Load() != 1 || strings.Contains(out.Body.String(), "malformed-fixture") {
						t.Fatal("bad JWT material dispatched, replayed or leaked")
					}
				}
			})
		}
	}
}

func TestProviderJWTWebsocketReusesSessionAcrossSigningClock(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var dials, calls atomic.Int32
			received := make(chan string, 8)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dial := dials.Add(1)
				secret := "fixture-signing-key"
				if dial > 1 {
					secret = "rotated-fixture-signing-key"
				}
				assertProviderJWT(t, r, "fixture-id", secret)
				conn, err := websocket.Accept(w, r, nil)
				if err != nil {
					return
				}
				defer conn.CloseNow()
				for {
					_, raw, err := conn.Read(r.Context())
					if err != nil {
						return
					}
					received <- string(raw)
					n := calls.Add(1)
					output := `[{"type":"message","content":[{"type":"output_text","text":"receipt"}]}]`
					if n == 1 {
						output = `[{"type":"function_call","id":"fc-auth","call_id":"call-auth","name":"echo","arguments":"{\"n\":9007199254740993}"}]`
					}
					body := fmt.Sprintf(`{"type":"response.completed","response":{"id":"auth-ws-%d","status":"completed","model":"provider-model","output":%s,"usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`, n, output)
					if err := conn.Write(r.Context(), websocket.MessageText, []byte(body)); err != nil {
						return
					}
				}
			}))
			t.Cleanup(upstream.Close)
			db, logs := installPlatformHTTPRuntime(t, dialect, "custom", "provider-model", store.DirectEndpoints{ResponsesWebsocket: &store.DirectEndpoint{URL: "ws" + strings.TrimPrefix(upstream.URL, "http") + "/native", Auth: store.DirectAuthBearer}}, store.DirectProtocolResponsesWebsocket, "")
			videoExec(t, db, `UPDATE upstream_credentials SET secret=?`, "fixture-id|fixture-signing-key")
			source := `export const meta={apiVersion:2,kind:"provider",key:"auth",name:"Auth",version:"1.0.0",protocols:["responsesWebsocket"],credentialKinds:["api_key"],hooks:["request","event"],` + providerJWTDeclaration + `};export function request(i){return {body:i.body};}export function event(i){return {body:i.body};}`
			if _, err := channelplugins.Save(t.Context(), db.DB, "auth", source, true, 0); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "auth", Protocols: []string{"responsesWebsocket"}}})
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			ctx, conn := dialDirectResponsesGateway(t, "fixture-auth-owner")
			input := `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"fixture"}]}`
			writeWSMessage(t, ctx, conn, input)
			if frame := readWSFrame(t, ctx, conn); frame["type"] != "response.completed" {
				t.Fatal("first signed WS turn failed")
			}
			platformUsage(t, logs, "success", 2, 1)
			<-received
			// Cross the signer clock without changing the underlying identity.
			time.Sleep(1100 * time.Millisecond)
			writeWSMessage(t, ctx, conn, `{"type":"response.create","model":"client-alias","previous_response_id":"auth-ws-1","input":[{"type":"function_call_output","call_id":"call-auth","output":"9007199254740993"}]}`)
			if frame := readWSFrame(t, ctx, conn); frame["type"] != "response.completed" {
				t.Fatal("second signed WS turn failed")
			}
			platformUsage(t, logs, "success", 2, 1)
			if body := <-received; !strings.Contains(body, `"previous_response_id":"auth-ws-1"`) || !strings.Contains(body, "9007199254740993") {
				t.Fatal("signed WS continuation lost its identity or opaque tool receipt")
			}
			if dials.Load() != 1 || calls.Load() != 2 {
				t.Fatalf("signing clock replaced the authorized WS session: dials=%d calls=%d", dials.Load(), calls.Load())
			}
			videoExec(t, db, `UPDATE upstream_credentials SET secret=?`, "fixture-id|rotated-fixture-signing-key")
			writeWSMessage(t, ctx, conn, input)
			if frame := readWSFrame(t, ctx, conn); frame["type"] != "response.completed" {
				t.Fatal("signed WS credential rotation failed")
			}
			platformUsage(t, logs, "success", 2, 1)
			<-received
			if dials.Load() != 2 {
				t.Fatal("rotated signing material reused an old authenticated socket")
			}
			upgraded := strings.Replace(source, "ttlSeconds:1800", "ttlSeconds:900", 1)
			if _, err := channelplugins.Save(t.Context(), db.DB, "auth", upgraded, true, 1); err != nil {
				t.Fatal(err)
			}
			writeWSMessage(t, ctx, conn, input)
			if frame := readWSFrame(t, ctx, conn); frame["type"] != "response.completed" {
				t.Fatal("signed WS authentication upgrade failed")
			}
			platformUsage(t, logs, "success", 2, 1)
			<-received
			if dials.Load() != 3 {
				t.Fatal("authentication upgrade reused the old handshake")
			}
			otherCtx, other := dialDirectResponsesGateway(t, "fixture-auth-other-owner")
			writeWSMessage(t, otherCtx, other, input)
			if frame := readWSFrame(t, otherCtx, other); frame["type"] != "response.completed" {
				t.Fatal("second signed WS client failed")
			}
			platformUsage(t, logs, "success", 2, 1)
			<-received
			if dials.Load() != 4 || calls.Load() != 5 {
				t.Fatal("signed WS socket crossed a downstream identity")
			}
			videoExec(t, db, `UPDATE upstream_grants SET enabled=FALSE`)
			writeWSMessage(t, ctx, conn, input)
			if frame := readWSFrame(t, ctx, conn); frame["type"] != "error" || dials.Load() != 4 || calls.Load() != 5 || len(logs) != 0 {
				t.Fatal("cached JWT bypassed current grant revocation or created a fake attempt")
			}
		})
	}
}

func TestProviderCustomAuthenticationNeverFollowsCrossOriginRedirect(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var received, redirected atomic.Int32
			foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(foreign.Close)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				redirected.Add(1)
				if r.Header.Get("X-Vendor-Key") != "Key fixture-current-key" || r.Header.Get("Authorization") != "" {
					t.Error("original origin did not use only custom authentication")
				}
				http.Redirect(w, r, foreign.URL+"/foreign", http.StatusTemporaryRedirect)
			}))
			t.Cleanup(upstream.Close)
			db, _ := installPlatformHTTPRuntime(t, dialect, "custom", "provider-model", store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: upstream.URL + "/native", Auth: store.DirectAuthBearer}}, store.DirectProtocolChat, "")
			videoExec(t, db, `UPDATE upstream_credentials SET secret=?`, "fixture-current-key")
			const source = `export const meta={apiVersion:2,kind:"provider",key:"auth",name:"Auth",version:"1.0.0",protocols:["chat"],credentialKinds:["api_key"],hooks:["request","response"],authentication:{type:"api_key",header:"X-Vendor-Key",prefix:"Key "}};export function request(i){return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}`
			if _, err := channelplugins.Save(t.Context(), db.DB, "auth", source, true, 0); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "auth", Protocols: []string{"chat"}}})
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			out := httptest.NewRecorder()
			directMediaRouter().ServeHTTP(out, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"fixture"}]}`))
			if out.Code < 400 || received.Load() != 0 || redirected.Load() == 0 || strings.Contains(out.Body.String(), "fixture-current-key") {
				t.Fatal("custom authentication followed a foreign origin or leaked into its error")
			}
		})
	}
}

func TestProviderHostAuthenticationWebsocketFallbackHasFreshHTTPAuthentication(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var upgrades, generations atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assertProviderJWT(t, r, "fixture-id", "fixture-signing-key")
				if r.URL.Path == "/ws" {
					upgrades.Add(1)
					http.Error(w, "synthetic upgrade refused", http.StatusUpgradeRequired)
					return
				}
				generations.Add(1)
				if r.URL.Path != "/http" || r.Method != http.MethodPost {
					t.Error("WS fallback did not use its independently configured HTTP endpoint")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"auth-fallback\",\"status\":\"completed\",\"model\":\"provider-model\",\"output\":[{\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"receipt\"}]}],\"usage\":{\"input_tokens\":2,\"output_tokens\":1,\"total_tokens\":3}}}\n\n")
			}))
			t.Cleanup(server.Close)
			endpoints := store.DirectEndpoints{
				Responses:          &store.DirectEndpoint{URL: server.URL + "/http", Auth: store.DirectAuthBearer},
				ResponsesWebsocket: &store.DirectEndpoint{URL: "ws" + strings.TrimPrefix(server.URL, "http") + "/ws", Auth: store.DirectAuthBearer},
			}
			db, logs := installPlatformHTTPRuntime(t, dialect, "custom", "provider-model", endpoints, store.DirectProtocolResponses|store.DirectProtocolResponsesWebsocket, "")
			videoExec(t, db, `UPDATE upstream_credentials SET secret=?`, "fixture-id|fixture-signing-key")
			source := `export const meta={apiVersion:2,kind:"provider",key:"auth",name:"Auth",version:"1.0.0",protocols:["responses","responsesWebsocket"],credentialKinds:["api_key"],hooks:["request","response","event"],` + providerJWTDeclaration + `};export function request(i){if(i.protocol==="responsesWebsocket")return {body:i.body};return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}export function event(i){return {body:i.body};}`
			if _, err := channelplugins.Save(t.Context(), db.DB, "auth", source, true, 0); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "auth", Protocols: []string{"responses", "responsesWebsocket"}}})
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			ctx, conn := dialDirectResponsesGateway(t, "fixture-auth-fallback-owner")
			input := `{"type":"response.create","model":"client-alias","input":[{"role":"user","content":"fixture"}]}`
			writeWSMessage(t, ctx, conn, input)
			if frame := readWSFrame(t, ctx, conn); frame["type"] != "response.completed" || upgrades.Load() != 1 || generations.Load() != 1 {
				t.Fatal("independently authenticated HTTP fallback did not complete")
			}
			platformUsage(t, logs, "success", 2, 1)
			if _, err := channelplugins.Save(t.Context(), db.DB, "auth", source, false, 1); err != nil {
				t.Fatal(err)
			}
			writeWSMessage(t, ctx, conn, input)
			if frame := readWSFrame(t, ctx, conn); frame["type"] != "error" || upgrades.Load() != 1 || generations.Load() != 1 || len(logs) != 0 {
				t.Fatal("disabled authentication fell back to an unsigned or raw-key request")
			}
		})
	}
}

func TestProviderHostAuthenticationMultipartAndBinaryConsumers(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, upload := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/upload=%v", dialect, upload), func(t *testing.T) {
				data := []byte{0, 1, 255, 13, 10, 0}
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					assertProviderJWT(t, r, "fixture-id", "fixture-signing-key")
					if upload {
						if err := r.ParseMultipartForm(1 << 20); err != nil {
							t.Error(err)
							w.WriteHeader(400)
							return
						}
						defer r.MultipartForm.RemoveAll()
						file, _, err := r.FormFile("file")
						if err != nil {
							t.Error(err)
							w.WriteHeader(400)
							return
						}
						defer file.Close()
						raw, _ := io.ReadAll(file)
						if !bytes.Equal(raw, data) || r.FormValue("model") != "provider-model" {
							t.Error("signing changed multipart data")
						}
						w.Header().Set("Content-Type", "application/json")
						fmt.Fprint(w, `{"text":"receipt"}`)
					} else {
						w.Header().Set("Content-Type", "audio/mpeg")
						_, _ = w.Write(data)
					}
				}))
				t.Cleanup(server.Close)
				key, path, bit := "audioSpeech", "/v1/audio/speech", store.DirectProtocolAudioSpeech
				endpoint := &store.DirectEndpoint{URL: server.URL + "/native", Auth: store.DirectAuthBearer}
				endpoints := store.DirectEndpoints{AudioSpeech: endpoint}
				if upload {
					key, path, bit = "audioTranscription", "/v1/audio/transcriptions", store.DirectProtocolAudioTranscription
					endpoints = store.DirectEndpoints{AudioTranscription: endpoint}
				}
				db := installDirectMediaRuntime(t, dialect, endpoints, bit, "")
				videoExec(t, db, `UPDATE upstream_credentials SET secret=?`, "fixture-id|fixture-signing-key")
				source := fmt.Sprintf(`export const meta={apiVersion:2,kind:"provider",key:"auth",name:"Auth",version:"1.0.0",protocols:[%q],credentialKinds:["api_key"],requestKinds:["json","multipart"],hooks:["request","response"],%s};export function request(i){if(i.bodyKind==="multipart")return {method:"POST",parts:[{field:"model",value:i.model},{field:"file",fileRef:i.files[0].ref}]};return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}`, key, providerJWTDeclaration)
				if _, err := channelplugins.Save(t.Context(), db.DB, "auth", source, true, 0); err != nil {
					t.Fatal(err)
				}
				videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "auth", Protocols: []string{key}}})
				getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
				request := makeProxyReq("POST", path, `{"model":"client-alias","input":"fixture"}`)
				if upload {
					var body bytes.Buffer
					writer := multipart.NewWriter(&body)
					_ = writer.WriteField("model", "client-alias")
					part, err := writer.CreateFormFile("file", "fixture.wav")
					if err != nil {
						t.Fatal(err)
					}
					_, _ = part.Write(data)
					_ = writer.Close()
					request = httptest.NewRequest("POST", path, &body)
					request.Header.Set("Content-Type", writer.FormDataContentType())
					request = request.WithContext(auth.WithProxyAuth(request.Context(), &auth.ProxyAuthContext{Source: "global", Token: "fixture-client"}))
				}
				out := httptest.NewRecorder()
				directMediaRouter().ServeHTTP(out, request)
				if out.Code != 200 || calls.Load() != 1 {
					t.Fatalf("authenticated media failed: status=%d calls=%d", out.Code, calls.Load())
				}
				if !upload && !bytes.Equal(out.Body.Bytes(), data) {
					t.Fatal("binary audio changed after host authentication")
				}
			})
		}
	}
}

func TestProviderHostAuthenticationTaskUsesArchivedContractAndCurrentKey(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				secret := "fixture-signing-key"
				if n > 1 {
					secret = "rotated-fixture-signing-key"
				}
				assertProviderJWT(t, r, "fixture-id", secret)
				if r.Header.Get("X-New-Authorization") != "" {
					t.Error("persisted task switched authentication contract")
				}
				if strings.HasSuffix(r.URL.Path, "/content") {
					w.Header().Set("Content-Type", "video/mp4")
					fmt.Fprint(w, "fixture-video-bytes")
					return
				}
				status, output := "queued", 0
				if r.Method == "GET" {
					status, output = "completed", 2
				}
				native := fmt.Sprintf(`{"id":"job-auth","object":"video","model":"provider-model","status":%q,"usage":{"input_tokens":7,"output_tokens":%d,"total_tokens":%d}}`, status, output, 7+output)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"vendor": map[string]any{"native": native, "model": "provider-model", "meter": map[string]int{"input": 7, "output": output, "total": 7 + output}}})
			}))
			t.Cleanup(server.Close)
			db, gateway, reopen := directVideoFixture(t, dialect, server.URL)
			videoExec(t, db, `UPDATE upstream_credentials SET secret=?`, "fixture-id|fixture-signing-key")
			source := strings.Replace(providerMediaSource, `hooks:[`, providerJWTDeclaration+`,hooks:[`, 1)
			if _, err := channelplugins.Save(t.Context(), db.DB, "media", source, true, 0); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_channels SET request_config=?`, store.DirectRequestConfig{Provider: &store.DirectProviderBinding{Key: "media", Protocols: []string{"video"}}})
			getUpstreamConfig().ChannelPlugins = channelplugins.New(db.DB)
			status, _, body := videoHTTP(t, gateway, "POST", "/v1/videos", `{"model":"client-alias","prompt":"fixture"}`, "client-one")
			var task struct {
				ID string `json:"id"`
			}
			if status != 200 || json.Unmarshal(body, &task) != nil || task.ID == "" {
				t.Fatalf("signed task creation failed: status=%d", status)
			}
			upgraded := strings.Replace(source, `header:"Authorization"`, `header:"X-New-Authorization"`, 1)
			if _, err := channelplugins.Save(t.Context(), db.DB, "media", upgraded, true, 1); err != nil {
				t.Fatal(err)
			}
			videoExec(t, db, `UPDATE upstream_credentials SET secret=?`, "fixture-id|rotated-fixture-signing-key")
			reopen()
			getUpstreamConfig().ChannelPlugins = channelplugins.New(store.GetDB().DB)
			status, _, body = videoHTTP(t, gateway, "GET", "/v1/videos/"+task.ID, "", "client-one")
			if status != 200 || !strings.Contains(string(body), `"completed"`) {
				t.Fatalf("reopened signed task failed: status=%d", status)
			}
			status, _, body = videoHTTP(t, gateway, "GET", "/v1/videos/"+task.ID+"/content", "", "client-one")
			if status != 200 || string(body) != "fixture-video-bytes" || calls.Load() != 3 {
				t.Fatalf("signed content failed: status=%d calls=%d", status, calls.Load())
			}
			if _, err := channelplugins.Save(t.Context(), store.GetDB().DB, "media", upgraded, false, 2); err != nil {
				t.Fatal(err)
			}
			status, _, _ = videoHTTP(t, gateway, "GET", "/v1/videos/"+task.ID, "", "client-one")
			if status != 503 || calls.Load() != 3 {
				t.Fatal("archived authentication bypassed current plugin disable")
			}
		})
	}
}
