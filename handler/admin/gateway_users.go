package admin

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strconv"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"modernc.org/sqlite"
)

// RegisterGatewayUserRoutes configures the single system's ordinary users and
// groups. Only the deployment administrator may use this surface. It does not
// issue a user credential, session or downstream Key.
func RegisterGatewayUserRoutes(r chi.Router, db *store.DB) {
	h := &gatewayUserHandler{users: store.NewGatewayUserStore(db)}
	r.Group(func(r chi.Router) {
		r.Use(requireGatewayUserManagement)
		r.Get("/api/admin/users", h.listUsers)
		r.Post("/api/admin/users", h.createUser)
		r.Get("/api/admin/users/{userId}", h.getUser)
		r.Put("/api/admin/users/{userId}", h.updateUser)
		r.Get("/api/admin/user-groups", h.listGroups)
		r.Post("/api/admin/user-groups", h.createGroup)
		r.Get("/api/admin/user-groups/{groupId}", h.getGroup)
		r.Put("/api/admin/user-groups/{groupId}", h.updateGroup)
		r.Get("/api/admin/user-groups/{groupId}/routes", h.listRoutes)
		r.Post("/api/admin/user-groups/{groupId}/routes", h.grantRoute)
		r.Delete("/api/admin/user-groups/{groupId}/routes/{routeId}", h.revokeRoute)
	})
}

