package admin

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// ---- Accounts/Sites Tag System ----

// accounts.tags / sites.tags store a JSON array text (["prod","priority"]).
// Filtering happens client-side on the full snapshot; this file provides the
// global tag index (GET /api/tags) and per-row writes (PUT.../{id}/tags).

// RegisterTagsRoutes mounts the tag endpoints behind admin auth.
func RegisterTagsRoutes(r chi.Router, db *sqlx.DB) {
	h := &tagsHandler{db: db}
	r.Get("/api/tags", h.listTags)
	r.Put("/api/accounts/{id}/tags", h.updateAccountTags)
	r.Put("/api/sites/{id}/tags", h.updateSiteTags)
}

type tagsHandler struct {
	db *sqlx.DB
}

// parseTagsJSON decodes a JSON array text into a deduped, trimmed string list.
// Returns an error for non-array / invalid JSON so callers can 400.
func parseTagsJSON(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}, nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(list))
	out := make([]string, 0, len(list))
	for _, t := range list {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out, nil
}

// encodeTagsJSON marshals a tag list to the stored JSON array text form.
// Empty list → "" so the column reads as empty (nil on scan).
func encodeTagsJSON(tags []string) string {
	if len(tags) == 0 {
		return ""
	}
	b, err := json.Marshal(tags)
	if err != nil {
		return ""
	}
	return string(b)
}

// GET /api/tags
// Returns the union of account + site tags with per-type counts, sorted by
// total usage descending. Drives filter chips on Accounts/Sites pages.
func (h *tagsHandler) listTags(w http.ResponseWriter, r *http.Request) {
	counts := map[string]map[string]int{} // tag → {"accounts": n, "sites": n}
	addRows := func(table string) error {
		rows, err := queryRowsErr(h.db, "SELECT tags FROM "+table+" WHERE COALESCE(tags, '') <> ''")
		if err != nil {
			return err
		}
		for _, row := range rows {
			tags, err := parseTagsJSON(coerceString(row["tags"]))
			if err != nil {
				continue
			}
			for _, t := range tags {
				if counts[t] == nil {
					counts[t] = map[string]int{"accounts": 0, "sites": 0}
				}
				counts[t][table]++
			}
		}
		return nil
	}
	if err := addRows("accounts"); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load tags")
		return
	}
	if err := addRows("sites"); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load tags")
		return
	}

	items := make([]map[string]any, 0, len(counts))
	for tag, c := range counts {
		items = append(items, map[string]any{
			"name":     tag,
			"accounts": c["accounts"],
			"sites":    c["sites"],
			"total":    c["accounts"] + c["sites"],
		})
	}
	// Stable sort: total desc, then name asc.
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			ti, tj := items[i]["total"].(int), items[j]["total"].(int)
			if ti < tj || (ti == tj && items[i]["name"].(string) > items[j]["name"].(string)) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// decodeTagsBody validates the {tags: []string} body (also tolerates tags as
// a raw JSON array). Returns the normalized list or writes a 400 response.
func decodeTagsBody(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	var body struct {
		Tags *[]string `json:"tags"`
	}
	if err := decodeJSONRequest(r, &body); err != nil || body.Tags == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"message": "body must be {\"tags\": [\"a\", \"b\"]}"})
		return nil, false
	}
	tags := make([]string, 0, len(*body.Tags))
	seen := make(map[string]struct{}, len(*body.Tags))
	for _, t := range *body.Tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		tags = append(tags, t)
	}
	return tags, true
}

// PUT /api/accounts/{id}/tags
func (h *tagsHandler) updateAccountTags(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tags, ok := decodeTagsBody(w, r)
	if !ok {
		return
	}
	var exists int
	if err := h.db.Get(&exists, "SELECT COUNT(*) FROM accounts WHERE id = ?", id); err != nil || exists == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "account not found"})
		return
	}
	_, err := h.db.Exec(rebindAdminQuery(h.db, "UPDATE accounts SET tags = ? WHERE id = ?"), encodeTagsJSON(tags), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to update account tags"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tags": tags})
}

// PUT /api/sites/{id}/tags
func (h *tagsHandler) updateSiteTags(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	tags, ok := decodeTagsBody(w, r)
	if !ok {
		return
	}
	var exists int
	if err := h.db.Get(&exists, "SELECT COUNT(*) FROM sites WHERE id = ?", id); err != nil || exists == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"message": "site not found"})
		return
	}
	_, err := h.db.Exec(rebindAdminQuery(h.db, "UPDATE sites SET tags = ? WHERE id = ?"), encodeTagsJSON(tags), id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"message": "failed to update site tags"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "tags": tags})
}
