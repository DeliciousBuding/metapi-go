package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/go-chi/chi/v5"
)

// GET /api/oauth/connections?limit=&offset=
func (h *oauthHandler) listConnections(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	limit := 50
	offset := 0
	if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
		limit = l
	}
	if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
		offset = o
	}

	result, err := oauth.ListOauthConnections(h.db, oauth.ListConnectionsInput{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load oauth connections")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"connections": result.Items,
		"items":       result.Items,
		"total":       result.Total,
		"limit":       result.Limit,
		"offset":      result.Offset,
	})
}

// POST /api/oauth/connections/:accountId/rebind
func (h *oauthHandler) rebindConnection(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "accountId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "invalid account id"})
		return
	}

	body, ok := decodeOptionalOAuthStartBody(w, r)
	if !ok {
		return
	}
	proxyURL, ok := gateOAuthProxyURL(w, r, body.ProxyURL)
	if !ok {
		return
	}

	result, err := oauth.StartOauthRebindFlow(h.db, id, requestOrigin(r), proxyURL, body.UseSystemProxy)
	if err != nil {
		msg := err.Error()
		status := http.StatusBadRequest
		if strings.Contains(msg, "not found") || strings.Contains(msg, "not managed by oauth") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"message": msg})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"provider":         result.Provider,
		"state":            result.State,
		"authorizationUrl": result.AuthorizationURL,
		"instructions":     result.Instructions,
		"accountId":        id,
	})
}

// PATCH /api/oauth/connections/:accountId/proxy
func (h *oauthHandler) updateProxy(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "accountId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "invalid account id"})
		return
	}

	// Body is optional: an empty body clears the per-connection proxy (falls
	// back to system proxy). Malformed non-empty JSON is a 400.
	var body struct {
		ProxyURL       *string `json:"proxyUrl"`
		UseSystemProxy *bool   `json:"useSystemProxy"`
	}
	if r.Body != nil && r.Body != http.NoBody {
		if err := decodeJSONRequest(r, &body); err != nil {
			if isEmptyJSONBodyError(err) {
				body = struct {
					ProxyURL       *string `json:"proxyUrl"`
					UseSystemProxy *bool   `json:"useSystemProxy"`
				}{}
			} else {
				writeJSON(w, http.StatusBadRequest, map[string]any{"message": "invalid request body"})
				return
			}
		}
	}

	proxyURL, ok := gateOAuthProxyURL(w, r, body.ProxyURL)
	if !ok {
		return
	}

	result, err := oauth.UpdateOauthConnectionProxySettings(h.db, id, proxyURL, body.UseSystemProxy)
	if err != nil {
		msg := err.Error()
		status := http.StatusNotFound
		writeJSON(w, status, map[string]any{"message": msg})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// DELETE /api/oauth/connections/:accountId
func (h *oauthHandler) deleteConnection(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "accountId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "invalid account id"})
		return
	}

	if err := oauth.DeleteOauthConnection(h.db, id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"message": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// POST /api/oauth/connections/:accountId/quota/refresh
func (h *oauthHandler) refreshQuota(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "accountId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "invalid account id"})
		return
	}

	snapshot, err := oauth.RefreshOauthQuotaSnapshot(h.db, id)
	if err != nil {
		msg := err.Error()
		status := http.StatusInternalServerError
		if strings.Contains(msg, "not found") || strings.Contains(msg, "not managed by oauth") {
			status = http.StatusNotFound
		}
		writeJSON(w, status, map[string]any{"message": msg})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "quota": snapshot})
}

// POST /api/oauth/connections/quota/refresh-batch
func (h *oauthHandler) refreshQuotaBatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccountIDs []int64 `json:"accountIds"`
	}
	if r.Body != nil && r.Body != http.NoBody {
		if err := decodeJSONRequest(r, &body); err != nil {
			if isEmptyJSONBodyError(err) {
				body = struct {
					AccountIDs []int64 `json:"accountIds"`
				}{}
			} else {
				writeJSON(w, http.StatusBadRequest, map[string]any{"message": "invalid request body"})
				return
			}
		}
	}

	result := oauth.RefreshOauthConnectionQuotaBatch(h.db, body.AccountIDs)
	writeJSON(w, http.StatusOK, result)
}
