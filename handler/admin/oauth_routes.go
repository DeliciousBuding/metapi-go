package admin

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service/oauth"
)

// RegisterOauthRoutes registers all /api/oauth routes.
// Start/rebind issue cryptographically random server-stored state tokens (TTL 10m);
// session lookup and manual callback validate that state.
func RegisterOauthRoutes(r chi.Router, db *sqlx.DB) {
	handler := &oauthHandler{db: db}

	r.Get("/api/oauth/providers", handler.listProviders)
	r.Post("/api/oauth/providers/{provider}/start", handler.startProviderFlow)
	r.Get("/api/oauth/sessions/{state}", handler.getSession)
	r.Post("/api/oauth/sessions/{state}/manual-callback", handler.manualCallback)
	r.Get("/api/oauth/connections", handler.listConnections)
	r.Post("/api/oauth/connections/{accountId}/rebind", handler.rebindConnection)
	r.Patch("/api/oauth/connections/{accountId}/proxy", handler.updateProxy)
	r.Delete("/api/oauth/connections/{accountId}", handler.deleteConnection)
	r.Post("/api/oauth/connections/{accountId}/quota/refresh", handler.refreshQuota)
	r.Post("/api/oauth/connections/quota/refresh-batch", handler.refreshQuotaBatch)
	r.Post("/api/oauth/import", handler.importConnections)
	r.Post("/api/oauth/route-units", handler.createRouteUnit)
	r.Patch("/api/oauth/route-units/{routeUnitId}", handler.updateRouteUnit)
	r.Delete("/api/oauth/route-units/{routeUnitId}", handler.deleteRouteUnit)
	r.Get("/api/oauth/callback/{provider}", handler.callback)
}

type oauthHandler struct {
	db *sqlx.DB
}

// GET /api/oauth/providers
func (h *oauthHandler) listProviders(w http.ResponseWriter, r *http.Request) {
	providers := oauth.ListOauthProviders()
	systemProxyConfigured := false
	// SystemProxyUrl is runtime-mutable; RuntimeSafe is nil only before boot
	// publication (never reachable from a live admin request).
	if rt := config.RuntimeSafe(); rt != nil {
		systemProxyConfigured = strings.TrimSpace(rt.SystemProxyUrl) != ""
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"providers": providers,
		"defaults": map[string]any{
			"systemProxyConfigured": systemProxyConfigured,
		},
	})
}

type oauthStartBody struct {
	AccountID      *int64  `json:"accountId,omitempty"`
	ProjectID      *string `json:"projectId,omitempty"`
	ProxyURL       *string `json:"proxyUrl,omitempty"`
	UseSystemProxy *bool   `json:"useSystemProxy,omitempty"`
}

// POST /api/oauth/providers/:provider/start
func (h *oauthHandler) startProviderFlow(w http.ResponseWriter, r *http.Request) {
	provider := strings.TrimSpace(chi.URLParam(r, "provider"))
	if provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "provider is required"})
		return
	}

	body, ok := decodeOptionalOAuthStartBody(w, r)
	if !ok {
		return
	}

	input := oauth.StartFlowInput{
		Provider:      provider,
		RequestOrigin: requestOrigin(r),
	}
	if body.AccountID != nil && *body.AccountID > 0 {
		input.RebindAccountID = *body.AccountID
	}
	if body.ProjectID != nil {
		input.ProjectID = strings.TrimSpace(*body.ProjectID)
	}
	if body.ProxyURL != nil {
		input.ProxyURL = strings.TrimSpace(*body.ProxyURL)
	}
	if body.UseSystemProxy != nil {
		input.UseSystemProxy = *body.UseSystemProxy
	}

	result, err := oauth.StartFlow(input)
	if err != nil {
		status := http.StatusBadRequest
		msg := err.Error()
		if strings.Contains(msg, "unsupported oauth provider") {
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
	})
}

// GET /api/oauth/sessions/:state
func (h *oauthHandler) getSession(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(chi.URLParam(r, "state"))
	if state == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "state is required"})
		return
	}

	status := oauth.GetSessionStatus(state)
	if status == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "oauth session not found"})
		return
	}

	writeJSON(w, http.StatusOK, status)
}

