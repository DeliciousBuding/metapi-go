package admin

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

// RegisterUpstreamPresetRoutes exposes only local configuration projections.
// Resolving a preset makes no outbound request and reads no credentials.
func RegisterUpstreamPresetRoutes(r chi.Router) {
	r.Get("/api/imported-upstreams/presets", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"items": service.ListUpstreamPresets()})
	})
	r.Post("/api/imported-upstreams/presets/resolve", resolveUpstreamPreset)
}

func resolveUpstreamPreset(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PresetID string `json:"presetId"`
		BaseURL  string `json:"baseUrl"`
	}
	if !catalogDecode(w, r, &input) {
		return
	}
	preset := service.GetUpstreamPreset(input.PresetID)
	if preset == nil {
		writeError(w, http.StatusBadRequest, "Unknown upstream preset")
		return
	}
	base := strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || strings.ContainsAny(base, "?#") || service.IsForbiddenSiteTargetURL(base) {
		writeError(w, http.StatusBadRequest, "Invalid upstream base URL")
		return
	}
	endpoint := func(protocol proxy.UpstreamEndpoint, auth string) *store.DirectEndpoint {
		return &store.DirectEndpoint{URL: proxy.BuildUpstreamURL(base, proxy.PathForEndpoint(protocol)), Auth: auth}
	}
	var config store.DirectEndpoints
	if len(preset.Protocols) == 0 {
		writeError(w, http.StatusBadRequest, "Preset has no executable endpoint contract")
		return
	}
	for _, protocol := range preset.Protocols {
		switch protocol {
		case "chat":
			config.Chat = endpoint(proxy.EndpointChat, store.DirectAuthBearer)
			original, _ := url.Parse(preset.DefaultURL)
			if original != nil && strings.EqualFold(original.Host, parsed.Host) {
				config.Chat.Profile = service.NativeChatRequestProfile(base, preset.Platform)
			}
		case "responses":
			config.Responses = endpoint(proxy.EndpointResponses, store.DirectAuthBearer)
		case "messages":
			auth := store.DirectAuthAPIKey
			if preset.Platform == "new-api" {
				auth = store.DirectAuthBearer
			}
			config.Messages = endpoint(proxy.EndpointMessages, auth)
		case "gemini":
			config.Gemini = endpoint(proxy.EndpointGemini, store.DirectAuthGoogle)
			config.Gemini.ModelPath = true
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": preset.Provider, "endpointConfig": config})
}
