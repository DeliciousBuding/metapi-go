package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestModelDiscovery_FailureIsNotAnEmptyModelList pins the contract every
// model-discovery ladder in this package has to keep: "the upstream answered and
// this account has no models" and "no rung reached the upstream" are two different
// answers. service/account_model_refresh classifies a failure only when err != nil,
// so a ladder that ends in `[]string{}, nil` turns an unreachable site, a rejected
// credential and a site that turned /v1/models off all into the single code
// `empty_models` — which is what #1232 was reported against.
//
// The adapters are taken from the registry rather than built by hand, so this covers
// what a deployment actually runs. sub2api and one-api already kept this contract and
// are the in-repo pattern the others were brought to.
func TestModelDiscovery_FailureIsNotAnEmptyModelList(t *testing.T) {
	cases := []struct {
		platform string
		// answeredEmpty is the payload this platform's own contract uses for "I
		// answered, there are no models". one-hub's /api/available_model fallback
		// reads the keys of `data` as model names, so it needs an object, not a list.
		answeredEmpty string
		answeredOne   string
		wantModel     string
	}{
		{"new-api", `{"success":true,"data":[]}`, `{"success":true,"data":[{"id":"gpt-test"}]}`, "gpt-test"},
		{"one-hub", `{"success":true,"data":{}}`, `{"success":true,"data":[{"id":"gpt-test"}]}`, "gpt-test"},
		{"gemini", `{"models":[],"data":[]}`, `{"models":[{"name":"gemini-test"}]}`, "gemini-test"},
		{"claude", `{"data":[]}`, `{"data":[{"id":"claude-test"}]}`, "claude-test"},
		{"sub2api", `{"data":[]}`, `{"data":[{"id":"gpt-test"}]}`, "gpt-test"},
		{"one-api", `{"success":true,"data":[]}`, `{"success":true,"data":[{"id":"gpt-test"}]}`, "gpt-test"},
	}

	for _, tc := range cases {
		adapter := GetAdapter(tc.platform)
		if adapter == nil {
			t.Fatalf("registry has no adapter for %q", tc.platform)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		t.Run(tc.platform+"/unreachable is a failure", func(t *testing.T) {
			models, err := adapter.GetModels(ctx, unreachableBaseURL(t), "sk-test-token", nil, nil)
			if err == nil {
				t.Errorf("GetModels reported an empty model list for an upstream nobody reached")
			}
			if len(models) != 0 {
				t.Errorf("models = %v, want none", models)
			}
		})

		t.Run(tc.platform+"/answered with no models is not a failure", func(t *testing.T) {
			srv := answeringModelServer(tc.answeredEmpty)
			defer srv.Close()
			models, err := adapter.GetModels(ctx, srv.URL, "sk-test-token", nil, nil)
			if err != nil {
				t.Errorf("an upstream that answered with no models is a true empty answer: %v", err)
			}
			if len(models) != 0 {
				t.Errorf("models = %v, want none", models)
			}
		})

		t.Run(tc.platform+"/answered with a model", func(t *testing.T) {
			srv := answeringModelServer(tc.answeredOne)
			defer srv.Close()
			models, err := adapter.GetModels(ctx, srv.URL, "sk-test-token", nil, nil)
			if err != nil {
				t.Fatalf("GetModels: %v", err)
			}
			found := false
			for _, m := range models {
				if m == tc.wantModel {
					found = true
				}
			}
			if !found {
				t.Errorf("models = %v, want %q in it", models, tc.wantModel)
			}
		})
		cancel()
	}
}

// answeringModelServer serves the same body on every path, which is enough for a
// model-discovery ladder: each rung reads its own shape out of it.
func answeringModelServer(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}
