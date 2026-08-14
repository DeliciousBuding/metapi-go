package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchJSON_SendsDefaultBrowserUserAgent(t *testing.T) {
	var gotUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	if _, err := fetchJSON(context.Background(), server.URL, "GET", nil, nil, nil); err != nil {
		t.Fatalf("fetchJSON error: %v", err)
	}
	if gotUA != DefaultBrowserUserAgent {
		t.Errorf("User-Agent = %q, want %q", gotUA, DefaultBrowserUserAgent)
	}
}

func TestFetchJSON_DoesNotOverrideExplicitUserAgent(t *testing.T) {
	var gotUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	custom := "custom-client/2.0"
	if _, err := fetchJSON(context.Background(), server.URL, "GET", nil, map[string]string{"User-Agent": custom}, nil); err != nil {
		t.Fatalf("fetchJSON error: %v", err)
	}
	if gotUA != custom {
		t.Errorf("User-Agent = %q, want explicit %q", gotUA, custom)
	}
}

func TestApplySiteIdentity_InjectsClearanceCookie(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	ApplySiteIdentity(req, &ProxyConfig{
		ClearanceCookie: "abc123",
		BrowserUA:       "custom-ua/1.0",
	})

	if cookie := req.Header.Get("Cookie"); cookie != "cf_clearance=abc123" {
		t.Errorf("Cookie = %q, want cf_clearance=abc123", cookie)
	}
	if ua := req.Header.Get("User-Agent"); ua != "custom-ua/1.0" {
		t.Errorf("User-Agent = %q, want custom-ua/1.0", ua)
	}
}

func TestApplySiteIdentity_PreservesExistingCookie(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	req.Header.Set("Cookie", "session=existing")
	ApplySiteIdentity(req, &ProxyConfig{ClearanceCookie: "abc123"})

	cookie := req.Header.Get("Cookie")
	if !strings.Contains(cookie, "session=existing") || !strings.Contains(cookie, "cf_clearance=abc123") {
		t.Errorf("Cookie = %q, should contain both session and cf_clearance", cookie)
	}
}

func TestApplySiteIdentity_NilConfig(t *testing.T) {
	req, _ := http.NewRequest("GET", "https://example.com", nil)
	ApplySiteIdentity(req, nil)
	if req.Header.Get("Cookie") != "" {
		t.Errorf("Cookie should be empty for nil config, got %q", req.Header.Get("Cookie"))
	}
}
