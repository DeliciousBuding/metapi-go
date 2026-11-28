package user

import (
	"net/http"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/store"
)

type loginPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	if h.sessions == nil {
		fail(w, 503, "User sessions unavailable")
		return
	}
	var body loginPayload
	if err := decode(r, &body); err != nil || len(body.Username) > 64 || len(body.Password) > 72 {
		fail(w, 400, "Invalid login payload")
		return
	}
	if !h.sessions.AllowAccount(body.Username) {
		fail(w, 429, "Too many authentication attempts")
		return
	}
	h.finishLogin(w, r, body.Username, body.Password)
}
func (h *handler) finishLogin(w http.ResponseWriter, r *http.Request, username, password string) {
	raw, _, err := h.sessions.Login(r.Context(), username, password)
	if err != nil {
		resultError(w, err)
		return
	}
	auth.SetUserSessionCookie(w, r, raw, h.secureMode)
	writeJSON(w, 200, map[string]bool{"success": true})
}
func (h *handler) activate(w http.ResponseWriter, r *http.Request) {
	if h.sessions == nil {
		fail(w, 503, "User sessions unavailable")
		return
	}
	var body struct {
		Username string `json:"username"`
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil || len(body.Username) > 64 {
		fail(w, 400, "Invalid activation payload")
		return
	}
	if !h.sessions.AllowAccount(body.Username) {
		fail(w, 429, "Too many authentication attempts")
		return
	}
	if err := h.sessions.Activate(r.Context(), body.Username, body.Code, body.Password); err != nil {
		resultError(w, err)
		return
	}
	h.finishLogin(w, r, body.Username, body.Password)
}
func (h *handler) current(w http.ResponseWriter, r *http.Request) (store.GatewayUser, bool) {
	u, err := auth.ResolveGatewayUser(r.Context(), h.users)
	if err != nil {
		resultError(w, err)
		return u, false
	}
	return u, true
}
func (h *handler) session(w http.ResponseWriter, r *http.Request) {
	u, ok := h.current(w, r)
	if !ok {
		return
	}
	g, err := h.users.GetUserGroup(r.Context(), u.GroupID)
	if err != nil {
		resultError(w, err)
		return
	}
	s := auth.GetUserSession(r.Context())
	if s == nil {
		fail(w, 401, "User login required")
		return
	}
	writeJSON(w, 200, map[string]any{"user": map[string]any{"id": u.ID, "username": u.Username, "displayName": u.DisplayName, "groupId": u.GroupID}, "group": map[string]any{"id": g.ID, "name": g.Name}, "expiresAt": s.ExpiresAt})
}
func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.UserSessionCookieName)
	if err != nil {
		fail(w, 401, "User login required")
		return
	}
	if err := h.sessions.Revoke(r.Context(), cookie.Value); err != nil {
		resultError(w, err)
		return
	}
	auth.ClearUserSessionCookie(w, r, h.secureMode)
	writeJSON(w, 200, map[string]bool{"success": true})
}
func (h *handler) password(w http.ResponseWriter, r *http.Request) {
	u, ok := h.current(w, r)
	if !ok {
		return
	}
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := decode(r, &body); err != nil || len(body.CurrentPassword) > 72 {
		fail(w, 400, "Invalid password payload")
		return
	}
	if err := h.sessions.ChangePassword(r.Context(), u.ID, u.Username, body.CurrentPassword, body.NewPassword); err != nil {
		resultError(w, err)
		return
	}
	auth.ClearUserSessionCookie(w, r, h.secureMode)
	writeJSON(w, 200, map[string]bool{"success": true})
}
