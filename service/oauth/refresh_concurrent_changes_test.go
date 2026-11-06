package oauth

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestAccountRefreshDoesNotOverwriteConcurrentChanges(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, change := range []string{"disable", "replace", "options", "delete", "fence"} {
			t.Run(dialect+"/"+change, func(t *testing.T) {
				var db *store.DB
				var cleanup func()
				if dialect == store.DialectPostgres {
					db, cleanup = setupPostgresRouteUnitDB(t)
					pgtest.Reset(t, db.DB)
					if err := store.AutoMigrate(db); err != nil {
						t.Fatal(err)
					}
				} else {
					db, cleanup = setupTestDB(t)
				}
				defer cleanup()
				extra := `{"oauth":{"provider":"codex","accountId":"account-guard","accountKey":"account-guard","refreshToken":"synthetic-old-refresh"}}`
				id := insertCodexTestAccount(t, db, "account-guard", extra)
				claims := codexJWTClaims{}
				claims.Auth.ChatGPTAccountID = "account-guard"
				idToken := makeCodexIDToken(t, claims)
				server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
					var err error
					switch change {
					case "disable":
						_, err = db.Exec(`UPDATE accounts SET status='disabled' WHERE id=?`, id)
					case "replace":
						_, err = db.Exec(`UPDATE accounts SET access_token='synthetic-replacement',extra_config=? WHERE id=?`, `{"oauth":{"provider":"codex","accountId":"account-guard","refreshToken":"synthetic-replacement-refresh"}}`, id)
					case "options":
						_, err = db.Exec(`UPDATE accounts SET extra_config=? WHERE id=?`, `{"operatorOption":true,"oauth":{"provider":"codex","accountId":"account-guard","refreshToken":"synthetic-old-refresh"}}`, id)
					case "delete":
						_, err = db.Exec(`DELETE FROM accounts WHERE id=?`, id)
					case "fence":
						_, err = db.Exec(`UPDATE credential_refresh_leases SET expires_at=0 WHERE lease_key=?`, fmt.Sprintf("account:%d", id))
					}
					if err != nil {
						t.Error(err)
					}
					return 200, codexTokenResponse{AccessToken: "synthetic-stale-refreshed", RefreshToken: "synthetic-stale-rotated", IDToken: idToken, ExpiresIn: 3600}
				})
				defer server.Close()
				withCodexTokenURLSwap(t, server.URL)
				result := doRefreshAccessToken(id)
				if result.Err == nil || result.AccessToken != "" {
					t.Fatal("refresh overwrote a concurrent change or published an unverified token")
				}
				if change != "delete" {
					var current store.Account
					if err := db.Get(&current, `SELECT * FROM accounts WHERE id=?`, id); err != nil {
						t.Fatal(err)
					}
					if current.AccessToken == "synthetic-stale-refreshed" || change == "disable" && current.Status != "disabled" {
						t.Fatal("concurrent account state was lost")
					}
				}
			})
		}
	}
}

func TestAccountRefreshDoesNotPublishConfigurationChangedAtCommit(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	id := insertCodexTestAccount(t, db, "account-guard", `{"oauth":{"provider":"codex","accountId":"account-guard","refreshToken":"synthetic-old-refresh"}}`)
	if _, err := db.Exec(`CREATE TRIGGER change_refresh_config AFTER UPDATE OF access_token ON accounts
		WHEN NEW.access_token='synthetic-guard-access'
		BEGIN UPDATE accounts SET extra_config='{"operatorChange":true}' WHERE id=NEW.id; END`); err != nil {
		t.Fatal(err)
	}
	claims := codexJWTClaims{}
	claims.Auth.ChatGPTAccountID = "account-guard"
	idToken := makeCodexIDToken(t, claims)
	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		return 200, codexTokenResponse{AccessToken: "synthetic-guard-access", RefreshToken: "synthetic-new-refresh", IDToken: idToken, ExpiresIn: 3600}
	})
	defer server.Close()
	withCodexTokenURLSwap(t, server.URL)
	result := doRefreshAccessToken(id)
	if result.Err == nil || result.ExtraConfig != nil || result.AccessToken != "" {
		t.Fatal("refresh published a configuration no longer present in storage")
	}
}
