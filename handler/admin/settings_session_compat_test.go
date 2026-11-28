package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestSettingsSessionCompatibilitySurvivesUnrelatedEdit(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			var db *store.DB
			if dialect == store.DialectSQLite {
				db = sqlitetest.Open(t)
			} else {
				dsn := os.Getenv("PG_TEST_DSN")
				if dsn == "" {
					t.Skip("PG_TEST_DSN not set")
				}
				var err error
				db, err = store.Open(dialect, dsn, false)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { db.Close() })
				pgtest.Reset(t, db.DB)
				if err := store.AutoMigrate(db); err != nil {
					t.Fatal(err)
				}
			}
			prev, prevRt := config.GetSafe(), config.RuntimeSafe()
			t.Cleanup(func() { config.Set(prev); config.SetRuntime(prevRt) })
			cfg, rt := config.Load(map[string]string{
				"ACCOUNT_CREDENTIAL_SECRET":               "synthetic-session-compat-secret",
				"PROXY_SESSION_CHANNEL_CONCURRENCY_LIMIT": "2",
				"PROXY_SESSION_CHANNEL_QUEUE_WAIT_MS":     "1500",
			})
			config.Set(cfg)
			config.SetRuntime(rt)
			r := chi.NewRouter()
			RegisterSettingsRoutes(r, db, cfg)
			for _, payload := range []map[string]any{
				{"proxySessionChannelConcurrencyLimit": 7, "proxySessionChannelQueueWaitMs": 2600},
				{"codexUpstreamWebsocketEnabled": true},
			} {
				resp := doPutJSON(t, r, "/api/settings/runtime", payload)
				if resp.Code != http.StatusOK {
					t.Fatalf("PUT: %d %s", resp.Code, resp.Body.String())
				}
			}
			persisted := readAllPersistedSettings(t, db)
			_, restarted := config.Load(map[string]string{
				"PROXY_SESSION_CHANNEL_CONCURRENCY_LIMIT": "1",
				"PROXY_SESSION_CHANNEL_QUEUE_WAIT_MS":     "100",
			})
			store.ApplyRuntimeSettings(cfg, restarted, persisted)
			if restarted.ProxySessionChannelConcurrencyLimit != 7 || restarted.ProxySessionChannelQueueWaitMs != 2600 || !restarted.CodexUpstreamWebsocketEnabled {
				t.Fatalf("unrelated save/restart changed legacy values: limit=%d wait=%d ws=%t",
					restarted.ProxySessionChannelConcurrencyLimit, restarted.ProxySessionChannelQueueWaitMs, restarted.CodexUpstreamWebsocketEnabled)
			}
			config.SetRuntime(restarted)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/settings/runtime", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("GET: %d", rec.Code)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["proxySessionChannelConcurrencyLimit"] != float64(7) || body["proxySessionChannelQueueWaitMs"] != float64(2600) {
				t.Fatalf("legacy camelCase API fields changed: limit=%v wait=%v",
					body["proxySessionChannelConcurrencyLimit"], body["proxySessionChannelQueueWaitMs"])
			}
		})
	}
}
