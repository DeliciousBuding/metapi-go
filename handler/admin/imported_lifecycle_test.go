package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/backup"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func lifecycleTestDB(t *testing.T, dialect string) *store.DB {
	t.Helper()
	dsn := ":memory:"
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
	t.Cleanup(func() { _ = db.Close() })
	if dialect == store.DialectPostgres {
		pgtest.Reset(t, db.DB)
	}
	if err = store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func lifecycleIDs(t *testing.T, db *store.DB) map[upstream.Kind]int64 {
	t.Helper()
	var channel, model, credential, grant, group, member, route int64
	err := db.QueryRow(`SELECT m.channel_id,m.id,k.id,g.id,i.group_id,i.id,rg.route_id FROM upstream_models m JOIN upstream_grants g ON g.model_id=m.id JOIN upstream_credentials k ON k.id=g.credential_id JOIN upstream_group_items i ON i.grant_id=g.id JOIN upstream_route_groups rg ON rg.group_id=i.group_id ORDER BY m.channel_id LIMIT 1`).Scan(&channel, &model, &credential, &grant, &group, &member, &route)
	if err != nil {
		t.Fatal(err)
	}
	return map[upstream.Kind]int64{upstream.KindChannel: channel, upstream.KindModel: model, upstream.KindCredential: credential, upstream.KindGrant: grant, upstream.KindGroup: group, upstream.KindMember: member, upstream.KindRoute: route}
}

func lifecycleRequest(mux http.Handler, method, path string) *httptest.ResponseRecorder {
	out := httptest.NewRecorder()
	mux.ServeHTTP(out, httptest.NewRequest(method, path, nil))
	return out
}

func lifecyclePreview(t *testing.T, mux http.Handler, path string) upstream.DeletionPreview {
	t.Helper()
	out := lifecycleRequest(mux, "GET", path+"/deletion-preview")
	if out.Code != 200 {
		t.Fatalf("preview: %d %s", out.Code, out.Body.String())
	}
	var preview upstream.DeletionPreview
	if err := json.Unmarshal(out.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Revision) != 64 || preview.AffectedRouteIDs == nil {
		t.Fatalf("invalid preview: %s", out.Body.String())
	}
	if strings.Contains(out.Body.String(), "fixture-key") {
		t.Fatal("preview exposed credential")
	}
	return preview
}

