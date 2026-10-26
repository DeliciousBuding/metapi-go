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

func TestConnectCatalogFallbackUsesConfiguredEndpoints(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, status := range []int{404, 405, 501} {
				t.Run(fmt.Sprint(status), func(t *testing.T) {
					db := connectDatabase(t, dialect)
					var primary, fallback, unused atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						switch r.URL.Path {
						case "/openai/v1/models":
							primary.Add(1)
							if r.Header.Get("Authorization") != "Bearer fixture-secret" || r.Header.Get("X-Api-Key") != "" {
								t.Error("primary catalog did not use its own authentication")
							}
							w.WriteHeader(status)
						case "/anthropic/v1/models":
							fallback.Add(1)
							if r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "fixture-secret" || r.Header.Get("Anthropic-Version") != "2023-06-01" || r.URL.Query().Get("limit") != "1000" {
								t.Error("fallback catalog lost its authentication or native format")
							}
							fmt.Fprint(w, `{"data":[{"id":"fallback-model"}]}`)
						default:
							unused.Add(1)
							fmt.Fprint(w, `{"models":[{"name":"models/must-not-be-merged"}]}`)
						}
					}))
					defer server.Close()
					input := connectInput(server.URL)
					input.Channel.Endpoints.Chat.URL = server.URL + "/openai/v1/chat/completions"
					input.Channel.Endpoints.Responses.URL = server.URL + "/openai/v1/responses"
					input.Channel.Endpoints.Messages.URL = server.URL + "/anthropic/v1/messages"
					input.Channel.Endpoints.Gemini = &store.DirectEndpoint{URL: server.URL + "/unused/v1beta/models", Auth: store.DirectAuthGoogle, ModelPath: true}
					input.RecommendedModels = []string{"must-not-use-preset"}
					out, err := upstream.Connect(t.Context(), db.DB, input)
					if err != nil || out.Discovery.Status != "discovered" || out.ModelCount != 1 || out.RouteCount != 1 {
						t.Fatalf("fallback connect=%+v err=%v", out, err)
					}
					if primary.Load() != 1 || fallback.Load() != 1 || unused.Load() != 0 {
						t.Fatalf("catalog requests: primary=%d fallback=%d unused=%d", primary.Load(), fallback.Load(), unused.Load())
					}
					models, err := upstream.ListModels(t.Context(), db.DB, out.ID)
					if err != nil || len(models) != 1 || models[0].Name != "fallback-model" || len(models[0].Grants) != 1 || models[0].Grants[0].Protocols != input.Channel.Endpoints.ProtocolMask() {
						t.Fatalf("fallback changed the configured grants: models=%+v err=%v", models, err)
					}
				})
			}
		})
	}
}

func TestConnectCatalogFallbackDistinguishesAuthenticationAndFormat(t *testing.T) {
	for _, difference := range []string{"authentication", "format"} {
		t.Run(difference, func(t *testing.T) {
			db := connectDatabase(t, store.DialectSQLite)
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != "/v1/models" {
					t.Errorf("unexpected catalog path %s", r.URL.Path)
				}
				second := r.Header.Get("X-Api-Key") == "fixture-secret"
				if difference == "format" {
					second = r.URL.Query().Get("limit") == "1000" && r.Header.Get("Anthropic-Version") == "2023-06-01"
				}
				if !second {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				fmt.Fprint(w, `{"data":[{"id":"same-url-model"}]}`)
			}))
			defer server.Close()
			input := connectInput(server.URL)
			if difference == "authentication" {
				input.Channel.Endpoints.Messages = nil
				input.Channel.Endpoints.Responses.Auth = store.DirectAuthAPIKey
			} else {
				input.Channel.Endpoints.Responses = nil
				input.Channel.Endpoints.Messages.Auth = store.DirectAuthBearer
			}
			out, err := upstream.Connect(t.Context(), db.DB, input)
			if err != nil || out.Discovery.Status != "discovered" || out.ModelCount != 1 || requests.Load() != 2 {
				t.Fatalf("distinct catalog contract was deduplicated: result=%+v requests=%d err=%v", out, requests.Load(), err)
			}
		})
	}
}

