package oauth

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestDirectAntigravityLazyRefreshProjectAndRotation(t *testing.T) {
	db, id := directCredentialFixture(t)
	if _, err := db.Exec(`UPDATE upstream_channels SET provider='antigravity' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	state, err := ParseAntigravityImportMaterial("synthetic-refresh-1|synthetic-project")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE upstream_credentials SET secret='',oauth_state=? WHERE id=1`, state); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	directToken := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		round := calls.Add(1)
		if r.PostFormValue("grant_type") != "refresh_token" || r.PostFormValue("client_id") != antigravityClientID ||
			r.PostFormValue("client_secret") != fixtureAntigravityClientSecret {
			t.Error("refresh did not use the source issuing client")
		}
		expected := "synthetic-refresh-1"
		if round == 2 {
			expected = "synthetic-refresh-2"
		}
		if r.PostFormValue("refresh_token") != expected {
			t.Error("refresh rotation was not consumed")
		}
		return http.StatusOK, map[string]any{"access_token": "synthetic-access", "refresh_token": "synthetic-refresh-2", "expires_in": 3600}
	})
	defer directToken.Close()
	withAntigravityEndpointSwap(t, directToken.URL, "http://127.0.0.1:1", "http://127.0.0.1:1")
	var group sync.WaitGroup
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			result, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
			if err != nil || result == nil || result.ProjectID != "synthetic-project" || result.AccessToken != "synthetic-access" {
				t.Errorf("lazy resolve lost project/token: %v", err)
			}
		}()
	}
	group.Wait()
	if calls.Load() != 1 {
		t.Fatalf("concurrent lazy requests exchanged %d times", calls.Load())
	}
	if _, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, true); err != nil {
		t.Fatal(err)
	}
	var persisted store.DirectOAuthState
	if err := db.Get(&persisted, `SELECT oauth_state FROM upstream_credentials WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || persisted.ProjectID != state.ProjectID || persisted.RefreshToken != "synthetic-refresh-2" ||
		persisted.ExpiresAt <= time.Now().UnixMilli() {
		t.Fatal("rotation did not persist current state")
	}
}

func TestDirectAntigravityImportAndReplacementValidation(t *testing.T) {
	for _, raw := range []string{"token", "|project", "token|", " token|project", "token|project|extra", "token|bad/project", "token\n|project"} {
		if _, err := ParseAntigravityImportMaterial(raw); !errors.Is(err, ErrInvalidDirectCredential) || strings.Contains(err.Error(), raw) {
			t.Errorf("malformed material was accepted or leaked")
		}
	}
	state, err := ParseAntigravityImportMaterial("synthetic-refresh|synthetic-project")
	if err != nil {
		t.Fatal(err)
	}
	secret, kind, got, _, err := directCredentialMaterial("antigravity", store.DirectEndpoints{}, store.DirectCredentialOAuth, nil, &DirectOAuthReplacement{DirectOAuthState: state}, nil)
	if err != nil || secret != "" || kind != store.DirectCredentialOAuth || got != state {
		t.Fatal("refresh-only credential replacement lost source state")
	}
	apiKey := "synthetic-static"
	if _, _, _, _, err := directCredentialMaterial("antigravity", store.DirectEndpoints{}, store.DirectCredentialAPIKey, &apiKey, nil, nil); err == nil {
		t.Fatal("Antigravity must not authorize static API-key fallback")
	}
}

func TestDirectAntigravityRefreshCASPreservesReplacementProject(t *testing.T) {
	db, id := directCredentialFixture(t)
	if _, err := db.Exec(`UPDATE upstream_channels SET provider='antigravity' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	state, _ := ParseAntigravityImportMaterial("synthetic-old-refresh|synthetic-old-project")
	if _, err := db.Exec(`UPDATE upstream_credentials SET secret='',oauth_state=? WHERE id=1`, state); err != nil {
		t.Fatal(err)
	}
	replacement, _ := ParseAntigravityImportMaterial("synthetic-new-refresh|synthetic-new-project")
	replacement.ExpiresAt = time.Now().Add(time.Hour).UnixMilli()
	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		if _, err := db.Exec(`UPDATE upstream_credentials SET secret='synthetic-new-access',oauth_state=? WHERE id=1`, replacement); err != nil {
			t.Error(err)
		}
		return http.StatusOK, map[string]any{"access_token": "synthetic-stale-access", "expires_in": 3600}
	})
	defer server.Close()
	withAntigravityEndpointSwap(t, server.URL, "http://127.0.0.1:1", "http://127.0.0.1:1")
	if _, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false); !errors.Is(err, ErrDirectCredentialChanged) {
		t.Fatalf("changed project/credential was not rejected: %v", err)
	}
	result, err := ResolveDirectCredential(context.Background(), db.DB, id, nil, false)
	if err != nil || result.ProjectID != replacement.ProjectID || result.AccessToken != "synthetic-new-access" {
		t.Fatal("refresh overwrote replacement identity")
	}
}
