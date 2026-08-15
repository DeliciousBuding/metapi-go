package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// statusFixtureServer serves a canned GET /api/status response body.
func statusFixtureServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func detectContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// TestOneApiNewApiDetect_DiscriminatesBySystemNameValue covers the
// value-based discriminator that keeps one-api and new-api apart now that both
// ship system_name in /api/status.
func TestOneApiNewApiDetect_DiscriminatesBySystemNameValue(t *testing.T) {
	tests := []struct {
		name        string
		statusBody  string
		wantNewApi  bool
		wantOneApi  bool
	}{
		{
			name: "one-api v0.6.10 shape (system_name One API)",
			statusBody: `{
				"success": true,
				"message": "",
				"data": {
					"system_name": "One API",
					"version": "v0.6.10",
					"start_time": 1710000000,
					"email_verification": false
				}
			}`,
			wantNewApi: false,
			wantOneApi: true,
		},
		{
			name: "legacy one-api v0.5 shape (no system_name)",
			statusBody: `{
				"success": true,
				"message": "",
				"data": {
					"version": "v0.5.11",
					"start_time": 1710000000
				}
			}`,
			wantNewApi: false,
			wantOneApi: true,
		},
		{
			name: "new-api v1 shape (system_name New API)",
			statusBody: `{
				"success": true,
				"message": "",
				"data": {
					"system_name": "New API",
					"version": "v1.0.0-rc.24",
					"logo": "/logo.png"
				}
			}`,
			wantNewApi: true,
			wantOneApi: false,
		},
		{
			name: "unknown /api/status (no system_name, no version)",
			statusBody: `{
				"success": true,
				"message": "",
				"data": {"uptime": 3600}
			}`,
			wantNewApi: false,
			wantOneApi: false,
		},
		{
			name: "renamed SYSTEM_NAME degrades to no match",
			statusBody: `{
				"success": true,
				"message": "",
				"data": {
					"system_name": "My Gateway",
					"version": "v0.6.10"
				}
			}`,
			wantNewApi: false,
			wantOneApi: false,
		},
		{
			name: "one-api name without space matches",
			statusBody: `{
				"success": true,
				"message": "",
				"data": {
					"system_name": "oneapi-community",
					"version": "v0.6.10"
				}
			}`,
			wantNewApi: false,
			wantOneApi: true,
		},
		{
			name: "new-api name without space matches",
			statusBody: `{
				"success": true,
				"message": "",
				"data": {
					"system_name": "MyNewApi",
					"version": "v1.0.0"
				}
			}`,
			wantNewApi: true,
			wantOneApi: false,
		},
		{
			name: "case-insensitive matching",
			statusBody: `{
				"success": true,
				"message": "",
				"data": {
					"system_name": "ONE API",
					"version": "v0.6.10"
				}
			}`,
			wantNewApi: false,
			wantOneApi: true,
		},
		{
			name: "success false never matches",
			statusBody: `{
				"success": false,
				"message": "no",
				"data": {
					"system_name": "One API",
					"version": "v0.6.10"
				}
			}`,
			wantNewApi: false,
			wantOneApi: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := statusFixtureServer(t, tt.statusBody)
			ctx := detectContext(t)

			newApi := &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}
			newOK, err := newApi.Detect(ctx, srv.URL)
			if err != nil {
				t.Fatalf("NewApiAdapter.Detect error: %v", err)
			}
			if newOK != tt.wantNewApi {
				t.Errorf("NewApiAdapter.Detect = %v, want %v", newOK, tt.wantNewApi)
			}

			oneApi := &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-api")}
			oneOK, err := oneApi.Detect(ctx, srv.URL)
			if err != nil {
				t.Fatalf("OneApiAdapter.Detect error: %v", err)
			}
			if oneOK != tt.wantOneApi {
				t.Errorf("OneApiAdapter.Detect = %v, want %v", oneOK, tt.wantOneApi)
			}
		})
	}
}

// TestNewApiAdapter_Detect_URLKeywordAliasesUnchanged keeps the pre-existing
// URL-keyword short-circuit for NewAPI fork aliases under test.
func TestNewApiAdapter_Detect_URLKeywordAliasesUnchanged(t *testing.T) {
	n := &NewApiAdapter{BaseAdapter: NewBaseAdapter("new-api")}
	ctx := detectContext(t)

	for _, keyword := range []string{"vo-api", "super-api", "rix-api", "neo-api"} {
		url := "https://example.com/" + keyword + "/dashboard"
		ok, err := n.Detect(ctx, url)
		if err != nil {
			t.Fatalf("Detect(%q) error: %v", url, err)
		}
		if !ok {
			t.Errorf("Detect(%q) = false, want true via URL keyword", url)
		}
	}
}
