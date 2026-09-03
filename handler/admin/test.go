package admin

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// RegisterTestRoutes registers all /api/test routes.
//
// Known limitation:
// - sync chat probes alias the forced-channel harness when a channel/site is forced
// - full path/multipart routing matrix without a forced channel returns an honest 501 residual
func RegisterTestRoutes(r chi.Router, db *sqlx.DB, cfg *config.Config) {
	handler := &testHandler{
		channel: &channelTestHandler{db: db, cfg: cfg},
	}

	// Chat test endpoint: sync forced-channel harness probe.
	r.Post("/api/test/chat", handler.chatTest)
}

type testHandler struct {
	channel *channelTestHandler
}

// flexibleTestBody accepts both harness-shaped fields and the richer frontend
// chat envelope (forcedChannelId, jsonBody, messages).
type flexibleTestBody struct {
	ChannelID       *int64          `json:"channelId"`
	ForcedChannelID *int64          `json:"forcedChannelId"`
	SiteID          *int64          `json:"siteId"`
	Model           string          `json:"model"`
	Prompt          string          `json:"prompt"`
	Mode            string          `json:"mode"`
	TimeoutMs       *int64          `json:"timeoutMs"`
	Path            string          `json:"path"`
	JSONBody        json.RawMessage `json:"jsonBody"`
	Messages        []testMessage   `json:"messages"`
}

type testMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// POST /api/test/chat
// Alias of the forced-channel harness when channelId/siteId/forcedChannelId is set.
func (h *testHandler) chatTest(w http.ResponseWriter, r *http.Request) {
	h.handleSyncProbe(w, r, "chat")
}

func (h *testHandler) handleSyncProbe(w http.ResponseWriter, r *http.Request, surface string) {
	var body flexibleTestBody
	if err := decodeJSONRequest(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req, ok := mapFlexibleToChannelTest(body)
	if !ok {
		// Full path/multipart/routing matrix without a forced channel is a known limitation.
		// Do not invent a successful probe when no channel/site is forced.
		writeNotImplementedResidual(w,
			strings.ToUpper(surface[:1])+surface[1:]+" test requires channelId, siteId, or forcedChannelId for the forced-channel harness",
			"full /api/test/"+surface+" path/multipart/routing matrix without a forced channel is residual; provide channelId/siteId/forcedChannelId or use POST /api/admin/test-channel",
		)
		return
	}

	h.channel.runChannelTest(w, r, req)
}

func mapFlexibleToChannelTest(body flexibleTestBody) (channelTestRequest, bool) {
	channelID := firstPositiveID(body.ChannelID, body.ForcedChannelID)
	siteID := body.SiteID
	if (channelID == nil || *channelID <= 0) && (siteID == nil || *siteID <= 0) {
		return channelTestRequest{}, false
	}

	model := strings.TrimSpace(body.Model)
	if model == "" && len(body.JSONBody) > 0 {
		var nested struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(body.JSONBody, &nested); err == nil {
			model = strings.TrimSpace(nested.Model)
		}
	}

	prompt := strings.TrimSpace(body.Prompt)
	if prompt == "" {
		prompt = lastUserPrompt(body.Messages)
	}
	if prompt == "" && len(body.JSONBody) > 0 {
		var nested struct {
			Messages []testMessage `json:"messages"`
			Prompt   string        `json:"prompt"`
			Input    string        `json:"input"`
		}
		if err := json.Unmarshal(body.JSONBody, &nested); err == nil {
			if p := strings.TrimSpace(nested.Prompt); p != "" {
				prompt = p
			} else if p := strings.TrimSpace(nested.Input); p != "" {
				prompt = p
			} else {
				prompt = lastUserPrompt(nested.Messages)
			}
		}
	}

	mode := strings.ToLower(strings.TrimSpace(body.Mode))
	if mode == "" {
		path := strings.ToLower(strings.TrimSpace(body.Path))
		if strings.Contains(path, "/models") && !strings.Contains(path, "chat") {
			mode = channelTestModeModels
		} else {
			mode = channelTestModeChat
		}
	}

	return channelTestRequest{
		ChannelID: channelID,
		SiteID:    siteID,
		Model:     model,
		Prompt:    prompt,
		Mode:      mode,
		TimeoutMs: body.TimeoutMs,
	}, true
}

func firstPositiveID(ids ...*int64) *int64 {
	for _, id := range ids {
		if id != nil && *id > 0 {
			return id
		}
	}
	return nil
}

func lastUserPrompt(messages []testMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		role := strings.ToLower(strings.TrimSpace(messages[i].Role))
		if role != "" && role != "user" {
			continue
		}
		if p := contentToPrompt(messages[i].Content); p != "" {
			return p
		}
	}
	return ""
}

func contentToPrompt(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	// OpenAI-style multipart content: [{"type":"text","text":"..."}]
	var parts []map[string]any
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, part := range parts {
			if t, _ := part["type"].(string); t != "" && t != "text" {
				continue
			}
			if text, ok := part["text"].(string); ok {
				if b.Len() > 0 {
					b.WriteByte(' ')
				}
				b.WriteString(strings.TrimSpace(text))
			}
		}
		return strings.TrimSpace(b.String())
	}
	return ""
}

// writeNotImplementedResidual returns HTTP 501 with success:false.
// Never use this helper to invent fake success, stream chunks, or job ids.
func writeNotImplementedResidual(w http.ResponseWriter, message, residual string) {
	writeJSON(w, http.StatusNotImplemented, map[string]any{
		"success":   false,
		"message":   message,
		"errorCode": ErrorCodeOperationNotImplemented,
		"residual":  residual,
	})
}
