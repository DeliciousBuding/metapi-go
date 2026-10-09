package admin

import (
	"errors"
	"net/http"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service/oauth"
)

func (h *importedUpstreamHandler) credentials(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	items, err := oauth.ListDirectCredentials(r.Context(), h.db, id)
	if err != nil {
		if errors.Is(err, oauth.ErrDirectCredentialNotFound) {
			writeError(w, 404, "Imported upstream not found")
			return
		}
		writeError(w, 500, "Failed to load upstream credentials")
		return
	}
	writeJSON(w, 200, map[string]any{"items": items})
}

func (h *importedUpstreamHandler) updateCredential(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var input oauth.DirectCredentialUpdate
	if err := decodeImportedJSON(r, &input); err != nil {
		writeError(w, 400, "Invalid credential replacement")
		return
	}
	if err := oauth.UpdateDirectCredential(r.Context(), h.db, id, input); err != nil {
		if errors.Is(err, oauth.ErrInvalidDirectCredential) {
			writeError(w, 400, err.Error())
		} else if errors.Is(err, oauth.ErrDirectCredentialNotFound) {
			writeError(w, 404, "Upstream credential unavailable")
		} else if errors.Is(err, oauth.ErrDirectCredentialConflict) {
			writeError(w, 409, "Credential name already exists")
		} else {
			writeError(w, 500, "Failed to update upstream credential storage")
		}
		return
	}
	routing.InvalidateCache()
	writeJSON(w, 200, map[string]any{"success": true, "id": id})
}
