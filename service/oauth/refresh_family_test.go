package oauth

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectOAuthSharedRefreshTokenAcrossRecordsIsConsumedOnce(t *testing.T) {
	db, id := directCredentialFixture(t)
	if _, err := db.Exec(`INSERT INTO upstream_credentials(id,origin_key,channel_id,source_id,name,secret,kind,oauth_state,enabled)
		SELECT 2,origin_key,channel_id,2,'mirror',secret,kind,oauth_state,enabled FROM upstream_credentials WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	started, release, duplicate := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	var calls atomic.Int32
	directClaudeEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) != 1 {
			close(duplicate)
			w.WriteHeader(400)
			_, _ = fmt.Fprint(w, `{"error":"invalid_grant"}`)
			return
		}
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		_, _ = fmt.Fprint(w, `{"access_token":"synthetic-family-access","refresh_token":"synthetic-family-rotated","expires_in":3600}`)
	})
	t.Cleanup(func() { unblock.Do(func() { close(release) }) })
	type outcome struct {
		result *DirectCredentialResult
		err    error
	}
	results := make(chan outcome, 2)
	go func() {
		result, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
		results <- outcome{result, err}
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("family's first exchange did not start")
	}
	go func() {
		result, err := ResolveDirectCredential(context.Background(), db.DB, 2, nil, false)
		results <- outcome{result, err}
	}()
	select {
	case <-duplicate:
	case <-time.After(150 * time.Millisecond):
	}
	unblock.Do(func() { close(release) })
	for range 2 {
		select {
		case got := <-results:
			if got.err != nil || got.result == nil || got.result.AccessToken != "synthetic-family-access" {
				t.Errorf("duplicated credential did not reuse the family publication: %v", got.err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("family refresh did not settle")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("same rotating token across IDs was exchanged %d times", calls.Load())
	}
}

func familyCredentialFixture(t *testing.T, dialect string) (*store.DB, int64) {
	t.Helper()
	dsn := ":memory:"
	if dialect == store.DialectPostgres {
		dsn = os.Getenv("PG_TEST_DSN")
		if dsn == "" {
			t.Skip("PG_TEST_DSN not set")
		}
	}
	return familyCredentialFixtureDSN(t, dialect, dsn)
}

func TestOAuthFamilyUsesIssuerClientScopeAcrossOrigins(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, sameClient := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/same-client=%t", dialect, sameClient), func(t *testing.T) {
				dsn := filepath.Join(t.TempDir(), "first-use.sqlite")
				if dialect == store.DialectPostgres {
					dsn = os.Getenv("PG_TEST_DSN")
					if dsn == "" {
						t.Skip("PG_TEST_DSN not set")
					}
				}
				db, id := familyCredentialFixtureDSN(t, dialect, dsn)
				first, err := loadDirectCredential(t.Context(), db.DB, id)
				if err != nil {
					t.Fatal(err)
				}
				second := first.State
				if !sameClient {
					second.ClientID = "synthetic-other-issuer-client"
				}
				if _, err := db.Exec(`INSERT INTO upstream_channels(id,origin_key,source_id,name,dialect,provider,enabled,base_url,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex)
					SELECT 2,'second-origin',source_id,'second',dialect,provider,enabled,base_url,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex FROM upstream_channels WHERE id=1`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.Exec(`INSERT INTO upstream_credentials(id,origin_key,channel_id,source_id,name,secret,kind,oauth_state,enabled)
					SELECT 2,'second-origin',2,source_id,'second',secret,kind,?,enabled FROM upstream_credentials WHERE id=1`, second); err != nil {
					t.Fatal(err)
				}
				var calls atomic.Int32
				directClaudeEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
					n := calls.Add(1)
					_, _ = fmt.Fprintf(w, `{"access_token":"synthetic-scoped-access-%d","refresh_token":"synthetic-scoped-rotated-%d","expires_in":3600}`, n, n)
				})
				secondDB, err := store.Open(dialect, dsn, false)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = secondDB.Close() })
				start := make(chan struct{})
				results := make(chan error, 2)
				for index, connection := range []*store.DB{db, secondDB} {
					credentialID := id
					if index == 1 {
						credentialID = 2
					}
					go func() {
						<-start
						result, err := ResolveDirectCredential(t.Context(), connection.DB, credentialID, nil, false)
						if err == nil && result == nil {
							err = ErrDirectCredentialUnavailable
						}
						results <- err
					}()
				}
				close(start)
				for range 2 {
					if err := <-results; err != nil {
						t.Error(err)
					}
				}
				want := int32(2)
				if sameClient {
					want = 1
				}
				if calls.Load() != want {
					t.Errorf("origins changed issuer scope: exchanges=%d want=%d", calls.Load(), want)
				}
				var families int
				if err := db.Get(&families, `SELECT COUNT(*) FROM credential_refresh_families`); err != nil || families != int(want) {
					t.Errorf("issuer/client families=%d want=%d err=%v", families, want, err)
				}
			})
		}
	}
}

func TestCopilotReusableTokenRecoversInterruptedIntent(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) { testCopilotReusableTokenRecovery(t, dialect) })
	}
}

func testCopilotReusableTokenRecovery(t *testing.T, dialect string) {
	t.Helper()
	db, id := familyCredentialFixture(t, dialect)
	state := store.DirectOAuthState{RefreshToken: "synthetic-durable-github-token", ClientID: copilotClientID, ExpiresAt: 1}
	if _, err := db.Exec(`UPDATE upstream_channels SET provider='github_copilot'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE upstream_credentials SET oauth_state=? WHERE id=?`, state, id); err != nil {
		t.Fatal(err)
	}
	family, err := acquireRefreshFamily(t.Context(), db.DB, string(ProviderCopilot), state.ClientID, state.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := family.Begin(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := family.Release(); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		calls.Add(1)
		return http.StatusOK, map[string]any{"token": "synthetic-copilot-access", "expires_at": time.Now().Add(time.Hour).Unix()}
	})
	t.Cleanup(server.Close)
	withCopilotEndpointSwap(t, "", "", server.URL, "")
	result, err := ResolveDirectCredential(t.Context(), db.DB, id, nil, false)
	if err != nil || result == nil || result.AccessToken != "synthetic-copilot-access" || calls.Load() != 1 {
		t.Fatalf("reusable GitHub token remained fenced after interruption: result=%v err=%v calls=%d", result, err, calls.Load())
	}
}

func familyCredentialFixtureDSN(t *testing.T, dialect, dsn string) (*store.DB, int64) {
	t.Helper()
	db, err := store.Open(dialect, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if dialect == store.DialectPostgres {
		pgtest.Reset(t, db.DB)
	}
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	seedDirectCredentialFixture(t, db)
	return db, 1
}

func TestOAuthFamilyTransportUncertaintySurvivesNewCalls(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db, id := familyCredentialFixture(t, dialect)
			var calls atomic.Int32
			directClaudeEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = conn.Close()
			})
			for range 2 {
				result, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
				if result != nil || !errors.Is(err, ErrOAuthRefreshUncertain) {
					t.Fatalf("uncertain exchange result=%v error=%v", result, err)
				}
			}
			if calls.Load() != 1 {
				t.Fatalf("transport uncertainty repeated the RT %d times", calls.Load())
			}
			var state string
			if db.Get(&state, `SELECT state FROM credential_refresh_families`) != nil || state != "uncertain" {
				t.Fatal("uncertainty was not persisted")
			}
		})
	}
}

func TestOAuthFamilyErrorWithNewTokenFieldsIsUncertain(t *testing.T) {
	db, id := directCredentialFixture(t)
	previous := retryBackoffFn
	retryBackoffFn = func(int) time.Duration { return 0 }
	t.Cleanup(func() { retryBackoffFn = previous })
	var calls atomic.Int32
	directClaudeEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(503)
		_, _ = fmt.Fprint(w, `{"error":"unavailable","access_token":"synthetic-issued-access","refresh_token":"synthetic-issued-refresh"}`)
	})
	for range 2 {
		result, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
		if result != nil || !errors.Is(err, ErrOAuthRefreshUncertain) {
			t.Errorf("contradictory error response was treated as a definite rejection: %v", err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("ambiguous error response repeated RT %d times", calls.Load())
	}
}

func TestOAuthFamilyRetiredTokenCannotBeReintroduced(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db, id := familyCredentialFixture(t, dialect)
			original, err := loadDirectCredential(context.Background(), db.DB, id)
			if err != nil {
				t.Fatal(err)
			}
			var calls atomic.Int32
			directClaudeEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = fmt.Fprint(w, `{"access_token":"synthetic-new-access","refresh_token":"synthetic-new-refresh","expires_in":3600}`)
			})
			if _, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false); err != nil {
				t.Fatal(err)
			}
			// Restore A after A→B, destroying the only cached successor. The
			// retired alias still forbids sending A again; require reauthorization.
			if _, err := db.Exec(`UPDATE upstream_credentials SET secret=?,oauth_state=? WHERE id=?`, original.Secret, original.RawOAuth, id); err != nil {
				t.Fatal(err)
			}
			result, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
			if result != nil || !errors.Is(err, ErrOAuthRefreshUncertain) || calls.Load() != 1 {
				t.Fatalf("retired ABA token was reused: calls=%d error=%v", calls.Load(), err)
			}
		})
	}
}

func TestOAuthFamilySharedAccountAndDirectRecords(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, first := range []string{"account", "direct"} {
			t.Run(dialect+"/"+first, func(t *testing.T) {
				db, id := familyCredentialFixture(t, dialect)
				previous := config.GetSafe()
				cfg := *config.Get()
				cfg.CodexClientId = "synthetic-family-client"
				config.Set(&cfg)
				t.Cleanup(func() { config.Set(previous) })
				if _, err := db.Exec(`UPDATE upstream_channels SET provider='codex'`); err != nil {
					t.Fatal(err)
				}
				state := store.DirectOAuthState{RefreshToken: "synthetic-shared-rt", ExpiresAt: 1, AccountID: "family-account"}
				if _, err := db.Exec(`UPDATE upstream_credentials SET oauth_state=? WHERE id=?`, state, id); err != nil {
					t.Fatal(err)
				}
				accountID := insertCodexTestAccount(t, db, "family-account", `{"operatorOption":true,"oauth":{"provider":"codex","accountKey":"family-account","accountId":"family-account","refreshToken":"synthetic-shared-rt"}}`)
				claims := codexJWTClaims{}
				claims.Auth.ChatGPTAccountID = "family-account"
				idToken := makeCodexIDToken(t, claims)
				var calls atomic.Int32
				server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
					calls.Add(1)
					return 200, codexTokenResponse{AccessToken: "synthetic-family-access", RefreshToken: "synthetic-family-rotated", IDToken: idToken, ExpiresIn: 3600}
				})
				t.Cleanup(server.Close)
				withCodexTokenURLSwap(t, server.URL)
				managed := func() {
					var account store.Account
					if db.Get(&account, `SELECT * FROM accounts WHERE id=?`, accountID) != nil {
						t.Fatal("account missing")
					}
					result := refreshManagedAccount(context.Background(), db, account)
					if result.Err != nil || result.AccessToken != "synthetic-family-access" {
						t.Fatalf("account result: %v", result.Err)
					}
				}
				direct := func() {
					result, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
					if err != nil || result == nil || result.AccessToken != "synthetic-family-access" {
						t.Fatalf("direct result: %v", err)
					}
				}
				if first == "account" {
					managed()
					direct()
				} else {
					direct()
					managed()
				}
				if calls.Load() != 1 {
					t.Fatalf("account/direct shared RT was exchanged %d times", calls.Load())
				}
				var extra string
				if db.Get(&extra, `SELECT extra_config FROM accounts WHERE id=?`, accountID) != nil || !strings.Contains(extra, `"operatorOption":true`) {
					t.Fatal("account options were overwritten by donor")
				}
			})
		}
	}
}

func TestOAuthFamilyProcessCrashCannotReplayToken(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			dsn := filepath.Join(t.TempDir(), "crash-fixture.db")
			if dialect == store.DialectPostgres {
				dsn = os.Getenv("PG_TEST_DSN")
				if dsn == "" {
					t.Skip("PG_TEST_DSN not set")
				}
			}
			db, id := familyCredentialFixtureDSN(t, dialect, dsn)
			if _, err := db.Exec(`UPDATE upstream_channels SET provider='codex'`); err != nil {
				t.Fatal(err)
			}
			state := store.DirectOAuthState{RefreshToken: "synthetic-process-refresh", ClientID: "synthetic-family-client", ExpiresAt: 1, AccountID: "fixture-process"}
			if _, err := db.Exec(`UPDATE upstream_credentials SET oauth_state=? WHERE id=?`, state, id); err != nil {
				t.Fatal(err)
			}
			entered := make(chan struct{})
			var calls atomic.Int32
			server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
				// Consume the POST body before waiting for disconnect. net/http
				// cannot start its background EOF read while a body remains unread.
				_, _ = io.Copy(io.Discard, r.Body)
				if calls.Add(1) == 1 {
					close(entered)
				}
				<-r.Context().Done()
				return 500, map[string]string{"error": "canceled"}
			})
			t.Cleanup(server.Close)
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, executable, "-test.run=^TestOAuthRefreshProcessWorker$", "-test.count=1")
			command.Env = []string{"GOMAXPROCS=1", "GORACE=halt_on_error=1", "METAPI_REFRESH_PROCESS_SCOPE=direct", "METAPI_REFRESH_PROCESS_DIALECT=" + dialect, "METAPI_REFRESH_PROCESS_DSN=" + dsn, "METAPI_REFRESH_PROCESS_ID=" + strconv.FormatInt(id, 10), "METAPI_REFRESH_PROCESS_ENDPOINT=" + server.URL}
			input, err := command.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer input.Close()
			output, err := command.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			command.Stderr = new(bytes.Buffer)
			if err := command.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if command.ProcessState == nil {
					_ = command.Process.Kill()
					_ = command.Wait()
				}
			})
			ready := make(chan bool, 1)
			go func() {
				scanner := bufio.NewScanner(output)
				ready <- scanner.Scan() && scanner.Text() == "ready"
				_, _ = io.Copy(io.Discard, output)
			}()
			select {
			case ok := <-ready:
				if !ok {
					t.Fatal("worker not ready")
				}
			case <-ctx.Done():
				t.Fatal("worker startup timed out")
			}
			if _, err := input.Write([]byte("1")); err != nil {
				t.Fatal(err)
			}
			select {
			case <-entered:
			case <-ctx.Done():
				t.Fatal("worker did not dispatch")
			}
			if err := command.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			if command.Wait() == nil {
				t.Fatal("worker was not interrupted")
			}
			// Transfer ownership as if the TTL expired; irreversible intent
			// remains despite the process and both record/family owners dying.
			if _, err := db.Exec(`UPDATE credential_refresh_leases SET expires_at=0`); err != nil {
				t.Fatal(err)
			}
			result, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
			if result != nil || !errors.Is(err, ErrOAuthRefreshUncertain) || calls.Load() != 1 {
				t.Fatalf("crash replayed old RT: calls=%d error=%v", calls.Load(), err)
			}
		})
	}
}

func TestDirectOAuthMalformedSuccessStopsLocalRetry(t *testing.T) {
	db, id := directCredentialFixture(t)
	original := retryBackoffFn
	retryBackoffFn = func(int) time.Duration { return 0 }
	t.Cleanup(func() { retryBackoffFn = original })
	var calls atomic.Int32
	directClaudeEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"access_token":`)
	})
	result, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
	if !errors.Is(err, ErrOAuthRefreshUncertain) || result != nil || calls.Load() != 1 {
		t.Fatalf("uncertain success was retried or misclassified: calls=%d error=%v", calls.Load(), err)
	}
}

func TestDirectOAuthUncertainSuccessfulExchangeIsNotRetried(t *testing.T) {
	db, id := directCredentialFixture(t)
	original := retryBackoffFn
	retryBackoffFn = func(int) time.Duration { return 0 }
	t.Cleanup(func() { retryBackoffFn = original })
	var calls atomic.Int32
	directClaudeEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		// HTTP 200 can consume/rotate the RT even if its response is truncated
		// or malformed. Neither lease expiry nor a new request proves otherwise.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = fmt.Fprint(w, `{"access_token":`)
	})
	for range 2 {
		result, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
		if err == nil || result != nil {
			t.Fatal("uncertain token exchange published a credential")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("uncertain old RT was dispatched %d times instead of remaining fenced", calls.Load())
	}
}
