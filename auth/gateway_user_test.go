package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestGatewayUserPrincipalAndCurrentGroup(t *testing.T) {
	db := sqlitetest.Open(t)
	s := store.NewGatewayUserStore(db)
	ctx := context.Background()
	a, err := s.CreateUserGroup(ctx, "normal", "Normal", true)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateUserGroup(ctx, "advanced", "Advanced", true)
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.CreateUser(ctx, "person", "Person", a.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	ctx = WithGatewayUserPrincipal(WithAdminAuth(ctx), GatewayUserPrincipal{UserID: u.ID})
	if IsAdmin(ctx) {
		t.Fatal("ordinary user retained administrator authority")
	}
	got, err := ResolveGatewayUser(ctx, s)
	if err != nil || got.ID != u.ID || got.GroupID != a.ID {
		t.Fatalf("resolved user: %+v %v", got, err)
	}
	if err := s.UpdateUser(ctx, u.ID, u.DisplayName, b.ID, true); err != nil {
		t.Fatal(err)
	}
	got, err = ResolveGatewayUser(ctx, s)
	if err != nil || got.GroupID != b.ID {
		t.Fatalf("group change not observed: %+v %v", got, err)
	}
	if err := s.UpdateUserGroup(ctx, b.ID, b.Name, false); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveGatewayUser(ctx, s); !errors.Is(err, ErrGatewayUserAccessDenied) {
		t.Fatalf("disabled group accepted: %v", err)
	}
	if err := s.UpdateUserGroup(ctx, b.ID, b.Name, true); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateUser(ctx, u.ID, u.DisplayName, b.ID, false); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveGatewayUser(ctx, s); !errors.Is(err, ErrGatewayUserAccessDenied) {
		t.Fatalf("disabled user accepted: %v", err)
	}
	for _, invalid := range []context.Context{context.Background(), WithAdminAuth(ctx), WithGatewayUserPrincipal(ctx, GatewayUserPrincipal{})} {
		if _, err := ResolveGatewayUser(invalid, s); !errors.Is(err, ErrGatewayUserAccessDenied) {
			t.Fatalf("invalid identity accepted: %v", err)
		}
	}
	var nilStore *store.GatewayUserStore
	for _, reader := range []GatewayUserReader{nil, nilStore, store.NewGatewayUserStore(nil)} {
		if _, err := ResolveGatewayUser(ctx, reader); !errors.Is(err, ErrGatewayUserAccessDenied) {
			t.Fatalf("missing reader accepted: %v", err)
		}
	}
}
