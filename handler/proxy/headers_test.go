package proxyhandler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deliciousbuding/metapi-go/platform"
)

func clientHeaders() http.Header {
	h := http.Header{}
	h.Set("anthropic-version", "2023-06-01")
	h.Add("anthropic-beta", "prompt-caching-2024-07-31")
	h.Add("anthropic-beta", "fine-grained-tool-streaming-2025-05-14")
	h.Set("openai-beta", "responses=experimental")
	h.Set("user-agent", "claude-cli/1.0.60 (external, cli)")
	h.Set("x-stainless-lang", "js")
	h.Set("x-stainless-package-version", "0.69.0")
	// Must never reach the upstream: the downstream credential and proxy hops.
	h.Set("authorization", "Bearer sk-downstream-secret")
	h.Set("x-api-key", "sk-downstream-secret")
	h.Set("cookie", "session=downstream")
	h.Set("x-forwarded-for", "203.0.113.9")
	h.Set("accept-encoding", "gzip")
	return h
}

func TestApplyClientProtocolHeaders_ForwardsWhitelistOnly(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/messages", nil)
	req.Header = http.Header{}

	applyClientProtocolHeaders(req, clientHeaders(), "/v1/messages")

	if got := req.Header.Get("anthropic-version"); got != "2023-06-01" {
		t.Errorf("anthropic-version = %q, want the client value", got)
	}
	if got := req.Header.Values("anthropic-beta"); len(got) != 2 {
		t.Errorf("anthropic-beta = %v, want both client values preserved", got)
	}
	if got := req.Header.Get("openai-beta"); got != "responses=experimental" {
		t.Errorf("openai-beta = %q, want the client value", got)
	}
	if got := req.Header.Get("user-agent"); got != "claude-cli/1.0.60 (external, cli)" {
		t.Errorf("user-agent = %q, want the client value", got)
	}
	if got := req.Header.Get("x-stainless-lang"); got != "js" {
		t.Errorf("x-stainless-lang = %q, want SDK telemetry forwarded", got)
	}
	if got := req.Header.Get("x-stainless-package-version"); got != "0.69.0" {
		t.Errorf("x-stainless-package-version = %q, want SDK telemetry forwarded", got)
	}

	for _, forbidden := range []string{"authorization", "x-api-key", "cookie", "x-forwarded-for", "accept-encoding"} {
		if got := req.Header.Get(forbidden); got != "" {
			t.Errorf("%s must not be forwarded upstream, got %q", forbidden, got)
		}
	}
}

// Site custom_headers (and the anti-bot identity they carry) are applied before
// the client passthrough, so a site-level User-Agent must win.
func TestApplyClientProtocolHeaders_IsFillOnly(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/messages", nil)
	req.Header = http.Header{}
	req.Header.Set("user-agent", "Mozilla/5.0 (site anti-bot identity)")

	applyClientProtocolHeaders(req, clientHeaders(), "/v1/messages")

	if got := req.Header.Get("user-agent"); got != "Mozilla/5.0 (site anti-bot identity)" {
		t.Errorf("site identity was overridden by the client header: %q", got)
	}
	if got := req.Header.Get("anthropic-version"); got != "2023-06-01" {
		t.Errorf("unset headers must still be filled, got %q", got)
	}
}

func TestApplyClientProtocolHeaders_DefaultsAnthropicVersionForMessagesOnly(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/messages", nil)
	req.Header = http.Header{}
	applyClientProtocolHeaders(req, http.Header{"accept": []string{"application/json"}}, "/v1/messages")
	if got := req.Header.Get("anthropic-version"); got != platform.ClaudeDefaultAnthropicVersion {
		t.Errorf("anthropic-version = %q, want the platform default %q", got, platform.ClaudeDefaultAnthropicVersion)
	}

	chatReq := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/chat/completions", nil)
	chatReq.Header = http.Header{}
	applyClientProtocolHeaders(chatReq, http.Header{"accept": []string{"application/json"}}, "/v1/chat/completions")
	if got := chatReq.Header.Get("anthropic-version"); got != "" {
		t.Errorf("chat completions must not gain an anthropic-version header, got %q", got)
	}

	// A client-supplied value wins over the default.
	explicit := httptest.NewRequest(http.MethodPost, "https://upstream.example/v1/messages", nil)
	explicit.Header = http.Header{}
	client := http.Header{}
	client.Set("anthropic-version", "2030-01-01")
	applyClientProtocolHeaders(explicit, client, "/anthropic/v1/messages")
	if got := explicit.Header.Get("anthropic-version"); got != "2030-01-01" {
		t.Errorf("anthropic-version = %q, want the explicit client value", got)
	}
}

func TestRelayUpstreamResponseHeaders_KeepsContentSemanticsDropsIdentity(t *testing.T) {
	upstream := http.Header{}
	upstream.Set("Content-Type", "application/json")
	upstream.Set("Retry-After", "7")
	upstream.Set("Content-Disposition", `attachment; filename="report.json"`)
	upstream.Set("Content-Length", "1234")
	upstream.Set("Set-Cookie", "upstream_session=abc; Path=/")
	upstream.Set("X-Request-Id", "upstream-request-id")
	upstream.Set("X-New-Api-Version", "v1.2.3")
	upstream.Set("Server", "nginx/1.27.0")
	upstream.Set("Cf-Ray", "8f0a-CDG")
	upstream.Set("X-Powered-By", "Express")
	upstream.Set("X-Ratelimit-Limit", "5000")

	recorder := httptest.NewRecorder()
	recorder.Header().Set("X-Request-Id", "metapi-request-id")

	relayUpstreamResponseHeaders(recorder, upstream)

	if got := recorder.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want it relayed", got)
	}
	if got := recorder.Header().Get("Retry-After"); got != "7" {
		t.Errorf("Retry-After = %q, want it relayed", got)
	}
	if got := recorder.Header().Get("Content-Disposition"); got == "" {
		t.Error("Content-Disposition must be relayed for file downloads")
	}
	if got := recorder.Header().Get("X-Request-Id"); got != "metapi-request-id" {
		t.Errorf("X-Request-Id = %q, want ours to stay authoritative", got)
	}
	for _, dropped := range []string{
		"Set-Cookie", "X-New-Api-Version", "Server", "Cf-Ray", "X-Powered-By",
		"Content-Length", "X-Ratelimit-Limit",
	} {
		if got := recorder.Header().Get(dropped); got != "" {
			t.Errorf("%s must not be relayed from the upstream, got %q", dropped, got)
		}
	}
}

func TestRelayBufferedUpstreamResponse_DoesNotLeakUpstreamIdentity(t *testing.T) {
	upstream := http.Header{}
	upstream.Set("Content-Type", "application/json")
	upstream.Set("X-Sub2Api-Version", "9.9.9")
	upstream.Set("Set-Cookie", "leak=1")

	recorder := httptest.NewRecorder()
	recorder.Header().Set("X-Request-Id", "metapi-request-id")
	relayBufferedUpstreamResponse(recorder, &http.Response{StatusCode: 200, Header: upstream}, []byte(`{"ok":true}`))

	if got := recorder.Header().Get("X-Sub2Api-Version"); got != "" {
		t.Errorf("upstream fingerprint header leaked: %q", got)
	}
	if got := recorder.Header().Get("Set-Cookie"); got != "" {
		t.Errorf("upstream cookie leaked: %q", got)
	}
	if recorder.Code != 200 || recorder.Body.String() != `{"ok":true}` {
		t.Errorf("status/body not relayed: %d %q", recorder.Code, recorder.Body.String())
	}
}
