package auth

import (
	"context"
	"errors"

	"github.com/deliciousbuding/metapi-go/store"
)

// GatewayUserPrincipal is set after user authentication. Group and enabled
// state are always loaded from current storage, never supplied by the client.
type GatewayUserPrincipal struct{ UserID int64 }

type gatewayUserPrincipalKey struct{}

func WithGatewayUserPrincipal(ctx context.Context, principal GatewayUserPrincipal) context.Context {
	ctx = context.WithValue(ctx, adminAuthKey, false)
	return context.WithValue(ctx, gatewayUserPrincipalKey{}, principal)
}

func GetGatewayUserPrincipal(ctx context.Context) (GatewayUserPrincipal, bool) {
	p, ok := ctx.Value(gatewayUserPrincipalKey{}).(GatewayUserPrincipal)
	return p, ok && p.UserID > 0 && !IsAdmin(ctx)
}

var ErrGatewayUserAccessDenied = errors.New("gateway user access denied")

type GatewayUserReader interface {
	// Reload user and current group enabled state; unavailable storage denies.
	GetActiveGatewayUser(context.Context, int64) (store.GatewayUser, error)
}

// ResolveGatewayUser establishes the current ordinary-user scope. Resource
// queries still require this user's ID; this does not grant admin authority.
func ResolveGatewayUser(ctx context.Context, users GatewayUserReader) (store.GatewayUser, error) {
	principal, ok := GetGatewayUserPrincipal(ctx)
	if !ok || users == nil {
		return store.GatewayUser{}, ErrGatewayUserAccessDenied
	}
	user, err := users.GetActiveGatewayUser(ctx, principal.UserID)
	if err != nil {
		return store.GatewayUser{}, errors.Join(ErrGatewayUserAccessDenied, err)
	}
	if user.ID != principal.UserID || user.GroupID <= 0 || !user.Enabled {
		return store.GatewayUser{}, ErrGatewayUserAccessDenied
	}
	return user, nil
}
