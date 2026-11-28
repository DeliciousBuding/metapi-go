package user

import (
	"crypto/rand"
	"encoding/hex"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
)

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

func newUserKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "sk-" + hex.EncodeToString(buf), nil
}

func keyID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, 400, "Invalid key ID")
		return 0, false
	}
	return id, true
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
	key, err := newUserKey()
	if err != nil {
		resultError(w, err)
		return
	}
	item, err := h.users.CreateOwnKey(r.Context(), u.ID, body.Name, key, models, body.ExpiresAt, body.MaxCost, body.MaxRequests)
	if err != nil {
		resultError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"item": item, "key": key})
}
func (h *handler) rotateKey(w http.ResponseWriter, r *http.Request) {
	u, ok := h.current(w, r)
	if !ok {
		return
	}
	id, ok := keyID(w, r)
	if !ok {
		return
	}
	key, err := newUserKey()
	if err != nil {
		resultError(w, err)
		return
	}
	item, err := h.users.RotateOwnKey(r.Context(), u.ID, id, key)
	if err != nil {
		resultError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"item": item, "key": key})
}
func (h *handler) deleteKey(w http.ResponseWriter, r *http.Request) {
	u, ok := h.current(w, r)
	if !ok {
		return
	}
	id, ok := keyID(w, r)
	if !ok {
		return
	}
	if err := h.users.DeleteOwnKey(r.Context(), u.ID, id); err != nil {
		resultError(w, err)
		return
	}
	writeJSON(w, 200, map[string]bool{"success": true})
}