func TestCatalogFallbackStopsAtAuthoritativeResult(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"empty", 200, `{"data":[]}`},
		{"malformed", 200, `fixture-secret`},
		{"wrong-schema", 200, `{"error":"fixture-secret"}`},
		{"unauthorized", 401, `fixture-secret`},
		{"forbidden", 403, `fixture-secret`},
		{"rate-limit", 429, `fixture-secret`},
		{"server-error", 500, `fixture-secret`},
		{"bad-gateway", 502, `fixture-secret`},
		{"redirect", 302, ""},
		{"network", 0, ""},
		{"second-page-missing", 200, `{"data":[{"id":"first-page"}],"has_more":true,"last_id":"first-page"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, operation := range []string{"connect", "refresh"} {
				t.Run(operation, func(t *testing.T) {
					db := connectDatabase(t, store.DialectSQLite)
					var active atomic.Bool
					var primary, fallback atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path != "/primary/v1/models" {
							fallback.Add(1)
							fmt.Fprint(w, `{"models":[{"name":"models/must-not-be-used"}]}`)
							return
						}
						if !active.Load() {
							fmt.Fprint(w, `{"data":[{"id":"baseline-model"}]}`)
							return
						}
						primary.Add(1)
						if test.name == "network" {
							connection, _, err := w.(http.Hijacker).Hijack()
							if err != nil {
								t.Error(err)
								return
							}
							connection.Close()
							return
						}
						if test.name == "second-page-missing" && r.URL.Query().Get("after_id") != "" {
							w.WriteHeader(http.StatusNotFound)
							return
						}
						w.Header().Set("Location", "/backup/v1beta/models")
						w.WriteHeader(test.status)
						fmt.Fprint(w, test.body)
					}))
					defer server.Close()
					input := connectInput(server.URL)
					input.RecommendedModels = []string{"preset-model"}
					input.Channel.Endpoints = store.DirectEndpoints{
						Messages: &store.DirectEndpoint{URL: server.URL + "/primary/v1/messages", Auth: store.DirectAuthAPIKey},
						Gemini:   &store.DirectEndpoint{URL: server.URL + "/backup/v1beta/models", Auth: store.DirectAuthGoogle, ModelPath: true},
					}
					var channelID int64
					if operation == "refresh" {
						out, err := upstream.Connect(t.Context(), db.DB, input)
						if err != nil {
							t.Fatal(err)
						}
						channelID = out.ID
					}
					before := connectCounts(t, db)
					active.Store(true)
					var err error
					if operation == "connect" {
						var out upstream.ConnectResult
						out, err = upstream.Connect(t.Context(), db.DB, input)
						if test.name == "empty" && (err != nil || out.Discovery.Status != "preset" || out.ModelCount != 1) {
							t.Fatalf("empty connect must retain preset fallback: result=%+v err=%v", out, err)
						}
					} else {
						var preview upstream.CatalogRefreshPreview
						preview, err = upstream.PreviewCatalogRefresh(t.Context(), db.DB, channelID, nil)
						if test.name == "empty" && (err != nil || preview.Discovery.Status != "empty" || len(preview.Stale) != 1 || preview.Stale[0].Name != "baseline-model") {
							t.Fatalf("empty refresh must stay authoritative: preview=%+v err=%v", preview, err)
						}
					}
					if test.name != "empty" {
						var domain *upstream.Error
						if !errors.As(err, &domain) || strings.Contains(err.Error(), "fixture-secret") {
							t.Fatalf("missing or unsafe terminal error: %v", err)
						}
					}
					if operation == "refresh" || test.name != "empty" {
						if after := connectCounts(t, db); !reflect.DeepEqual(after, before) {
							t.Fatalf("terminal discovery changed stored rows: before=%v after=%v", before, after)
						}
					}
					wantPrimary := int32(1)
					if test.name == "second-page-missing" {
						wantPrimary = 2
					}
					if primary.Load() != wantPrimary || fallback.Load() != 0 {
						t.Fatalf("terminal catalog tried another endpoint: primary=%d fallback=%d", primary.Load(), fallback.Load())
					}
				})
			}
		})
	}
}

func TestCatalogRefreshFallbackPreservesGrants(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := connectDatabase(t, dialect)
			var changed atomic.Bool
			var primary, fallback atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/openai/v1/models" {
					primary.Add(1)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				fallback.Add(1)
				if r.URL.Path != "/anthropic/v1/models" || r.Header.Get("X-Api-Key") != "fixture-secret" || r.Header.Get("Authorization") != "" {
					t.Error("refresh fallback lost the endpoint contract")
				}
				if changed.Load() {
					fmt.Fprint(w, `{"data":[{"id":"original-model"},{"id":"added-model"}]}`)
				} else {
					fmt.Fprint(w, `{"data":[{"id":"original-model"}]}`)
				}
			}))
			defer server.Close()
			input := connectInput(server.URL)
			input.Channel.Endpoints.Chat.URL = server.URL + "/openai/v1/chat/completions"
			input.Channel.Endpoints.Responses.URL = server.URL + "/openai/v1/responses"
			input.Channel.Endpoints.Messages.URL = server.URL + "/anthropic/v1/messages"
			out, err := upstream.Connect(t.Context(), db.DB, input)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_grants SET protocols=?`), store.DirectProtocolChat); err != nil {
				t.Fatal(err)
			}
			changed.Store(true)
			preview, err := upstream.PreviewCatalogRefresh(t.Context(), db.DB, out.ID, nil)
			if err != nil || strings.Join(preview.Added, ",") != "added-model" || strings.Join(preview.Kept, ",") != "original-model" {
				t.Fatalf("fallback preview=%+v err=%v", preview, err)
			}
			applied, err := upstream.ApplyCatalogRefresh(t.Context(), db.DB, out.ID, preview.BasisToken, nil)
			if err != nil || applied.AddedCount != 1 || applied.StaleCount != 0 {
				t.Fatalf("fallback apply=%+v err=%v", applied, err)
			}
			models, err := upstream.ListModels(t.Context(), db.DB, out.ID)
			if err != nil || len(models) != 2 || models[0].Name != "added-model" || len(models[0].Grants) != 0 || models[1].Name != "original-model" || len(models[1].Grants) != 1 || models[1].Grants[0].Protocols != store.DirectProtocolChat {
				t.Fatalf("refresh expanded model authorization: models=%+v err=%v", models, err)
			}
			if primary.Load() != 3 || fallback.Load() != 3 {
				t.Fatalf("connect/preview/apply requests: primary=%d fallback=%d", primary.Load(), fallback.Load())
			}
		})
	}
}

