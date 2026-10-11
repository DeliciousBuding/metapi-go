package admin

import (
	"fmt"
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
	preset, config, base, err := resolveUpstreamPresetConfig(input.PresetID, input.BaseURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"presetId": preset.ID, "provider": preset.Provider, "baseUrl": base, "endpointConfig": config})
}

// resolveUpstreamPresetConfig is shared with quick connect. It is a local
// projection only: no model discovery, credential access or outbound request.
func resolveUpstreamPresetConfig(presetID, baseURL string) (*service.UpstreamPreset, store.DirectEndpoints, string, error) {
	preset := service.GetUpstreamPreset(presetID)
	if preset == nil {
		return nil, store.DirectEndpoints{}, "", fmt.Errorf("unknown upstream preset")
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	useDefaults := base == "" || preset.IsDefaultBaseURL(base)
	if useDefaults {
		base = preset.DefaultURL
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil || strings.ContainsAny(base, "?#") || service.IsForbiddenSiteTargetURL(base) {
		return nil, store.DirectEndpoints{}, "", fmt.Errorf("invalid upstream base URL")
	}
	config := service.UpstreamPresetEndpointPaths(preset.ID, preset.Platform)
	if !config.IsConfigured() {
		return nil, store.DirectEndpoints{}, "", fmt.Errorf("preset has no executable endpoint contract")
	}
	for _, entry := range config.Entries() {
		if entry.Endpoint == nil {
			continue
		}
		for _, address := range entry.Endpoint.URLFields() {
			endpointBase := base
			if useDefaults {
				endpointBase = preset.EndpointBaseURL(entry.Key)
			}
			if strings.HasPrefix(*address, "/beta/") || strings.HasPrefix(*address, "/v1beta/") {
				// Sibling APIs retain their version with an OpenAI /v1 base.
				endpointBase = strings.TrimSuffix(endpointBase, "/v1")
			}
			*address = proxy.BuildUpstreamURL(endpointBase, *address)
		}
	}
	raw, err := config.Value()
	if err != nil {
		return nil, store.DirectEndpoints{}, "", err
	}
	var validated store.DirectEndpoints
	if err := validated.Scan(raw); err != nil {
		return nil, store.DirectEndpoints{}, "", fmt.Errorf("invalid preset endpoint contract: %w", err)
	}
	return preset, config, base, nil
}
