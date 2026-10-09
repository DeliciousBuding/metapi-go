package proxyhandler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// Exercise persisted site/account/route selection, rather than supplying a
// preselected channel: native account credentials must survive the full relay.
func installSiteNativeFixture(t *testing.T, provider, baseURL, actualModel, customHeaders string, override bool) *store.DB {
	t.Helper()
	db := openProbeTestDB(t)
	seedProbeChannel(t, db, baseURL, actualModel, "fixture-site-key")
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`UPDATE sites SET platform=?, custom_headers=?, custom_headers_override_request_headers=?`, []any{provider, customHeaders, override}},
		{`UPDATE accounts SET access_token='', extra_config='{"credentialMode":"apikey"}'`, nil},
		{`UPDATE token_routes SET display_name='client-alias'`, nil},
		{`INSERT INTO model_availability (account_id, model_name, available) SELECT id, ?, ? FROM accounts`, []any{actualModel, true}},
	} {
		if _, err := db.Exec(db.Rebind(statement.query), statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	previousRuntime := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() { config.SetRuntime(previousRuntime) })
	previous := getUpstreamConfig()
	SetUpstreamConfig(&UpstreamConfig{
		Router:   routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil),
		LogProxy: func(context.Context, proxy.ProxyLogEntry) error { return nil },
	})
	t.Cleanup(func() { SetUpstreamConfig(previous) })
	return db
}

func siteNativeHTTPResponse(t *testing.T, handler http.HandlerFunc, path, body string) (int, string) {
	t.Helper()
	downstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(auth.WithProxyAuth(r.Context(), &auth.ProxyAuthContext{
			Token: "fixture-downstream-key", Source: "global", Policy: auth.EmptyDownstreamRoutingPolicy,
		}))
		handler(w, r)
	}))
	defer downstream.Close()
	req, err := http.NewRequest(http.MethodPost, downstream.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for _, header := range []string{"Authorization", "x-api-key", "x-goog-api-key"} {
		req.Header.Set(header, "downstream-must-not-leak")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	output, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(output)
}

func TestSiteNativeProtocolHTTPRelay(t *testing.T) {
	for _, fixture := range directWireFixtures {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", fixture.provider, stream), func(t *testing.T) {
				provider, header, credential := "openai", "Authorization", "Bearer fixture-site-key"
				switch fixture.provider {
				case "anthropic":
					provider, header, credential = "claude", "x-api-key", "fixture-site-key"
				case "gemini":
					provider, header, credential = "gemini", "x-goog-api-key", "fixture-site-key"
				}
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Header.Get(header) != credential {
						t.Errorf("%s authentication header %s does not contain the selected credential", provider, header)
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					for _, other := range []string{"Authorization", "x-api-key", "x-goog-api-key"} {
						if other != header && r.Header.Get(other) != "" {
							t.Errorf("foreign credential header leaked: %s", other)
						}
					}
					if r.Header.Get("X-Fixture-Option") != "preserved" {
						t.Error("ordinary custom header was lost")
					}
					var request map[string]any
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
					}
					if provider == "gemini" {
						action := "generateContent"
						if stream {
							action = "streamGenerateContent"
						}
						if r.URL.Path != "/v1beta/models/provider-model:"+action {
							t.Errorf("routed Gemini resource = %s", r.URL.String())
						}
						if stream && r.URL.Query().Get("alt") != "sse" {
							t.Error("native streaming request is missing alt=sse")
						}
						if _, exists := request["model"]; exists {
							t.Error("native Gemini body gained a model field")
						}
					} else if request["model"] != "provider-model" {
						t.Errorf("routed model = %v", request["model"])
					}
					if provider == "claude" && r.Header.Get("anthropic-version") != "2023-06-01" {
						t.Error("missing Anthropic protocol version")
					}
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						for _, frame := range directNativeSSE(fixture) {
							_, _ = io.WriteString(w, frame)
							w.(http.Flusher).Flush()
						}
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, fixture.response)
					}
				}))
				defer upstream.Close()
				installSiteNativeFixture(t, provider, upstream.URL, "provider-model", `{"X-Fixture-Option":"preserved"}`, false)
				path, body := fixture.path, fixture.request
				if stream {
					if provider == "gemini" {
						path = strings.Replace(path, ":generateContent", ":streamGenerateContent", 1)
					} else {
						var request map[string]any
						_ = json.Unmarshal([]byte(body), &request)
						request["stream"] = true
						encoded, _ := json.Marshal(request)
						body = string(encoded)
					}
				}
				status, output := siteNativeHTTPResponse(t, fixture.handler, path, body)
				if status != http.StatusOK || !strings.Contains(output, "receipt-text") || calls.Load() != 1 {
					t.Fatalf("site relay status=%d calls=%d body=%s", status, calls.Load(), output)
				}
			})
		}
	}
}

