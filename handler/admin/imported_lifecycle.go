package admin

import (
	"errors"
	"net/http"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func RegisterUpstreamLifecycleRoutes(r chi.Router, db *store.DB) {
	r.Get("/api/imported-upstreams/{id}/deletion-preview", lifecyclePreviewHandler(db, upstream.KindChannel))
	r.Delete("/api/imported-upstreams/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleteLifecycleEntity(w, r, db, upstream.KindChannel, false)
	})
	r.Get("/api/imported-upstreams/models/{id}/deletion-preview", lifecyclePreviewHandler(db, upstream.KindModel))
	r.Delete("/api/imported-upstreams/models/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleteLifecycleEntity(w, r, db, upstream.KindModel, false)
	})
	r.Get("/api/imported-upstreams/credentials/{id}/deletion-preview", lifecyclePreviewHandler(db, upstream.KindCredential))
	r.Delete("/api/imported-upstreams/credentials/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleteLifecycleEntity(w, r, db, upstream.KindCredential, false)
	})
	r.Get("/api/imported-upstreams/grants/{id}/deletion-preview", lifecyclePreviewHandler(db, upstream.KindGrant))
	r.Delete("/api/imported-upstreams/grants/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleteLifecycleEntity(w, r, db, upstream.KindGrant, false)
	})
	r.Get("/api/imported-upstreams/groups/{id}/deletion-preview", lifecyclePreviewHandler(db, upstream.KindGroup))
	r.Delete("/api/imported-upstreams/groups/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleteLifecycleEntity(w, r, db, upstream.KindGroup, false)
	})
	r.Get("/api/imported-upstreams/members/{id}/deletion-preview", lifecyclePreviewHandler(db, upstream.KindMember))
	r.Delete("/api/imported-upstreams/members/{id}", func(w http.ResponseWriter, r *http.Request) {
		deleteLifecycleEntity(w, r, db, upstream.KindMember, false)
	})
	r.Get("/api/routes/{id}/deletion-preview", lifecyclePreviewHandler(db, upstream.KindRoute))
}

func lifecyclePreviewHandler(db *store.DB, kind upstream.Kind) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(w, r)
		if !ok {
			return
		}
		preview, err := upstream.PreviewDeletion(r.Context(), db.DB, kind, id)
		if err != nil {
			writeLifecycleError(w, r, err, nil)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		writeJSON(w, http.StatusOK, preview)
	}
}

func writeLifecycleError(w http.ResponseWriter, r *http.Request, err error, preview *upstream.DeletionPreview) {
	if errors.Is(err, upstream.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, upstream.ErrConfirmationRequired) || errors.Is(err, upstream.ErrRevisionMismatch) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "preview": preview})
		return
	}
	writeErrorWithRequest(w, r, http.StatusInternalServerError, "Failed to change upstream lifecycle")
}

// Native route deletion preserves its existing contract. A route paired with a
// direct group uses the same closure confirmation as deleting that group.
func deleteLifecycleEntity(w http.ResponseWriter, r *http.Request, db *store.DB, kind upstream.Kind, nativeRouteCompatibility bool) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	cascade := r.URL.Query().Get("cascade")
	if cascade != "" && cascade != "true" && cascade != "false" {
		writeError(w, 400, "cascade must be true or false")
		return
	}
	revision := r.URL.Query().Get("expectedRevision")
	result, err := upstream.Delete(r.Context(), db.DB, kind, id, upstream.DeleteOptions{
		Cascade: cascade == "true", ExpectedRevision: revision, AllowUnpairedRoute: nativeRouteCompatibility,
	})
	if err != nil {
		var preview *upstream.DeletionPreview
		if result != nil {
			preview = &result.DeletionPreview
		}
		writeLifecycleError(w, r, err, preview)
		return
	}
	routing.InvalidateCache()
	invalidateChannelsSnapshotCache()
	writeJSON(w, http.StatusOK, struct {
		Success bool `json:"success"`
		upstream.DeletionResult
	}{true, *result})
}
