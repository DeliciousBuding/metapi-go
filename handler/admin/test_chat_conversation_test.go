package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestChatConversationUpstreamProtocolMatrix(t *testing.T) {
	for _, format := range []string{"openai", "claude", "responses", "gemini"} {
		t.Run(format, func(t *testing.T) {
			db, channel, router := setupTestRoutes(t)
			_, _, _, channelID := insertHarnessFixtures(t, db)
			seen := make(chan map[string]any, 2)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var payload map[string]any
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				wantPath, wantHeader := "/v1/chat/completions", "Authorization"
				switch format {
				case "claude":
					wantPath, wantHeader = "/v1/messages", "x-api-key"
				case "responses":
					wantPath = "/v1/responses"
				case "gemini":
					wantPath, wantHeader = "/v1beta/models/fixture-model:generateContent", "x-goog-api-key"
				}
				if r.URL.Path != wantPath || r.Header.Get(wantHeader) == "" {
					t.Errorf("path=%s authHeaderPresent=%v", r.URL.Path, r.Header.Get(wantHeader) != "")
				}
				seen <- payload
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"reply":"`+strings.Repeat("完整回复", 1000)+`"}`)
			}))
			defer upstream.Close()
			// Keep the synthetic site identity, route only the test transport to
			// httptest. Production URL policy and transports stay unchanged.
			channel.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				r.URL.Scheme = "http"
				r.URL.Host = strings.TrimPrefix(upstream.URL, "http://")
				return http.DefaultTransport.RoundTrip(r)
			})
			history := []map[string]string{{"role": "system", "content": "系统指令"}, {"role": "user", "content": strings.Repeat("完整问题", 100)}}
			for turn := 0; turn < 2; turn++ {
				resp := doPostJSON(t, router, "/api/test/chat", map[string]any{"channelId": channelID, "model": "fixture-model", "messages": history, "targetFormat": format, "temperature": 0.2, "top_p": 0.8, "max_tokens": 512})
				if resp.Code != 200 {
					t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
				}
				var result map[string]any
				if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if result["success"] != true || result["bodyTruncated"] != false || !json.Valid([]byte(result["truncatedBody"].(string))) {
					t.Fatalf("response lost content: %v", result)
				}
				payload := <-seen
				var messages []any
				var sampling map[string]any
				switch format {
				case "openai", "claude":
					messages = payload["messages"].([]any)
					sampling = payload
				case "responses":
					messages = payload["input"].([]any)
					sampling = payload
				case "gemini":
					messages = payload["contents"].([]any)
					sampling = payload["generationConfig"].(map[string]any)
				}
				if format == "openai" {
					if len(messages) != len(history) {
						t.Fatalf("messages=%v", messages)
					}
				} else if len(messages) != len(history)-1 {
					t.Fatalf("history lost: %v", payload)
				}
				raw, _ := json.Marshal(payload)
				for _, text := range []string{"系统指令", history[1]["content"]} {
					if !bytes.Contains(raw, []byte(text)) {
						t.Errorf("missing full text %q", text)
					}
				}
				if turn == 1 && (!bytes.Contains(raw, []byte("第一轮回复")) || !bytes.Contains(raw, []byte("继续问题"))) {
					t.Error("second turn lost prior assistant or current user")
				}
				maxKey, topKey := "max_tokens", "top_p"
				if format == "responses" {
					maxKey = "max_output_tokens"
				}
				if format == "gemini" {
					maxKey, topKey = "maxOutputTokens", "topP"
				}
				if sampling[maxKey] != float64(512) || sampling["temperature"] != 0.2 || sampling[topKey] != 0.8 {
					t.Errorf("sampling=%v", sampling)
				}
				history = append(history, map[string]string{"role": "assistant", "content": "第一轮回复"}, map[string]string{"role": "user", "content": "继续问题"})
			}
		})
	}
}

func TestChatConversationCancellationReachesUpstream(t *testing.T) {
	db, channel, router := setupTestRoutes(t)
	_, _, _, channelID := insertHarnessFixtures(t, db)
	started, canceled := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer upstream.Close()
	channel.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r.URL.Scheme = "http"
		r.URL.Host = strings.TrimPrefix(upstream.URL, "http://")
		return http.DefaultTransport.RoundTrip(r)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body, _ := json.Marshal(map[string]any{"channelId": channelID, "messages": []map[string]string{{"role": "user", "content": "cancel this"}}})
	req := httptest.NewRequest(http.MethodPost, "/api/test/chat", bytes.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { router.ServeHTTP(rec, req); close(done) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream did not start")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream did not receive cancellation")
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler did not finish")
	}
}

func TestChatConversationValidation(t *testing.T) {
	_, _, router := setupTestRoutes(t)
	for _, values := range []map[string]any{{"targetFormat": "invalid"}, {"temperature": 3}, {"top_p": -1}, {"max_tokens": 0}, {"messages": []map[string]any{{"role": "user", "content": 42}}}} {
		values["channelId"] = 1
		resp := doPostJSON(t, router, "/api/test/chat", values)
		if resp.Code != http.StatusBadRequest {
			t.Errorf("status=%d for %v", resp.Code, values)
		}
	}
}

func TestChatConversationUpstreamErrorAndDefaults(t *testing.T) {
	db, channel, router := setupTestRoutes(t)
	_, _, _, channelID := insertHarnessFixtures(t, db)
	channel.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["max_tokens"] != float64(1024) {
			t.Errorf("max_tokens=%v", payload["max_tokens"])
		}
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"provider unavailable"}}`)), Header: make(http.Header), Request: r}, nil
	})
	resp := doPostJSON(t, router, "/api/test/chat", map[string]any{"channelId": channelID, "messages": []map[string]any{{"role": "user", "content": []map[string]string{{"type": "text", "text": "compatibility"}}}}})
	var result map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if resp.Code != 200 || result["success"] != false || result["statusCode"] != float64(503) || !strings.Contains(result["truncatedBody"].(string), "provider unavailable") {
		t.Errorf("status=%d result=%v", resp.Code, result)
	}
}
