package proxy

import (
	"testing"
)

func TestBuildUpstreamURL(t *testing.T) {
	tests := []struct {
		siteURL string
		path    string
		want    string
	}{
		{"https://api.example.com", "/v1/chat/completions", "https://api.example.com/v1/chat/completions"},
		{"https://api.example.com/", "/v1/chat/completions", "https://api.example.com/v1/chat/completions"},
		{"https://api.example.com", "v1/chat/completions", "https://api.example.com/v1/chat/completions"},
		{"https://api.example.com/path", "/v1/messages", "https://api.example.com/path/v1/messages"},
		{"https://api.example.com/path/", "/v1/messages", "https://api.example.com/path/v1/messages"},
		{"https://api.example.com/v1", "/v1/chat/completions", "https://api.example.com/v1/chat/completions"},
		{"https://ark.cn-beijing.volces.com/api/v3", "/v1/chat/completions", "https://ark.cn-beijing.volces.com/api/v3/chat/completions"},
		{"https://api.example.com/api/v3/", "v1/responses", "https://api.example.com/api/v3/responses"},
	}

	for _, tt := range tests {
		got := BuildUpstreamURL(tt.siteURL, tt.path)
		if got != tt.want {
			t.Errorf("BuildUpstreamURL(%q, %q) = %q, want %q", tt.siteURL, tt.path, got, tt.want)
		}
	}
}

func TestResolveEndpointCandidates(t *testing.T) {
	t.Run("chat primary includes multi-protocol order", func(t *testing.T) {
		got := ResolveEndpointCandidates("/v1/chat/completions", false)
		want := []UpstreamEndpoint{EndpointChat, EndpointMessages, EndpointResponses}
		if len(got) != len(want) {
			t.Fatalf("len=%d want %d (%v)", len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v want %v", got, want)
			}
		}
	})
	t.Run("disableCrossProtocolFallback returns primary only", func(t *testing.T) {
		got := ResolveEndpointCandidates("/v1/messages", true)
		if len(got) != 1 || got[0] != EndpointMessages {
			t.Fatalf("got %v, want [messages]", got)
		}
	})
	t.Run("non chat-family returns nil", func(t *testing.T) {
		got := ResolveEndpointCandidates("/v1/embeddings", false)
		if got != nil {
			t.Fatalf("got %v, want nil", got)
		}
	})
	t.Run("responses-only site forces responses only", func(t *testing.T) {
		got := ResolveEndpointCandidatesWithOptions("/v1/chat/completions", EndpointCandidateOptions{
			Preference: SiteProtocolPreference{ResponsesOnly: true, PreferResponses: true},
		})
		if len(got) != 1 || got[0] != EndpointResponses {
			t.Fatalf("got %v, want [responses]", got)
		}
	})
	t.Run("prefer-responses reorders candidates", func(t *testing.T) {
		got := ResolveEndpointCandidatesWithOptions("/v1/chat/completions", EndpointCandidateOptions{
			Preference: SiteProtocolPreference{PreferResponses: true},
		})
		want := []UpstreamEndpoint{EndpointResponses, EndpointChat, EndpointMessages}
		if len(got) != len(want) {
			t.Fatalf("got %v want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("got %v want %v", got, want)
			}
		}
	})
	t.Run("responses-only ignores disableCrossProtocolFallback primary", func(t *testing.T) {
		got := ResolveEndpointCandidatesWithOptions("/v1/chat/completions", EndpointCandidateOptions{
			DisableCrossProtocolFallback: true,
			Preference:                   SiteProtocolPreference{ResponsesOnly: true},
		})
		if len(got) != 1 || got[0] != EndpointResponses {
			t.Fatalf("got %v, want [responses]", got)
		}
	})
}

func TestPathForEndpointAndFromPath(t *testing.T) {
	if PathForEndpoint(EndpointChat) != "/v1/chat/completions" {
		t.Fatalf("PathForEndpoint chat = %q", PathForEndpoint(EndpointChat))
	}
	ep, ok := EndpointFromPath("/v1/responses")
	if !ok || ep != EndpointResponses {
		t.Fatalf("EndpointFromPath responses = %v %v", ep, ok)
	}
}

func TestShouldDowngradeToNextEndpoint(t *testing.T) {
	if !ShouldDowngradeToNextEndpoint(400, "please use /v1/chat/completions") {
		t.Fatal("expected protocol redirect to downgrade")
	}
	if !ShouldDowngradeToNextEndpoint(404, "not found") {
		t.Fatal("expected 404 to downgrade")
	}
	if ShouldDowngradeToNextEndpoint(500, "internal") {
		t.Fatal("generic 500 should not auto-downgrade")
	}
	if ShouldDowngradeToNextEndpoint(0, "first byte timeout") {
		t.Fatal("status 0 is handled by timeout path, not downgrade helper")
	}
}

func BenchmarkBuildUpstreamURL(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = BuildUpstreamURL("https://api.example.com/path", "/v1/chat/completions")
	}
}
