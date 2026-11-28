package user

import (
	"net/http"
	"strconv"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func (h *handler) listRequests(w http.ResponseWriter, r *http.Request) {
	u, ok := h.current(w, r)
	if !ok {
		return
	}
	page, pageSize := 1, 20
	query := r.URL.Query()
	for _, name := range []string{"page", "pageSize", "search", "status", "quotaState"} {
		if len(query[name]) > 1 {
			fail(w, 400, "Request filters must be single values")
			return
		}
	}
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
	filter := store.UserRequestFilter{Search: query.Get("search"), Status: query.Get("status"), QuotaState: query.Get("quotaState")}
	if err := filter.Validate(); err != nil {
		fail(w, 400, err.Error())
		return
	}
	items, total, err := h.users.ListOwnRequestsFiltered(r.Context(), u.ID, page, pageSize, filter)
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
