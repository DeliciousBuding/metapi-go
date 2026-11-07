package oauth

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/store"
)

// Each helper has an independent Go runtime, connection pool and singleflight
// map. Only the database and the loopback OAuth endpoint are shared.
func TestOAuthRefreshProcessWorker(t *testing.T) {
	scope := os.Getenv("METAPI_REFRESH_PROCESS_SCOPE")
	if scope == "" {
		return
	}
	db, err := store.Open(os.Getenv("METAPI_REFRESH_PROCESS_DIALECT"), os.Getenv("METAPI_REFRESH_PROCESS_DSN"), false)
	if err != nil {
		t.Fatal("worker database unavailable")
	}
	defer db.Close()
	id, err := strconv.ParseInt(os.Getenv("METAPI_REFRESH_PROCESS_ID"), 10, 64)
	if err != nil {
		t.Fatal("invalid fixture record")
	}
	codexTokenURL = os.Getenv("METAPI_REFRESH_PROCESS_ENDPOINT")
	var snapshot store.Account
	var directSnapshot *directCredentialRow
	if scope == "account" {
		if err := db.Get(&snapshot, `SELECT * FROM accounts WHERE id=?`, id); err != nil {
			t.Fatal("worker could not load account snapshot")
		}
	}
	if scope == "direct" {
		directSnapshot, err = loadDirectCredential(context.Background(), db.DB, id)
		if err != nil {
			t.Fatal("worker could not load direct snapshot")
		}
	}
	// Snapshots are captured before either process consumes a token.
	// This makes winner reuse deterministic, including for a forced refresh.
	_, _ = fmt.Fprintln(os.Stdout, "ready")
	var start [1]byte
	if _, err := io.ReadFull(os.Stdin, start[:]); err != nil || start[0] != '1' {
		t.Fatal("worker start handshake failed")
	}
	switch scope {
	case "account":
		result := refreshManagedAccount(context.Background(), db, snapshot)
		if result.Err != nil || result.AccessToken != "synthetic-process-access" {
			t.Fatal("managed worker did not reuse the persisted token")
		}
	case "direct":
		result, err := refreshDirectCredentialSnapshot(context.Background(), db.DB, id, nil, true, directSnapshot)
		if err != nil || result == nil || result.AccessToken != "synthetic-process-access" {
			t.Fatal("direct worker did not reuse the persisted token")
		}
	default:
		t.Fatal("unknown fixture scope")
	}
}

