package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterUserBudgetRoutes inherits the admin audit middleware. The local
// guard also protects standalone mounts and requires reauthentication on writes.
func RegisterUserBudgetRoutes(r chi.Router, db *store.DB) {
	h := &userBudgetHandler{users: store.NewGatewayUserStore(db)}
	r.Group(func(r chi.Router) {
		r.Use(requireGatewayUserManagement)
		r.Get("/api/admin/users/{userId}/budget", h.budget)
		r.Put("/api/admin/users/{userId}/budget", h.budget)
		r.Post("/api/admin/users/{userId}/budget/reconcile", h.reconcile)
		r.Get("/api/admin/users/{userId}/budget/holds", h.holds)
		r.Get("/api/admin/user-groups/{groupId}/pricing", h.pricing)
		r.Put("/api/admin/user-groups/{groupId}/pricing", h.pricing)
	})
}

type userBudgetHandler struct{ users *store.GatewayUserStore }

func (h *userBudgetHandler) holds(w http.ResponseWriter, r *http.Request) {
	id, ok := gatewayUserID(w, r, "userId")
	if !ok {
		return
	}
	offset := 0
	if raw := r.URL.Query().Get("offset"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			writeError(w, 400, "Invalid offset")
			return
		}
		offset = value
	}
	rows, err := h.users.ListUserBudgetHolds(r.Context(), id, offset)
	if err != nil {
		gatewayUserError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": rows, "limit": 100, "offset": offset})
}

func (h *userBudgetHandler) reconcile(w http.ResponseWriter, r *http.Request) {
	id, ok := gatewayUserID(w, r, "userId")
	if !ok {
		return
	}
	var body struct {
		RequestID         *string `json:"requestId"`
		TotalTokens       *int64  `json:"totalTokens"`
		ConfirmTerminated *bool   `json:"confirmTerminated"`
	}
	if err := decodeBudgetPayload(r, &body, "requestId", "totalTokens", "confirmTerminated"); err != nil || body.RequestID == nil || body.TotalTokens == nil || body.ConfirmTerminated == nil || !*body.ConfirmTerminated {
		writeError(w, 400, "Explicit requestId, verified totalTokens and confirmTerminated=true required")
		return
	}
	if err := h.users.ReconcileUserBudget(r.Context(), id, *body.RequestID, *body.TotalTokens); err != nil {
		gatewayUserError(w, err)
		return
	}
	out, err := h.users.GetUserBudget(r.Context(), id)
	if err != nil {
		gatewayUserError(w, err)
		return
	}
	writeJSON(w, 200, out)
}

func (h *userBudgetHandler) budget(w http.ResponseWriter, r *http.Request) {
	id, ok := gatewayUserID(w, r, "userId")
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		out, err := h.users.GetUserBudget(r.Context(), id)
		if err != nil {
			gatewayUserError(w, err)
			return
		}
		writeJSON(w, 200, out)
		return
	}
	var body struct {
		Limit     *float64 `json:"limit"`
		Unlimited *bool    `json:"unlimited"`
	}
	if err := decodeBudgetPayload(r, &body, "limit", "unlimited"); err != nil || body.Limit == nil || body.Unlimited == nil {
		writeError(w, 400, "Explicit limit and unlimited required")
		return
	}
	out, err := h.users.SetUserBudget(r.Context(), id, *body.Limit, *body.Unlimited)
	if err != nil {
		gatewayUserError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *userBudgetHandler) pricing(w http.ResponseWriter, r *http.Request) {
	id, ok := gatewayUserID(w, r, "groupId")
	if !ok {
		return
	}
	if r.Method == http.MethodGet {
		out, err := h.users.GetUserGroupPricing(r.Context(), id)
		if err != nil {
			gatewayUserError(w, err)
			return
		}
		writeJSON(w, 200, out)
		return
	}
	var body struct {
		Multiplier *float64 `json:"multiplier"`
	}
	if err := decodeBudgetPayload(r, &body, "multiplier"); err != nil || body.Multiplier == nil {
		writeError(w, 400, "Explicit multiplier required")
		return
	}
	out, err := h.users.SetUserGroupPricing(r.Context(), id, *body.Multiplier)
	if err != nil {
		gatewayUserError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func decodeBudgetPayload(r *http.Request, dst any, fields ...string) error {
	raw, err := decodeJSONRequestRaw(r, dst)
	if err != nil {
		return err
	}
	var obj map[string]json.RawMessage
	if err = json.Unmarshal(raw, &obj); err != nil {
		return err
	}
	if len(obj) != len(fields) {
		return store.ErrGatewayUserInvalid
	}
	for _, field := range fields {
		value, ok := obj[field]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return store.ErrGatewayUserInvalid
		}
	}
	return nil
}
