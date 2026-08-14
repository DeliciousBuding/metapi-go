package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
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
