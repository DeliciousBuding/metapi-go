package upstream_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

func connectDatabase(t *testing.T, dialect string) *store.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "connect.db")
	if dialect == store.DialectPostgres {
		dsn = os.Getenv("PG_TEST_DSN")
		if dsn == "" {
			t.Skip("PG_TEST_DSN not set")
		}
	}
	db, err := store.Open(dialect, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if dialect == store.DialectPostgres {
		pgtest.Reset(t, db.DB)
	}
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func connectInput(address string) upstream.ConnectInput {
	return upstream.ConnectInput{Channel: upstream.ChannelCreate{Name: "Fixture", Provider: "new-api", BaseURL: address, Endpoints: store.DirectEndpoints{
		Chat:      &store.DirectEndpoint{URL: address + "/v1/chat/completions", Auth: store.DirectAuthBearer},
		Responses: &store.DirectEndpoint{URL: address + "/v1/responses", Auth: store.DirectAuthBearer},
		Messages:  &store.DirectEndpoint{URL: address + "/v1/messages", Auth: store.DirectAuthAPIKey},
	}}, Secret: "fixture-secret", CredentialKind: store.DirectCredentialAPIKey}
}

func connectCounts(t *testing.T, db *store.DB) []int {
	t.Helper()
	counts := []int{}
	for _, table := range []string{"upstream_channels", "upstream_credentials", "upstream_models", "upstream_grants", "upstream_groups", "upstream_group_items", "token_routes", "upstream_route_groups"} {
		var n int
		if err := db.Get(&n, "SELECT COUNT(*) FROM "+table); err != nil {
			t.Fatal(err)
		}
		counts = append(counts, n)
	}
	return counts
}

func TestConnectAtomicGraphAndExistingRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer fixture-secret" {
			t.Errorf("wrong discovery request: path=%s authenticated=%v", r.URL.Path, r.Header.Get("Authorization") != "")
		}
		fmt.Fprint(w, `{"data":[{"id":"z-model"},{"id":"a-model"},{"id":"z-model"}]}`)
	}))
	defer server.Close()
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := connectDatabase(t, dialect)
			input := connectInput(server.URL)
			input.AutoName = true
			result, err := upstream.Connect(t.Context(), db.DB, input)
			if err != nil {
				t.Fatal(err)
			}
			if result.Discovery.Status != "discovered" || result.ModelCount != 2 || result.RouteCount != 2 || !result.Enabled || result.Ownership != "native" {
				t.Fatalf("result: %+v", result)
			}
			if got := connectCounts(t, db); !reflect.DeepEqual(got, []int{1, 1, 2, 2, 2, 2, 2, 2}) {
				t.Fatalf("graph counts %v", got)
			}
			var masks []int
			if err := db.Select(&masks, `SELECT protocols FROM upstream_grants ORDER BY id`); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(masks, []int{14, 14}) {
				t.Fatalf("grant protocols %v", masks)
			}
			if _, err := db.Exec(`UPDATE token_routes SET routing_strategy='round_robin',sort_order=17 WHERE model_pattern='a-model'`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`UPDATE upstream_group_items SET priority=5,weight=7`); err != nil {
				t.Fatal(err)
			}
			second, err := upstream.Connect(t.Context(), db.DB, input)
			if err != nil {
				t.Fatal(err)
			}
			if second.Name != "Fixture (2)" || second.ID == result.ID {
				t.Fatalf("second connection: %+v", second)
			}
			if got := connectCounts(t, db); !reflect.DeepEqual(got, []int{2, 2, 4, 4, 2, 4, 2, 2}) {
				t.Fatalf("merged graph counts %v", got)
			}
			var strategy string
			var order int
			if err := db.QueryRow(`SELECT routing_strategy,sort_order FROM token_routes WHERE model_pattern='a-model'`).Scan(&strategy, &order); err != nil {
				t.Fatal(err)
			}
			if strategy != "round_robin" || order != 17 {
				t.Fatal("existing route was modified")
			}
			var oldMembers int
			if err := db.Get(&oldMembers, `SELECT COUNT(*) FROM upstream_group_items WHERE priority=5 AND weight=7`); err != nil || oldMembers != 2 {
				t.Fatalf("existing priorities changed: count=%d err=%v", oldMembers, err)
			}
			var routeID int64
			if err := db.Get(&routeID, `SELECT id FROM token_routes WHERE model_pattern='a-model'`); err != nil {
				t.Fatal(err)
			}
			candidates, err := service.NewProxyRoutingStore(db).LoadRouteChannels(t.Context(), []int64{routeID})
			if err != nil || len(candidates) != 2 {
				t.Fatalf("route candidates count=%d err=%v", len(candidates), err)
			}
			for _, candidate := range candidates {
				if candidate.Channel.Direct == nil || candidate.Channel.Direct.ModelName != "a-model" {
					t.Fatal("model routes leaked into one another")
				}
			}
		})
	}
}

func TestConnectRollsBackLateFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":[{"id":"fixture-model"}]}`) }))
	defer server.Close()
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := connectDatabase(t, dialect)
			if dialect == store.DialectSQLite {
				if _, err := db.Exec(`CREATE TRIGGER connect_fail BEFORE INSERT ON upstream_group_items BEGIN SELECT RAISE(ABORT,'fixture late failure'); END`); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := db.Exec(`CREATE FUNCTION connect_fail() RETURNS trigger AS $$ BEGIN RAISE EXCEPTION 'fixture late failure'; END; $$ LANGUAGE plpgsql; CREATE TRIGGER connect_fail BEFORE INSERT ON upstream_group_items FOR EACH ROW EXECUTE FUNCTION connect_fail()`); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if _, err := db.Exec(`DROP TRIGGER connect_fail ON upstream_group_items; DROP FUNCTION connect_fail()`); err != nil {
						t.Error(err)
					}
				})
			}
			before := connectCounts(t, db)
			if _, err := upstream.Connect(t.Context(), db.DB, connectInput(server.URL)); err == nil {
				t.Fatal("late fixture failure unexpectedly committed")
			}
			if got := connectCounts(t, db); !reflect.DeepEqual(got, before) {
				t.Fatalf("partial graph persisted: %v -> %v", before, got)
			}
		})
	}
}

func TestConnectDiscoveryFallbacks(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, status := range []int{200, 404, 405, 501} {
				t.Run(fmt.Sprint(status), func(t *testing.T) {
					db := connectDatabase(t, dialect)
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(status); fmt.Fprint(w, `{"data":[]}`) }))
					defer server.Close()
					input := connectInput(server.URL)
					input.RecommendedModels = []string{"recommended"}
					result, err := upstream.Connect(t.Context(), db.DB, input)
					if err != nil || result.Discovery.Status != "preset" || result.ModelCount != 1 {
						t.Fatalf("preset result=%+v err=%v", result, err)
					}
					input.Channel.Name = "Empty"
					input.RecommendedModels = nil
					result, err = upstream.Connect(t.Context(), db.DB, input)
					if err != nil || result.Discovery.Status != "empty" || result.ModelCount != 0 || result.RouteCount != 0 {
						t.Fatalf("empty result=%+v err=%v", result, err)
					}
					var count int
					if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM upstream_credentials WHERE channel_id=?`), result.ID); err != nil || count != 1 {
						t.Fatal("empty connection lost its credential")
					}
				})
			}
		})
	}
}

func TestConnectRejectsDiscoveryErrorsWithoutWrites(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"unauthorized", 401, `fixture-secret`}, {"forbidden", 403, `fixture-secret`}, {"rate-limit", 429, `fixture-secret`}, {"server-error", 500, `fixture-secret`},
		{"malformed", 200, `fixture-secret`}, {"wrong-schema", 200, `{"error":"fixture-secret"}`}, {"bad-name", 200, `{"data":[{"id":"re:fixture-secret"}]}`},
		{"partial-catalog", 200, `{"data":[{"id":"model"}],"has_more":true}`},
		{"too-large", 200, strings.Repeat("x", (16<<20)+1)},
		{"too-many", 200, `{"data":[` + strings.Repeat(`{"id":"fixture-model"},`, 10000) + `{"id":"fixture-model"}]}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(test.status); fmt.Fprint(w, test.body) }))
			defer server.Close()
			input := connectInput(server.URL)
			input.RecommendedModels = []string{"fallback-must-not-hide-failure"}
			// nil DB proves errors terminate before any persistence is attempted.
			_, err := upstream.Connect(t.Context(), nil, input)
			var domain *upstream.Error
			if !errors.As(err, &domain) || strings.Contains(err.Error(), "fixture-secret") {
				t.Fatalf("missing or unsafe discovery error: %v", err)
			}
			if (test.status == 401 || test.status == 403) && domain.Status != 400 {
				t.Fatalf("credential error status=%d", domain.Status)
			}
		})
	}
}

