package platform

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewApiAdapter_GetModels_FallsBackToUserModelsWhenV1Unavailable(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		response string
	}{
		{name: "404", status: http.StatusNotFound, response: `{"success":false,"message":"not found"}`},
		{name: "405", status: http.StatusMethodNotAllowed, response: `{"success":false,"message":"method not allowed"}`},
		{name: "200 explicit business failure", status: http.StatusOK, response: `{"success":false,"message":"models endpoint disabled"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var userModelCalls int
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/models":
					w.WriteHeader(tc.status)
					_, _ = fmt.Fprint(w, tc.response)
				case "/api/user/self":
					_, _ = fmt.Fprint(w, `{"success":true,"data":{"id":42,"username":"operator"}}`)
				case "/api/user/models":
					userModelCalls++
					if got := r.Header.Get("Authorization"); got != "Bearer dashboard-pat" {
						t.Errorf("/api/user/models Authorization = %q, want dashboard PAT", got)
						w.WriteHeader(http.StatusUnauthorized)
						_, _ = fmt.Fprint(w, `{"success":false,"message":"unauthorized"}`)
						return
					}
					_, _ = fmt.Fprint(w, `{"success":true,"data":["gpt-4o-mini","claude-3-5-sonnet"]}`)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(srv.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			models, err := newApiAdapterUnderTest().GetModels(ctx, srv.URL, "dashboard-pat", nil, nil)
			if err != nil {
				t.Fatalf("GetModels with a valid dashboard credential: %v", err)
			}
			if len(models) != 2 || models[0] != "claude-3-5-sonnet" || models[1] != "gpt-4o-mini" {
				t.Fatalf("models = %v, want the dashboard model list", models)
			}
			if userModelCalls != 1 {
				t.Fatalf("/api/user/models calls = %d, want 1", userModelCalls)
			}
		})
	}
}

func TestNewApiAdapter_GetModels_APIKeyOnlyV1FailureIsNotEmptySuccess(t *testing.T) {
	const apiKey = "sk-test-relay"
	cases := []struct {
		name     string
		status   int
		response string
	}{
		{name: "404", status: http.StatusNotFound, response: `{"success":false,"message":"not found"}`},
		{name: "405", status: http.StatusMethodNotAllowed, response: `{"success":false,"message":"method not allowed"}`},
		{name: "200 explicit business failure", status: http.StatusOK, response: `{"success":false,"message":"models endpoint disabled"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.URL.Path == "/v1/models" {
					w.WriteHeader(tc.status)
					_, _ = fmt.Fprint(w, tc.response)
					return
				}
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = fmt.Fprintf(w, `{"success":false,"message":"token %s rejected"}`, apiKey)
			}))
			t.Cleanup(srv.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			models, err := newApiAdapterUnderTest().GetModels(ctx, srv.URL, apiKey, nil, nil)
			if err == nil {
				t.Fatalf("API-key-only discovery returned success with models=%v; /v1/models failed and /api/user/models cannot authenticate a relay key", models)
			}
			if len(models) != 0 {
				t.Fatalf("models = %v, want none on failure", models)
			}
			if strings.Contains(err.Error(), apiKey) || strings.Contains(err.Error(), "LEAKME") {
				t.Fatalf("model discovery error leaked the credential: %v", err)
			}
		})
	}
}

func TestNewApiAdapter_GetModels_UserModelEmptyAndFailureAreDistinct(t *testing.T) {
	cases := []struct {
		name     string
		response string
		wantErr  bool
		secret   string
	}{
		{name: "answered empty", response: `{"success":true,"data":[]}`},
		{name: "explicit business failure", response: `{"success":false,"message":"credential expired sk-LEAKME-abcdef"}`, wantErr: true, secret: "sk-LEAKME-abcdef"},
		{name: "malformed success envelope", response: `{"success":true}`, wantErr: true},
		{name: "malformed model item", response: `{"success":true,"data":[{"id":"gpt-4o-mini"}]}`, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/v1/models":
					w.WriteHeader(http.StatusNotFound)
					_, _ = fmt.Fprint(w, `{"success":false,"message":"disabled"}`)
				case "/api/user/self":
					_, _ = fmt.Fprint(w, `{"success":true,"data":{"id":42}}`)
				case "/api/user/models":
					_, _ = fmt.Fprint(w, tc.response)
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(srv.Close)

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			models, err := newApiAdapterUnderTest().GetModels(ctx, srv.URL, "dashboard-pat", nil, nil)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("GetModels returned success with models=%v; an unsuccessful /api/user/models response is not an empty account", models)
				}
				if tc.secret != "" && strings.Contains(err.Error(), tc.secret) {
					t.Fatalf("model discovery error leaked the credential: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetModels on a valid empty response: %v", err)
			}
			if len(models) != 0 {
				t.Fatalf("models = %v, want none", models)
			}
		})
	}
}

func TestNewApiAdapter_GetModels_UserModelsDoesNotRequireUserIDDiscovery(t *testing.T) {
	var authenticatedUserModelCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			w.WriteHeader(http.StatusNotFound)
			_, _ = fmt.Fprint(w, `{"success":false,"message":"disabled"}`)
		case "/api/user/models":
			if r.Header.Get("Authorization") == "Bearer dashboard-pat" && r.Header.Get("Cookie") == "" {
				authenticatedUserModelCalls++
				_, _ = fmt.Fprint(w, `{"success":true,"data":["gpt-4o-mini"]}`)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"success":false,"message":"unauthorized"}`)
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"success":false,"message":"unauthorized"}`)
		}
	}))
	t.Cleanup(srv.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	models, err := newApiAdapterUnderTest().GetModels(ctx, srv.URL, "dashboard-pat", nil, nil)
	if err != nil {
		t.Fatalf("GetModels with a valid dashboard credential whose user-info probe is unavailable: %v", err)
	}
	if len(models) != 1 || models[0] != "gpt-4o-mini" {
		t.Fatalf("models = %v, want [gpt-4o-mini]", models)
	}
	if authenticatedUserModelCalls != 1 {
		t.Fatalf("authenticated /api/user/models calls = %d, want 1", authenticatedUserModelCalls)
	}
}
