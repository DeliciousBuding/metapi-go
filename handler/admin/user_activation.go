package admin

import (
	"net/http"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterUserActivationRoutes is admin-only, reconfirmed and audited by the
// existing admin group. Reset codes appear only in this no-store POST result.
func RegisterUserActivationRoutes(r chi.Router, db *store.DB, sessions *auth.UserSessionManager) {
	r.With(requireGatewayUserManagement).Post("/api/admin/users/{userId}/activation", func(w http.ResponseWriter, r *http.Request) {
		id, ok := gatewayUserID(w, r, "userId")
		if !ok {
			return
		}
		code, expires, err := sessions.IssueActivation(r.Context(), id)
		if err != nil {
			gatewayUserError(w, err)
			return
		}
		setNoStore(w)
		writeJSON(w, 200, map[string]string{"code": code, "expiresAt": expires.UTC().Format(time.RFC3339Nano)})
	})
}