func TestConnectDiscoveryNetworkBoundaries(t *testing.T) {
	t.Run("forbidden-target", func(t *testing.T) {
		_, err := upstream.Connect(t.Context(), nil, connectInput("http://169.254.169.254"))
		var domain *upstream.Error
		if !errors.As(err, &domain) || domain.Status != 400 {
			t.Fatalf("forbidden target: %v", err)
		}
	})
	t.Run("redirect", func(t *testing.T) {
		var received atomic.Int32
		target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { received.Add(1) }))
		defer target.Close()
		redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
		defer redirect.Close()
		if _, err := upstream.Connect(t.Context(), nil, connectInput(redirect.URL)); err == nil {
			t.Fatal("redirect accepted")
		}
		if received.Load() != 0 {
			t.Fatal("credentials followed redirect")
		}
	})
	t.Run("cancel", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
		defer server.Close()
		ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		if _, err := upstream.Connect(ctx, nil, connectInput(server.URL)); err == nil {
			t.Fatal("cancellation accepted")
		}
		if time.Since(start) > time.Second {
			t.Fatal("discovery ignored cancellation")
		}
	})
}

func TestConnectDiscoveryProxyAndNativeCatalogs(t *testing.T) {
	for _, mode := range []string{"channel", "system"} {
		t.Run(mode, func(t *testing.T) {
			db := connectDatabase(t, store.DialectSQLite)
			var requests atomic.Int32
			proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Host != "provider.invalid" || r.URL.Path != "/v1/models" {
					t.Errorf("unexpected proxy target %s", r.URL)
				}
				fmt.Fprint(w, `{"data":[{"id":"proxy-model"}]}`)
			}))
			defer proxy.Close()
			input := connectInput("http://provider.invalid")
			old := config.RuntimeSafe()
			defer func() {
				config.UpdateRuntime(func(rt *config.RuntimeSettings) {
					if old != nil {
						*rt = *old
					} else {
						*rt = config.RuntimeSettings{}
					}
				})
			}()
			if mode == "channel" {
				input.Channel.ChannelProxy = proxy.URL
				input.Channel.UseSystemProxy = true
				config.UpdateRuntime(func(rt *config.RuntimeSettings) { rt.SystemProxyUrl = "http://invalid-proxy.invalid" })
			} else {
				input.Channel.UseSystemProxy = true
				config.UpdateRuntime(func(rt *config.RuntimeSettings) { rt.SystemProxyUrl = proxy.URL })
			}
			if _, err := upstream.Connect(t.Context(), db.DB, input); err != nil {
				t.Fatal(err)
			}
			if requests.Load() != 1 {
				t.Fatal("proxy was bypassed")
			}
		})
	}
	for _, kind := range []string{"gemini", "ollama", "anthropic"} {
		t.Run(kind, func(t *testing.T) {
			db := connectDatabase(t, store.DialectSQLite)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch kind {
				case "gemini":
					if r.URL.Path != "/v1beta/models" || r.Header.Get("X-Goog-Api-Key") != "fixture-secret" {
						t.Error("Gemini catalog contract")
					}
					fmt.Fprint(w, `{"models":[{"name":"models/gemini-fixture"}]}`)
				case "ollama":
					if r.URL.Path != "/api/tags" || r.Header.Get("Authorization") != "" {
						t.Error("Ollama catalog contract")
					}
					fmt.Fprint(w, `{"models":[{"name":"local-fixture"}]}`)
				case "anthropic":
					if r.URL.Path != "/v1/models" || r.Header.Get("X-Api-Key") != "fixture-secret" || r.Header.Get("Anthropic-Version") == "" {
						t.Error("Anthropic catalog contract")
					}
					fmt.Fprint(w, `{"data":[{"id":"claude-fixture"}]}`)
				}
			}))
			defer server.Close()
			input := connectInput(server.URL)
			switch kind {
			case "gemini":
				input.Channel.Endpoints = store.DirectEndpoints{Gemini: &store.DirectEndpoint{URL: server.URL + "/v1beta/models", Auth: store.DirectAuthGoogle, ModelPath: true}}
			case "ollama":
				input.Channel.Provider = "ollama"
				input.Secret = ""
				input.CredentialKind = store.DirectCredentialNone
				input.Channel.Endpoints = store.DirectEndpoints{Ollama: &store.DirectEndpoint{URL: server.URL + "/api/chat", Auth: store.DirectAuthNone, Profile: "ollama"}}
			case "anthropic":
				input.Channel.Endpoints = store.DirectEndpoints{Messages: &store.DirectEndpoint{URL: server.URL + "/v1/messages", Auth: store.DirectAuthAPIKey}}
			}
			out, err := upstream.Connect(t.Context(), db.DB, input)
			if err != nil || out.ModelCount != 1 || out.Discovery.Status != "discovered" {
				t.Fatalf("native discovery %+v err=%v", out, err)
			}
			encoded, _ := json.Marshal(out)
			if strings.Contains(string(encoded), "fixture-secret") {
				t.Fatal("secret exposed")
			}
		})
	}
}
