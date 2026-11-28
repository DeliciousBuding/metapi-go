package admin

import (
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/go-chi/chi/v5"
)

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
		normalized, ok := gateOAuthProxyURL(w, r, body.ProxyURL)
		if !ok {
			return
		}
		input.ProxyURL = *normalized
	}
	if body.UseSystemProxy != nil {
		input.UseSystemProxy = *body.UseSystemProxy
	}

	result, err := oauth.StartFlow(h.db, input)
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

	status := oauth.GetSessionStatus(r.Context(), state)
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