func TestUpstreamLifecycleDeleteAndReimport(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, kind := range []upstream.Kind{upstream.KindChannel, upstream.KindModel, upstream.KindCredential, upstream.KindGrant, upstream.KindGroup, upstream.KindMember, upstream.KindRoute} {
				t.Run(string(kind), func(t *testing.T) {
					db := lifecycleTestDB(t, dialect)
					if _, err := backup.ImportOctopusV5(db, []byte(backupsvcFixtureV5), "lifecycle"); err != nil {
						t.Fatal(err)
					}
					ids := lifecycleIDs(t, db)
					id := ids[kind]
					mux := chi.NewRouter()
					RegisterUpstreamLifecycleRoutes(mux, db.DB)
					RegisterTokenRoutesWithDeps(mux, db.DB, TokenRoutesDeps{})
					path := fmt.Sprintf("/api/imported-upstreams/%ss/%d", kind, id)
					if kind == upstream.KindChannel {
						path = fmt.Sprintf("/api/imported-upstreams/%d", id)
					}
					if kind == upstream.KindRoute {
						path = fmt.Sprintf("/api/routes/%d", id)
					}
					var originalMappings int
					if err := db.Get(&originalMappings, `SELECT COUNT(*) FROM external_source_ids`); err != nil {
						t.Fatal(err)
					}
					if _, err := db.Exec(db.Rebind(`UPDATE upstream_groups SET mode='manual',active_item_id=? WHERE id=?`), ids[upstream.KindMember], ids[upstream.KindGroup]); err != nil {
						t.Fatal(err)
					}
					previous := config.RuntimeSafe()
					config.SetRuntime(&config.RuntimeSettings{RoutingFallbackUnitCost: 1})
					defer config.SetRuntime(previous)
					defer routing.SetGlobalCache(nil)
					router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{TokenRouterCacheTtlMs: 60000}, nil, nil)
					policy := routing.EmptyDownstreamRoutingPolicy
					policy.RequiredUpstreamProtocol = routing.UpstreamProtocolChat
					if selected, err := router.SelectChannel(t.Context(), "client-model", policy); err != nil || selected == nil {
						t.Fatalf("initial route: %v %v", selected, err)
					}
					preview := lifecyclePreview(t, mux, path)
					if preview.Kind != kind || preview.ID != id || len(preview.AffectedRouteIDs) != 1 || preview.AffectedRouteIDs[0] != ids[upstream.KindRoute] {
						t.Fatalf("wrong impact: %+v", preview)
					}
					if preview.RequiresCascade {
						for _, query := range []string{"", "?cascade=true", "?expectedRevision=" + preview.Revision, "?cascade=true&expectedRevision=stale"} {
							if out := lifecycleRequest(mux, "DELETE", path+query); out.Code != 409 {
								t.Fatalf("confirmation bypass: %s %d %s", query, out.Code, out.Body.String())
							}
						}
					}
					out := lifecycleRequest(mux, "DELETE", path+"?cascade=true&expectedRevision="+preview.Revision)
					if out.Code != 200 {
						t.Fatalf("delete: %d %s", out.Code, out.Body.String())
					}
					if out := lifecycleRequest(mux, "GET", path+"/deletion-preview"); out.Code != 404 {
						t.Fatalf("deleted root remains: %d %s", out.Code, out.Body.String())
					}
					if selected, err := router.SelectChannel(t.Context(), "client-model", policy); err != nil || selected != nil {
						t.Fatalf("manual source deletion fell back or stayed cached: %v %v", selected, err)
					}
					if kind != upstream.KindGroup && kind != upstream.KindRoute {
						var active int64
						if err := db.Get(&active, db.Rebind(`SELECT active_item_id FROM upstream_groups WHERE id=?`), ids[upstream.KindGroup]); err != nil || active != 0 {
							t.Fatalf("deleted active member not cleared: %d %v", active, err)
						}
					}
					for i := 0; i < 2; i++ {
						if _, err := backup.ImportOctopusV5(db, []byte(backupsvcFixtureV5), "lifecycle"); err != nil {
							t.Fatalf("reimport %d: %v", i, err)
						}
					}
					var mappings int
					if err := db.Get(&mappings, `SELECT COUNT(*) FROM external_source_ids`); err != nil || mappings != originalMappings {
						t.Fatalf("mapping closure not rebuilt idempotently: %d vs %d, %v", mappings, originalMappings, err)
					}
					routing.InvalidateCache()
					if selected, err := router.SelectChannel(t.Context(), "client-model", policy); err != nil || selected == nil {
						t.Fatalf("reimport did not rebuild executable route: %v %v", selected, err)
					}
				})
			}
		})
	}
}

