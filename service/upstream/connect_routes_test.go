package upstream_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestConnectPreservesRouteOwnership(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"data":[{"id":"model"}]}`) }))
	defer server.Close()
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			t.Run("existing-route-without-group", func(t *testing.T) {
				db := connectDatabase(t, dialect)
				var id int64
				if err := db.Get(&id, db.Rebind(`INSERT INTO token_routes(model_pattern,display_name,route_mode,routing_strategy,enabled) VALUES ('model','Model label','pattern','round_robin',?) RETURNING id`), false); err != nil {
					t.Fatal(err)
				}
				out, err := upstream.Connect(t.Context(), db.DB, connectInput(server.URL))
				if err != nil {
					t.Fatal(err)
				}
				if out.RouteCount != 1 {
					t.Fatal("expected one associated route")
				}
				var count int
				var enabled bool
				var strategy, label string
				if err := db.QueryRowx(db.Rebind(`SELECT enabled,routing_strategy,display_name FROM token_routes WHERE id=?`), id).Scan(&enabled, &strategy, &label); err != nil {
					t.Fatal(err)
				}
				if enabled || strategy != "round_robin" || label != "Model label" {
					t.Fatal("existing route changed")
				}
				if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM upstream_route_groups WHERE route_id=?`), id); err != nil || count != 1 {
					t.Fatalf("group association %d err=%v", count, err)
				}
			})
			t.Run("alias-conflict", func(t *testing.T) {
				db := connectDatabase(t, dialect)
				if _, err := db.Exec(db.Rebind(`INSERT INTO token_routes(model_pattern,display_name,route_mode,enabled) VALUES ('another','model','pattern',?)`), true); err != nil {
					t.Fatal(err)
				}
				before := connectCounts(t, db)
				if _, err := upstream.Connect(t.Context(), db.DB, connectInput(server.URL)); err == nil {
					t.Fatal("alias conflict accepted")
				}
				if after := connectCounts(t, db); !reflect.DeepEqual(before, after) {
					t.Fatalf("alias conflict left graph %v -> %v", before, after)
				}
			})
			t.Run("manual-group", func(t *testing.T) {
				db := connectDatabase(t, dialect)
				input := connectInput(server.URL)
				if _, err := upstream.Connect(t.Context(), db.DB, input); err != nil {
					t.Fatal(err)
				}
				var member int64
				if err := db.Get(&member, `SELECT id FROM upstream_group_items`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(db.Rebind(`UPDATE upstream_groups SET mode='manual',active_item_id=?`), member); err != nil {
					t.Fatal(err)
				}
				input.Channel.Name = "Second"
				if _, err := upstream.Connect(t.Context(), db.DB, input); err != nil {
					t.Fatal(err)
				}
				var actual int64
				var mode string
				if err := db.QueryRow(`SELECT mode,active_item_id FROM upstream_groups`).Scan(&mode, &actual); err != nil {
					t.Fatal(err)
				}
				if mode != "manual" || actual != member {
					t.Fatal("manual selection changed")
				}
			})
		})
	}
}

func TestConnectNativeWithoutCatalogUsesPreset(t *testing.T) {
	db := connectDatabase(t, store.DialectSQLite)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("native action without catalog was probed") }))
	defer server.Close()
	input := connectInput(server.URL)
	input.Channel.Endpoints = store.DirectEndpoints{SystemOne: &store.DirectEndpoint{URL: server.URL + "/systemone", Auth: store.DirectAuthBearer}}
	input.RecommendedModels = []string{"jev-fixture"}
	out, err := upstream.Connect(t.Context(), db.DB, input)
	if err != nil || out.Discovery.Status != "preset" || out.ModelCount != 1 {
		t.Fatalf("native preset result=%+v err=%v", out, err)
	}
}
