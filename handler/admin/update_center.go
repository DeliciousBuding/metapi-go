package admin

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// RegisterUpdateCenterRoutes registers the /api/update-center routes.
//
// Known limitation:
// - status is a local stub (never invent updateAvailable=true)
// - deploy/rollback/task-stream surfaces are removed; product updates stay external.
// See limitation-update-center.md.
func RegisterUpdateCenterRoutes(r chi.Router) {
	handler := &updateCenterHandler{}

	r.Get("/api/update-center/status", handler.status)
}

type updateCenterHandler struct{}

// localUpdateCenterStatus is the honest local-only payload for status.
// Never set updateAvailable=true without a real remote registry/helper client.
func localUpdateCenterStatus() map[string]any {
	return map[string]any{
		"currentVersion":  "0.0.0",
		"latestVersion":   "0.0.0",
		"updateAvailable": false,
		"lastCheckedAt":   nil,
		// UC-1: product mode is external (GHCR/ops); never invent updateAvailable.
		"mode": "external",
		// field makes the stub explicit for operators/UI (UC-1).
		"residual": "external deploy only; no remote registry/helper polling or in-app version discovery",
	}
}

// GET /api/update-center/status
// Local status only — remote version discovery is a known limitation.
// Never invents updateAvailable=true or a fake lastCheckedAt.
func (h *updateCenterHandler) status(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, localUpdateCenterStatus())
}
