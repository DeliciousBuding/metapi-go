package service_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service"
)

// Assert path construction against provider endpoint contracts, without claiming
// that a provider exposes model discovery (plans may require manual models).
func TestDomesticPresetEndpointPaths(t *testing.T) {
	cases := []struct{ id, requestPath, modelsPath string }{
		{"bailian-claude", "/apps/anthropic/v1/messages", "/apps/anthropic/v1/models"},
		{"zhipu-openai", "/api/paas/v4/chat/completions", "/api/paas/v4/models"},
		{"zai-openai", "/api/paas/v4/chat/completions", "/api/paas/v4/models"},
		{"zai-coding-plan-openai", "/api/coding/paas/v4/chat/completions", "/api/coding/paas/v4/models"},
		{"zai-coding-plan-claude", "/api/anthropic/v1/messages", "/api/anthropic/v1/models"},
		{"doubao-openai", "/api/v3/chat/completions", "/api/v3/models"},
		{"doubao-coding-claude", "/api/coding/v1/messages", "/api/coding/v1/models"},
		{"kimi-coding-openai", "/coding/v1/chat/completions", "/coding/v1/models"},
		{"kimi-coding-claude", "/coding/v1/messages", "/coding/v1/models"},
		{"xiaomi-openai", "/v1/chat/completions", "/v1/models"},
		{"xiaomi-token-plan-claude", "/anthropic/v1/messages", "/anthropic/v1/models"},
		{"ppio-openai", "/openai/v1/chat/completions", "/openai/v1/models"},
		{"qiniu-openai", "/v1/chat/completions", "/v1/models"},
		{"qiniu-claude", "/v1/messages", "/v1/models"},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			preset := service.GetSiteInitializationPreset(tc.id)
			if preset == nil {
				t.Fatal("missing preset")
			}
			parsed, err := url.Parse(preset.DefaultURL)
			if err != nil {
				t.Fatal(err)
			}
			downstream := "/v1/chat/completions"
			if preset.Platform == "claude" {
				downstream = "/v1/messages"
			}
			if got := proxy.BuildUpstreamURL(preset.DefaultURL, downstream); got != parsed.Scheme+"://"+parsed.Host+tc.requestPath {
				t.Fatalf("request URL=%s; want path %s", got, tc.requestPath)
			}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.modelsPath {
					t.Errorf("models path=%s; want %s", r.URL.Path, tc.modelsPath)
				}
				if preset.Platform == "claude" {
					if r.Header.Get("x-api-key") != "fixture-key" {
						t.Error("missing Anthropic model-discovery credential")
					}
				} else if r.Header.Get("Authorization") != "Bearer fixture-key" {
					t.Error("missing OpenAI model-discovery credential")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"data":[{"id":"fixture-model"}]}`))
			}))
			defer upstream.Close()
			_, err = platform.GetAdapter(preset.Platform).GetModels(context.Background(), upstream.URL+parsed.Path, "fixture-key", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
