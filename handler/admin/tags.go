package admin

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// ---- Accounts/Sites Tag System ----

// accounts.tags / sites.tags store a JSON array text (["prod","priority"]).
// Filtering happens client-side on the full snapshot; this file provides the
// global tag index (GET /api/tags) and per-row writes (PUT.../{id}/tags).

// RegisterTagsRoutes mounts the tag endpoints behind admin auth.
func RegisterTagsRoutes(r chi.Router, db *store.DB) {
	h := &tagsHandler{db: db}
	r.Get("/api/tags", h.listTags)
	r.Put("/api/accounts/{id}/tags", h.updateTags("account"))
	r.Put("/api/sites/{id}/tags", h.updateTags("site"))
}

type tagsHandler struct {
	db *store.DB
}

// GET /api/tags
// Returns the union of account + site tags with per-type counts, sorted by
// total usage descending. Drives filter chips on Accounts/Sites pages.
func (h *tagsHandler) listTags(w http.ResponseWriter, r *http.Request) {
	items, err := h.db.TagUsage()
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load tags")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// decodeTagsBody validates the {tags: []string} body and normalizes its tags.
func decodeTagsBody(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	var body struct {
		Tags *[]string `json:"tags"`
	}
	if err := decodeJSONRequest(r, &body); err != nil || body.Tags == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "body must be {\"tags\": [\"a\", \"b\"]}"})
		return nil, false
	}
	return store.NormalizeTags(*body.Tags), true
}

func (h *tagsHandler) updateTags(owner string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		tags, ok := decodeTagsBody(w, r)
		if !ok {
			return
		}
		err := h.db.SetTags(owner, id, tags)
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"message": owner + " not found"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to update " + owner + " tags"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "tags": tags})
	}
}
