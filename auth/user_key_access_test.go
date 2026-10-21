package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestUserKeyAdmissionFailClosedAndScopedDiagnosis(t *testing.T) {
	setupTestDB(t)
	previous := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() { config.SetRuntime(previous) })
	db := testDB(t)
	users := store.NewGatewayUserStore(db)
	user, err := users.CreateUser(context.Background(), "person", "", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	resourceKey := int64(7)
	userOwner := DirectVideoTaskOwner(&ProxyAuthContext{Source: "managed", KeyID: &resourceKey, OwnerUserID: &user.ID})
	systemOwner := DirectVideoTaskOwner(&ProxyAuthContext{Source: "managed", KeyID: &resourceKey})
	if userOwner == systemOwner || userOwner != store.UserKeyResourceOwnerPrefix+systemOwner {
		t.Fatal("media ownership namespaces collide")
	}
	var keyID int64
	if err := db.Get(&keyID, `INSERT INTO downstream_api_keys (name,key,key_owner_type,owner_user_id,supported_models) VALUES ('fixture','sk-user-fixture','user',?,'["*"]') RETURNING id`, user.ID); err != nil {
		t.Fatal(err)
	}
	result := AuthorizeDownstreamToken("sk-user-fixture", nil)
	if !result.OK || result.Policy.UserRouteIDs == nil || len(*result.Policy.UserRouteIDs) != 0 {
		t.Fatalf("empty group widened: %+v", result)
	}
	// Diagnostic reads must not accidentally resolve the ID against global DB.
	other := sqlitetest.Open(t)
	var otherID int64
	if err := other.Get(&otherID, `INSERT INTO downstream_api_keys(name,key,key_owner_type,owner_user_id) VALUES ('orphan','sk-orphan-fixture','user',999999) RETURNING id`); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadManagedKeyContextByID(other.DB, otherID)
	if err != nil || loaded == nil || loaded.ExplainAdmission("").Verdict != "deny" || loaded.Policy.UserRouteIDs == nil || len(*loaded.Policy.UserRouteIDs) != 0 {
		t.Fatalf("orphan diagnostic: %+v %v", loaded, err)
	}
	if err := users.UpdateUser(context.Background(), user.ID, "", user.GroupID, false); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("GET", "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer sk-user-fixture")
	w := httptest.NewRecorder()
	ProxyAuth()(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Error("disabled user admitted") })).ServeHTTP(w, req)
	if w.Code != 403 {
		t.Fatalf("disabled user status=%d %s", w.Code, w.Body.String())
	}
	if _, err := db.Exec(`DELETE FROM gateway_users WHERE id=?`, user.ID); err != nil {
		t.Fatal(err)
	}
	if result := ReauthorizeManagedKeyForTurn(context.Background(), "sk-user-fixture", "", 1); result.OK || result.StatusCode != 403 {
		t.Fatalf("missing owner became system Key: %+v", result)
	}
}
