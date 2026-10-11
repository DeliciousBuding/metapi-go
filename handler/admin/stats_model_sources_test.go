package admin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

func checkMarketplaceSupplySources(t *testing.T, db *sqlx.DB) {
	t.Helper()
	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	insert := func(entity string, args ...any) int64 {
		t.Helper()
		id, err := upstream.InsertNative(context.Background(), tx, entity, args...)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	cid := insert("channels", "Supply fixture", "generic", "newapi", true, "https://supply.example.test", store.DirectEndpoints{}, "", "", "", false, "", "", "", "")
	kid := insert("credentials", cid, "Primary", "fixture-only", true, "api_key", "{}")
	mid := insert("models", cid, "actual-coder", true)
	grant := insert("grants", mid, kid, 2, true)
	group := insert("groups", "Manual fixture", "manual", 0, "{}", true)
	insert("group_items", group, grant, 0, 1, store.DirectProtocolOrder{})
	var route int64
	if err := tx.Get(&route, tx.Rebind(`INSERT INTO token_routes (model_pattern, route_mode, enabled) VALUES (?, 'exact', true) RETURNING id`), "coding"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(tx.Rebind(`INSERT INTO upstream_route_groups (route_id, group_id) VALUES (?, ?)`), route, group); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	h := &statsHandler{db: db}
	read := func() map[string]map[string]any {
		t.Helper()
		models, err := h.buildMarketplaceModels()
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := json.Marshal(models)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "fixture-only") || strings.Contains(string(encoded), "baseUrl") {
			t.Fatal("unsafe supply projection")
		}
		out := map[string]map[string]any{}
		for _, model := range models {
			out[model["name"].(string)] = model
		}
		return out
	}
	models := read()
	for _, name := range []string{"actual-coder", "coding"} {
		model := models[name]
		if model == nil || model["supplyCount"] != 1 || model["accountCount"] != 0 {
			t.Fatalf("%s supply = %#v", name, model)
		}
		sources := model["supplySources"].([]marketplaceModelSource)
		source := sources[0]
		if source.Kind != "direct" || source.ID != grant || source.ChannelID != cid || source.UpstreamModel != "actual-coder" || source.Evidence != "configured" || !source.Enabled {
			t.Fatalf("source = %#v", source)
		}
		if source.SuccessCount == nil || *source.SuccessCount != 0 {
			t.Fatalf("unobserved source = %#v", source)
		}
		if len(source.Bindings) != 1 || source.Bindings[0].Selected || source.Bindings[0].RouteID != route {
			t.Fatalf("manual binding = %#v", source.Bindings)
		}
	}
	// Disabled credentials remain explainable; they cannot turn into a green
	// availability claim. A disabled public route no longer contributes an alias.
	if _, err := db.Exec(db.Rebind(`UPDATE upstream_credentials SET enabled=false WHERE id=?`), kid); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(db.Rebind(`UPDATE token_routes SET enabled=false WHERE id=?`), route); err != nil {
		t.Fatal(err)
	}
	models = read()
	if models["coding"] != nil {
		t.Fatal("disabled route contributed public alias")
	}
	if models["actual-coder"]["supplySources"].([]marketplaceModelSource)[0].Enabled {
		t.Fatal("disabled credential appears enabled")
	}
}

func TestMarketplaceDirectSupplySources(t *testing.T) {
	db, _ := setupStatsSQLiteTest(t)
	checkMarketplaceSupplySources(t, db.DB)
}

func TestMarketplaceAccountSupplyKinds(t *testing.T) {
	db, _ := setupStatsSQLiteTest(t)
	_, account, _, _ := seedModelsSurfacesFixture(t, db)
	if _, err := db.Exec(`UPDATE accounts SET oauth_provider='fixture-oauth' WHERE id=?`, account); err != nil {
		t.Fatal(err)
	}
	models, err := (&statsHandler{db: db.DB}).buildMarketplaceModels()
	if err != nil {
		t.Fatal(err)
	}
	for _, model := range models {
		if model["name"] != "gpt-4o" {
			continue
		}
		sources := model["supplySources"].([]marketplaceModelSource)
		if len(sources) != 1 || sources[0].Kind != "oauth" || sources[0].ID != account || sources[0].Evidence != "availability" {
			t.Fatalf("account source = %#v", sources)
		}
		return
	}
	t.Fatal("missing OAuth supply model")
}