func requireGatewayUserManagement(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setNoStore(w)
		if !auth.IsAdmin(r.Context()) {
			writeError(w, http.StatusForbidden, "System administrator access required")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !auth.ReauthConfirmed(r) {
			auth.WriteReauthRequired(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type gatewayUserHandler struct{ users *store.GatewayUserStore }

type gatewayUserCreatePayload struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	GroupID     int64  `json:"groupId"`
	Enabled     bool   `json:"enabled"`
}

type gatewayUserUpdatePayload struct {
	DisplayName string `json:"displayName"`
	GroupID     int64  `json:"groupId"`
	Enabled     bool   `json:"enabled"`
}

type gatewayGroupCreatePayload struct {
	Slug    string `json:"slug"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

type gatewayGroupUpdatePayload struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

func gatewayUserID(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "Invalid user or group ID")
		return 0, false
	}
	return id, true
}

func gatewayUserError(w http.ResponseWriter, err error) {
	var pgErr *pgconn.PgError
	var sqliteErr *sqlite.Error
	conflict := errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23503")
	if errors.As(err, &sqliteErr) {
		conflict = sqliteErr.Code() == 1555 || sqliteErr.Code() == 2067 || sqliteErr.Code() == 787
	}
	switch {
	case errors.Is(err, store.ErrGatewayUserUnavailable):
		writeError(w, http.StatusServiceUnavailable, "User store unavailable")
	case conflict:
		writeError(w, http.StatusConflict, "User or group conflicts with existing data")
	case errors.Is(err, store.ErrGatewayUserInvalid):
		writeError(w, http.StatusBadRequest, "Invalid user or group input")
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, http.StatusNotFound, "User, group or route not found")
	default:
		writeError(w, http.StatusInternalServerError, "Could not persist user or group")
	}
}

// Each new API accepts only its canonical fields; credentials, role and
// ownership cannot be planted through metadata or a case-insensitive alias.
func decodeGatewayUserRequest(r *http.Request, dst any) error {
	raw, err := decodeJSONRequestRaw(r, dst)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || len(fields) == 0 {
		return errors.New("user payload must be a nonempty object")
	}
	typeOf := reflect.TypeOf(dst).Elem()
	for key := range fields {
		known := false
		for i := range typeOf.NumField() {
			if typeOf.Field(i).Tag.Get("json") == key {
				known = true
				break
			}
		}
		if !known {
			return errors.New("unknown user field")
		}
	}
	if _, needsEnabled := typeOf.FieldByName("Enabled"); needsEnabled {
		value, present := fields["enabled"]
		if !present || (string(value) != "true" && string(value) != "false") {
			return errors.New("enabled must be an explicit boolean")
		}
	}
	// Omission/zero deliberately selects the default on user creation. JSON
	// null must not silently become that selector through integer decoding.
	if value, present := fields["groupId"]; present && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return errors.New("groupId must be an integer")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(dst)
}

func (h *gatewayUserHandler) listUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.users.ListUsers(r.Context())
	if err != nil {
		gatewayUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows, "limit": store.GatewayUserDirectoryLimit})
}

func (h *gatewayUserHandler) getUser(w http.ResponseWriter, r *http.Request) {
	id, ok := gatewayUserID(w, r, "userId")
	if !ok {
		return
	}
	row, err := h.users.GetUser(r.Context(), id)
	if err != nil {
		gatewayUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *gatewayUserHandler) createUser(w http.ResponseWriter, r *http.Request) {
	var body gatewayUserCreatePayload
	if err := decodeGatewayUserRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid user payload")
		return
	}
	row, err := h.users.CreateUser(r.Context(), body.Username, body.DisplayName, body.GroupID, body.Enabled)
	if err != nil {
		gatewayUserError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (h *gatewayUserHandler) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := gatewayUserID(w, r, "userId")
	if !ok {
		return
	}
	var body gatewayUserUpdatePayload
	if err := decodeGatewayUserRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid user payload")
		return
	}
	if err := h.users.UpdateUser(r.Context(), id, body.DisplayName, body.GroupID, body.Enabled); err != nil {
		gatewayUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *gatewayUserHandler) listGroups(w http.ResponseWriter, r *http.Request) {
	rows, err := h.users.ListUserGroups(r.Context())
	if err != nil {
		gatewayUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows, "limit": store.GatewayUserDirectoryLimit})
}

func (h *gatewayUserHandler) getGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := gatewayUserID(w, r, "groupId")
	if !ok {
		return
	}
	row, err := h.users.GetUserGroup(r.Context(), id)
	if err != nil {
		gatewayUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, row)
}

func (h *gatewayUserHandler) createGroup(w http.ResponseWriter, r *http.Request) {
	var body gatewayGroupCreatePayload
	if err := decodeGatewayUserRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid group payload")
		return
	}
	row, err := h.users.CreateUserGroup(r.Context(), body.Slug, body.Name, body.Enabled)
	if err != nil {
		gatewayUserError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, row)
}

func (h *gatewayUserHandler) updateGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := gatewayUserID(w, r, "groupId")
	if !ok {
		return
	}
	var body gatewayGroupUpdatePayload
	if err := decodeGatewayUserRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid group payload")
		return
	}
	if err := h.users.UpdateUserGroup(r.Context(), id, body.Name, body.Enabled); err != nil {
		gatewayUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *gatewayUserHandler) listRoutes(w http.ResponseWriter, r *http.Request) {
	id, ok := gatewayUserID(w, r, "groupId")
	if !ok {
		return
	}
	rows, err := h.users.ListUserGroupRouteGrants(r.Context(), id)
	if err != nil {
		gatewayUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (h *gatewayUserHandler) grantRoute(w http.ResponseWriter, r *http.Request) {
	id, ok := gatewayUserID(w, r, "groupId")
	if !ok {
		return
	}
	var body struct {
		RouteID int64 `json:"routeId"`
	}
	if err := decodeGatewayUserRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid route grant payload")
		return
	}
	if err := h.users.GrantUserGroupRoute(r.Context(), id, body.RouteID); err != nil {
		gatewayUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

func (h *gatewayUserHandler) revokeRoute(w http.ResponseWriter, r *http.Request) {
	groupID, ok := gatewayUserID(w, r, "groupId")
	if !ok {
		return
	}
	routeID, ok := gatewayUserID(w, r, "routeId")
	if !ok {
		return
	}
	if err := h.users.RevokeUserGroupRoute(r.Context(), groupID, routeID); err != nil {
		gatewayUserError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}
