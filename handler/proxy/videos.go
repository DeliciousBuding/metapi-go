package proxyhandler

import (
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/go-chi/chi/v5"
)

// HandleVideosCreate handles POST /v1/videos.
// Supports multipart/form-data or JSON body. Model is required.

// On successful non-stream upstream 2xx, dispatch rewrites response `id` to a
// generated publicId and SaveProxyVideoTask (process-local).
func HandleVideosCreate(w http.ResponseWriter, r *http.Request) {
	ctx, errResp := PrepareCtx(r, SurfConfig{
		Endpoint:       "videos",
		DownstreamPath: "/v1/videos",
		RequireModel:   true,
	})
	if errResp != nil {
		writeJSONError(w, errResp.Status, errResp.Error, errResp.ErrorType)
		return
	}

	if ctx.RequestedModel == "" {
		writeJSONError(w, 400, "model is required", "invalid_request_error")
		return
	}

	dispatchUpstream(w, r, ctx)
}

// HandleVideosGet handles GET /v1/videos/{id}.
func HandleVideosGet(w http.ResponseWriter, r *http.Request) {
	if ctx, _ := prepareVideoTaskRequest(w, r); ctx != nil {
		dispatchUpstream(w, r, ctx)
	}
}

// HandleVideosDelete handles DELETE /v1/videos/{id}. A successful upstream
// delete releases the mapping in the response hook.
func HandleVideosDelete(w http.ResponseWriter, r *http.Request) {
	if ctx, publicID := prepareVideoTaskRequest(w, r); ctx != nil {
		ctx.managedVideoTaskID = publicID
		dispatchUpstream(w, r, ctx)
	}
}

// prepareVideoTaskRequest serves direct video tasks locally. For a shared
// upstream it requires an owned mapping and pins the original channel.
func prepareVideoTaskRequest(w http.ResponseWriter, r *http.Request) (*Ctx, string) {
	if strings.HasPrefix(chi.URLParam(r, "id"), directVideoIDPrefix) {
		handleDirectVideoTask(w, r, "")
		return nil, ""
	}
	pac := GetProxyAuth(r)
	if pac == nil {
		writeJSONError(w, http.StatusUnauthorized, "unauthorized", "invalid_request_error")
		return nil, ""
	}
	publicID := chi.URLParam(r, "id")
	if publicID == "" {
		writeJSONError(w, 400, "missing video id", "invalid_request_error")
		return nil, ""
	}
	if !validProxiedResourceID(publicID) {
		writeJSONError(w, 400, "invalid video id", "invalid_request_error")
		return nil, ""
	}

	task := GetProxyVideoTaskByPublicID(proxyScopeFromContext(r.Context()).db, publicID)
	if !proxyVideoTaskOwnedBy(task, pac) {
		writeJSONError(w, http.StatusNotFound, "video not found", "invalid_request_error")
		return nil, ""
	}
	upstreamID := resolveVideoUpstreamIDFromTask(publicID, task)
	if !validProxiedResourceID(upstreamID) {
		writeJSONError(w, 400, "invalid video id", "invalid_request_error")
		return nil, ""
	}

	ctx, errResp := PrepareCtx(r, SurfConfig{
		Endpoint:       "videos",
		DownstreamPath: "/v1/videos/" + upstreamID,
		RequireModel:   false,
	})
	if errResp != nil {
		writeJSONError(w, errResp.Status, errResp.Error, errResp.ErrorType)
		return nil, ""
	}
	applyVideoTaskStickyPin(ctx, task)
	return ctx, publicID
}

// proxyVideoTaskOwnedBy reports whether task is a shared-upstream mapping owned
// by pac. Ownerless rows (written before the owner column existed) fail closed:
// an unknown owner cannot be treated as "everyone's" for an opaque upstream id.
func proxyVideoTaskOwnedBy(task *ProxyVideoTask, pac *auth.ProxyAuthContext) bool {
	if task == nil {
		return false
	}
	owner := directVideoOwner(pac)
	return owner != "" && task.Owner != "" && task.Owner == owner
}

// resolveVideoUpstreamIDFromTask returns the upstream path id for a client-facing id.
// When a mapping exists with a non-empty UpstreamVideoID different from publicID,
// that upstream id is used; otherwise publicID is passed through unchanged.
func resolveVideoUpstreamIDFromTask(publicID string, task *ProxyVideoTask) string {
	if task == nil {
		return publicID
	}
	if task.UpstreamVideoID != "" && task.UpstreamVideoID != publicID {
		return task.UpstreamVideoID
	}
	return publicID
}

// applyVideoTaskStickyPin forces preferred channel selection when the mapping
// recorded a channel at create time. Also seeds RequestedModel from the
// mapping when the client omitted model (typical for GET/DELETE).
func applyVideoTaskStickyPin(ctx *Ctx, task *ProxyVideoTask) {
	if ctx == nil || task == nil {
		return
	}
	if ctx.RequestedModel == "" {
		if task.RequestedModel != "" {
			ctx.RequestedModel = task.RequestedModel
		} else if task.ActualModel != "" {
			ctx.RequestedModel = task.ActualModel
		}
	}
	if task.ChannelID > 0 {
		ch := task.ChannelID
		ctx.ForcedChannelID = &ch
	}
}
