package upstream_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

type oauthCatalogFixture struct {
	db                    *store.DB
	id                    int64
	catalogCalls, refresh atomic.Int32
	resolverCalls         atomic.Int32
	afterExchange         func() error
}

func setupOAuthCatalog(t *testing.T, dialect string) *oauthCatalogFixture {
	t.Helper()
	f := &oauthCatalogFixture{db: connectDatabase(t, dialect)}
	var initialized atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !initialized.Load() {
			if r.Header.Get("Authorization") != "Bearer fixture-secret" {
				t.Error("connect authentication changed")
			}
			_, _ = fmt.Fprint(w, `{"data":[{"id":"existing-model"}]}`)
			return
		}
		f.catalogCalls.Add(1)
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer synthetic-fresh-catalog" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"error":{"message":"expired synthetic credential"}}`)
			return
		}
		_, _ = fmt.Fprint(w, `{"data":[{"id":"existing-model"},{"id":"new-model"}]}`)
	}))
	t.Cleanup(server.Close)
	out, err := upstream.Connect(t.Context(), f.db.DB, connectInput(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	f.id = out.ID
	if _, err := f.db.Exec(`UPDATE upstream_channels SET provider='codex' WHERE id=?`, f.id); err != nil {
		t.Fatal(err)
	}
	state := store.DirectOAuthState{RefreshToken: "synthetic-catalog-refresh", ClientID: "synthetic-source-client", AccountID: "synthetic-catalog-account", ExpiresAt: 1}
	if _, err := f.db.Exec(`UPDATE upstream_credentials SET kind='oauth',secret='synthetic-cached-old',oauth_state=? WHERE channel_id=?`, state, f.id); err != nil {
		t.Fatal(err)
	}
	initialized.Store(true)
	// Only the external token exchange is controlled. The real credential
	// resolver, record lease/CAS and model-catalog HTTP path all execute.
	definition := oauth.GetProviderDefinition("codex")
	original := definition.RefreshAccessToken
	t.Cleanup(func() { definition.RefreshAccessToken = original })
	definition.RefreshAccessToken = func(ctx context.Context, input oauth.RefreshTokenInput) (*oauth.TokenSet, error) {
		f.refresh.Add(1)
		if input.RefreshToken != "synthetic-catalog-refresh" || input.ClientID != "synthetic-source-client" {
			t.Error("catalog refresh lost the issuing client or refresh material")
		}
		if f.afterExchange != nil {
			if err := f.afterExchange(); err != nil {
				return nil, err
			}
		}
		return &oauth.TokenSet{AccessToken: "synthetic-fresh-catalog", RefreshToken: "synthetic-catalog-rotated", AccountID: "synthetic-catalog-account", TokenExpiresAt: time.Now().Add(time.Hour).UnixMilli()}, nil
	}
	return f
}

func (f *oauthCatalogFixture) resolve(ctx context.Context, id int64, proxyURL *string) (string, error) {
	f.resolverCalls.Add(1)
	result, err := oauth.ResolveDirectCredential(ctx, f.db.DB, id, proxyURL, false)
	if err != nil {
		return "", err
	}
	return result.AccessToken, nil
}

func TestProviderCatalogUsesCurrentOAuthAndRevalidatesAfterExchange(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, mode := range []string{"refresh-and-reuse", "changed-during-exchange", "rejected-token"} {
			t.Run(dialect+"/"+mode, func(t *testing.T) {
				fixture := setupOAuthCatalog(t, dialect)
				var endpoints store.DirectEndpoints
				if err := fixture.db.Get(&endpoints, fixture.db.Rebind(`SELECT endpoint_config FROM upstream_channels WHERE id=?`), fixture.id); err != nil {
					t.Fatal(err)
				}
				source := strings.Replace(providerCatalogSource, `credentialKinds:["api_key"]`, `credentialKinds:["oauth"]`, 1)
				source = strings.Replace(source, `models:value.items`, `models:value.data.map(item=>item.id)`, 1)
				if _, err := channelplugins.Save(t.Context(), fixture.db.DB, "catalog", source, true, 0); err != nil {
					t.Fatal(err)
				}
				binding := &store.DirectProviderBinding{Key: "catalog", Protocols: []string{"chat"}, Catalog: &store.DirectCatalogEndpoint{URL: strings.TrimSuffix(endpoints.Chat.URL, "/chat/completions") + "/models", Auth: store.DirectAuthBearer}}
				config := store.DirectRequestConfig{Provider: binding}
				if _, err := fixture.db.Exec(fixture.db.Rebind(`UPDATE upstream_channels SET request_config=? WHERE id=?`), config, fixture.id); err != nil {
					t.Fatal(err)
				}
				if mode == "changed-during-exchange" {
					fixture.afterExchange = func() error {
						_, err := fixture.db.Exec(fixture.db.Rebind(`UPDATE upstream_channels SET custom_header=? WHERE id=?`), `[{"header_key":"X-Catalog-Config","header_value":"changed"}]`, fixture.id)
						return err
					}
				}
				var resolve upstream.DiscoveryCredentialResolver = fixture.resolve
				if mode == "rejected-token" {
					resolve = func(context.Context, int64, *string) (string, error) {
						return "", errors.New("synthetic rejected token")
					}
				}
				preview, err := upstream.PreviewCatalogRefresh(t.Context(), fixture.db.DB, fixture.id, resolve)
				if mode != "refresh-and-reuse" {
					if err == nil || fixture.catalogCalls.Load() != 0 {
						t.Fatalf("stale/rejected credential sent catalog request: calls=%d %v", fixture.catalogCalls.Load(), err)
					}
					return
				}
				if err != nil || fixture.refresh.Load() != 1 || fixture.catalogCalls.Load() != 1 || preview.UpstreamCount != 2 {
					t.Fatalf("provider OAuth refresh not used: %+v refresh=%d calls=%d %v", preview, fixture.refresh.Load(), fixture.catalogCalls.Load(), err)
				}
				if _, err := upstream.ApplyCatalogRefresh(t.Context(), fixture.db.DB, fixture.id, preview.BasisToken, fixture.resolve); err != nil {
					t.Fatal(err)
				}
				if fixture.refresh.Load() != 1 || fixture.catalogCalls.Load() != 2 {
					t.Fatalf("valid token was not reused: refresh=%d calls=%d", fixture.refresh.Load(), fixture.catalogCalls.Load())
				}
			})
		}
	}
}

