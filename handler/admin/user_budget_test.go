package admin

import (
	"context"
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

func TestUserBudgetAdministratorAPI(t *testing.T) {
	db := sqlitetest.Open(t)
	users := store.NewGatewayUserStore(db)
	ctx := context.Background()
	u, err := users.CreateUser(ctx, "budget-admin-fixture", "", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	runtime := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{AuthToken: "budget-admin-confirm"})
	defer config.SetRuntime(runtime)
	r := chi.NewRouter()
	r.Use(AuditMiddleware(db.DB))
	RegisterUserBudgetRoutes(r, db)
	path := fmt.Sprintf("/api/admin/users/%d/budget", u.ID)
	pricing := fmt.Sprintf("/api/admin/user-groups/%d/pricing", u.GroupID)
	call := func(admin, confirm bool, method, path, body string, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if admin {
			req = req.WithContext(auth.WithAdminAuth(req.Context()))
		} else {
			req = req.WithContext(auth.WithGatewayUserPrincipal(req.Context(), auth.GatewayUserPrincipal{UserID: u.ID}))
		}
		if confirm {
			req.Header.Set(auth.ReauthConfirmHeader, "budget-admin-confirm")
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s want %d", method, path, w.Code, w.Body.String(), want)
		}
		if !strings.Contains(w.Header().Get("Cache-Control"), "no-store") {
			t.Fatal("budget cacheable")
		}
		return w
	}
	for _, p := range []string{path, pricing, path + "/holds"} {
		call(false, true, "GET", p, "", 403)
	}
	call(true, false, "PUT", path, `{"limit":1,"unlimited":false}`, 403)
	for _, body := range []string{`{}`, `{"limit":0}`, `{"limit":null,"unlimited":false}`, `{"limit":1,"unlimited":null}`, `{"limit":-1,"unlimited":false}`, `{"limit":1,"Unlimited":true}`, `{"limit":1,"unlimited":false,"used":0}`, `{"limit":1,"limit":0,"unlimited":false}`, `{"limit":1.00000001,"unlimited":false}`} {
		call(true, true, "PUT", path, body, 400)
	}
	if w := call(true, true, "PUT", path, `{"limit":1,"unlimited":false}`, 200); !strings.Contains(w.Body.String(), `"limit":1`) {
		t.Fatalf("wrong DTO %s", w.Body.String())
	}
	for _, body := range []string{`{}`, `{"multiplier":0}`, `{"multiplier":null}`, `{"multiplier":-1}`, `{"multiplier":1,"used":0}`} {
		call(true, true, "PUT", pricing, body, 400)
	}
	call(true, true, "PUT", pricing, `{"multiplier":2}`, 200)
	if _, err = users.ReserveUserBudget(ctx, "req_admin_fixture", u.ID, u.GroupID, 100); err != nil {
		t.Fatal(err)
	}
	if err = users.SettleUserBudget(ctx, "req_admin_fixture", nil, true); err != nil {
		t.Fatal(err)
	}
	if w := call(true, false, "GET", path+"/holds", "", 200); !strings.Contains(w.Body.String(), `"requestId":"req_admin_fixture"`) {
		t.Fatal("hold unavailable")
	}
	for _, body := range []string{`{"requestId":"req_admin_fixture","totalTokens":10}`, `{"requestId":"req_admin_fixture","totalTokens":10,"confirmTerminated":false}`, `{"requestId":"req_admin_fixture","totalTokens":10,"confirmTerminated":null}`} {
		call(true, true, "POST", path+"/reconcile", body, 400)
	}
	valid := `{"requestId":"req_admin_fixture","totalTokens":10,"confirmTerminated":true}`
	call(true, false, "POST", path+"/reconcile", valid, 403)
	release := store.TrackUserBudgetRequest("req_admin_fixture")
	call(true, true, "POST", path+"/reconcile", valid, 409)
	release()
	call(true, true, "POST", path+"/reconcile", valid, 200)
	call(true, true, "POST", path+"/reconcile", valid, 200)
	call(true, true, "POST", path+"/reconcile", `{"requestId":"req_admin_fixture","totalTokens":0,"confirmTerminated":true}`, 409)
	call(true, false, "GET", path+"/holds?offset=-1", "", 400)
	var audited int
	if err = db.Get(&audited, `SELECT COUNT(*) FROM admin_audit_logs WHERE method='POST' AND path=? AND status=200`, path+"/reconcile"); err != nil || audited != 2 {
		t.Fatalf("reconciliation audit=%d %v", audited, err)
	}
}
