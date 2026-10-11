package admin

import (
	"errors"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterUpstreamCatalogRoutes adds explicit native catalog creation without
// taking over the imported connection/member maintenance endpoints.
func RegisterUpstreamCatalogRoutes(r chi.Router, db *sqlx.DB) {
	h := &upstreamCatalogHandler{db: db}
	r.Post("/api/imported-upstreams", h.createChannel)
	r.Post("/api/imported-upstreams/connect", h.connect)
	r.Post("/api/imported-upstreams/{id}/credentials", h.createCredential)
	r.Get("/api/imported-upstreams/{id}/models", h.models)
	r.Post("/api/imported-upstreams/{id}/models", h.createModels)
	r.Patch("/api/imported-upstreams/models/{id}", h.updateModel)
	r.Post("/api/imported-upstreams/grants", h.createGrant)
	r.Patch("/api/imported-upstreams/grants/{id}", h.updateGrant)
	r.Get("/api/imported-upstreams/groups", h.groups)
	r.Post("/api/imported-upstreams/groups", h.createGroup)
	r.Patch("/api/imported-upstreams/groups/{id}", h.updateGroup)
	r.Post("/api/imported-upstreams/groups/{id}/members", h.createMember)
}

type upstreamCatalogHandler struct{ db *sqlx.DB }

func catalogError(w http.ResponseWriter, err error) {
	var domain *upstream.Error
	if errors.As(err, &domain) {
		if len(domain.MemberIDs) > 0 {
			writeJSON(w, domain.Status, map[string]any{"error": domain.Message, "conflictingMemberIds": domain.MemberIDs})
		} else {
			writeError(w, domain.Status, domain.Message)
		}
		return
	}
	if errors.Is(err, oauth.ErrInvalidDirectCredential) {
		writeError(w, 400, "Invalid credential material or name")
		return
	}
	importedWriteError(w, err)
}
func catalogDecode(w http.ResponseWriter, r *http.Request, in any) bool {
	if decodeImportedJSON(r, in) != nil {
		writeError(w, 400, "Invalid catalog request")
		return false
	}
	return true
}

func (h *upstreamCatalogHandler) createChannel(w http.ResponseWriter, r *http.Request) {
	var in upstream.ChannelCreate
	if !catalogDecode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Provider = strings.TrimSpace(in.Provider)
	if in.Dialect == "" {
		in.Dialect = "generic"
	}
	if in.Dialect != "generic" || in.Provider == "" || len(in.Provider) > 100 || !in.Endpoints.IsConfigured() {
		writeError(w, 400, "Native channel requires a provider, generic dialect and explicit endpoints")
		return
	}
	row := importedChannelConfig{Name: in.Name, Provider: in.Provider, Dialect: in.Dialect, BaseURL: in.BaseURL, Endpoints: in.Endpoints, ChatPath: in.ChatPath, ResponsesPath: in.ResponsesPath, MessagesPath: in.MessagesPath, UseSystemProxy: in.UseSystemProxy, ChannelProxy: in.ChannelProxy, CustomHeaders: in.CustomHeaders, ParamOverride: in.ParamOverride}
	if err := validateImportedConfig(row); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	id, err := upstream.CreateChannel(r.Context(), h.db, in)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"id": id, "name": in.Name, "enabled": in.Enabled, "ownership": "native"})
}
func (h *upstreamCatalogHandler) createCredential(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in oauth.DirectCredentialCreate
	if !catalogDecode(w, r, &in) {
		return
	}
	out, err := oauth.CreateDirectCredential(r.Context(), h.db, id, in)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, 201, out)
}
func (h *upstreamCatalogHandler) models(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	out, err := upstream.ListModels(r.Context(), h.db, id)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": out})
}
func (h *upstreamCatalogHandler) createModels(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in upstream.ModelsCreate
	if !catalogDecode(w, r, &in) {
		return
	}
	out, err := upstream.CreateModels(r.Context(), h.db, id, in)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, 201, map[string]any{"items": out})
}
func (h *upstreamCatalogHandler) updateModel(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in upstream.ModelUpdate
	if !catalogDecode(w, r, &in) {
		return
	}
	out, err := upstream.UpdateModel(r.Context(), h.db, id, in)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"id": id, "success": true, "affectedRouteIds": out})
}
func (h *upstreamCatalogHandler) createGrant(w http.ResponseWriter, r *http.Request) {
	var in upstream.GrantCreate
	if !catalogDecode(w, r, &in) {
		return
	}
	out, err := upstream.CreateGrant(r.Context(), h.db, in)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, 201, out)
}
func (h *upstreamCatalogHandler) updateGrant(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in upstream.GrantUpdate
	if !catalogDecode(w, r, &in) {
		return
	}
	out, err := upstream.UpdateGrant(r.Context(), h.db, id, in)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *upstreamCatalogHandler) groups(w http.ResponseWriter, r *http.Request) {
	out, err := upstream.ListGroups(r.Context(), h.db)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"items": out})
}
func (h *upstreamCatalogHandler) createGroup(w http.ResponseWriter, r *http.Request) {
	var in upstream.GroupCreate
	if !catalogDecode(w, r, &in) {
		return
	}
	out, err := upstream.CreateGroup(r.Context(), h.db, in)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, 201, out)
}
func (h *upstreamCatalogHandler) updateGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in upstream.GroupUpdate
	if !catalogDecode(w, r, &in) {
		return
	}
	out, err := upstream.UpdateGroup(r.Context(), h.db, id, in)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, 200, out)
}
func (h *upstreamCatalogHandler) createMember(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in upstream.MemberCreate
	if !catalogDecode(w, r, &in) {
		return
	}
	out, err := upstream.CreateMember(r.Context(), h.db, id, in)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, 201, out)
}
