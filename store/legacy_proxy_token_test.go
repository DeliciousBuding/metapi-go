package store

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

func migrationTestDB(t *testing.T, dialect string) *DB {
	t.Helper()
	if dialect == DialectSQLite {
		return openTestSQLite(t)
	}
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set")
	}
	admin, err := Open(DialectPostgres, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("root_key_%d", time.Now().UnixNano())
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`); _ = admin.Close() })
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := Open(DialectPostgres, u.String(), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestMigrateLegacyProxyToken(t *testing.T) {
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			for _, existing := range []bool{false, true} {
				t.Run(fmt.Sprint("existing=", existing), func(t *testing.T) {
					db := migrationTestDB(t, dialect)
					token := "sk-synthetic-old-root"
					if existing {
						if _, err := db.Exec(`INSERT INTO downstream_api_keys (name,key,enabled,supported_models,max_rpm) VALUES ('Existing',?,FALSE,'["limited"]',5)`, token); err != nil {
							t.Fatal(err)
						}
					}
					identity, _ := json.Marshal(map[string]any{"version": 2, "owner": DirectVideoTaskOwner("global", token, nil), "channelId": 81})
					if _, err := db.Exec(`INSERT INTO proxy_video_tasks (public_id,upstream_video_id,site_url,token_value,direct_identity) VALUES ('video_direct_upgrade','upstream','','',?)`, string(identity)); err != nil {
						t.Fatal(err)
					}
					if _, err := db.Exec(`INSERT INTO proxy_video_tasks (public_id,upstream_video_id,site_url,token_value,direct_identity) VALUES ('video_direct_unreadable','upstream','','','{invalid')`); err != nil {
						t.Fatal(err)
					}
					rt := &config.RuntimeSettings{ProxyToken: token}
					if err := MigrateLegacyProxyToken(db, rt, 29); err != nil {
						t.Fatal(err)
					}
					if rt.ProxyToken != "" {
						t.Fatal("runtime retained root credential")
					}
					var row struct {
						ID      int64  `db:"id"`
						Enabled bool   `db:"enabled"`
						Models  string `db:"supported_models"`
						RPM     int    `db:"max_rpm"`
					}
					if err := db.Get(&row, `SELECT id,enabled,supported_models,max_rpm FROM downstream_api_keys WHERE key=?`, token); err != nil {
						t.Fatal(err)
					}
					if existing {
						if row.Enabled || row.Models != `["limited"]` || row.RPM != 5 {
							t.Fatalf("existing policy changed: %+v", row)
						}
					} else if !row.Enabled || row.Models != `["*"]` || row.RPM != 29 {
						t.Fatalf("root access was not preserved: %+v", row)
					}
					var raw string
					if err := db.Get(&raw, `SELECT direct_identity FROM proxy_video_tasks WHERE public_id='video_direct_upgrade'`); err != nil {
						t.Fatal(err)
					}
					var moved map[string]any
					if err := json.Unmarshal([]byte(raw), &moved); err != nil {
						t.Fatal(err)
					}
					if moved["owner"] != DirectVideoTaskOwner("managed", token, &row.ID) || moved["channelId"] != float64(81) {
						t.Fatal("video identity was not preserved")
					}
					if err := db.Get(&raw, `SELECT direct_identity FROM proxy_video_tasks WHERE public_id='video_direct_unreadable'`); err != nil {
						t.Fatal(err)
					}
					if raw != "{invalid" {
						t.Fatal("unrelated unreadable identity changed")
					}
					if _, err := db.Exec(`DELETE FROM downstream_api_keys WHERE id=?`, row.ID); err != nil {
						t.Fatal(err)
					}
					rt.ProxyToken = token
					if err := MigrateLegacyProxyToken(db, rt, 29); err != nil {
						t.Fatal(err)
					}
					var count int
					if err := db.Get(&count, `SELECT COUNT(*) FROM downstream_api_keys`); err != nil {
						t.Fatal(err)
					}
					if count != 0 {
						t.Fatal("restart resurrected deleted token")
					}
				})
			}
			t.Run("rollback", func(t *testing.T) {
				db := migrationTestDB(t, dialect)
				if _, err := db.Exec(`ALTER TABLE proxy_video_tasks RENAME TO unavailable_tasks`); err != nil {
					t.Fatal(err)
				}
				rt := &config.RuntimeSettings{ProxyToken: "sk-synthetic-upgrade"}
				if err := MigrateLegacyProxyToken(db, rt, 0); err == nil {
					t.Fatal("migration succeeded without required ownership table")
				}
				var count int
				if err := db.Get(&count, `SELECT COUNT(*) FROM settings WHERE key=?`, LegacyProxyTokenMigrationKey); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatal("failed migration retained marker")
				}
				if err := db.Get(&count, `SELECT COUNT(*) FROM downstream_api_keys`); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatal("failed migration retained key")
				}
			})
			t.Run("fresh", func(t *testing.T) {
				db := migrationTestDB(t, dialect)
				rt := &config.RuntimeSettings{ProxyToken: config.DefaultProxyToken}
				if err := MigrateLegacyProxyToken(db, rt, 0); err != nil {
					t.Fatal(err)
				}
				var count int
				if err := db.Get(&count, `SELECT COUNT(*) FROM downstream_api_keys`); err != nil {
					t.Fatal(err)
				}
				if count != 0 {
					t.Fatal("default placeholder became a key")
				}
			})
		})
	}
}

func TestLoadRuntimeSettingsMigratesSavedRootBeforeEnvironment(t *testing.T) {
	hydrateTestDB(t)
	db := GetDB()
	if _, err := db.Exec(`INSERT INTO settings (key,value) VALUES ('proxy_token','"sk-synthetic-saved"')`); err != nil {
		t.Fatal(err)
	}
	cfg, rt := config.Load(map[string]string{"PROXY_TOKEN": "sk-synthetic-env"})
	if err := LoadRuntimeSettings(cfg, rt); err != nil {
		t.Fatal(err)
	}
	var saved, env, legacy int
	if err := db.Get(&saved, `SELECT COUNT(*) FROM downstream_api_keys WHERE key='sk-synthetic-saved'`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&env, `SELECT COUNT(*) FROM downstream_api_keys WHERE key='sk-synthetic-env'`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&legacy, `SELECT COUNT(*) FROM settings WHERE key='proxy_token'`); err != nil {
		t.Fatal(err)
	}
	if saved != 1 || env != 0 || legacy != 0 || rt.ProxyToken != "" {
		t.Fatal("saved root key did not replace the environment import value")
	}
	if _, err := db.Exec(`UPDATE downstream_api_keys SET enabled=FALSE`); err != nil {
		t.Fatal(err)
	}
	_, restarted := config.Load(map[string]string{"PROXY_TOKEN": "sk-synthetic-new-env"})
	if err := LoadRuntimeSettings(cfg, restarted); err != nil {
		t.Fatal(err)
	}
	var enabled bool
	if err := db.Get(&enabled, `SELECT enabled FROM downstream_api_keys WHERE key='sk-synthetic-saved'`); err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("restart re-enabled migrated token")
	}
	if err := db.Get(&saved, `SELECT COUNT(*) FROM downstream_api_keys`); err != nil {
		t.Fatal(err)
	}
	if saved != 1 {
		t.Fatal("restart imported a different environment key")
	}
}
