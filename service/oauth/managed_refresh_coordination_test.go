package oauth

import (
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestAccountConcurrentRefreshConsumesTokenOnce(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	id := insertCodexTestAccount(t, db, "fixture-coordinated", `{"oauth":{"provider":"codex","accountKey":"fixture-coordinated","refreshToken":"synthetic-old-refresh"}}`)
	claims := codexJWTClaims{}
	claims.Auth.ChatGPTAccountID = "fixture-coordinated"
	idToken := makeCodexIDToken(t, claims)
	entered, release, duplicate := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var unblock sync.Once
	var calls atomic.Int32
	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		if calls.Add(1) != 1 {
			close(duplicate)
			return 400, map[string]string{"error": "invalid_grant"}
		}
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
			return 500, map[string]string{"error": "canceled"}
		}
		return 200, codexTokenResponse{AccessToken: "synthetic-shared-access", RefreshToken: "synthetic-rotated-refresh", IDToken: idToken, ExpiresIn: 3600}
	})
	t.Cleanup(server.Close)
	t.Cleanup(func() { unblock.Do(func() { close(release) }) })
	withCodexTokenURLSwap(t, server.URL)
	results := make(chan refreshResult, 2)
	go func() { results <- doRefreshAccessToken(id) }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first account refresh did not start")
	}
	go func() { results <- doRefreshAccessToken(id) }()
	select {
	case <-duplicate:
	case <-time.After(150 * time.Millisecond):
	}
	unblock.Do(func() { close(release) })
	for range 2 {
		select {
		case result := <-results:
			if result.Err != nil || result.AccessToken != "synthetic-shared-access" {
				t.Errorf("account did not reuse the persisted refresh winner: %v", result.Err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("account refresh did not finish")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("rotating account token consumed %d times", calls.Load())
	}
}
