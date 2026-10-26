package upstream_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestCatalogDiscoveryCustomHeadersKeepEndpointAuthentication(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, auth := range []string{store.DirectAuthBearer, store.DirectAuthAPIKey, store.DirectAuthGoogle, store.DirectAuthNone} {
				t.Run(auth, func(t *testing.T) {
					db := connectDatabase(t, dialect)
					var requests atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						requests.Add(1)
						if r.Header.Get("X-Tenant") != "fixture-tenant" {
							w.WriteHeader(http.StatusForbidden)
							return
						}
						if r.Header.Get("Accept") != "application/json" || r.Header.Get("X-Client-Trace") != "prefix--suffix" {
							t.Error("catalog defaults or clientless template expansion changed")
						}
						wantBearer, wantAPIKey, wantGoogle := "", "", ""
						switch auth {
						case store.DirectAuthBearer:
							wantBearer = "Bearer fixture-secret"
						case store.DirectAuthAPIKey:
							wantAPIKey = "fixture-secret"
							if r.Header.Get("Anthropic-Version") != "2023-06-01" {
								t.Error("custom header replaced the native Anthropic version")
							}
						case store.DirectAuthGoogle:
							wantGoogle = "fixture-secret"
						}
						if r.Header.Get("Authorization") != wantBearer || r.Header.Get("X-Api-Key") != wantAPIKey || r.Header.Get("X-Goog-Api-Key") != wantGoogle {
							t.Error("custom headers changed or injected endpoint credentials")
						}
						if r.Header.Get("Cookie") != "" || r.Header.Get("Proxy-Authorization") != "" || r.Header.Get("X-Metapi-Responses-Only") != "" || r.Host == "unselected.example" {
							t.Error("denied custom header reached the catalog")
						}
						if auth == store.DirectAuthGoogle || auth == store.DirectAuthNone {
							fmt.Fprint(w, `{"models":[{"name":"tenant-model"}]}`)
						} else {
							fmt.Fprint(w, `{"data":[{"id":"tenant-model"}]}`)
						}
					}))
					defer server.Close()
					input := connectInput(server.URL)
					input.Channel.Endpoints = store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: server.URL + "/v1/chat/completions", Auth: auth}}
					switch auth {
					case store.DirectAuthAPIKey:
						input.Channel.Endpoints = store.DirectEndpoints{Messages: &store.DirectEndpoint{URL: server.URL + "/v1/messages", Auth: auth}}
					case store.DirectAuthGoogle:
						input.Channel.Endpoints = store.DirectEndpoints{Gemini: &store.DirectEndpoint{URL: server.URL + "/v1beta/models", Auth: auth, ModelPath: true}}
					case store.DirectAuthNone:
						input.Channel.Provider = "ollama"
						input.CredentialKind = store.DirectCredentialNone
						input.Secret = ""
						input.Channel.Endpoints = store.DirectEndpoints{Ollama: &store.DirectEndpoint{URL: server.URL + "/api/chat", Auth: auth, Profile: "ollama"}}
					}
					input.Channel.CustomHeaders = `[
						{"header_key":"X-Tenant","header_value":"fixture-tenant"},
						{"header_key":"X-Client-Trace","header_value":"prefix-{client_header:x-request-id}-suffix"},
						{"header_key":"Accept","header_value":"text/event-stream"},
						{"header_key":"Anthropic-Version","header_value":"unselected-version"},
						{"header_key":"authorization","header_value":"Bearer fixture-unselected"},
						{"header_key":"X-API-Key","header_value":"fixture-unselected"},
						{"header_key":"x-goog-api-key","header_value":"fixture-unselected"},
						{"header_key":"Cookie","header_value":"fixture-unselected"},
						{"header_key":"Host","header_value":"unselected.example"},
						{"header_key":"Proxy-Authorization","header_value":"fixture-unselected"},
						{"header_key":"X-Metapi-Responses-Only","header_value":"1"}
					]`
					out, err := upstream.Connect(t.Context(), db.DB, input)
					if err != nil || out.ModelCount != 1 || out.Discovery.Status != "discovered" {
						t.Fatalf("tenant catalog connect=%+v err=%v", out, err)
					}
					// Refresh reads persisted request configuration independently of
					// the quick-connect write path's configuration support.
					if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET custom_header=? WHERE id=?`), input.Channel.CustomHeaders, out.ID); err != nil {
						t.Fatal(err)
					}
					preview, err := upstream.PreviewCatalogRefresh(t.Context(), db.DB, out.ID, nil)
					if err != nil || strings.Join(preview.Kept, ",") != "tenant-model" || requests.Load() != 2 {
						t.Fatalf("tenant catalog refresh=%+v requests=%d err=%v", preview, requests.Load(), err)
					}
				})
			}
		})
	}
}

func TestCatalogFallbackAndPaginationKeepCustomHeaders(t *testing.T) {
	db := connectDatabase(t, store.DialectSQLite)
	var primary, pages atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tenant") != "fixture-tenant" || r.Header.Get("Accept") != "application/json" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.URL.Path == "/primary/v1/models" {
			primary.Add(1)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		pages.Add(1)
		if r.URL.Path != "/fallback/v1/models" || r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "fixture-secret" || r.Header.Get("Anthropic-Version") != "2023-06-01" {
			t.Error("fallback lost its native authentication or version")
		}
		if r.URL.Query().Get("after_id") == "" {
			fmt.Fprint(w, `{"data":[{"id":"first-model"}],"has_more":true,"last_id":"first-model"}`)
		} else {
			fmt.Fprint(w, `{"data":[{"id":"second-model"}]}`)
		}
	}))
	defer server.Close()
	input := connectInput(server.URL)
	input.Channel.Endpoints.Chat.URL = server.URL + "/primary/v1/chat/completions"
	input.Channel.Endpoints.Responses = nil
	input.Channel.Endpoints.Messages.URL = server.URL + "/fallback/v1/messages"
	input.Channel.CustomHeaders = `[{"header_key":"X-Tenant","header_value":"fixture-tenant"},{"header_key":"Accept","header_value":"text/plain"},{"header_key":"Anthropic-Version","header_value":"unselected-version"}]`
	out, err := upstream.Connect(t.Context(), db.DB, input)
	if err != nil || out.ModelCount != 2 {
		t.Fatalf("tenant fallback connect=%+v err=%v", out, err)
	}
	if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET custom_header=? WHERE id=?`), input.Channel.CustomHeaders, out.ID); err != nil {
		t.Fatal(err)
	}
	preview, err := upstream.PreviewCatalogRefresh(t.Context(), db.DB, out.ID, nil)
	if err != nil || strings.Join(preview.Kept, ",") != "first-model,second-model" || primary.Load() != 2 || pages.Load() != 4 {
		t.Fatalf("tenant fallback refresh=%+v primary=%d pages=%d err=%v", preview, primary.Load(), pages.Load(), err)
	}
}