// POST /api/oauth/sessions/:state/manual-callback
func (h *oauthHandler) manualCallback(w http.ResponseWriter, r *http.Request) {
	state := strings.TrimSpace(chi.URLParam(r, "state"))
	if state == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "state is required"})
		return
	}

	var body struct {
		CallbackURL string `json:"callbackUrl"`
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "invalid request body"})
		return
	}
	callbackURL := strings.TrimSpace(body.CallbackURL)
	if callbackURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "callbackUrl is required"})
		return
	}

	// Reject unknown / expired state before parsing callback content further.
	if oauth.GetSession(state) == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"message": "oauth session not found"})
		return
	}

	err := oauth.SubmitManualCallback(oauth.ManualCallbackInput{
		State:       state,
		CallbackURL: callbackURL,
	})
	if err != nil {
		msg := err.Error()
		status := http.StatusBadRequest
		switch {
		case strings.Contains(msg, "oauth session not found"):
			status = http.StatusNotFound
		case strings.Contains(msg, "state mismatch"):
			status = http.StatusBadRequest
		}
		writeJSON(w, status, map[string]any{"message": msg})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

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

	// Prefer service implementation when the global DB is available; fall back to
	// the handler's injected *sqlx.DB for tests that only wire the chi handler.
	if result, err := oauth.ListOauthConnections(oauth.ListConnectionsInput{
		Limit:  limit,
		Offset: offset,
	}); err == nil && result != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"connections": result.Items,
			"items":       result.Items,
			"total":       result.Total,
			"limit":       result.Limit,
			"offset":      result.Offset,
		})
		return
	}

	rows, err := queryRowsErr(h.db,
		`SELECT a.*, s.name as site_name, s.platform as site_platform
		 FROM accounts a
		 INNER JOIN sites s ON a.site_id = s.id
		 WHERE a.oauth_provider IS NOT NULL
		 ORDER BY a.id ASC LIMIT ? OFFSET ?`,
		limit, offset)
	if err != nil {
		writeErrorCode(w, http.StatusInternalServerError, ErrorCodeResourceLoadFailed, "failed to load oauth connections")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"connections": normalizeSlice(rows),
		"total":       len(rows),
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

	result, err := oauth.StartOauthRebindFlow(id, requestOrigin(r), body.ProxyURL, body.UseSystemProxy)
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

	result, err := oauth.UpdateOauthConnectionProxySettings(id, body.ProxyURL, body.UseSystemProxy)
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

	if err := oauth.DeleteOauthConnection(id); err != nil {
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

	snapshot, err := oauth.RefreshOauthQuotaSnapshot(id)
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

	result := oauth.RefreshOauthConnectionQuotaBatch(body.AccountIDs)
	writeJSON(w, http.StatusOK, result)
}

// POST /api/oauth/import
func (h *oauthHandler) importConnections(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// POST /api/oauth/route-units
func (h *oauthHandler) createRouteUnit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// PATCH /api/oauth/route-units/:routeUnitId
func (h *oauthHandler) updateRouteUnit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// DELETE /api/oauth/route-units/:routeUnitId
func (h *oauthHandler) deleteRouteUnit(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

// GET /api/oauth/callback/:provider
// Loopback providers use their own callback servers; this admin callback surface
// only acknowledges the browser redirect and relies on session state already
// issued by start/rebind (validated on getSession / manual-callback).
func (h *oauthHandler) callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`<!doctype html>
<html lang="zh-CN">
  <head><meta charset="utf-8" /><title>OAuth Callback</title></head>
  <body>
    <script>window.close();</script>
    OAuth authorization succeeded. You can close this window.
  </body>
</html>`))
}

func requestOrigin(r *http.Request) string {
	if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" {
		return origin
	}
	if referer := strings.TrimSpace(r.Header.Get("Referer")); referer != "" {
		return referer
	}
	return ""
}

// decodeOptionalOAuthStartBody accepts missing/empty bodies as {}.
// Malformed non-empty JSON is rejected with 400.
func decodeOptionalOAuthStartBody(w http.ResponseWriter, r *http.Request) (oauthStartBody, bool) {
	var body oauthStartBody
	if r.Body == nil || r.Body == http.NoBody {
		return body, true
	}
	if err := decodeJSONRequest(r, &body); err != nil {
		if isEmptyJSONBodyError(err) {
			return oauthStartBody{}, true
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": "invalid request body"})
		return oauthStartBody{}, false
	}
	return body, true
}

func isEmptyJSONBodyError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "unexpected end of JSON input")
}
