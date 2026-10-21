// Package user owns the ordinary user's independent HTTP surface. It must not
// register any administrator or upstream-management API.
package user

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
			r.Get("/api/user/session", h.session)
			r.Post("/api/user/auth/logout", h.logout)
			r.With(auth.AdminRateLimit(1, 5)).Post("/api/user/auth/password", h.password)
			r.Get("/api/user/models", h.listModels)
			r.Get("/api/user/keys", h.listKeys)
			r.Post("/api/user/keys", h.createKey)
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
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
func resultError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrUserCredentials), errors.Is(err, auth.ErrGatewayUserAccessDenied):
		fail(w, 401, "Invalid user credentials")
	case errors.Is(err, auth.ErrUserPassword), errors.Is(err, store.ErrGatewayUserInvalid):
		fail(w, 400, "Invalid input; password must contain 12 to 72 bytes")
	case errors.Is(err, sql.ErrNoRows):
		fail(w, 404, "Not found")
	default:
		fail(w, 503, "User service unavailable")
	}
}

// Exact field spelling and a bounded single object prevent metadata/identity
// smuggling through Go JSON's case-insensitive matching or trailing documents.
func decode(r *http.Request, dst any) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 16385))
	if err != nil || len(raw) > 16384 {
		return errors.New("invalid body")
	}
	document := json.NewDecoder(bytes.NewReader(raw))
	first, err := document.Token()
	if err != nil || first != json.Delim('{') {
		return errors.New("invalid object")
	}
	fields := map[string]json.RawMessage{}
	for document.More() {
		token, err := document.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("invalid field")
		}
		if _, exists := fields[key]; exists {
			return errors.New("duplicate field")
		}
		var value json.RawMessage
		if err := document.Decode(&value); err != nil {
			return err
		}
		fields[key] = value
	}
	last, err := document.Token()
	if err != nil || last != json.Delim('}') {
		return errors.New("invalid object")
	}
	if _, err := document.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing document")
	}
	typ := reflect.TypeOf(dst).Elem()
	for key, value := range fields {
		known := false
		for i := range typ.NumField() {
			if typ.Field(i).Tag.Get("json") == key {
				known = true
				break
			}
		}
		if !known || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("unknown or null field")
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(dst)
}

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
func (h *handler) listKeys(w http.ResponseWriter, r *http.Request) {
	u, ok := h.current(w, r)
	if !ok {
		return
	}
	items, err := h.users.ListOwnKeys(r.Context(), u.ID)
	if err != nil {
		resultError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
func (h *handler) createKey(w http.ResponseWriter, r *http.Request) {
	u, ok := h.current(w, r)
	if !ok {
		return
	}
	var body struct {
		Name            string    `json:"name"`
		SupportedModels *[]string `json:"supportedModels"`
		ExpiresAt       *string   `json:"expiresAt"`
		MaxCost         *float64  `json:"maxCost"`
		MaxRequests     *int64    `json:"maxRequests"`
	}
	if err := decode(r, &body); err != nil {
		fail(w, 400, "Invalid key payload")
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" || !utf8.ValidString(body.Name) || utf8.RuneCountInString(body.Name) > 100 || (body.MaxCost != nil && (*body.MaxCost < 0 || math.IsNaN(*body.MaxCost) || math.IsInf(*body.MaxCost, 0))) || (body.MaxRequests != nil && *body.MaxRequests < 0) {
		fail(w, 400, "Invalid key limits or name")
		return
	}
	if body.ExpiresAt != nil {
		expires, err := time.Parse(time.RFC3339Nano, *body.ExpiresAt)
		if err != nil || !expires.After(time.Now()) {
			fail(w, 400, "Key expiry must be in the future")
			return
		}
		v := expires.UTC().Format(time.RFC3339Nano)
		body.ExpiresAt = &v
	}
	models := []string{"*"}
	if body.SupportedModels != nil {
		models = []string{}
		seen := map[string]bool{}
		if len(*body.SupportedModels) > 100 {
			fail(w, 400, "Too many model restrictions")
			return
		}
		for _, model := range *body.SupportedModels {
			model = strings.TrimSpace(model)
			if model == "" || len(model) > 200 {
				fail(w, 400, "Invalid model restriction")
				return
			}
			if !seen[model] {
				models = append(models, model)
				seen[model] = true
			}
		}
		if len(models) == 0 {
			fail(w, 400, "Model restrictions must not be empty")
			return
		}
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		resultError(w, err)
		return
	}
	key := "sk-" + hex.EncodeToString(buf)
	item, err := h.users.CreateOwnKey(r.Context(), u.ID, body.Name, key, models, body.ExpiresAt, body.MaxCost, body.MaxRequests)
	if err != nil {
		resultError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"item": item, "key": key})
}
func (h *handler) deleteKey(w http.ResponseWriter, r *http.Request) {
	u, ok := h.current(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, 400, "Invalid key ID")
		return
	}
	if err := h.users.DeleteOwnKey(r.Context(), u.ID, id); err != nil {
		resultError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"success": true})
}
func (h *handler) listRequests(w http.ResponseWriter, r *http.Request) {
	u, ok := h.current(w, r)
	if !ok {
		return
	}
	page, pageSize := 1, 20
	for name, dst := range map[string]*int{"page": &page, "pageSize": &pageSize} {
		if raw := r.URL.Query().Get(name); raw != "" {
			v, err := strconv.Atoi(raw)
			if err != nil || v < 1 {
				fail(w, 400, "Invalid pagination")
				return
			}
			*dst = v
		}
	}
	if page > 1000000 || pageSize > 100 {
		fail(w, 400, "Pagination limit exceeded")
		return
	}
	items, total, err := h.users.ListOwnRequests(r.Context(), u.ID, page, pageSize)
	if err != nil {
		resultError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "page": page, "pageSize": pageSize})
}
func (h *handler) request(w http.ResponseWriter, r *http.Request) {
	u, ok := h.current(w, r)
	if !ok {
		return
	}
	item, err := h.users.OwnRequest(r.Context(), u.ID, chi.URLParam(r, "id"))
	if err != nil {
		resultError(w, err)
		return
	}
	writeJSON(w, 200, item)
}
func (h *handler) usage(w http.ResponseWriter, r *http.Request) {
	u, ok := h.current(w, r)
	if !ok {
		return
	}
	item, err := h.users.OwnUsage(r.Context(), u.ID)
	if err != nil {
		resultError(w, err)
		return
	}
	writeJSON(w, 200, item)
}
func (h *handler) listModels(w http.ResponseWriter, r *http.Request) {
	u, ok := h.current(w, r)
	if !ok {
		return
	}
	_, routes, err := h.users.ResolveUserKeyAccess(r.Context(), u.ID)
	if err != nil {
		resultError(w, err)
		return
	}
	type model struct {
		ID      string `json:"id"`
		RouteID int64  `json:"routeId"`
	}
	items := []model{}
	if len(routes) == 0 {
		writeJSON(w, 200, map[string]any{"items": items})
		return
	}
	if h.models == nil || h.models() == nil {
		fail(w, 503, "Model catalog unavailable")
		return
	}
	catalog := h.models()
	names, err := catalog.GetAvailableModels(r.Context())
	if err != nil {
		resultError(w, err)
		return
	}
	sort.Strings(names)
	for _, name := range names {
		// The router's catalog can contain a wildcard/regex route selector.
		// It is not a concrete public model and is not an actionable model ID.
		if name == "" || !routing.IsExactRouteModelPattern(name) {
			continue
		}
		decision, err := catalog.ExplainSelection(r.Context(), name, nil, routing.DownstreamRoutingPolicy{UserRouteIDs: &routes, SupportedModels: []string{"*"}, RequiredUpstreamProtocol: -1})
		if err != nil {
			resultError(w, err)
			return
		}
		if !decision.Matched || decision.RouteID == nil {
			continue
		}
		for _, candidate := range decision.Candidates {
			if candidate.Eligible {
				items = append(items, model{name, *decision.RouteID})
				break
			}
		}
	}
	writeJSON(w, 200, map[string]any{"items": items})
}
