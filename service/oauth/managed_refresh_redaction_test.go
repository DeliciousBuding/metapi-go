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
	server := newJSONTestServer(t, func(r *http.Request) (int, interface{}) {
		return 400, map[string]string{"error": "invalid_grant", "refresh_token": "synthetic-provider-reflected-refresh", "access_token": "synthetic-provider-reflected-access"}
	})
	t.Cleanup(server.Close)
	withCodexTokenURLSwap(t, server.URL)
	result := doRefreshAccessToken(id)
	if result.Err == nil || !strings.Contains(result.Err.Error(), "invalid_grant") {
		t.Fatal("refresh did not preserve the rejection classification")
	}
	if strings.Contains(result.Err.Error(), "synthetic-provider-") || result.AccessToken != "" || result.ExtraConfig != nil {
		t.Fatal("refresh returned secret-bearing provider error material")
	}
}
