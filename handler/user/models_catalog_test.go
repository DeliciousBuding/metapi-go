package user

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/pricingcatalog"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

func TestUserModelReferencePricePreservesFreeUnknownAndSafeMetadata(t *testing.T) {
	snapshot, err := pricingcatalog.ParseLLMMetadata([]byte(`{"private-provider":{"id":"private-provider","models":{
		"paid":{"id":"paid","cost":{"input":2,"output":8,"cache_read":0,"cache_write":4},"last_updated":"2026-10-20"},
		"free":{"id":"free","cost":{"input":0,"output":0},"last_updated":"https://private-source.invalid/secret-catalog"},
		"metadata-only":{"id":"metadata-only"}
	}}}`))
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Source = "https://private-source.invalid/secret-catalog"
	for _, tc := range []struct {
		model string
		want  string
	}{
		{"paid", `{"currency":"USD","unit":"per_million_tokens","input":2,"output":8,"cacheRead":0,"cacheWrite":4,"updatedAt":"2026-10-20"}`},
		{"free", `{"currency":"USD","unit":"per_million_tokens","input":0,"output":0,"cacheRead":null,"cacheWrite":null,"updatedAt":null}`},
		{"metadata-only", `null`},
		{"missing", `null`},
	} {
		t.Run(tc.model, func(t *testing.T) {
			actual, err := json.Marshal(publicReferencePrice(snapshot, tc.model))
			if err != nil {
				t.Fatal(err)
			}
			if string(actual) != tc.want {
				t.Fatalf("public price = %s, want %s", actual, tc.want)
			}
		})
	}
	if publicReferencePrice(nil, "paid") != nil {
		t.Fatal("disabled catalog invented a price")
	}
}

func TestUserPortalModelsExposeOnlyConfiguredProtocolExamples(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := openUserModelsDatabase(t, dialect)
			ctx := t.Context()
			users := store.NewGatewayUserStore(db)
			group, err := users.CreateUserGroup(ctx, "protocols", "Protocols", true)
			if err != nil {
				t.Fatal(err)
			}
			u, err := users.CreateUser(ctx, "alice", "Alice", group.ID, true)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = users.SetUserGroupPricing(ctx, group.ID, 0.625); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name      string
				endpoints store.DirectEndpoints
			}{
				{"writer", store.DirectEndpoints{Chat: &store.DirectEndpoint{URL: "https://private-source.invalid/v1/chat/completions", Auth: "bearer"}}},
				{"vectors", store.DirectEndpoints{Embeddings: &store.DirectEndpoint{URL: "https://private-source.invalid/v1/embeddings", Auth: "bearer"}}},
				{"pictures", store.DirectEndpoints{ImageGeneration: &store.DirectEndpoint{URL: "https://private-source.invalid/v1/images/generations", Auth: "bearer"}}},
			} {
				routeID := createUserModelDirectRoute(t, db, tc.name, tc.endpoints)
				if err := users.GrantUserGroupRoute(ctx, group.ID, routeID); err != nil {
					t.Fatal(err)
				}
			}
			catalog := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil)
			h := &handler{users: users, models: func() ModelCatalog { return catalog }}
			get := func() userModelCatalog {
				t.Helper()
				req := httptest.NewRequest("GET", "/api/user/models", nil)
				req = req.WithContext(auth.WithGatewayUserPrincipal(req.Context(), auth.GatewayUserPrincipal{UserID: u.ID}))
				w := httptest.NewRecorder()
				h.listModels(w, req)
				if w.Code != 200 {
					t.Fatalf("models: %d %s", w.Code, w.Body.String())
				}
				for _, private := range []string{"private-source", "private-provider", "private-model", "secret-", "candidate", "grantId"} {
					if strings.Contains(w.Body.String(), private) {
						t.Fatalf("private model metadata disclosed: %s", w.Body.String())
					}
				}
				var result userModelCatalog
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				return result
			}
			result := get()
			if result.Quota != (userModelQuota{Unit: "token_credits", TokensPerUnit: 1_000_000, GroupMultiplier: 0.625}) {
				t.Fatalf("incorrect group quota: %+v", result.Quota)
			}
			want := map[string][]string{"writer": {"chat/completions", "responses", "messages"}, "vectors": {"embeddings"}, "pictures": {}}
			if len(result.Items) != len(want) {
				t.Fatalf("models: %+v", result.Items)
			}
			for _, item := range result.Items {
				protocols, ok := want[item.ID]
				if !ok || !reflect.DeepEqual(item.Protocols, protocols) {
					t.Fatalf("protocol examples for %s = %v, want %v", item.ID, item.Protocols, protocols)
				}
				if item.ReferencePrice != nil {
					t.Fatalf("missing reference price invented for %s", item.ID)
				}
			}
			var writerModelID int64
			if err := db.Get(&writerModelID, `SELECT id FROM upstream_models WHERE name='private-model-writer'`); err != nil {
				t.Fatal(err)
			}
			disabled := false
			if _, err := upstream.UpdateModel(ctx, db.DB, writerModelID, upstream.ModelUpdate{Enabled: &disabled}); err != nil {
				t.Fatal(err)
			}
			result = get()
			if len(result.Items) != 2 {
				t.Fatalf("disabled model remains in catalog: %+v", result.Items)
			}
			for _, item := range result.Items {
				if item.ID == "writer" {
					t.Fatal("disabled generation model still exposes examples")
				}
			}
			// Reassigning the current group changes both visibility and the
			// displayed multiplier; no cache may retain the former group's rate.
			other, err := users.CreateUserGroup(ctx, "empty", "Empty", true)
			if err != nil {
				t.Fatal(err)
			}
			if err := users.UpdateUser(ctx, u.ID, "Alice", other.ID, true); err != nil {
				t.Fatal(err)
			}
			result = get()
			if len(result.Items) != 0 || result.Items == nil || result.Quota.GroupMultiplier != 1 {
				t.Fatalf("group change retained stale models or multiplier: %+v", result)
			}
		})
	}
}

func createUserModelDirectRoute(t *testing.T, db *store.DB, name string, endpoints store.DirectEndpoints) int64 {
	t.Helper()
	ctx := t.Context()
	var grantID int64
	err := upstream.Write(ctx, db.DB, func(tx *sqlx.Tx) error {
		channelID, err := upstream.InsertNative(ctx, tx, "channels", name, "generic", "private-provider", true, "https://private-source.invalid", endpoints, "", "", "", false, "", "", "", "")
		if err != nil {
			return err
		}
		credentialID, err := upstream.InsertNative(ctx, tx, "credentials", channelID, "private-credential", "secret-synthetic-token", true, store.DirectCredentialAPIKey, "{}")
		if err != nil {
			return err
		}
		modelID, err := upstream.InsertNative(ctx, tx, "models", channelID, "private-model-"+name, true)
		if err != nil {
			return err
		}
		grantID, err = upstream.InsertNative(ctx, tx, "grants", modelID, credentialID, endpoints.ProtocolMask(), true)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	input := upstream.GroupCreate{Name: name, Enabled: true, Members: []upstream.MemberCreate{{GrantID: grantID}}}
	input.Route.ModelPattern = name
	group, err := upstream.CreateGroup(ctx, db.DB, input)
	if err != nil {
		t.Fatal(err)
	}
	return group.RouteID
}
