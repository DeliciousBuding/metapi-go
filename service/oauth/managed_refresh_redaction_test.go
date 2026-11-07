package oauth

import (
	"net/http"
	"strings"
	"testing"
)

func TestAccountRefreshFailureDoesNotReturnProviderSecrets(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	id := insertCodexTestAccount(t, db, "fixture-redaction", `{"oauth":{"provider":"codex","accountKey":"fixture-redaction","refreshToken":"synthetic-redaction-refresh"}}`)
	var accessToken string
	if err := db.Get(&accessToken, db.Rebind(`SELECT access_token FROM accounts WHERE id=?`), id); err != nil {
		t.Fatal(err)
	}
	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		// A definite rejection may echo known input tokens. Different token
		// fields mean an ambiguous rotation, covered by the family regression.
		return 400, map[string]string{"error": "invalid_grant", "refresh_token": "synthetic-redaction-refresh", "access_token": accessToken, "error_description": "synthetic-provider-reflected-private-material"}
	})
	t.Cleanup(server.Close)
	withCodexTokenURLSwap(t, server.URL)
	result := doRefreshAccessToken(id)
	if result.Err == nil || !strings.Contains(result.Err.Error(), "invalid_grant") {
		t.Fatal("refresh did not preserve the rejection classification")
	}
	if strings.Contains(result.Err.Error(), "synthetic-provider-") || strings.Contains(result.Err.Error(), accessToken) || strings.Contains(result.Err.Error(), "synthetic-redaction-refresh") || result.AccessToken != "" || result.ExtraConfig != nil {
		t.Fatal("refresh returned secret-bearing provider error material")
	}
}
