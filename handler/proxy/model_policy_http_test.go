package proxyhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestDownstreamModelPolicyHTTP(t *testing.T) {
	db := sqlitetest.Open(t)
	previousDB, previousRuntime, previousUpstream := store.GetDB(), config.RuntimeSafe(), getUpstreamConfig()
	store.OverrideDB(db)
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() {
		store.OverrideDB(previousDB)
		config.SetRuntime(previousRuntime)
		SetUpstreamConfig(previousUpstream)
		routing.SetGlobalCache(nil)
	})
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"fixture","model":"policy-model","choices":[{"index":0,"message":{"role":"assistant","content":"receipt"},"finish_reason":"stop"}]}`))
	}))
	t.Cleanup(upstream.Close)
	seedProbeChannel(t, db, upstream.URL, "policy-model", "fixture-upstream")
	seedProbeChannel(t, db, upstream.URL+"/hidden", "hidden-model", "fixture-upstream")
	var routeID int64
	if err := db.Get(&routeID, `SELECT id FROM token_routes WHERE model_pattern='policy-model'`); err != nil {
		t.Fatal(err)
	}
	router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil)
	SetUpstreamConfig(&UpstreamConfig{Router: router, LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil }})
	r := chi.NewRouter()
	r.Use(auth.ProxyAuth())
	r.Route("/v1", RegisterProxyRoutes)
	for n, tc := range []struct {
		name, patterns, routes, access string
		allowed                        bool
	}{
		{"exact", `["policy-model"]`, `[]`, `null`, true},
		{"interior glob", `["po*model"]`, `[]`, `null`, true},
		{"single rune glob", `["policy-?odel"]`, `[]`, `null`, true},
		{"regex", `["re:^policy-model$"]`, `[]`, `null`, true},
		{"route grant", `["unrelated-*"]`, fmt.Sprintf(`[%d]`, routeID), `null`, true},
		{"route only", `[]`, fmt.Sprintf(`[%d]`, routeID), `null`, true},
		{"unmatched model", `["unrelated-*"]`, `[]`, `null`, false},
		{"invalid regex", `["re:["]`, `[]`, `null`, false},
		{"unknown route", `["unrelated-*"]`, `[99999]`, `null`, false},
		{"empty grants", `[]`, `[]`, `null`, false},
		{"access restriction", `["policy-*"]`, fmt.Sprintf(`[%d]`, routeID), `{"modelIds":["hidden-model"]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			key := fmt.Sprintf("sk-model-policy-%d", n)
			videoExec(t, db, `INSERT INTO downstream_api_keys (name,key,enabled,supported_models,allowed_route_ids,access_policy) VALUES (?,?,TRUE,?,?,?)`, key, key, tc.patterns, tc.routes, tc.access)
			request := func(method, path, body string) *httptest.ResponseRecorder {
				req := httptest.NewRequest(method, path, strings.NewReader(body))
				req.Header.Set("Authorization", "Bearer "+key)
				req.Header.Set("Content-Type", "application/json")
				out := httptest.NewRecorder()
				r.ServeHTTP(out, req)
				return out
			}
			before := calls.Load()
			listed := request("GET", "/v1/models", "")
			if calls.Load() != before {
				t.Fatal("model listing dispatched upstream")
			}
			var catalog struct {
				Data []struct{ ID string }
			}
			if err := json.Unmarshal(listed.Body.Bytes(), &catalog); err != nil || listed.Code != 200 {
				t.Fatalf("model list: status=%d err=%v body=%s", listed.Code, err, listed.Body.String())
			}
			if tc.allowed {
				if len(catalog.Data) != 1 || catalog.Data[0].ID != "policy-model" {
					t.Errorf("allowed model missing or unrelated model listed: %+v", catalog.Data)
				}
			} else if len(catalog.Data) != 0 {
				t.Errorf("denied policy listed models: %+v", catalog.Data)
			}
			out := request("POST", "/v1/chat/completions", `{"model":"policy-model","messages":[{"role":"user","content":"hello"}]}`)
			if tc.allowed {
				if out.Code != 200 || calls.Load()-before != 1 {
					t.Fatalf("allowed request: status=%d calls=%d body=%s", out.Code, calls.Load()-before, out.Body.String())
				}
			} else if out.Code < 400 || calls.Load() != before {
				t.Fatalf("denied request: status=%d calls=%d", out.Code, calls.Load()-before)
			}
			before = calls.Load()
			denied := request("POST", "/v1/chat/completions", `{"model":"hidden-model","messages":[{"role":"user","content":"hello"}]}`)
			if denied.Code < 400 || calls.Load() != before {
				t.Fatalf("ungranted model dispatched: status=%d calls=%d", denied.Code, calls.Load()-before)
			}
		})
	}
}
