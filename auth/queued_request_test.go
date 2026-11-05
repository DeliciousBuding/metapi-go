package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQueuedRequestAuthorizationDoesNotRepeatAdmission(t *testing.T) {
	setupTestDB(t)
	ResetKeyAdmissionForTest()
	t.Cleanup(ResetKeyAdmissionForTest)
	maxRequests, maxRPM := int64(1), int64(1)
	id := insertTestKeyWithRPM(t, "synthetic-queued-once", &maxRequests, 0, &maxRPM, nil)
	admitted := ReauthorizeManagedKeyForTurn(context.Background(), "synthetic-queued-once", "127.0.0.1", 0)
	if !admitted.OK {
		t.Fatalf("initial admission: %+v", admitted)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	previous := &ProxyAuthContext{Source: "managed", KeyID: &id, Token: admitted.Token, Policy: admitted.Policy}
	for range 2 {
		refreshed := RevalidateQueuedRequest(req, previous)
		if !refreshed.OK {
			t.Fatalf("already-admitted request rejected by its own consumed quota: %+v", refreshed)
		}
		if got := readUsedRequests(t, id); got != 1 {
			t.Fatalf("queued authorization consumed request again: %d", got)
		}
	}
	if _, err := testDB(t).Exec(`UPDATE downstream_api_keys SET supported_models='["different-model"]' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	refreshed := RevalidateQueuedRequest(req, previous)
	if !refreshed.OK || len(refreshed.Policy.SupportedModels) != 1 || refreshed.Policy.SupportedModels[0] != "different-model" {
		t.Fatalf("queued authorization retained old policy: %+v", refreshed)
	}
	if _, err := testDB(t).Exec(`UPDATE downstream_api_keys SET enabled=FALSE WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	if refreshed = RevalidateQueuedRequest(req, previous); refreshed.OK || refreshed.StatusCode != http.StatusForbidden {
		t.Fatalf("disabled queued key still authorized: %+v", refreshed)
	}
}
