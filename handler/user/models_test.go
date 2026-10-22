package user

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/google/uuid"
)

func TestUserPortalModelsUseRealGroupRouting(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := openUserModelsDatabase(t, dialect)
			users := store.NewGatewayUserStore(db)
			ctx := context.Background()
			group, err := users.CreateUserGroup(ctx, "models", "Models", true)
			if err != nil {
				t.Fatal(err)
			}
			otherGroup, err := users.CreateUserGroup(ctx, "other", "Other", true)
			if err != nil {
				t.Fatal(err)
			}
			u, err := users.CreateUser(ctx, "alice", "Alice", group.ID, true)
			if err != nil {
				t.Fatal(err)
			}
			var site, account, token int64
			for _, entry := range []struct {
				out   *int64
				query string
				args  []any
			}{
				{&site, `INSERT INTO sites(name,url,platform,created_at,updated_at) VALUES ('Private supply','http://private-supply.invalid','new-api','now','now') RETURNING id`, nil},
				{&account, `INSERT INTO accounts(site_id,username,access_token,api_token,balance,created_at,updated_at) VALUES (?,'private-account','secret-management','secret-upstream',100,'now','now') RETURNING id`, []any{int64(1)}},
			} {
				if entry.out == &account {
					entry.args = []any{site}
				}
				if err := db.Get(entry.out, entry.query, entry.args...); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Get(&token, `INSERT INTO account_tokens(account_id,name,token,is_default) VALUES (?,'private-token','secret-token',TRUE) RETURNING id`, account); err != nil {
				t.Fatal(err)
			}
			var allowed, foreign, wildcard, alias int64
			for i, entry := range []struct {
				id      *int64
				pattern string
				display any
			}{{&allowed, "public-a", nil}, {&foreign, "private-model", nil}, {&wildcard, "wild-*", nil}, {&alias, "alias-*", "public-alias"}} {
				if err := db.Get(entry.id, `INSERT INTO token_routes(model_pattern,display_name,created_at,updated_at) VALUES (?,?,'now','now') RETURNING id`, entry.pattern, entry.display); err != nil {
					t.Fatal(err)
				}
				source := entry.pattern
				if i == 3 {
					source = "alias-concrete"
				}
				if _, err := db.Exec(`INSERT INTO route_channels(route_id,account_id,token_id,source_model) VALUES (?,?,?,?)`, *entry.id, account, token, source); err != nil {
					t.Fatal(err)
				}
			}
			for _, id := range []int64{allowed, wildcard, alias} {
				if err := users.GrantUserGroupRoute(ctx, group.ID, id); err != nil {
					t.Fatal(err)
				}
			}
			if err := users.GrantUserGroupRoute(ctx, otherGroup.ID, foreign); err != nil {
				t.Fatal(err)
			}
			catalog := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil)
			routeIDs := []int64{allowed, wildcard, alias}
			if _, err := catalog.ExplainSelection(ctx, "public-a", nil, routing.DownstreamRoutingPolicy{UserRouteIDs: &routeIDs, SupportedModels: []string{"*"}, RequiredUpstreamProtocol: -1}); err != nil {
				t.Fatalf("real catalog explain: %v", err)
			}
			h := &handler{users: users, models: func() ModelCatalog { return catalog }}
			get := func() map[string]int64 {
				t.Helper()
				req := httptest.NewRequest("GET", "/api/user/models", nil)
				req = req.WithContext(auth.WithGatewayUserPrincipal(req.Context(), auth.GatewayUserPrincipal{UserID: u.ID}))
				w := httptest.NewRecorder()
				h.listModels(w, req)
				if w.Code != 200 {
					t.Fatalf("models: %d %s", w.Code, w.Body.String())
				}
				for _, secret := range []string{"private-account", "private-supply", "secret-", "candidates", "private-model", "wild-*"} {
					if strings.Contains(w.Body.String(), secret) {
						t.Fatalf("model disclosure: %s", secret)
					}
				}
				var body struct {
					Items []userModel `json:"items"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				out := map[string]int64{}
				for _, item := range body.Items {
					if item.Protocols == nil || len(item.Protocols) != 0 {
						t.Fatalf("legacy eligibility invented protocol capabilities: %+v", item)
					}
					out[item.ID] = item.RouteID
				}
				return out
			}
			got := get()
			if got["public-a"] != allowed || got["public-alias"] != alias || len(got) != 2 {
				t.Fatalf("catalog %+v (allowed %d alias %d)", got, allowed, alias)
			}
			if err := users.RevokeUserGroupRoute(ctx, group.ID, allowed); err != nil {
				t.Fatal(err)
			}
			got = get()
			if len(got) != 1 || got["public-alias"] != alias {
				t.Fatalf("revoked route visible: %+v", got)
			}
			if err := users.UpdateUser(ctx, u.ID, "Alice", otherGroup.ID, true); err != nil {
				t.Fatal(err)
			}
			// No global cached identity scope: this user's immediately changed group
			// can see the foreign model, and no longer sees the former group's alias.
			req := httptest.NewRequest("GET", "/api/user/models", nil)
			req = req.WithContext(auth.WithGatewayUserPrincipal(req.Context(), auth.GatewayUserPrincipal{UserID: u.ID}))
			w := httptest.NewRecorder()
			h.listModels(w, req)
			if w.Code != 200 || !strings.Contains(w.Body.String(), fmt.Sprintf(`"routeId":%d`, foreign)) || strings.Contains(w.Body.String(), "public-alias") {
				t.Fatalf("group reload: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func openUserModelsDatabase(t *testing.T, dialect string) *store.DB {
	t.Helper()
	dsn := ":memory:"
	if dialect == store.DialectPostgres {
		dsn = os.Getenv("PG_TEST_DSN")
		if dsn == "" {
			t.Skip("PG_TEST_DSN not set")
		}
		control, err := store.Open(dialect, dsn, false)
		if err != nil {
			t.Fatal(err)
		}
		schema := "user_models_" + strings.ReplaceAll(uuid.NewString(), "-", "")
		if _, err := control.Exec("CREATE SCHEMA " + schema); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := control.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
				t.Errorf("drop own schema: %v", err)
			}
			_ = control.Close()
		})
		u, err := url.Parse(dsn)
		if err != nil {
			t.Fatal(err)
		}
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		dsn = u.String()
	}
	db, err := store.Open(dialect, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	previous := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{RoutingFallbackUnitCost: 1})
	t.Cleanup(func() {
		config.SetRuntime(previous)
		routing.SetGlobalCache(nil)
		routing.ResetSiteRuntimeHealthState()
	})
	return db
}
