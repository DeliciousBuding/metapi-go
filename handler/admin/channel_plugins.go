package admin

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/service/channelplugins"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

func RegisterChannelPluginRoutes(r chi.Router, db *sqlx.DB) {
	r.Post("/api/channel-plugin-versions/prune", func(w http.ResponseWriter, r *http.Request) {
		setNoStore(w)
		var in struct {
			Key         string `json:"key"`
			Fingerprint string `json:"fingerprint"`
		}
		if !catalogDecode(w, r, &in) {
			return
		}
		if err := channelplugins.PruneArchivedVersion(r.Context(), db, in.Key, in.Fingerprint); err != nil {
			channelPluginError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"key": in.Key, "fingerprint": in.Fingerprint, "pruned": true})
	})
	r.Get("/api/channel-plugin-versions", func(w http.ResponseWriter, r *http.Request) {
		setNoStore(w)
		items, err := channelplugins.ListArchivedVersions(r.Context(), db)
		if err != nil {
			channelPluginError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": items})
	})
	r.Post("/api/channel-plugin-versions/restore", func(w http.ResponseWriter, r *http.Request) {
		setNoStore(w)
		var in struct {
			Key         string `json:"key"`
			Fingerprint string `json:"fingerprint"`
			Source      string `json:"source"`
		}
		if !catalogDecode(w, r, &in) {
			return
		}
		if err := channelplugins.RestoreArchivedVersion(r.Context(), db, in.Key, in.Fingerprint, in.Source); err != nil {
			channelPluginError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"key": in.Key, "fingerprint": in.Fingerprint, "restored": true})
	})
	r.Get("/api/channel-plugins", func(w http.ResponseWriter, r *http.Request) {
		setNoStore(w)
		items, err := channelplugins.List(r.Context(), db)
		if err != nil {
			channelPluginError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"items": items})
	})
	r.Get("/api/channel-plugins/{key}", func(w http.ResponseWriter, r *http.Request) {
		setNoStore(w)
		row, err := store.LoadChannelPlugin(r.Context(), db, chi.URLParam(r, "key"))
		if err != nil {
			channelPluginError(w, err)
			return
		}
		writeJSON(w, 200, row)
	})
	r.Put("/api/channel-plugins/{key}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Source           string `json:"source"`
			Enabled          bool   `json:"enabled"`
			ExpectedRevision int64  `json:"expectedRevision"`
		}
		if !catalogDecode(w, r, &in) {
			return
		}
		row, err := channelplugins.Save(r.Context(), db, chi.URLParam(r, "key"), in.Source, in.Enabled, in.ExpectedRevision)
		if err != nil {
			channelPluginError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"key": row.Key, "revision": row.Revision, "enabled": row.Enabled})
	})
	r.Delete("/api/channel-plugins/{key}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			ExpectedRevision int64 `json:"expectedRevision"`
		}
		if !catalogDecode(w, r, &in) {
			return
		}
		if in.ExpectedRevision <= 0 {
			channelPluginError(w, store.ErrChannelPluginConflict)
			return
		}
		err := store.DeleteChannelPlugin(r.Context(), db, chi.URLParam(r, "key"), in.ExpectedRevision)
		if err != nil {
			channelPluginError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"success": true})
	})
	r.Patch("/api/channel-plugins/{key}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Enabled          *bool `json:"enabled"`
			ExpectedRevision int64 `json:"expectedRevision"`
		}
		if !catalogDecode(w, r, &in) {
			return
		}
		if in.Enabled == nil || in.ExpectedRevision <= 0 {
			channelPluginError(w, channelplugins.ErrBinding)
			return
		}
		key := chi.URLParam(r, "key")
		stored, err := store.LoadChannelPlugin(r.Context(), db, key)
		if err != nil {
			channelPluginError(w, err)
			return
		}
		if stored.Revision != in.ExpectedRevision {
			channelPluginError(w, store.ErrChannelPluginConflict)
			return
		}
		row, err := channelplugins.Save(r.Context(), db, key, stored.Source, *in.Enabled, in.ExpectedRevision)
		if err != nil {
			channelPluginError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"key": row.Key, "revision": row.Revision, "enabled": row.Enabled})
	})
}

func channelPluginError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeError(w, 404, "channel_plugin_not_found")
	case errors.Is(err, store.ErrChannelPluginConflict):
		writeError(w, 409, "channel_plugin_revision_conflict")
	case errors.Is(err, channelplugins.ErrVersionNotReferenced):
		writeError(w, 409, "channel_plugin_version_not_referenced")
	case errors.Is(err, channelplugins.ErrVersionNotPrunable):
		writeError(w, 409, "channel_plugin_version_not_prunable")
	case errors.Is(err, channelplugins.ErrBinding), errors.Is(err, channelplugin.ErrCompile), errors.Is(err, channelplugin.ErrManifest), errors.Is(err, channelplugin.ErrExecution):
		writeError(w, 400, "channel_plugin_invalid")
	default:
		writeError(w, 500, "channel_plugin_operation_failed")
	}
}
