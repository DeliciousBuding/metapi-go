package user

import (
	"net/http"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterBudgetRoutes must be mounted inside ordinary-user session auth, not
// admin auth. ResolveGatewayUser independently rejects missing/disabled scope.
func RegisterBudgetRoutes(r chi.Router, db *store.DB) {
	users := store.NewGatewayUserStore(db)
	r.Get("/api/user/budget", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		user, err := auth.ResolveGatewayUser(r.Context(), users)
		if err != nil {
			shared.WriteJSON(w, http.StatusForbidden, map[string]string{"error": "User access denied"})
			return
		}
		budget, err := users.GetUserBudget(r.Context(), user.ID)
		if err != nil {
			shared.WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "User budget unavailable"})
			return
		}
		shared.WriteJSON(w, http.StatusOK, budget)
	})
}