func TestSiteNativeCredentialCustomHeaderPrecedence(t *testing.T) {
	for _, fixture := range directWireFixtures[2:] {
		for _, override := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/override=%t", fixture.provider, override), func(t *testing.T) {
				provider, header := "claude", "x-api-key"
				if fixture.provider == "gemini" {
					provider, header = "gemini", "x-goog-api-key"
				}
				want := "fixture-site-key"
				if override {
					want = "fixture-explicit-key"
				}
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get(header) != want || r.Header.Get("Authorization") != "" {
						t.Error("native auth ignored the explicit custom-header collision policy")
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, fixture.response)
				}))
				defer upstream.Close()
				custom, _ := json.Marshal(map[string]string{header: "fixture-explicit-key", "Authorization": "Bearer forbidden-custom"})
				installSiteNativeFixture(t, provider, upstream.URL, "provider-model", string(custom), override)
				status, output := siteNativeHTTPResponse(t, fixture.handler, fixture.path, fixture.request)
				if status != http.StatusOK {
					t.Fatalf("status=%d body=%s", status, output)
				}
			})
		}
	}
}

func TestSiteGeminiResourceURLHTTPRelay(t *testing.T) {
	for _, tc := range []struct{ name, basePath, clientPath, expectedPath string }{
		{"versioned base", "/v1beta", "/v1beta/models/client-alias:generateContent", "/v1beta/models/provider%2Fmodel:generateContent"},
		{"semantic base", "/api/provider", "/v1beta/models/client-alias:generateContent", "/api/provider/v1beta/models/provider%2Fmodel:generateContent"},
		{"dynamic API version", "/api/provider", "/gemini/v1/models/client-alias:generateContent", "/api/provider/v1/models/provider%2Fmodel:generateContent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.EscapedPath() != tc.expectedPath {
					t.Errorf("resource path=%s, want %s", r.URL.EscapedPath(), tc.expectedPath)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, directWireFixtures[3].response)
			}))
			defer upstream.Close()
			installSiteNativeFixture(t, "gemini", upstream.URL+tc.basePath, "provider/model", "{}", false)
			status, output := siteNativeHTTPResponse(t, HandleGeminiGenerateContent, tc.clientPath, directWireFixtures[3].request)
			if status != http.StatusOK {
				t.Fatalf("status=%d body=%s", status, output)
			}
		})
	}
}

func TestSiteNativeProtocolsKeepGatewayBearer(t *testing.T) {
	for _, fixture := range directWireFixtures[2:] {
		t.Run(fixture.provider, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer fixture-site-key" || r.Header.Get("x-api-key") != "" || r.Header.Get("x-goog-api-key") != "" {
					t.Error("native client protocol changed gateway Bearer authentication")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, fixture.response)
			}))
			defer upstream.Close()
			installSiteNativeFixture(t, "new-api", upstream.URL, "provider-model", "{}", false)
			status, output := siteNativeHTTPResponse(t, fixture.handler, fixture.path, fixture.request)
			if status != http.StatusOK {
				t.Fatalf("status=%d body=%s", status, output)
			}
		})
	}
}