func TestCatalogDiscoveryRejectsInvalidCustomHeaders(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
	}{
		{"malformed", `[fixture-header-secret]`},
		{"object", `{"header_key":"X-Tenant","header_value":"fixture-header-secret"}`},
		{"null", `null`},
		{"null-row", `[null]`},
		{"unknown-field", `[{"key":"X-Tenant","value":"fixture-header-secret"}]`},
		{"wrong-value-type", `[{"header_key":"X-Tenant","header_value":7}]`},
		{"trailing-json", `[] {"secret":"fixture-header-secret"}`},
		{"invalid-template", `[{"header_key":"X-Tenant","header_value":"fixture-header-secret-{client_header:authorization}"}]`},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, operation := range []string{"connect", "refresh"} {
				t.Run(operation, func(t *testing.T) {
					db := connectDatabase(t, store.DialectSQLite)
					var requests atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						requests.Add(1)
						fmt.Fprint(w, `{"data":[{"id":"baseline-model"}]}`)
					}))
					defer server.Close()
					input := connectInput(server.URL)
					input.RecommendedModels = []string{"must-not-hide-invalid-headers"}
					var channelID int64
					if operation == "refresh" {
						out, err := upstream.Connect(t.Context(), db.DB, input)
						if err != nil {
							t.Fatal(err)
						}
						channelID = out.ID
						if _, err := db.Exec(db.Rebind(`UPDATE upstream_channels SET custom_header=? WHERE id=?`), test.raw, channelID); err != nil {
							t.Fatal(err)
						}
					}
					requests.Store(0)
					before := connectCounts(t, db)
					var err error
					if operation == "connect" {
						input.Channel.CustomHeaders = test.raw
						_, err = upstream.Connect(t.Context(), db.DB, input)
					} else {
						_, err = upstream.PreviewCatalogRefresh(t.Context(), db.DB, channelID, nil)
					}
					var domain *upstream.Error
					if !errors.As(err, &domain) || domain.Status != 400 || domain.Message != "Invalid custom headers" {
						t.Fatalf("invalid headers did not return a content-free error: %v", err)
					}
					if requests.Load() != 0 {
						t.Fatal("invalid headers reached a catalog endpoint")
					}
					if after := connectCounts(t, db); !reflect.DeepEqual(after, before) {
						t.Fatalf("invalid headers changed stored rows: before=%v after=%v", before, after)
					}
				})
			}
		})
	}
}

func TestConnectRejectsInvalidCatalogHeadersBeforePresetFallback(t *testing.T) {
	input := connectInput("https://catalog.invalid")
	input.Channel.Endpoints = store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: "https://catalog.invalid/inference", Auth: store.DirectAuthBearer}}
	input.Channel.CustomHeaders = `[fixture-header-secret]`
	input.RecommendedModels = []string{"must-not-hide-invalid-headers"}
	// No catalog URL can be derived, but malformed configuration must still
	// fail before preset fallback could attempt to persist the connection.
	_, err := upstream.Connect(t.Context(), nil, input)
	var domain *upstream.Error
	if !errors.As(err, &domain) || domain.Status != 400 || domain.Message != "Invalid custom headers" {
		t.Fatalf("invalid headers bypassed validation through preset fallback: %v", err)
	}
}
