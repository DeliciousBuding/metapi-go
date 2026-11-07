package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/jmoiron/sqlx"
)

func TestOAuthPublicationRecoversStorageFailureWithoutReexchange(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, scope := range []string{"direct", "account"} {
			for _, acknowledgementLost := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/ack-lost=%t", dialect, scope, acknowledgementLost), func(t *testing.T) {
					db, id := familyCredentialFixture(t, dialect)
					var exchanges, commits atomic.Int32
					var invoke func() error
					if scope == "direct" {
						directClaudeEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
							exchanges.Add(1)
							_, _ = fmt.Fprint(w, `{"access_token":"synthetic-publication-access","refresh_token":"synthetic-publication-rotated","expires_in":3600}`)
						})
						invoke = func() error {
							result, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
							if err != nil {
								return err
							}
							if result == nil || result.AccessToken != "synthetic-publication-access" {
								return errors.New("direct publication not returned")
							}
							return nil
						}
					} else {
						old := config.GetSafe()
						cfg := *config.Get()
						cfg.CodexClientId = "synthetic-publication-client"
						config.Set(&cfg)
						t.Cleanup(func() { config.Set(old) })
						id = insertCodexTestAccount(t, db, "publication-account", `{"oauth":{"provider":"codex","accountKey":"publication-account","refreshToken":"synthetic-publication-rt"}}`)
						claims := codexJWTClaims{}
						claims.Auth.ChatGPTAccountID = "publication-account"
						idToken := makeCodexIDToken(t, claims)
						server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
							exchanges.Add(1)
							return 200, codexTokenResponse{AccessToken: "synthetic-publication-access", RefreshToken: "synthetic-publication-rotated", IDToken: idToken, ExpiresIn: 3600}
						})
						t.Cleanup(server.Close)
						withCodexTokenURLSwap(t, server.URL)
						invoke = func() error {
							var account store.Account
							if err := db.Get(&account, `SELECT * FROM accounts WHERE id=?`, id); err != nil {
								return err
							}
							result := refreshManagedAccount(context.Background(), db, account)
							if result.Err != nil {
								return result.Err
							}
							if result.AccessToken != "synthetic-publication-access" {
								return errors.New("account publication not returned")
							}
							return nil
						}
					}
					originalCommit := commitRefreshPublication
					commitRefreshPublication = func(tx *sqlx.Tx) error {
						if commits.Add(1) == 1 {
							if acknowledgementLost {
								if err := tx.Commit(); err != nil {
									return err
								}
							}
							return errors.New("synthetic database commit outcome unavailable")
						}
						return tx.Commit()
					}
					t.Cleanup(func() { commitRefreshPublication = originalCommit })
					if err := invoke(); err != nil {
						t.Errorf("publication did not recover: %v", err)
					}
					wantCommits := int32(2)
					if acknowledgementLost {
						wantCommits = 1
					}
					if exchanges.Load() != 1 || commits.Load() != wantCommits {
						t.Errorf("exchange=%d commits=%d want one exchange/%d commits", exchanges.Load(), commits.Load(), wantCommits)
					}
					var generation int64
					var state string
					if err := db.QueryRow(`SELECT generation,state FROM credential_refresh_families`).Scan(&generation, &state); err != nil {
						t.Fatal(err)
					}
					if generation != 1 || state != "active" {
						t.Errorf("recovered publication not atomic: generation=%d state=%s", generation, state)
					}
				})
			}
		}
	}
}
