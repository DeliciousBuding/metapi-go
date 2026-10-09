package proxyhandler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDomesticThinkingKeepsExactToolHistory(t *testing.T) {
	for _, reasoning := range []string{"", "null", `"already reasoned"`} {
		field := ""
		if reasoning != "" {
			field = `,"reasoning_content":` + reasoning
		}
		body := `{"model":"model","messages":[{"role":"assistant","content":null,"tool_calls":[{"id":"call","type":"function","function":{"name":"echo","arguments":"{\"id\":9007199254740993}"}}]` + field + `}],"tools":[{"type":"function","function":{"name":"echo","parameters":{"minimum":9007199254740993}}}],"reasoning_effort":"high"}`
		out, err := prepareDomesticChatRequest([]byte(body), "deepseek")
		if err != nil {
			t.Fatal(err)
		}
		want := reasoning
		if want == "" || want == "null" {
			want = `""`
		}
		if !strings.Contains(string(out), `"reasoning_content":`+want) || !strings.Contains(string(out), `9007199254740993`) {
			t.Fatalf("tool history changed: %s", out)
		}
	}
	for _, profile := range []string{"deepseek", "zai"} {
		out, err := prepareDomesticChatRequest([]byte(`{"messages":[{"role":"user","content":"hi"}],"thinking":{"type":"disabled","clear_thinking":false},"reasoning_effort":"high"}`), profile)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), `"type":"disabled"`) || !strings.Contains(string(out), `"clear_thinking":false`) {
			t.Fatalf("explicit provider thinking lost: %s", out)
		}
	}
}

type domesticTestTransport func(*http.Request) (*http.Response, error)

func (f domesticTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNativeSitePresetUsesDomesticThinkingMapping(t *testing.T) {
	for _, tc := range []struct{ url, platform, profile string }{
		{"https://api.deepseek.com/v1", "openai", "deepseek"},
		{"https://open.bigmodel.cn/api/paas/v4", "openai", "zai"},
		{"https://api.z.ai/api/coding/paas/v4", "openai", "zai"},
		{"https://api.xiaomimimo.com", "openai", "zai"},
		{"https://api.deepseek.com.evil.invalid/v1", "openai", ""},
		{"https://api.deepseek.com/unrecognized", "openai", ""},
		{"https://api.deepseek.com/v1", "new-api", ""},
		{"https://gateway.example/v1", "openai", ""},
	} {
		t.Run(tc.url+tc.platform, func(t *testing.T) {
			var observed map[string]json.RawMessage
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&observed)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, auditDomesticAnswer)
			}))
			defer upstream.Close()
			target, _ := url.Parse(upstream.URL)
			previous := defaultUpstreamClient
			defaultUpstreamClient = &http.Client{Transport: domesticTestTransport(func(r *http.Request) (*http.Response, error) {
				copy := r.Clone(r.Context())
				targetURL := *r.URL
				targetURL.Scheme = target.Scheme
				targetURL.Host = target.Host
				copy.URL = &targetURL
				return http.DefaultTransport.RoundTrip(copy)
			})}
			defer func() { defaultUpstreamClient = previous }()
			oldRuntime := config.RuntimeSafe()
			config.SetRuntime(&config.RuntimeSettings{})
			defer config.SetRuntime(oldRuntime)
			old := getUpstreamConfig()
			SetUpstreamConfig(&UpstreamConfig{Router: &upstreamTestRouter{selected: routing.SelectedChannel{Site: store.Site{ID: 1, URL: tc.url, Platform: tc.platform}, Account: store.Account{ID: 1}, Channel: store.RouteChannel{ID: 1, Enabled: true}, TokenValue: "fixture", ActualModel: "provider-model"}}, LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }})
			defer SetUpstreamConfig(old)
			out := httptest.NewRecorder()
			HandleChatCompletions(out, makeProxyReq("POST", "/v1/chat/completions", `{"model":"client-alias","messages":[{"role":"user","content":"hi"}],"reasoning_effort":"none"}`))
			if out.Code != 200 {
				t.Fatalf("native relay failed: %d %s", out.Code, out.Body.String())
			}
			if tc.profile == "" {
				if observed["thinking"] != nil || string(observed["reasoning_effort"]) != `"none"` {
					t.Fatalf("generic native request was rewritten: %s", observed)
				}
			} else if string(observed["thinking"]) != `{"type":"disabled"}` {
				t.Fatalf("native preset missing thinking mapping: %s", observed)
			}
		})
	}
}
