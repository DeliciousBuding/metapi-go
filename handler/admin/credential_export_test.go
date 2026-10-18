package admin

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/trustedproxy"
)

func TestBuildCredentialExportProfiles_Content(t *testing.T) {
	ps := buildCredentialExportProfiles("n", "sk-abc", "https://gw.test")
	if len(ps) != 6 {
		t.Fatalf("len=%d", len(ps))
	}
	if ps[0]["id"] != "openai" {
		t.Fatalf("%v", ps[0])
	}
	content, _ := ps[0]["content"].(string)
	if !strings.Contains(content, "OPENAI_BASE_URL") || !strings.Contains(content, "sk-abc") {
		t.Fatalf("content=%q", content)
	}
	cherry, _ := ps[1]["content"].(map[string]any)
	if cherry["apiKey"] != "sk-abc" {
		t.Fatalf("cherry=%v", cherry)
	}
	claude, _ := ps[3]["content"].(string)
	if ps[3]["id"] != "claude-code" ||
		!strings.Contains(claude, "ANTHROPIC_BASE_URL") ||
		!strings.Contains(claude, "ANTHROPIC_AUTH_TOKEN") ||
		!strings.Contains(claude, "sk-abc") ||
		!strings.Contains(claude, "https://gw.test/v1") {
		t.Fatalf("claude=%q", claude)
	}
	codex, _ := ps[4]["content"].(string)
	if ps[4]["id"] != "codex" ||
		!strings.Contains(codex, `model_provider = "metapi"`) ||
		!strings.Contains(codex, "[model_providers.metapi]") ||
		!strings.Contains(codex, "base_url") ||
		!strings.Contains(codex, "sk-abc") {
		t.Fatalf("codex=%q", codex)
	}
	webui, _ := ps[5]["content"].(map[string]any)
	if ps[5]["id"] != "openwebui" ||
		webui["baseUrl"] != "https://gw.test/v1" ||
		webui["apiKey"] != "sk-abc" {
		t.Fatalf("openwebui=%v", webui)
	}
}

func TestResolveExportBaseURL(t *testing.T) {
	rt := &config.RuntimeSettings{}
	config.SetRuntime(rt)
	t.Cleanup(func() { config.SetRuntime(nil) })

	newRequest := func() *http.Request {
		req := httptest.NewRequest(http.MethodGet, "http://internal.metapi:4000/api/downstream-keys/1/export", nil)
		return req
	}

	t.Run("direct client host wins and forwarding headers are ignored", func(t *testing.T) {
		req := newRequest()
		req.Host = "api.example.com"
		req.Header.Set("X-Forwarded-Host", "evil.example")
		req.Header.Set("X-Forwarded-Proto", "https")
		if got := resolveExportBaseURL(req); got != "http://api.example.com" {
			t.Fatalf("baseURL = %q, want the request Host with direct-client scheme", got)
		}
	})

	t.Run("direct TLS uses https", func(t *testing.T) {
		req := newRequest()
		req.Host = "api.example.com"
		req.TLS = &tls.ConnectionState{}
		if got := resolveExportBaseURL(req); got != "https://api.example.com" {
			t.Fatalf("baseURL = %q", got)
		}
	})

	t.Run("trusted proxy proto is honored but Host still wins over X-Forwarded-Host", func(t *testing.T) {
		req := newRequest()
		req.Host = "api.example.com"
		req.Header.Set("X-Forwarded-Host", "evil.example")
		req.Header.Set("X-Forwarded-Proto", "https")
		req = req.WithContext(trustedproxy.WithTrust(req.Context()))
		if got := resolveExportBaseURL(req); got != "https://api.example.com" {
			t.Fatalf("baseURL = %q, want Host with trusted https", got)
		}
	})

	t.Run("trusted proxy may supply the host when Host was rewritten empty", func(t *testing.T) {
		req := newRequest()
		req.Host = ""
		req.Header.Set("X-Forwarded-Host", "gw.example.com")
		req.Header.Set("X-Forwarded-Proto", "https")
		req = req.WithContext(trustedproxy.WithTrust(req.Context()))
		if got := resolveExportBaseURL(req); got != "https://gw.example.com" {
			t.Fatalf("baseURL = %q, want trusted X-Forwarded-Host", got)
		}
	})

	t.Run("untrusted X-Forwarded-Host is ignored", func(t *testing.T) {
		req := newRequest()
		req.Host = ""
		req.Header.Set("X-Forwarded-Host", "evil.example")
		if got := resolveExportBaseURL(req); got != "http://localhost:4000" {
			t.Fatalf("baseURL = %q, want the localhost fallback", got)
		}
	})

	t.Run("malformed Host cannot smuggle a path", func(t *testing.T) {
		req := newRequest()
		req.Host = "evil.example/path"
		if got := resolveExportBaseURL(req); got != "http://localhost:4000" {
			t.Fatalf("baseURL = %q, want fallback for malformed Host", got)
		}
	})

	t.Run("SERVER_ADDRESS overrides request-derived values", func(t *testing.T) {
		config.SetRuntime(&config.RuntimeSettings{ServerAddress: "https://public.example/"})
		req := newRequest()
		req.Host = "api.example.com"
		if got := resolveExportBaseURL(req); got != "https://public.example" {
			t.Fatalf("baseURL = %q, want SERVER_ADDRESS", got)
		}
	})
}
