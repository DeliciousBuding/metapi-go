package oauth

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectRefreshSameDatabaseMultipleHandlesConsumesTokenOnce(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "credentials.db")
	first, id := directCredentialFixture(t, dsn)
	second, err := store.Open(store.DialectSQLite, dsn, false)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	entered, duplicate, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	var calls atomic.Int32
	directClaudeEndpoint(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
			_, _ = fmt.Fprint(w, `{"access_token":"synthetic-new-access","refresh_token":"synthetic-new-refresh","expires_in":3600}`)
		} else {
			close(duplicate)
			w.WriteHeader(400)
			_, _ = fmt.Fprint(w, `{"error":"invalid_grant"}`)
		}
	})
	type outcome struct {
		result *DirectCredentialResult
		err    error
	}
	results := make(chan outcome, 2)
	go func() {
		result, err := ResolveDirectCredential(context.Background(), first.DB, id, nil, false)
		results <- outcome{result, err}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first refresh did not start")
	}
	go func() {
		result, err := ResolveDirectCredential(context.Background(), second.DB, id, nil, false)
		results <- outcome{result, err}
	}()
	select {
	case <-duplicate:
	case <-time.After(100 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(release) })
	for range 2 {
		select {
		case got := <-results:
			if got.err != nil || got.result == nil || got.result.AccessToken != "synthetic-new-access" {
				t.Errorf("shared credential refresh did not reuse the winner: %v", got.err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("refresh did not finish")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("same rotating credential consumed %d times", calls.Load())
	}
}
