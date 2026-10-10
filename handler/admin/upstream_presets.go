package admin

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service"
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
	config := service.UpstreamPresetEndpointPaths(preset.ID, preset.Platform)
	if !config.IsConfigured() {
		writeError(w, http.StatusBadRequest, "Preset has no executable endpoint contract")
		return
	}
	for _, entry := range config.Entries() {
		if entry.Endpoint == nil {
			continue
		}
		for _, address := range entry.Endpoint.URLFields() {
			endpointBase := base
			if strings.HasPrefix(*address, "/beta/") || strings.HasPrefix(*address, "/v1beta/") {
				// Sibling APIs retain their version with an OpenAI /v1 base.
				endpointBase = strings.TrimSuffix(endpointBase, "/v1")
			}
			*address = proxy.BuildUpstreamURL(endpointBase, *address)
		}
	}
	original, _ := url.Parse(preset.DefaultURL)
	if config.Chat != nil && config.Chat.Profile == "" && original != nil && strings.EqualFold(original.Host, parsed.Host) {
		config.Chat.Profile = service.NativeChatRequestProfile(base, preset.Platform)
	}
	writeJSON(w, http.StatusOK, map[string]any{"provider": preset.Provider, "endpointConfig": config})
}
