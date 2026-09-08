package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

type newAPIRefreshFixture struct {
	server               *httptest.Server
	relayV1Calls         atomic.Int32
	relayUserModelCalls  atomic.Int32
	accessV1Calls        atomic.Int32
	accessUserModelCalls atomic.Int32
}

func startNewAPIRefreshFixture(t *testing.T, relayToken, accessToken string, relayV1Status int) *newAPIRefreshFixture {
	t.Helper()
	f := &newAPIRefreshFixture{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		auth := r.Header.Get("Authorization")

		switch r.URL.Path {
		case "/v1/models":
			switch {
			case relayToken != "" && auth == "Bearer "+relayToken:
				f.relayV1Calls.Add(1)
				if relayV1Status != http.StatusOK {
					w.WriteHeader(relayV1Status)
					_, _ = fmt.Fprint(w, `{"success":false,"message":"models endpoint disabled"}`)
					return
				}
				_, _ = fmt.Fprint(w, `{"object":"list","data":[{"id":"gpt-4o-mini"}]}`)
			case accessToken != "" && auth == "Bearer "+accessToken:
				f.accessV1Calls.Add(1)
				w.WriteHeader(http.StatusNotFound)
				_, _ = fmt.Fprint(w, `{"success":false,"message":"models endpoint disabled"}`)
			default:
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = fmt.Fprint(w, `{"success":false,"message":"unauthorized"}`)
			}
		case "/api/user/models":
			switch {
			case accessToken != "" && auth == "Bearer "+accessToken:
				f.accessUserModelCalls.Add(1)
				_, _ = fmt.Fprint(w, `{"success":true,"data":["gpt-4o-mini"]}`)
			case relayToken != "" && auth == "Bearer "+relayToken:
				f.relayUserModelCalls.Add(1)
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = fmt.Fprint(w, `{"success":false,"message":"relay key is not a dashboard token"}`)
			default:
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = fmt.Fprint(w, `{"success":false,"message":"unauthorized"}`)
			}
		case "/api/user/self":
			if accessToken != "" && auth == "Bearer "+accessToken {
				_, _ = fmt.Fprint(w, `{"success":true,"data":{"id":42,"username":"operator"}}`)
				return
			}
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = fmt.Fprint(w, `{"success":false,"message":"unauthorized"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func seedNewAPIAccountCredentials(t *testing.T, db *sqlx.DB, siteID int64, apiToken, accessToken string) (accountID, relayTokenID int64) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	var apiValue interface{}
	if apiToken != "" {
		apiValue = apiToken
	}

	res, err := db.Exec(
		`INSERT INTO accounts (site_id, username, access_token, api_token, status, checkin_enabled, sort_order, extra_config, created_at, updated_at)
		 VALUES (?, ?, ?, ?, 'active', FALSE, 0, '{"platformUserId":42}', ?, ?)`,
		siteID, "newapi-refresh-fixture", accessToken, apiValue, now, now,
	)
	if err != nil {
		t.Fatalf("insert new-api account: %v", err)
	}
	accountID, _ = res.LastInsertId()

	if apiToken == "" {
		return accountID, 0
	}
	tokenRes, err := db.Exec(
		`INSERT INTO account_tokens (account_id, name, token, value_status, source, enabled, is_default, created_at, updated_at)
		 VALUES (?, 'relay', ?, 'ready', 'manual', TRUE, TRUE, ?, ?)`,
		accountID, apiToken, now, now,
	)
	if err != nil {
		t.Fatalf("insert relay token: %v", err)
	}
	relayTokenID, _ = tokenRes.LastInsertId()
	return accountID, relayTokenID
}

func TestRefreshAccountModels_NewAPIAccessTokenFallbackWhenV1ModelsDisabled(t *testing.T) {
	db := openModelRefreshTestDB(t)
	fixture := startNewAPIRefreshFixture(t, "sk-relay-valid", "dashboard-pat", http.StatusNotFound)
	siteID := seedSiteOnPlatform(t, db, "new-api-fallback", fixture.server.URL, "new-api")
	accountID, relayTokenID := seedNewAPIAccountCredentials(t, db, siteID, "sk-relay-valid", "dashboard-pat")

	result := RefreshAccountModels(context.Background(), db, accountID, true, true)
	if !result.Success {
		t.Fatalf("refresh failed: %s / %s", result.ErrorCode, result.ErrorMessage)
	}
	if len(result.Models) != 1 || result.Models[0] != "gpt-4o-mini" {
		t.Fatalf("models = %v, want [gpt-4o-mini]", result.Models)
	}
	if fixture.relayV1Calls.Load() != 1 {
		t.Fatalf("relay /v1/models calls = %d, want 1", fixture.relayV1Calls.Load())
	}
	if fixture.relayUserModelCalls.Load() == 0 {
		t.Fatal("relay credential was not tried against /api/user/models before management fallback")
	}
	if fixture.accessV1Calls.Load() != 1 || fixture.accessUserModelCalls.Load() != 1 {
		t.Fatalf("management calls: /v1/models=%d /api/user/models=%d, want 1/1", fixture.accessV1Calls.Load(), fixture.accessUserModelCalls.Load())
	}
	if result.TokenBackfilled {
		t.Fatal("management-token fallback backfilled relay token availability")
	}

	var relayAvailability int
	if err := db.Get(&relayAvailability, `SELECT COUNT(*) FROM token_model_availability WHERE token_id = ?`, relayTokenID); err != nil {
		t.Fatalf("count relay token availability: %v", err)
	}
	if relayAvailability != 0 {
		t.Fatalf("relay token_model_availability rows = %d, want 0: the relay key did not discover these models", relayAvailability)
	}

	var accountAvailability int
	if err := db.Get(&accountAvailability, `SELECT COUNT(*) FROM model_availability WHERE account_id = ? AND available = TRUE`, accountID); err != nil {
		t.Fatalf("count account availability: %v", err)
	}
	if accountAvailability != 1 {
		t.Fatalf("account model_availability rows = %d, want 1", accountAvailability)
	}
}

func TestRefreshAccountModels_NewAPIAPIKeyOnlyFailsWhenV1ModelsDisabled(t *testing.T) {
	db := openModelRefreshTestDB(t)
	fixture := startNewAPIRefreshFixture(t, "sk-relay-only", "", http.StatusNotFound)
	siteID := seedSiteOnPlatform(t, db, "new-api-api-key-only", fixture.server.URL, "new-api")
	accountID, _ := seedNewAPIAccountCredentials(t, db, siteID, "sk-relay-only", "")

	result := RefreshAccountModels(context.Background(), db, accountID, true, true)
	if result.Success {
		t.Fatalf("API-key-only refresh succeeded without a management credential: %+v", result)
	}
	if result.ErrorCode == "" || result.ErrorCode == "empty_models" {
		t.Fatalf("API-key-only failure was classified as %q / %q", result.ErrorCode, result.ErrorMessage)
	}
	if fixture.relayV1Calls.Load() != 1 {
		t.Fatalf("relay /v1/models calls = %d, want 1", fixture.relayV1Calls.Load())
	}
	if fixture.accessV1Calls.Load() != 0 || fixture.accessUserModelCalls.Load() != 0 {
		t.Fatalf("API-key-only account made management calls: /v1/models=%d /api/user/models=%d", fixture.accessV1Calls.Load(), fixture.accessUserModelCalls.Load())
	}

	var availability int
	if err := db.Get(&availability, `SELECT COUNT(*) FROM model_availability WHERE account_id = ?`, accountID); err != nil {
		t.Fatalf("count availability: %v", err)
	}
	if availability != 0 {
		t.Fatalf("failed API-key-only refresh wrote %d model_availability rows", availability)
	}
}

func TestRefreshAccountModels_NewAPIManagementTokenDoesNotMaskRejectedRelayKey(t *testing.T) {
	db := openModelRefreshTestDB(t)
	fixture := startNewAPIRefreshFixture(t, "sk-relay-rejected", "dashboard-pat", http.StatusUnauthorized)
	siteID := seedSiteOnPlatform(t, db, "new-api-rejected-relay", fixture.server.URL, "new-api")
	accountID, relayTokenID := seedNewAPIAccountCredentials(t, db, siteID, "sk-relay-rejected", "dashboard-pat")

	result := RefreshAccountModels(context.Background(), db, accountID, true, true)
	if result.Success {
		t.Fatalf("refresh succeeded after /v1/models rejected the relay key: %+v", result)
	}
	if result.ErrorCode != "unauthorized" {
		t.Fatalf("ErrorCode = %q / %q, want unauthorized", result.ErrorCode, result.ErrorMessage)
	}
	if fixture.relayV1Calls.Load() != 1 {
		t.Fatalf("relay /v1/models calls = %d, want 1", fixture.relayV1Calls.Load())
	}
	if fixture.accessV1Calls.Load() != 0 || fixture.accessUserModelCalls.Load() != 0 {
		t.Fatalf("management credential was used after the relay key was rejected: /v1/models=%d /api/user/models=%d", fixture.accessV1Calls.Load(), fixture.accessUserModelCalls.Load())
	}

	var accountAvailability int
	if err := db.Get(&accountAvailability, `SELECT COUNT(*) FROM model_availability WHERE account_id = ?`, accountID); err != nil {
		t.Fatalf("count account availability: %v", err)
	}
	if accountAvailability != 0 {
		t.Fatalf("rejected relay key wrote %d account model_availability rows", accountAvailability)
	}

	var tokenAvailability int
	if err := db.Get(&tokenAvailability, `SELECT COUNT(*) FROM token_model_availability WHERE token_id = ?`, relayTokenID); err != nil {
		t.Fatalf("count token availability: %v", err)
	}
	if tokenAvailability != 0 {
		t.Fatalf("rejected relay key wrote %d token_model_availability rows", tokenAvailability)
	}
}