func TestCatalogRefreshMixedAnonymousAuthentication(t *testing.T) {
	for _, mode := range []string{"anonymous-first", "missing-credential", "authenticated-fallback"} {
		t.Run(mode, func(t *testing.T) {
			db := connectDatabase(t, store.DialectSQLite)
			var missing atomic.Bool
			var authenticated atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/anonymous/api/tags" {
					if r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "" || r.Header.Get("X-Goog-Api-Key") != "" {
						t.Error("anonymous catalog received credentials")
					}
					if missing.Load() {
						w.WriteHeader(http.StatusNotFound)
						return
					}
				} else {
					authenticated.Add(1)
					if r.URL.Path != "/authenticated/api/tags" || r.Header.Get("Authorization") != "Bearer fixture-refresh-key" {
						t.Error("authenticated fallback received no usable credential")
					}
				}
				fmt.Fprint(w, `{"models":[{"name":"ollama-model"}]}`)
			}))
			defer server.Close()
			input := connectInput(server.URL)
			input.Channel.Provider = "ollama"
			input.Secret = ""
			input.CredentialKind = store.DirectCredentialNone
			input.Channel.Endpoints = store.DirectEndpoints{
				Messages: &store.DirectEndpoint{URL: server.URL + "/anonymous/v1/messages", Auth: store.DirectAuthNone, Profile: "ollama-messages"},
				Ollama:   &store.DirectEndpoint{URL: server.URL + "/authenticated/api/chat", Auth: store.DirectAuthBearer, Profile: "ollama"},
			}
			out, err := upstream.Connect(t.Context(), db.DB, input)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "authenticated-fallback" {
				if _, err := db.Exec(db.Rebind(`UPDATE upstream_credentials SET kind=?,secret=? WHERE channel_id=?`), store.DirectCredentialAPIKey, "fixture-refresh-key", out.ID); err != nil {
					t.Fatal(err)
				}
			}
			missing.Store(mode != "anonymous-first")
			preview, err := upstream.PreviewCatalogRefresh(t.Context(), db.DB, out.ID, nil)
			if mode == "missing-credential" {
				if err == nil || !strings.Contains(err.Error(), "no enabled credential") {
					t.Fatalf("missing fallback credential must fail before HTTP: %v", err)
				}
			} else if err != nil || preview.Discovery.Status != "discovered" || strings.Join(preview.Kept, ",") != "ollama-model" {
				t.Fatalf("mixed-auth refresh=%+v err=%v", preview, err)
			}
			wantAuthenticated := int32(0)
			if mode == "authenticated-fallback" {
				wantAuthenticated = 1
			}
			if authenticated.Load() != wantAuthenticated {
				t.Fatalf("authenticated requests=%d want=%d", authenticated.Load(), wantAuthenticated)
			}
		})
	}
}

