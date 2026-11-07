package oauth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectRefreshDoesNotOverwriteChangedChannelAuthority(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, change := range []string{"provider", "channel", "endpoint", "fence"} {
			t.Run(dialect+"/"+change, func(t *testing.T) {
				var db *store.DB
				if dialect == store.DialectPostgres {
					var cleanup func()
					db, cleanup = setupPostgresRouteUnitDB(t)
					t.Cleanup(cleanup)
					pgtest.Reset(t, db.DB)
					if err := store.AutoMigrate(db); err != nil {
						t.Fatal(err)
					}
					seedDirectCredentialFixture(t, db)
				} else {
					db, _ = directCredentialFixture(t)
				}
				directClaudeEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
					var err error
					switch change {
					case "provider":
						_, err = db.Exec(`UPDATE upstream_channels SET provider='codex' WHERE id=1`)
					case "channel":
						_, err = db.Exec(`INSERT INTO upstream_channels(id,origin_key,source_id,name,dialect,provider,enabled,base_url,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex)
							SELECT 2,origin_key,2,'replacement-channel',dialect,provider,enabled,base_url,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex FROM upstream_channels WHERE id=1`)
						if err == nil {
							_, err = db.Exec(`UPDATE upstream_credentials SET channel_id=2 WHERE id=1`)
						}
					case "endpoint":
						_, err = db.Exec(`UPDATE upstream_channels SET endpoint_config=? WHERE id=1`, `{"messages":{"url":"https://replacement.invalid/v1/messages","auth":"bearer"}}`)
					case "fence":
						_, err = db.Exec(`UPDATE credential_refresh_leases SET expires_at=0 WHERE lease_key='direct:1'`)
					}
					if err != nil {
						t.Error(err)
					}
					_, _ = fmt.Fprint(w, `{"access_token":"synthetic-stale-authority","refresh_token":"synthetic-rotated","expires_in":3600}`)
				})
				result, err := ResolveDirectCredential(context.Background(), db.DB, 1, nil, false)
				if !errors.Is(err, ErrDirectCredentialChanged) || result != nil {
					t.Errorf("refresh published credentials under changed channel authority: %v", err)
				}
				var secret string
				if err := db.Get(&secret, `SELECT secret FROM upstream_credentials WHERE id=1`); err != nil {
					t.Fatal(err)
				}
				if secret != "fixture-old-access-secret" {
					t.Error("stale provider refresh overwrote changed channel credentials")
				}
			})
		}
	}
}

func TestDirectRefreshDoesNotPublishStateChangedAtCommit(t *testing.T) {
	db, id := directCredentialFixture(t)
	if _, err := db.Exec(`CREATE TRIGGER change_direct_refresh_state AFTER UPDATE OF secret ON upstream_credentials
		WHEN NEW.secret='synthetic-stale-authority'
		BEGIN UPDATE upstream_credentials SET oauth_state='{"refreshToken":"synthetic-replacement","clientId":"different-client"}' WHERE id=NEW.id; END`); err != nil {
		t.Fatal(err)
	}
	directClaudeEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"access_token":"synthetic-stale-authority","refresh_token":"synthetic-rotated","expires_in":3600}`)
	})
	result, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
	if !errors.Is(err, ErrDirectCredentialChanged) || result != nil {
		t.Fatal("refresh published a credential state superseded at commit")
	}
}
