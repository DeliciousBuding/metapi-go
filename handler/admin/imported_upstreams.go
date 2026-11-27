package admin

import (
	"net/http"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// Imported upstreams are distinct from native account-backed route channels.
// These projections deliberately never select credentials or custom headers.
func RegisterImportedUpstreamRoutes(r chi.Router, db *store.DB) {
	RegisterChannelPluginRoutes(r, db)
	RegisterUpstreamCatalogRoutes(r, db)
	RegisterUpstreamLifecycleRoutes(r, db)
	RegisterUpstreamPresetRoutes(r)
	h := &importedUpstreamHandler{db: db}
	r.Get("/api/imported-upstreams", h.list)
	r.Get("/api/imported-upstreams/{id}", h.detail)
	r.Get("/api/imported-upstreams/{id}/request-config", h.requestConfig)
	r.Patch("/api/imported-upstreams/{id}", h.update)
	r.Patch("/api/imported-upstreams/members/{id}", h.updateMember)
	r.Post("/api/imported-upstreams/members/{id}/cooldown/clear", h.clearMemberCooldown)
	r.Get("/api/imported-upstreams/{id}/credentials", h.credentials)
	r.Patch("/api/imported-upstreams/credentials/{id}", h.updateCredential)
}

type importedUpstreamHandler struct{ db *store.DB }

func (h *importedUpstreamHandler) list(w http.ResponseWriter, r *http.Request) {
	channels, members, err := upstream.ListImported(h.db)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": normalizeSlice(channels), "members": normalizeSlice(members)})
}