func TestUpstreamLifecycleConfirmationGrowthAndRollback(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := lifecycleTestDB(t, dialect)
			if _, err := backup.ImportOctopusV5(db, []byte(backupsvcFixtureV5), "lifecycle"); err != nil {
				t.Fatal(err)
			}
			ids := lifecycleIDs(t, db)
			mux := chi.NewRouter()
			RegisterUpstreamLifecycleRoutes(mux, db.DB)
			path := fmt.Sprintf("/api/imported-upstreams/%d", ids[upstream.KindChannel])
			before := lifecyclePreview(t, mux, path)
			var nativeModel int64
			if err := db.Get(&nativeModel, db.Rebind(`INSERT INTO upstream_models(origin_key,source_id,channel_id,name,enabled) VALUES ('native:local',-500,?,'local-model',?) RETURNING id`), ids[upstream.KindChannel], true); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(db.Rebind(`UPDATE upstream_models SET source_id=? WHERE id=?`), -nativeModel, nativeModel); err != nil {
				t.Fatal(err)
			}
			if out := lifecycleRequest(mux, "DELETE", path+"?cascade=true&expectedRevision="+before.Revision); out.Code != 409 {
				t.Fatalf("new child escaped confirmation: %d %s", out.Code, out.Body.String())
			}
			after := lifecyclePreview(t, mux, path)
			if after.Revision == before.Revision || after.Counts["models"] != before.Counts["models"]+1 {
				t.Fatalf("native descendant absent from impact: %+v", after)
			}
			// Deliberately map the same child through both supported source aliases.
			for i, alias := range []string{"channel_models", "upstream_models"} {
				if _, err := db.Exec(db.Rebind(`INSERT INTO external_source_ids(origin_key,entity_type,source_id,target_id) VALUES (?,?,?,?)`), "other-origin", alias, 900+i, nativeModel); err != nil {
					t.Fatal(err)
				}
			}
			confirmed := lifecyclePreview(t, mux, path)
			tx, err := db.BeginTxx(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if err = upstream.LockLifecycleTx(t.Context(), tx); err != nil {
				t.Fatal(err)
			}
			plan, err := upstream.PlanDeletionTx(t.Context(), tx, []upstream.Reference{{Kind: upstream.KindChannel, ID: ids[upstream.KindChannel]}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err = upstream.ApplyDeletionTx(t.Context(), tx, plan); err != nil {
				t.Fatal(err)
			}
			if err = tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			if rolled := lifecyclePreview(t, mux, path); rolled.Revision != confirmed.Revision {
				t.Fatal("rollback left partial graph or source mapping changes")
			}
			if out := lifecycleRequest(mux, "DELETE", path+"?cascade=true&expectedRevision="+confirmed.Revision); out.Code != 200 {
				t.Fatal(out.Body.String())
			}
			var orphanMappings int
			if err = db.Get(&orphanMappings, `SELECT COUNT(*) FROM external_source_ids WHERE origin_key='other-origin'`); err != nil || orphanMappings != 0 {
				t.Fatalf("cross-origin alias mappings orphaned: %d %v", orphanMappings, err)
			}
		})
	}
}

func TestUpstreamLifecycleRouteKeysRemainClosed(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db := lifecycleTestDB(t, dialect)
			if _, err := backup.ImportOctopusV5(db, []byte(backupsvcFixtureV5), "lifecycle"); err != nil {
				t.Fatal(err)
			}
			ids := lifecycleIDs(t, db)
			route := ids[upstream.KindRoute]
			if _, err := db.Exec(db.Rebind(`INSERT INTO downstream_api_keys(name,key,allowed_route_ids) VALUES ('only','only-key',?),('mixed','mixed-key',?)`), fmt.Sprintf("[%d]", route), fmt.Sprintf("[%d,999999]", route)); err != nil {
				t.Fatal(err)
			}
			mux := chi.NewRouter()
			RegisterUpstreamLifecycleRoutes(mux, db.DB)
			path := fmt.Sprintf("/api/imported-upstreams/groups/%d", ids[upstream.KindGroup])
			preview := lifecyclePreview(t, mux, path)
			if preview.Counts["downstreamKeys"] != 2 {
				t.Fatalf("missing affected keys: %+v", preview)
			}
			out := lifecycleRequest(mux, "DELETE", path+"?cascade=true&expectedRevision="+preview.Revision)
			if out.Code != 200 || !strings.Contains(out.Body.String(), "downstreamKeysWithOnlyDeletedRoutes") {
				t.Fatalf("delete omitted last-scope notice: %s", out.Body.String())
			}
			var only, mixed string
			if err := db.Get(&only, `SELECT allowed_route_ids FROM downstream_api_keys WHERE name='only'`); err != nil {
				t.Fatal(err)
			}
			if err := db.Get(&mixed, `SELECT allowed_route_ids FROM downstream_api_keys WHERE name='mixed'`); err != nil {
				t.Fatal(err)
			}
			if only != fmt.Sprintf("[%d]", route) || mixed != "[999999]" {
				t.Fatalf("key scope widened or failed to prune: %s %s", only, mixed)
			}
			if _, err := backup.ImportOctopusV5(db, []byte(backupsvcFixtureV5), "lifecycle"); err != nil {
				t.Fatal(err)
			}
			previous := config.RuntimeSafe()
			config.SetRuntime(&config.RuntimeSettings{})
			defer config.SetRuntime(previous)
			defer routing.SetGlobalCache(nil)
			router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil)
			policy := routing.EmptyDownstreamRoutingPolicy
			policy.RequiredUpstreamProtocol = routing.UpstreamProtocolChat
			policy.AllowedRouteIDs = []int64{route}
			if selected, err := router.SelectChannel(t.Context(), "client-model", policy); err != nil || selected != nil {
				t.Fatalf("recreated route inherited dead key scope: %v %v", selected, err)
			}
		})
	}
}

