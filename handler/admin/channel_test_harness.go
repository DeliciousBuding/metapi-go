package admin

import (
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

const (
	channelTestDefaultPrompt    = "ping"
	channelTestDefaultTimeoutMs = 15_000
	channelTestMinTimeoutMs     = 1_000
	channelTestMaxTimeoutMs     = 60_000
	channelTestMaxBodyBytes     = 2 << 10 // ~2 KiB safe summary
	channelTestMaxPromptRunes   = 256
	channelTestDefaultModel     = "gpt-4o-mini"
	channelTestModeChat         = "chat"
	channelTestModeModels       = "models"
	chatTestMaxBodyBytes        = 1 << 20
)

// RegisterChannelTestRoutes registers admin-only forced-channel probe endpoints.
//
//	POST /api/admin/test-channel
//	POST /api/debug/channel-probe (alias)
func RegisterChannelTestRoutes(r chi.Router, db *store.DB, cfg *config.Config) {
	h := &channelTestHandler{db: db, cfg: cfg}
	r.Post("/api/admin/test-channel", h.testChannel)
	r.Post("/api/debug/channel-probe", h.testChannel)
}

type channelTestHandler struct {
	db  *store.DB
	cfg *config.Config

	// transport is an optional override for tests (stub upstream).
	// When nil, production uses platform.DoWithProxy / default client.
	transport http.RoundTripper
}

type channelTestRequest struct {
	ChannelID *int64           `json:"channelId"`
	SiteID    *int64           `json:"siteId"`
	Model     string           `json:"model"`
	Prompt    string           `json:"prompt"`
	Mode      string           `json:"mode"`
	TimeoutMs *int64           `json:"timeoutMs"`
	Chat      *chatTestOptions `json:"-"`
}

// POST /api/admin/test-channel
// POST /api/debug/channel-probe
func (h *channelTestHandler) testChannel(w http.ResponseWriter, r *http.Request) {
	var body channelTestRequest
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	h.runChannelTest(w, r, body)
}

// runChannelTest is the shared forced-channel probe implementation used by
// /api/admin/test-channel and the /api/test/{proxy,chat} aliases.
func (h *channelTestHandler) runChannelTest(w http.ResponseWriter, r *http.Request, body channelTestRequest) {
	mode := strings.ToLower(strings.TrimSpace(body.Mode))
	if mode == "" {
		mode = channelTestModeChat
	}
	if mode != channelTestModeChat && mode != channelTestModeModels {
		writeError(w, http.StatusBadRequest, "Invalid mode. Expected chat or models.")
		return
	}

	if (body.ChannelID == nil || *body.ChannelID <= 0) && (body.SiteID == nil || *body.SiteID <= 0) {
		writeError(w, http.StatusBadRequest, "channelId or siteId is required")
		return
	}

	model := strings.TrimSpace(body.Model)
	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		prompt = channelTestDefaultPrompt
	}
	if body.Chat == nil && utf8.RuneCountInString(prompt) > channelTestMaxPromptRunes {
		// Bound operator input; never store prompt corpora.
		runes := []rune(prompt)
		prompt = string(runes[:channelTestMaxPromptRunes])
	}

	timeoutMs := int64(channelTestDefaultTimeoutMs)
	if body.Chat != nil {
		timeoutMs = channelTestMaxTimeoutMs
	}
	if body.TimeoutMs != nil && *body.TimeoutMs > 0 {
		timeoutMs = *body.TimeoutMs
	}
	if timeoutMs < channelTestMinTimeoutMs {
		timeoutMs = channelTestMinTimeoutMs
	}
	if timeoutMs > channelTestMaxTimeoutMs {
		timeoutMs = channelTestMaxTimeoutMs
	}

	target, err := h.resolveTarget(r.Context(), body.ChannelID, body.SiteID, model)
	if err != nil {
		if isHarnessNotFound(err) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if model == "" {
		if target.SourceModel != "" {
			model = target.SourceModel
		} else {
			model = channelTestDefaultModel
		}
	}

	var result map[string]any
	if target.TokenValue == "" {
		diagnostic := "No usable token on channel/account (api token / oauth access token required)"
		if target.OAuthRouteUnitID != nil {
			diagnostic = "OAuth route unit has no eligible member (check pool status, member credentials and cooldowns)"
		}
		result = harnessFail(target, mode, model, "", "", 0, 0, "", diagnostic)
	} else {
		result = h.executeProbe(r.Context(), target, mode, model, prompt, timeoutMs, body.Chat)
	}
	if target.OAuthRouteUnitID != nil {
		result["oauthRouteUnitId"] = *target.OAuthRouteUnitID
		// OAuth access tokens can be opaque and lack a recognizable prefix.
		if target.TokenValue != "" {
			for _, key := range []string{"truncatedBody", "error", "upstreamReportedModel"} {
				if text, ok := result[key].(string); ok {
					result[key] = strings.ReplaceAll(text, target.TokenValue, "[redacted]")
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, result)
}

func isHarnessNotFound(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "not found")
}
