package oauth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

func directCredentialFixture(t *testing.T) (*store.DB, int64) {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO upstream_channels(id,origin_key,source_id,name,dialect,provider,enabled,base_url,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex) VALUES(1,'fixture',1,'fixture','generic','claudecode',1,'https://provider.invalid','/v1/chat/completions','/v1/responses','/v1/messages',0,'','[]','','')`)
	if err != nil {
		t.Fatal(err)
	}
	state := store.DirectOAuthState{RefreshToken: "fixture-old-refresh-secret", ClientID: "source-client", ExpiresAt: 1}
	_, err = db.Exec(`INSERT INTO upstream_credentials(id,origin_key,channel_id,source_id,name,secret,kind,oauth_state,enabled) VALUES(1,'fixture',1,1,'default','fixture-old-access-secret','oauth',?,1)`, state)
	if err != nil {
		t.Fatal(err)
	}
	return db, 1
}

func directClaudeEndpoint(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	upstream := httptest.NewServer(handler)
	t.Cleanup(upstream.Close)
	previous := claudeTokenURL
	claudeTokenURL = upstream.URL
	t.Cleanup(func() { claudeTokenURL = previous })
}

func TestDirectOAuthRefreshUsesSourceClientAndSingleflight(t *testing.T) {
	db, id := directCredentialFixture(t)
	var calls atomic.Int32
	directClaudeEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		if json.NewDecoder(r.Body).Decode(&body) != nil || body["client_id"] != "source-client" || body["refresh_token"] != "fixture-old-refresh-secret" {
			t.Error("wrong refresh contract")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"fixture-fresh-access","expires_in":3600}`)
	})
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			got, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
			if err != nil || got == nil || got.AccessToken != "fixture-fresh-access" {
				t.Errorf("resolve failed: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("refresh calls=%d want1", calls.Load())
	}
	var token string
	var state store.DirectOAuthState
	if err := db.QueryRow(`SELECT secret,oauth_state FROM upstream_credentials WHERE id=1`).Scan(&token, &state); err != nil {
		t.Fatal(err)
	}
	if token != "fixture-fresh-access" || state.RefreshToken != "fixture-old-refresh-secret" || state.ExpiresAt <= time.Now().UnixMilli() {
		t.Fatal("rotated state not persisted or missing RT not preserved")
	}
}

func TestDirectOAuthRefreshCASDoesNotOverwriteReimport(t *testing.T) {
	db, id := directCredentialFixture(t)
	directClaudeEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		state := store.DirectOAuthState{ClientID: "source-client", RefreshToken: "replacement-refresh", ExpiresAt: time.Now().Add(time.Hour).UnixMilli()}
		if _, err := db.Exec(`UPDATE upstream_credentials SET secret='replacement-access',oauth_state=? WHERE id=1`, state); err != nil {
			t.Error(err)
		}
		_, _ = fmt.Fprint(w, `{"access_token":"stale-refreshed-access","refresh_token":"stale-refreshed-secret","expires_in":3600}`)
	})
	_, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
	if !errors.Is(err, ErrDirectCredentialChanged) {
		t.Fatalf("error=%v", err)
	}
	got, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
	if err != nil || got.AccessToken != "replacement-access" {
		t.Fatalf("reimport overwritten: %v", err)
	}
}

func TestDirectOAuthRefreshFailureDoesNotLeakOrReplaceSecrets(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		status, calls  int
		want           error
	}{
		{"revoked", `{"error":"invalid_grant","refresh_token":"fixture-old-refresh-secret"}`, 400, 1, ErrDirectCredentialRevoked},
		{"transient", `{"error":"unavailable","access_token":"fixture-old-access-secret"}`, 503, MaxRefreshRetries + 1, ErrDirectCredentialRefresh},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, id := directCredentialFixture(t)
			var calls atomic.Int32
			previous := retryBackoffFn
			retryBackoffFn = func(int) time.Duration { return 0 }
			t.Cleanup(func() { retryBackoffFn = previous })
			directClaudeEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.response)
			})
			_, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
			if !errors.Is(err, tc.want) || strings.Contains(fmt.Sprint(err), "secret") || calls.Load() != int32(tc.calls) {
				t.Fatalf("error=%v calls=%d", err, calls.Load())
			}
			var token string
			if err := db.Get(&token, `SELECT secret FROM upstream_credentials WHERE id=1`); err != nil {
				t.Fatal(err)
			}
			if token != "fixture-old-access-secret" {
				t.Fatal("failed refresh changed credential")
			}
		})
	}
}

func TestDirectCredentialUnavailableAndExpiry(t *testing.T) {
	db, id := directCredentialFixture(t)
	state := store.DirectOAuthState{ExpiresAt: 1}
	if _, err := db.Exec(`UPDATE upstream_credentials SET oauth_state=? WHERE id=1`, state); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false); !errors.Is(err, ErrDirectCredentialExpired) {
		t.Fatalf("expired token accepted: %v", err)
	}
	if _, err := db.Exec(`UPDATE upstream_credentials SET enabled=0 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false); !errors.Is(err, ErrDirectCredentialUnavailable) {
		t.Fatalf("disabled token accepted: %v", err)
	}
}

func TestDirectCodexRefreshPreservesOptionalTokenFields(t *testing.T) {
	db, id := directCredentialFixture(t)
	if _, err := db.Exec(`UPDATE upstream_channels SET provider='codex' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	claims := codexJWTClaims{}
	claims.Auth.ChatGPTAccountID = "account-fixture"
	access := makeCodexIDToken(t, claims)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("client_id") != "source-client" || r.PostForm.Get("scope") != "" {
			t.Error("source OAuth client/scope changed")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": access, "expires_in": 3600})
	}))
	defer server.Close()
	withCodexTokenURLSwap(t, server.URL)
	got, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
	if err != nil || got.AccountID != "account-fixture" || got.AccessToken != access {
		t.Fatalf("Codex identity/refresh failed: %v", err)
	}
	var state store.DirectOAuthState
	if err := db.QueryRow(`SELECT oauth_state FROM upstream_credentials WHERE id=1`).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state.RefreshToken != "fixture-old-refresh-secret" {
		t.Fatal("optional missing refresh token erased the stored refresh token")
	}
}
