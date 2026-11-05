package oauth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestGetOauthInfoFromAccount_NormalizedState(t *testing.T) {
	want := &OauthInfo{
		Provider:             "codex",
		AccountID:            "synthetic-account",
		AccountKey:           "synthetic-account",
		ProjectID:            "synthetic-project",
		Email:                "synthetic@example.invalid",
		PlanType:             "plus",
		TokenExpiresAt:       1700000000,
		RefreshToken:         "synthetic-refresh",
		IDToken:              "synthetic-id-token",
		SessionToken:         "synthetic-session-token",
		ProviderData:         map[string]interface{}{"organization": "synthetic-org"},
		ModelDiscoveryStatus: OauthModelDiscoveryHealthy,
		LastModelSyncAt:      "2026-11-05T00:00:00Z",
		LastModelSyncError:   "synthetic-history",
		LastDiscoveredModels: []string{"synthetic-model"},
	}
	for _, tc := range []struct {
		name            string
		legacyIdentity  bool
		partialIdentity bool
	}{
		{name: "normalized"},
		{name: "legacy", legacyIdentity: true},
		{name: "providerless legacy identity", partialIdentity: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encoded, err := json.Marshal(BuildStoredOauthState(want))
			if err != nil {
				t.Fatal(err)
			}
			var state map[string]interface{}
			if err := json.Unmarshal(encoded, &state); err != nil {
				t.Fatal(err)
			}
			if tc.legacyIdentity {
				state["provider"] = "legacy-provider"
			}
			if tc.partialIdentity {
				state["accountId"] = "untrusted-legacy-account"
				state["accountKey"] = "untrusted-legacy-key"
				state["projectId"] = "untrusted-legacy-project"
			}
			encoded, err = json.Marshal(map[string]interface{}{"oauth": state})
			if err != nil {
				t.Fatal(err)
			}
			account := &store.Account{
				OAuthProvider:   accountStrPtr(want.Provider),
				OAuthAccountKey: accountStrPtr(want.AccountKey),
				OAuthProjectID:  accountStrPtr(want.ProjectID),
				ExtraConfig:     accountStrPtr(string(encoded)),
			}
			got := GetOauthInfoFromAccount(account)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("column identity and stored OAuth state did not round-trip: got %+v", got)
			}
			stored, err := BuildStoredOauthStateFromAccount(account, nil)
			if err != nil || !reflect.DeepEqual(stored, BuildStoredOauthState(want)) {
				t.Errorf("stored-state rebuild lost fields: got %+v, err=%v", stored, err)
			}
			if !tc.legacyIdentity {
				if GetOauthInfoFromExtraConfig(account.ExtraConfig) != nil {
					t.Error("the standalone legacy getter must still require its own provider")
				}
				account.OAuthProvider = nil
				if GetOauthInfoFromAccount(account) != nil {
					t.Error("stored state without any provider identity must not create an OAuth account")
				}
			}
		})
	}
}

func TestRefreshAccessTokenSingleflight_PreservesStateAcrossRefreshes(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	claims := codexJWTClaims{Email: "synthetic@example.invalid"}
	claims.Auth.ChatGPTAccountID = "synthetic-account"
	idToken := makeCodexIDToken(t, claims)
	var calls atomic.Int32
	tokenServer := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		round := calls.Add(1)
		expected := fmt.Sprintf("synthetic-refresh-%d", round-1)
		if got := r.PostFormValue("refresh_token"); got != expected {
			t.Errorf("refresh %d used token %q, want %q", round, got, expected)
		}
		return http.StatusOK, codexTokenResponse{
			AccessToken:  fmt.Sprintf("synthetic-access-%d", round),
			RefreshToken: fmt.Sprintf("synthetic-refresh-%d", round),
			IDToken:      idToken,
			ExpiresIn:    float64(3600),
		}
	})
	defer tokenServer.Close()
	withCodexTokenURLSwap(t, tokenServer.URL)

	extra, err := json.Marshal(map[string]interface{}{
		"oauth": map[string]interface{}{
			"provider":     "codex",
			"accountKey":   "synthetic-account",
			"refreshToken": fmt.Sprintf("synthetic-refresh-%d", 0),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	accountID := insertCodexTestAccount(t, db, "synthetic-account", string(extra))
	for round := 1; round <= 2; round++ {
		result, err := RefreshAccessTokenSingleflight(accountID)
		if err != nil {
			t.Fatalf("refresh %d failed after reading persisted state: %v", round, err)
		}
		if result.AccessToken != fmt.Sprintf("synthetic-access-%d", round) {
			t.Errorf("refresh %d returned an unexpected access token", round)
		}
		var account store.Account
		if err := db.Get(&account, "SELECT * FROM accounts WHERE id = ?", accountID); err != nil {
			t.Fatal(err)
		}
		if account.OAuthProvider == nil || *account.OAuthProvider != "codex" {
			t.Fatal("refresh lost the authoritative provider column")
		}
		var stored parsedExtraConfig
		if account.ExtraConfig == nil || json.Unmarshal([]byte(*account.ExtraConfig), &stored) != nil || stored.OAuth == nil {
			t.Fatal("refresh did not persist OAuth state")
		}
		if stored.OAuth.Provider != "" || stored.OAuth.AccountID != nil || stored.OAuth.AccountKey != nil || stored.OAuth.ProjectID != nil {
			t.Error("refresh must not duplicate column identity in stored OAuth state")
		}
		if stored.OAuth.RefreshToken != fmt.Sprintf("synthetic-refresh-%d", round) {
			t.Errorf("refresh %d did not persist its rotated refresh token", round)
		}
	}
	if calls.Load() != 2 {
		t.Errorf("sequential refreshes made %d provider calls, want 2", calls.Load())
	}
}