func TestOAuthRefreshAcrossProcessesConsumesTokenOnce(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		for _, scope := range []string{"account", "direct"} {
			t.Run(dialect+"/"+scope, func(t *testing.T) {
				dsn := filepath.Join(t.TempDir(), "process-fixture.db")
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
				if err := store.AutoMigrate(db); err != nil {
					t.Fatal(err)
				}
				id := int64(1)
				if scope == "account" {
					id = insertCodexTestAccount(t, db, "fixture-process", `{"oauth":{"provider":"codex","accountKey":"fixture-process","refreshToken":"synthetic-process-refresh"}}`)
				} else {
					if _, err := db.Exec(`INSERT INTO upstream_channels(id,origin_key,source_id,name,dialect,provider,enabled,base_url,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex)
						VALUES(1,'process-fixture',1,'fixture','generic','codex',?,'https://provider.invalid','/v1/chat/completions','/v1/responses','/v1/messages',?,'','[]','','')`, true, false); err != nil {
						t.Fatal(err)
					}
					state := store.DirectOAuthState{RefreshToken: "synthetic-process-refresh", ExpiresAt: 1, AccountID: "fixture-process"}
					if _, err := db.Exec(`INSERT INTO upstream_credentials(id,origin_key,channel_id,source_id,name,secret,kind,oauth_state,enabled) VALUES(1,'process-fixture',1,1,'fixture','synthetic-old-access','oauth',?,?)`, state, true); err != nil {
						t.Fatal(err)
					}
				}
				claims := codexJWTClaims{}
				claims.Auth.ChatGPTAccountID = "fixture-process"
				idToken := makeCodexIDToken(t, claims)
				entered, release := make(chan struct{}), make(chan struct{})
				var unblock sync.Once
				var calls atomic.Int32
				server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
					if r.PostFormValue("refresh_token") != "synthetic-process-refresh" {
						t.Error("worker used unexpected refresh material")
					}
					if calls.Add(1) != 1 {
						return 400, map[string]string{"error": "invalid_grant"}
					}
					close(entered)
					select {
					case <-release:
					case <-r.Context().Done():
						return 500, map[string]string{"error": "canceled"}
					}
					return 200, codexTokenResponse{AccessToken: "synthetic-process-access", RefreshToken: "synthetic-process-rotated", IDToken: idToken, ExpiresIn: 3600}
				})
				t.Cleanup(server.Close)
				t.Cleanup(func() { unblock.Do(func() { close(release) }) })
				executable, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				t.Cleanup(cancel)
				commands := make([]*exec.Cmd, 2)
				inputs := make([]io.WriteCloser, 2)
				outputs := make([]*bytes.Buffer, 2)
				for i := range 2 {
					command := exec.CommandContext(ctx, executable, "-test.run=^TestOAuthRefreshProcessWorker$", "-test.count=1")
					// Do not inherit the operator's shell/provider credentials.
					command.Env = []string{
						"GOMAXPROCS=1", "GORACE=halt_on_error=1",
						"METAPI_REFRESH_PROCESS_SCOPE=" + scope,
						"METAPI_REFRESH_PROCESS_DIALECT=" + dialect,
						"METAPI_REFRESH_PROCESS_DSN=" + dsn,
						"METAPI_REFRESH_PROCESS_ID=" + strconv.FormatInt(id, 10),
						"METAPI_REFRESH_PROCESS_ENDPOINT=" + server.URL,
					}
					input, err := command.StdinPipe()
					if err != nil {
						t.Fatal(err)
					}
					output, err := command.StdoutPipe()
					if err != nil {
						t.Fatal(err)
					}
					buffer := new(bytes.Buffer)
					command.Stderr = buffer
					if err := command.Start(); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						_ = input.Close()
						if command.ProcessState == nil {
							_ = command.Process.Kill()
							_ = command.Wait()
						}
					})
					commands[i], inputs[i], outputs[i] = command, input, buffer
					ready := make(chan bool, 1)
					go func() {
						scanner := bufio.NewScanner(output)
						ready <- scanner.Scan() && scanner.Text() == "ready"
						for scanner.Scan() {
							// Drain test output; it contains no token values.
						}
						_ = output.Close()
					}()
					select {
					case ok := <-ready:
						if !ok {
							t.Fatal("worker did not report readiness")
						}
					case <-ctx.Done():
						t.Fatal("worker startup timed out")
					}
				}
				if commands[0].Process.Pid == commands[1].Process.Pid {
					t.Fatal("workers are not independent processes")
				}
				if _, err := inputs[0].Write([]byte("1")); err != nil {
					t.Fatal(err)
				}
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("first process did not exchange")
				}
				if _, err := inputs[1].Write([]byte("1")); err != nil {
					t.Fatal(err)
				}
				unblock.Do(func() { close(release) })
				for i, command := range commands {
					_ = inputs[i].Close()
					if err := command.Wait(); err != nil {
						t.Errorf("process %d failed: %v; %s", i, err, outputs[i].String())
					}
				}
				if calls.Load() != 1 {
					t.Fatalf("rotating token consumed %d times across processes", calls.Load())
				}
				if scope == "account" {
					var account store.Account
					if err := db.Get(&account, `SELECT * FROM accounts WHERE id=?`, id); err != nil {
						t.Fatal(err)
					}
					state := GetOauthInfoFromAccount(&account)
					if state == nil || state.RefreshToken != "synthetic-process-rotated" || account.AccessToken != "synthetic-process-access" || state.TokenExpiresAt <= time.Now().UnixMilli() {
						t.Fatal("managed process result was not durably persisted")
					}
				} else {
					current, err := loadDirectCredential(context.Background(), db.DB, id)
					if err != nil || current.Secret != "synthetic-process-access" || current.State.RefreshToken != "synthetic-process-rotated" || current.State.ExpiresAt <= time.Now().UnixMilli() {
						t.Fatal("direct process result was not durably persisted")
					}
				}
			})
		}
	}
}
