package user

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestUserBudgetOnlyCurrentOwner(t *testing.T) {
	db := sqlitetest.Open(t)
	users := store.NewGatewayUserStore(db)
	ctx := context.Background()
	u, err := users.CreateUser(ctx, "budget-first", "", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	other, err := users.CreateUser(ctx, "budget-other", "", u.GroupID, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = users.SetUserBudget(ctx, u.ID, 1, false); err != nil {
		t.Fatal(err)
	}
	if _, err = users.SetUserBudget(ctx, other.ID, 999, true); err != nil {
		t.Fatal(err)
	}
	r := chi.NewRouter()
	RegisterBudgetRoutes(r, db)
	call := func(ctx context.Context, want int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/user/budget?userId=999", nil).WithContext(ctx)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != want {
			t.Fatalf("status %d %s", w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("budget cacheable")
		}
		return w
	}
	call(ctx, 403)
	call(auth.WithAdminAuth(ctx), 403)
	userCtx := auth.WithGatewayUserPrincipal(ctx, auth.GatewayUserPrincipal{UserID: u.ID})
	var budget store.UserBudget
	if err = json.Unmarshal(call(userCtx, 200).Body.Bytes(), &budget); err != nil || budget.Limit != 1 || budget.Unlimited {
		t.Fatalf("wrong user budget %+v %v", budget, err)
	}
	if err = users.UpdateUser(ctx, u.ID, "", u.GroupID, false); err != nil {
		t.Fatal(err)
	}
	call(userCtx, 403)
}