func TestUpstreamLifecycleReplacementRevisionHTTP(t *testing.T) {
	for _, kind := range []string{"octopus", "axonhub"} {
		t.Run(kind, func(t *testing.T) {
			db := lifecycleTestDB(t, store.DialectSQLite)
			full := []byte(backupsvcFixtureV5)
			if kind == "axonhub" {
				full = []byte(backupFixtureAxonHubV14)
			}
			h := &backupHandler{db: db.DB}
			commit := func(raw []byte, revision string) *httptest.ResponseRecorder {
				req := httptest.NewRequest("POST", "/api/settings/backup/import", strings.NewReader(string(raw)))
				req.Header.Set("X-External-Origin-Key", "lifecycle")
				req.Header.Set("X-Octopus-Replace-Origin", "true")
				req.Header.Set("X-AxonHub-Replace-Origin", "true")
				req.Header.Set("X-External-Replacement-Revision", revision)
				out := httptest.NewRecorder()
				h.importBackup(out, req)
				return out
			}
			if out := commit(full, ""); out.Code != 200 {
				t.Fatal(out.Body.String())
			}
			var channel int64
			if err := db.Get(&channel, `SELECT id FROM upstream_channels WHERE source_id=12`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(db.Rebind(`INSERT INTO upstream_models(origin_key,source_id,channel_id,name,enabled) VALUES ('native:local',-900,?,'local-model',?)`), channel, true); err != nil {
				t.Fatal(err)
			}
			var reduced []byte
			if kind == "octopus" {
				d, err := backup.ParseOctopusV5(full)
				if err != nil {
					t.Fatal(err)
				}
				d.Channels = d.Channels[:1]
				d.Credentials = d.Credentials[:1]
				d.Models = d.Models[:1]
				d.Grants = d.Grants[:1]
				d.GroupItems = d.GroupItems[:1]
				reduced, _ = json.Marshal(d)
			} else {
				var obj map[string]any
				_ = json.Unmarshal(full, &obj)
				channels := obj["channels"].([]any)
				obj["channels"] = append(channels[:1], channels[2:]...)
				reduced, _ = json.Marshal(obj)
			}
			previewReq := httptest.NewRequest("POST", "/api/settings/backup/import/preview", strings.NewReader(string(reduced)))
			previewReq.Header.Set("X-External-Origin-Key", "lifecycle")
			out := httptest.NewRecorder()
			h.previewBackupImport(out, previewReq)
			if out.Code != 200 {
				t.Fatal(out.Body.String())
			}
			var preview struct {
				Plan struct {
					RemovalImpact upstream.DeletionPreview `json:"removalImpact"`
				} `json:"plan"`
			}
			if err := json.Unmarshal(out.Body.Bytes(), &preview); err != nil {
				t.Fatal(err)
			}
			if len(preview.Plan.RemovalImpact.Revision) != 64 || preview.Plan.RemovalImpact.Counts["models"] != 2 {
				t.Fatalf("native deletion impact absent: %s", out.Body.String())
			}
			for _, revision := range []string{"", "stale"} {
				if out := commit(reduced, revision); out.Code != 409 {
					t.Fatalf("unreviewed native deletion: %d %s", out.Code, out.Body.String())
				}
			}
			if out := commit(reduced, preview.Plan.RemovalImpact.Revision); out.Code != 200 {
				t.Fatalf("reviewed replacement failed: %d %s", out.Code, out.Body.String())
			}
		})
	}
}
