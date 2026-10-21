package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestGatewayUserAdministration(t *testing.T) {
	db := sqlitetest.Open(t)
	config.SetRuntime(&config.RuntimeSettings{AuthToken: "user-admin-fixture"})
	t.Cleanup(func() { config.SetRuntime(nil) })
	r := chi.NewRouter()
	RegisterGatewayUserRoutes(r, db)
	admin := auth.WithAdminAuth(context.Background())
	ordinary := auth.WithGatewayUserPrincipal(admin, auth.GatewayUserPrincipal{UserID: 7})
	call := func(ctx context.Context, method, path, body string, confirm bool, expected int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
		if confirm {
			req.Header.Set(auth.ReauthConfirmHeader, "user-admin-fixture")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != expected {
			t.Fatalf("%s %s: %d %s, want %d", method, path, w.Code, w.Body.String(), expected)
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("user response cacheable")
		}
		return w
	}
	for _, endpoint := range []struct{ method, path string }{
		{"GET", "/api/admin/users"}, {"POST", "/api/admin/users"},
		{"GET", "/api/admin/users/1"}, {"PUT", "/api/admin/users/1"},
		{"GET", "/api/admin/user-groups"}, {"POST", "/api/admin/user-groups"},
		{"GET", "/api/admin/user-groups/1"}, {"PUT", "/api/admin/user-groups/1"},
		{"GET", "/api/admin/user-groups/1/routes"}, {"POST", "/api/admin/user-groups/1/routes"},
		{"DELETE", "/api/admin/user-groups/1/routes/1"},
	} {
		call(ordinary, endpoint.method, endpoint.path, `{}`, true, 403)
		if endpoint.method != "GET" {
			call(admin, endpoint.method, endpoint.path, `{}`, false, 403)
		}
	}
	for _, body := range []string{`null`, `{}`, `{"slug":"fixture","name":"Fixture"}`, `{"slug":"fixture","name":"Fixture","enabled":null}`, `{"slug":"fixture","name":"Fixture","enabled":true,"Enabled":false}`, `{"slug":"fixture","name":"Fixture","name":"other","enabled":true}`, `{"slug":"fixture","name":"Fixture","enabled":true,"role":"admin"}`, `{"slug":"fixture","name":"Fixture","enabled":true}{}`} {
		call(admin, "POST", "/api/admin/user-groups", body, true, 400)
	}
	w := call(admin, "POST", "/api/admin/user-groups", `{"slug":"fixture","name":"Fixture","enabled":true}`, true, 201)
	var group store.GatewayUserGroup
	if err := json.Unmarshal(w.Body.Bytes(), &group); err != nil {
		t.Fatal(err)
	}
	groupPath := fmt.Sprintf("/api/admin/user-groups/%d", group.ID)
	call(admin, "GET", groupPath, "", false, 200)
	call(admin, "PUT", groupPath, `{"slug":"changed","name":"Fixture","enabled":true}`, true, 400)
	call(admin, "POST", "/api/admin/user-groups", `{"slug":"fixture","name":"Duplicate","enabled":true}`, true, 409)
	body := fmt.Sprintf(`{"username":"person","displayName":"Person","groupId":%d,"enabled":true}`, group.ID)
	w = call(admin, "POST", "/api/admin/users", body, true, 201)
	var user store.GatewayUser
	if err := json.Unmarshal(w.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	userPath := fmt.Sprintf("/api/admin/users/%d", user.ID)
	call(admin, "GET", userPath, "", false, 200)
	call(admin, "POST", "/api/admin/users", body, true, 409)
	call(admin, "PUT", userPath, body, true, 400) // Username is immutable.
	call(admin, "PUT", userPath, `{"displayName":"Person","groupId":999999,"enabled":true}`, true, 409)
	call(admin, "POST", "/api/admin/users", `{"username":"other","displayName":"Other","groupId":1,"enabled":true,"password":"fixture"}`, true, 400)
	call(admin, "POST", "/api/admin/users", `{"username":"null-group","displayName":"Other","groupId":null,"enabled":true}`, true, 400)
	call(admin, "GET", "/api/admin/users/999999", "", false, 404)
	call(admin, "GET", "/api/admin/users/0", "", false, 400)
	call(admin, "GET", "/api/admin/users", "", false, 200)
	call(admin, "GET", "/api/admin/user-groups", "", false, 200)
	var routeID int64
	if err := db.Get(&routeID, `INSERT INTO token_routes (model_pattern, enabled, created_at, updated_at) VALUES ('user-fixture', TRUE, 'now', 'now') RETURNING id`); err != nil {
		t.Fatal(err)
	}
	routeBody := fmt.Sprintf(`{"routeId":%d}`, routeID)
	call(admin, "POST", groupPath+"/routes", routeBody, true, 200)
	call(admin, "POST", groupPath+"/routes", routeBody, true, 409)
	call(admin, "GET", groupPath+"/routes", "", false, 200)
	call(admin, "DELETE", fmt.Sprintf("%s/routes/%d", groupPath, routeID), "", true, 200)
	call(admin, "PUT", userPath, fmt.Sprintf(`{"displayName":"Person","groupId":%d,"enabled":false}`, group.ID), true, 200)
}

func TestGatewayUserAdministrationUnavailable(t *testing.T) {
	r := chi.NewRouter()
	RegisterGatewayUserRoutes(r, nil)
	req := httptest.NewRequest("GET", "/api/admin/users", nil).WithContext(auth.WithAdminAuth(context.Background()))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 503 {
		t.Fatalf("unavailable store status=%d", w.Code)
	}
}