func TestCatalogRefreshOAuthUsesCurrentCredentialLifecycle(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			fixture := setupOAuthCatalog(t, dialect)
			preview, err := upstream.PreviewCatalogRefresh(t.Context(), fixture.db.DB, fixture.id, fixture.resolve)
			if err != nil {
				t.Fatalf("OAuth catalog did not refresh before HTTP discovery: %v", err)
			}
			if len(preview.Added) != 1 || preview.Added[0] != "new-model" {
				t.Fatal("OAuth catalog did not return the observed model diff")
			}
			if _, err := upstream.ApplyCatalogRefresh(t.Context(), fixture.db.DB, fixture.id, preview.BasisToken, fixture.resolve); err != nil {
				t.Fatal(err)
			}
			if fixture.refresh.Load() != 1 || fixture.resolverCalls.Load() != 2 || fixture.catalogCalls.Load() != 2 {
				t.Fatalf("catalog lifecycle counts: exchange=%d resolver=%d HTTP=%d", fixture.refresh.Load(), fixture.resolverCalls.Load(), fixture.catalogCalls.Load())
			}
			var token string
			var state store.DirectOAuthState
			if err := fixture.db.QueryRow(`SELECT secret,oauth_state FROM upstream_credentials WHERE channel_id=?`, fixture.id).Scan(&token, &state); err != nil {
				t.Fatal(err)
			}
			if token != "synthetic-fresh-catalog" || state.RefreshToken != "synthetic-catalog-rotated" {
				t.Fatal("catalog result bypassed durable token rotation")
			}
			var grants int
			if err := fixture.db.Get(&grants, `SELECT COUNT(*) FROM upstream_grants g JOIN upstream_models m ON m.id=g.model_id WHERE m.channel_id=? AND m.name='new-model'`, fixture.id); err != nil || grants != 0 {
				t.Fatal("new catalog models received unrequested credential access")
			}
		})
	}
}

func TestCatalogRefreshOAuthNeverFallsBackToStoredToken(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, failure := range []string{"no-resolver", "rejected", "disabled-during-exchange"} {
			t.Run(dialect+"/"+failure, func(t *testing.T) {
				fixture := setupOAuthCatalog(t, dialect)
				before := connectCounts(t, fixture.db)
				var resolve upstream.DiscoveryCredentialResolver = fixture.resolve
				switch failure {
				case "no-resolver":
					resolve = nil
				case "rejected":
					resolve = func(context.Context, int64, *string) (string, error) {
						return "", errors.New("synthetic-private-provider-error")
					}
				case "disabled-during-exchange":
					fixture.afterExchange = func() error {
						_, err := fixture.db.Exec(`UPDATE upstream_credentials SET enabled=? WHERE channel_id=?`, false, fixture.id)
						return err
					}
				}
				_, err := upstream.PreviewCatalogRefresh(t.Context(), fixture.db.DB, fixture.id, resolve)
				if err == nil || strings.Contains(err.Error(), "synthetic-private-") || fixture.catalogCalls.Load() != 0 {
					t.Fatal("OAuth catalog sent a stale token or exposed a provider failure")
				}
				if !reflect.DeepEqual(before, connectCounts(t, fixture.db)) {
					t.Fatal("failed credential resolution mutated the catalog graph")
				}
			})
		}
	}
}