func TestCatalogFallbackDoesNotSendCredentialsToAnonymousEndpoint(t *testing.T) {
	db := connectDatabase(t, store.DialectSQLite)
	var authenticated, anonymous atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/authenticated/api/tags":
			authenticated.Add(1)
			if r.Header.Get("Authorization") != "Bearer fixture-secret" {
				t.Error("authenticated catalog lost its credential")
			}
			w.WriteHeader(http.StatusNotImplemented)
		case "/anonymous/api/tags":
			anonymous.Add(1)
			if r.Header.Get("Authorization") != "" || r.Header.Get("X-Api-Key") != "" || r.Header.Get("X-Goog-Api-Key") != "" {
				t.Error("anonymous fallback received the previous endpoint's credential")
			}
			fmt.Fprint(w, `{"models":[{"name":"local-model"}]}`)
		default:
			t.Errorf("unexpected catalog path %s", r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	input := connectInput(server.URL)
	input.Channel.Provider = "ollama"
	input.Channel.Endpoints = store.DirectEndpoints{
		Messages: &store.DirectEndpoint{URL: server.URL + "/authenticated/v1/messages", Auth: store.DirectAuthBearer, Profile: "ollama-messages"},
		Ollama:   &store.DirectEndpoint{URL: server.URL + "/anonymous/api/chat", Auth: store.DirectAuthNone, Profile: "ollama"},
	}
	out, err := upstream.Connect(t.Context(), db.DB, input)
	if err != nil || out.ModelCount != 1 {
		t.Fatalf("anonymous fallback connect=%+v err=%v", out, err)
	}
	preview, err := upstream.PreviewCatalogRefresh(t.Context(), db.DB, out.ID, nil)
	if err != nil || strings.Join(preview.Kept, ",") != "local-model" || authenticated.Load() != 2 || anonymous.Load() != 2 {
		t.Fatalf("anonymous fallback refresh=%+v authenticated=%d anonymous=%d err=%v", preview, authenticated.Load(), anonymous.Load(), err)
	}
}
