package admin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

func (h *upstreamCatalogHandler) connect(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PresetID          string                 `json:"presetId"`
		APIKey            *string                `json:"apiKey,omitempty"`
		GCPServiceAccount *string                `json:"gcpServiceAccount,omitempty"`
		BaseURL           string                 `json:"baseUrl,omitempty"`
		ProjectID         string                 `json:"projectId,omitempty"`
		Name              string                 `json:"name,omitempty"`
		UseSystemProxy    bool                   `json:"useSystemProxy,omitempty"`
		ChannelProxy      string                 `json:"channelProxy,omitempty"`
		Protocols         []string               `json:"protocols,omitempty"`
		EndpointConfig    *store.DirectEndpoints `json:"endpointConfig,omitempty"`
		CustomHeaders     string                 `json:"customHeaders,omitempty"`
		ParamOverride     string                 `json:"paramOverride,omitempty"`
	}
	if !catalogDecode(w, r, &input) {
		return
	}
	var preset *service.UpstreamPreset
	var endpoints store.DirectEndpoints
	var base string
	var err error
	if input.EndpointConfig != nil {
		preset, base, _, err = resolveUpstreamPresetBase(input.PresetID, input.BaseURL)
	} else {
		preset, endpoints, base, err = resolveUpstreamPresetConfig(input.PresetID, input.BaseURL, input.ProjectID)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if preset.CredentialMode == "oauth" {
		writeError(w, http.StatusBadRequest, "Use the OAuth connection flow for this platform")
		return
	}
	if input.EndpointConfig != nil {
		if input.Protocols != nil || !input.EndpointConfig.IsConfigured() {
			writeError(w, http.StatusBadRequest, "Use a non-empty endpointConfig or select preset protocols, not both")
			return
		}
		if input.ProjectID != "" {
			writeError(w, http.StatusBadRequest, "Custom endpoints use fully resolved URLs; omit projectId")
			return
		}
		// Advanced configuration is an explicit replacement, not a subset of
		// the template. Credential, URL and wire-profile validation below still
		// applies before discovery or any catalog write.
		endpoints = *input.EndpointConfig
	} else {
		endpoints, err = selectUpstreamPresetEndpoints(endpoints, input.Protocols)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if input.EndpointConfig == nil && preset.CredentialMode == "optional" && input.APIKey != nil && strings.TrimSpace(*input.APIKey) != "" {
		for _, entry := range endpoints.Entries() {
			if ep := entry.Endpoint; ep != nil && (ep.Profile == "ollama" || ep.Profile == "ollama-messages") {
				ep.Auth = store.DirectAuthBearer
			}
		}
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = preset.Name
	}
	channel := upstream.ChannelCreate{Name: name, Provider: preset.Provider, Dialect: "generic", Enabled: true, BaseURL: base, Endpoints: endpoints, UseSystemProxy: input.UseSystemProxy, ChannelProxy: strings.TrimSpace(input.ChannelProxy), CustomHeaders: input.CustomHeaders, ParamOverride: input.ParamOverride}
	if err := validateImportedConfig(importedChannelConfig{Name: name, Provider: preset.Provider, Dialect: "generic", BaseURL: base, Endpoints: endpoints, UseSystemProxy: channel.UseSystemProxy, ChannelProxy: channel.ChannelProxy, CustomHeaders: channel.CustomHeaders, ParamOverride: channel.ParamOverride}); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	kind := store.DirectCredentialAPIKey
	if preset.CredentialMode == "optional" && (input.APIKey == nil || strings.TrimSpace(*input.APIKey) == "") {
		kind = store.DirectCredentialNone
		input.APIKey = nil
	}
	if preset.CredentialMode == "gcpServiceAccount" {
		kind = store.DirectCredentialGCPServiceAccount
		if input.APIKey != nil && strings.TrimSpace(*input.APIKey) != "" {
			writeError(w, http.StatusBadRequest, "This platform uses a GCP service account JSON, not an API key")
			return
		}
	}
	secret, kind, state, gcpMaterial, err := oauth.ValidateDirectCredentialMaterial(preset.Provider, endpoints, kind, input.APIKey, nil, input.GCPServiceAccount)
	if err != nil {
		catalogError(w, err)
		return
	}
	connect := upstream.ConnectInput{Channel: channel, AutoName: strings.TrimSpace(input.Name) == "", Secret: secret, CredentialKind: kind, OAuthState: state, RecommendedModels: preset.RecommendedModels}
	if gcpMaterial != "" {
		connect.GCPServiceAccount = &gcpMaterial
	}
	out, err := upstream.Connect(r.Context(), h.db, connect)
	if err != nil {
		catalogError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func selectUpstreamPresetEndpoints(endpoints store.DirectEndpoints, protocols []string) (store.DirectEndpoints, error) {
	if protocols == nil {
		return endpoints, nil
	}
	if len(protocols) == 0 {
		return store.DirectEndpoints{}, fmt.Errorf("Select at least one upstream protocol")
	}
	available := make(map[string]*store.DirectEndpoint)
	for _, entry := range endpoints.Entries() {
		if entry.Endpoint != nil {
			available[entry.Key] = entry.Endpoint
		}
	}
	selected := make(map[string]*store.DirectEndpoint, len(protocols))
	for _, key := range protocols {
		if available[key] == nil || selected[key] != nil {
			return store.DirectEndpoints{}, fmt.Errorf("Invalid preset protocol selection; use endpointConfig for custom interfaces")
		}
		selected[key] = available[key]
	}
	raw, err := json.Marshal(selected)
	if err != nil {
		return store.DirectEndpoints{}, err
	}
	var result store.DirectEndpoints
	if err := json.Unmarshal(raw, &result); err != nil {
		return store.DirectEndpoints{}, err
	}
	return result, nil
}
