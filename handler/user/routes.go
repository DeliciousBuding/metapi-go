// Package user owns the ordinary user's independent HTTP surface. It must not
// register any administrator or upstream-management API.
package user

import (
	"context"
	"net/http"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

type ModelCatalog interface {
	GetAvailableModels(context.Context) ([]string, error)
	ExplainSelection(context.Context, string, []int64, routing.DownstreamRoutingPolicy) (routing.RouteDecisionExplanation, error)
}
type handler struct {
	users      *store.GatewayUserStore
	sessions   *auth.UserSessionManager
	secureMode string
	models     func() ModelCatalog
}

// RegisterRoutes is mounted OUTSIDE the admin group. It owns its cookie-only
// auth and same-origin boundaries; no bearer/confirmation header grants access.
func RegisterRoutes(r chi.Router, db *store.DB, sessions *auth.UserSessionManager, secureMode string, models func() ModelCatalog) {
	h := &handler{store.NewGatewayUserStore(db), sessions, secureMode, models}
	r.Group(func(r chi.Router) {
		r.Use(noStore)
		r.Use(auth.UserSameOriginGuard)
		r.Use(auth.AdminRateLimit(10, 30))
		r.Group(func(r chi.Router) {
			r.Use(auth.AdminRateLimit(1, 5))
			r.Post("/api/user/auth/activate", h.activate)
			r.Post("/api/user/auth/login", h.login)
		})
		r.Group(func(r chi.Router) {
			r.Use(auth.UserSessionAuth(sessions))
			RegisterBudgetRoutes(r, db)
			r.Get("/api/user/session", h.session)
			r.Post("/api/user/auth/logout", h.logout)
			r.With(auth.AdminRateLimit(1, 5)).Post("/api/user/auth/password", h.password)
			r.Get("/api/user/models", h.listModels)
			r.Get("/api/user/keys", h.listKeys)
			r.Post("/api/user/keys", h.createKey)
			r.With(auth.AdminRateLimit(1, 5)).Post("/api/user/keys/{id}/rotate", h.rotateKey)
			r.Delete("/api/user/keys/{id}", h.deleteKey)
			r.Get("/api/user/requests", h.listRequests)
			r.Get("/api/user/requests/{id}", h.request)
			r.Get("/api/user/usage", h.usage)
		})
	})
}

func noStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, private")
		w.Header().Set("Pragma", "no-cache")
		next.ServeHTTP(w, r)
	})
}
