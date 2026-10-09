package admin

import (
	"encoding/json"
	"errors"
	"io"
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
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&input); err != nil {
		writeError(w, 400, "Invalid credential replacement")
		return
	}
	var extra any
	if dec.Decode(&extra) != io.EOF {
		writeError(w, 400, "Expected one JSON object")
		return
	}
	if err := oauth.UpdateDirectCredential(r.Context(), h.db, id, input); err != nil {
		if errors.Is(err, oauth.ErrInvalidDirectCredential) {
			writeError(w, 400, err.Error())
		} else {
			writeError(w, 404, "Upstream credential unavailable")
		}
		return
	}
	routing.InvalidateCache()
	writeJSON(w, 200, map[string]any{"success": true, "id": id})
}
