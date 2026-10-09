package proxyhandler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/deliciousbuding/metapi-go/transform/openai/responses"
)

func TestDirectCodexProviderWireHTTPAndTerminalAggregation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			t.Error("invalid request")
		}
		if body["stream"] != true || body["store"] != false || body["parallel_tool_calls"] != true {
			t.Error("missing Codex wire fields")
		}
		for _, key := range []string{"max_output_tokens", "metadata", "user"} {
			if _, ok := body[key]; ok {
				t.Errorf("Codex rejected field retained: %s", key)
			}
		}
		if r.Header.Get("Authorization") != "Bearer fixture-access" || r.Header.Get("Chatgpt-Account-Id") != "account-fixture" || r.Header.Get("Session-Id") != "session-fixture" || r.Header.Get("X-Openai-Internal-Codex-Responses-Lite") != "" {
			t.Error("wrong Codex request headers")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: response.output_item.done\ndata: {\"type\":\"response.output_item.done\",\"output_index\":0,\"item\":{\"type\":\"function_call\",\"id\":\"fc\",\"call_id\":\"call\",\"name\":\"echo\",\"arguments\":\"{\\\"value\\\":1}\"}}\n\n")
		_, _ = io.WriteString(w, "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-fixture\",\"status\":\"completed\",\"usage\":{\"input_tokens\":5,\"output_tokens\":3},\"large\":9007199254740993}}\n\n")
	}))
	defer upstream.Close()
	headers := http.Header{}
	headers.Set("Session-Id", "session-fixture")
	headers.Set("Chatgpt-Account-Id", "attacker")
	headers.Set("X-Openai-Internal-Codex-Responses-Lite", "true")
	wire, err := prepareDirectProviderWire(&store.DirectEndpoint{URL: upstream.URL, Profile: "codex"}, 1, &oauth.DirectCredentialResult{Kind: "oauth", AccountID: "account-fixture"}, []byte(`{"model":"gpt-test","input":"hello","max_output_tokens":40,"metadata":{"x":1},"user":"user"}`), headers)
	if err != nil || !wire.ForceStream {
		t.Fatalf("wire error=%v", err)
	}
	req, _ := http.NewRequest("POST", upstream.URL, bytes.NewReader(wire.Body))
	req.Header = headers.Clone()
	req.Header.Set("Authorization", "Bearer fixture-access")
	applyDirectProviderHeaders(req, wire)
	response, err := upstream.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	collector := responses.CodexResponseCollector{}
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		if data, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			if err := collector.AddData([]byte(data)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	result, err := collector.Result()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(result, []byte(`"large":9007199254740993`)) || !bytes.Contains(result, []byte(`"name":"echo"`)) || !bytes.Contains(result, []byte(`"input_tokens":5`)) {
		t.Fatalf("aggregation lost tool/usage data: %s", result)
	}
}

func TestDirectClaudeProviderToolsAndHeaders(t *testing.T) {
	for _, tc := range []struct {
		name, kind, ua string
		prefix         bool
	}{
		{"OAuth non CLI", "oauth", "client/1", true},
		{"OAuth CLI", "oauth", "claude-cli/2.1.170", false},
		{"static key", "api_key", "client/1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			downstream := http.Header{}
			downstream.Set("User-Agent", tc.ua)
			downstream.Set("Anthropic-Beta", "client-beta")
			downstream.Set("X-Claude-Code-Session-Id", "session")
			wire, err := prepareDirectProviderWire(&store.DirectEndpoint{URL: "https://api.example.invalid/v1/messages", Profile: "claudecode"}, 1, &oauth.DirectCredentialResult{Kind: tc.kind}, []byte(`{"model":"claude","max_tokens":10,"system":"Keep this instruction","thinking":{"type":"enabled","budget_tokens":1024},"tools":[{"name":"echo","input_schema":{"type":"object"}}],"tool_choice":{"type":"tool","name":"echo"},"messages":[{"role":"user","content":"hello"}]}`), downstream)
			if err != nil {
				t.Fatal(err)
			}
			if wire.StripToolPrefix != tc.prefix || wire.ForceStream {
				t.Fatal("wrong Claude wire mode")
			}
			var body map[string]json.RawMessage
			_ = json.Unmarshal(wire.Body, &body)
			if _, ok := body["thinking"]; ok {
				t.Fatal("forced tool with thinking")
			}
			if !bytes.Contains(body["system"], []byte("Keep this instruction")) || !bytes.Contains(body["system"], []byte("You are Claude Code")) {
				t.Fatal("system instructions lost")
			}
			name := "echo"
			if tc.prefix {
				name = "proxy_echo"
			}
			if !bytes.Contains(body["tools"], []byte(`"name":"`+name+`"`)) || !bytes.Contains(body["tool_choice"], []byte(`"name":"`+name+`"`)) {
				t.Fatal("tool declaration/choice disagree")
			}
			req, _ := http.NewRequest("POST", "https://api.example.invalid/v1/messages", nil)
			applyDirectProviderHeaders(req, wire)
			if req.URL.Query().Get("beta") != "true" || !strings.Contains(req.Header.Get("Anthropic-Beta"), "client-beta") || !strings.Contains(req.Header.Get("Anthropic-Beta"), "claude-code-20250219") {
				t.Fatal("required Claude headers/query missing")
			}
			ctx := withDirectProviderWire(context.Background(), wire)
			event := []byte(`{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call","name":"proxy_echo","input":{}},"large":9007199254740993}`)
			got := restoreDirectProviderResponse(ctx, event)
			wantName := "proxy_echo"
			if tc.prefix {
				wantName = "echo"
			}
			if !bytes.Contains(got, []byte(`"name":"`+wantName+`"`)) || !bytes.Contains(got, []byte(`9007199254740993`)) {
				t.Fatalf("tool SSE response changed incorrectly: %s", got)
			}
		})
	}
}
