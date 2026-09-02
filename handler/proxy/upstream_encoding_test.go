package proxyhandler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/platform"
)

// These unit tests pin the two data-plane glue helpers directly; the end-to-end
// behaviour on the production dispatch path is pinned in
// upstream_encoding_honesty_test.go.

func TestStripUpstreamAcceptEncoding(t *testing.T) {
	t.Cleanup(resetUpstreamAcceptEncodingWarnings)
	resetUpstreamAcceptEncodingWarnings()

	cases := []struct {
		name   string
		header map[string][]string
	}{
		{name: "canonical single value", header: map[string][]string{"Accept-Encoding": {"br"}}},
		{name: "lowercase key", header: map[string][]string{"accept-encoding": {"gzip"}}},
		{name: "multi value", header: map[string][]string{"Accept-Encoding": {"gzip", "br"}}},
		{name: "mixed case value", header: map[string][]string{"Accept-Encoding": {"GZIP"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			warns := captureWarnLogs(t)
			req, err := http.NewRequest(http.MethodPost, "https://upstream.example/v1/chat/completions", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			for k, values := range tc.header {
				for _, v := range values {
					req.Header.Add(k, v)
				}
			}

			stripUpstreamAcceptEncoding(req, nil, 3, 42)

			if got := req.Header.Values("Accept-Encoding"); len(got) != 0 {
				t.Fatalf("Accept-Encoding after strip = %v, want it gone: this header decides whether net/http transparently decodes the answer, so it is never site-configurable", got)
			}
			out := warns.String()
			if !strings.Contains(out, "Accept-Encoding") {
				t.Fatalf("expected the stable drop WARN, got:\n%s", out)
			}
			if !strings.Contains(out, "site_id=3") || !strings.Contains(out, "channel_id=42") {
				t.Fatalf("WARN must name the misconfigured site and channel, got:\n%s", out)
			}
		})
	}

	t.Run("absent header is a silent no-op", func(t *testing.T) {
		warns := captureWarnLogs(t)
		req, err := http.NewRequest(http.MethodPost, "https://upstream.example/v1/chat/completions", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		stripUpstreamAcceptEncoding(req, nil, 3, 42)
		if got := warns.String(); got != "" {
			t.Fatalf("unexpected WARN for the healthy default path:\n%s", got)
		}
	})

	t.Run("other site custom headers survive", func(t *testing.T) {
		resetUpstreamAcceptEncodingWarnings()
		req, err := http.NewRequest(http.MethodPost, "https://upstream.example/v1/chat/completions", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Accept-Encoding", "br")
		req.Header.Set("X-Site-Header", "keep-me")
		req.Header.Set("Anthropic-Beta", "fine-grained-tool-streaming-2025-05-14")

		stripUpstreamAcceptEncoding(req, nil, 3, 42)

		if got := req.Header.Get("X-Site-Header"); got != "keep-me" {
			t.Fatalf("X-Site-Header = %q, want %q", got, "keep-me")
		}
		if got := req.Header.Get("Anthropic-Beta"); got != "fine-grained-tool-streaming-2025-05-14" {
			t.Fatalf("Anthropic-Beta = %q, want it untouched", got)
		}
	})

	t.Run("warns once per site/channel/value", func(t *testing.T) {
		resetUpstreamAcceptEncodingWarnings()
		warns := captureWarnLogs(t)
		for i := 0; i < 5; i++ {
			req, err := http.NewRequest(http.MethodPost, "https://upstream.example/v1/chat/completions", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			req.Header.Set("Accept-Encoding", "br")
			stripUpstreamAcceptEncoding(req, nil, 3, 42)
		}
		if got := strings.Count(warns.String(), "site_id=3"); got != 1 {
			t.Fatalf("WARN emitted %d times for one static misconfiguration, want exactly 1:\n%s", got, warns.String())
		}

		// A different channel on the same site is a different fix, so it gets its
		// own single line.
		req, err := http.NewRequest(http.MethodPost, "https://upstream.example/v1/chat/completions", nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Accept-Encoding", "br")
		stripUpstreamAcceptEncoding(req, nil, 3, 43)
		if got := strings.Count(warns.String(), "channel_id=43"); got != 1 {
			t.Fatalf("WARN for channel 43 emitted %d times, want 1:\n%s", got, warns.String())
		}
	})
}

// TestBufferedUpstreamBodyUnreadableContract pins the glue contract the dispatch
// path relies on: unreadable bytes yield the explicit unknown usage source and an
// explicit "no evidence" fact for the single content judge — never invented
// tokens and never noise handed to a keyword scan.
func TestBufferedUpstreamBodyUnreadableContract(t *testing.T) {
	unreadable := bufferedUpstreamBody{bytes: []byte{0x8b, 0x21, 0xff}, readable: false}

	usage := unreadable.parseUsage()
	if usage.Source != usageSourceUnknown || usage.Found {
		t.Fatalf("parseUsage() = %+v, want the explicit unknown source", usage)
	}
	if usage.PromptTokens != 0 || usage.CompletionTokens != 0 || usage.TotalTokens != 0 {
		t.Fatalf("parseUsage() invented tokens: %+v", usage)
	}

	facts := unreadable.judgeFacts(http.StatusOK, usage)
	if !facts.Unreadable {
		t.Fatal("judgeFacts().Unreadable = false, want the judge to be told there is no evidence")
	}
	if facts.RawText != "" {
		t.Fatalf("judgeFacts().RawText = %q, want empty: undecodable bytes must not reach the keyword scan", facts.RawText)
	}

	readable := bufferedUpstreamBody{
		bytes:    []byte(`{"choices":[{"message":{"content":"hi"}}],"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`),
		readable: true,
	}
	if got := readable.parseUsage(); got.Source != usageSourceUpstream || got.TotalTokens != 18 {
		t.Fatalf("readable parseUsage() = %+v, want upstream/18", got)
	}
	if got := readable.judgeFacts(http.StatusOK, readable.parseUsage()); got.Unreadable || got.RawText == "" {
		t.Fatalf("readable judgeFacts() = %+v, want a readable fact set", got)
	}
}

// TestStripUpstreamAcceptEncodingClearsCustomHeadersSource pins the source-level
// half of the strip: platform.DoWithProxy / SiteProxy.Do re-apply
// proxyConfig.CustomHeaders to the request after the data plane built it, so the
// value has to leave the map too — clearing req.Header alone would leak it back
// onto the wire on the site-proxy path.
func TestStripUpstreamAcceptEncodingClearsCustomHeadersSource(t *testing.T) {
	t.Cleanup(resetUpstreamAcceptEncodingWarnings)
	resetUpstreamAcceptEncodingWarnings()
	warns := captureWarnLogs(t)

	proxyConfig := &platform.ProxyConfig{
		CustomHeaders: map[string]string{
			"accept-encoding": "br",
			"X-Site-Header":   "keep-me",
		},
	}
	req, err := http.NewRequest(http.MethodPost, "https://upstream.example/v1/chat/completions", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	platform.ApplyCustomHeadersWithOptions(req, proxyConfig.CustomHeaders, platform.ApplyCustomHeadersOptions{})

	stripUpstreamAcceptEncoding(req, proxyConfig, 3, 42)

	if got := req.Header.Values("Accept-Encoding"); len(got) != 0 {
		t.Fatalf("request Accept-Encoding = %v, want it gone", got)
	}
	for name := range proxyConfig.CustomHeaders {
		if strings.EqualFold(strings.TrimSpace(name), "accept-encoding") {
			t.Fatalf("custom_headers still carries %q — platform would re-apply it on the site-proxy path", name)
		}
	}
	if got := proxyConfig.CustomHeaders["X-Site-Header"]; got != "keep-me" {
		t.Fatalf("custom_headers X-Site-Header = %q, want %q (the strip must not collateral-damage other site headers)", got, "keep-me")
	}
	if got := req.Header.Get("X-Site-Header"); got != "keep-me" {
		t.Fatalf("request X-Site-Header = %q, want %q", got, "keep-me")
	}
	if !strings.Contains(warns.String(), "site_id=3") {
		t.Fatalf("expected one drop WARN naming the site, got:\n%s", warns.String())
	}

	// Idempotent: a second attempt for the same dispatch shape finds nothing to
	// strip and stays quiet.
	before := warns.String()
	stripUpstreamAcceptEncoding(req, proxyConfig, 3, 42)
	if warns.String() != before {
		t.Fatalf("second strip logged again:\n%s", warns.String())
	}
}
