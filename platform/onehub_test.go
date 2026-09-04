package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOneHubAdapter_Detect(t *testing.T) {
	o := &OneHubAdapter{
		OneApiAdapter: &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-hub")},
	}

	ctx := context.Background()

	tests := []struct {
		url     string
		matches bool
	}{
		{"https://onehub.example.com", true},
		{"https://ONEHUB.example.com", true},
		{"https://one-hub.example.com", true},
		{"https://ONE-HUB.example.com", true},
		{"https://oneapi.example.com", false},
		{"https://newapi.example.com", false},
	}
	for _, tt := range tests {
		ok, err := o.Detect(ctx, tt.url)
		if err != nil {
			t.Errorf("Detect(%q) error: %v", tt.url, err)
			continue
		}
		if ok != tt.matches {
			t.Errorf("Detect(%q) = %v, want %v", tt.url, ok, tt.matches)
		}
	}
}

// Both directions: the /api/available_model fallback still has to work, and
// "neither ladder rung reached the upstream" is a failure rather than an empty
// model list. The second half is what this test used to assert away.
func TestOneHubAdapter_GetModels_Fallback(t *testing.T) {
	o := &OneHubAdapter{
		OneApiAdapter: &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-hub")},
	}
	ctx := context.Background()

	t.Run("available_model fallback still yields models", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Path == "/api/available_model" {
				_, _ = w.Write([]byte(`{"success":true,"data":{"gpt-fallback":{}}}`))
				return
			}
			http.NotFound(w, r)
		}))
		defer srv.Close()

		models, err := o.GetModels(ctx, srv.URL, "token", nil, nil)
		if err != nil {
			t.Fatalf("fallback path: %v", err)
		}
		if len(models) != 1 || models[0] != "gpt-fallback" {
			t.Fatalf("models = %v, want [gpt-fallback]", models)
		}
	})

	t.Run("both rungs unreachable is a failure", func(t *testing.T) {
		models, err := o.GetModels(ctx, unreachableBaseURL(t), "token", nil, nil)
		if err == nil {
			t.Error("GetModels must report an unreachable upstream, not an empty model list")
		}
		if len(models) != 0 {
			t.Error("GetModels on unreachable should return no models")
		}
	})
}

func TestOneHubAdapter_GetUserGroups_Fallback(t *testing.T) {
	o := &OneHubAdapter{
		OneApiAdapter: &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-hub")},
	}
	ctx := context.Background()

	// On unreachable URL, may error or fall back to ["default"]
	_, err := o.GetUserGroups(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err != nil {
		t.Logf("GetUserGroups error on unreachable (expected): %v", err)
	}
}
