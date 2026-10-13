package admin

import (
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/service/oauth"
	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

func (h *upstreamCatalogHandler) connect(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PresetID          string  `json:"presetId"`
		APIKey            *string `json:"apiKey,omitempty"`
		GCPServiceAccount *string `json:"gcpServiceAccount,omitempty"`
		BaseURL           string  `json:"baseUrl,omitempty"`
		ProjectID         string  `json:"projectId,omitempty"`
		Name              string  `json:"name,omitempty"`
		UseSystemProxy    bool    `json:"useSystemProxy,omitempty"`
		ChannelProxy      string  `json:"channelProxy,omitempty"`
	}
	if !catalogDecode(w, r, &input) {
		return
	}
	preset, endpoints, base, err := resolveUpstreamPresetConfig(input.PresetID, input.BaseURL, input.ProjectID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if preset.CredentialMode == "oauth" {
		writeError(w, http.StatusBadRequest, "Use the OAuth connection flow for this platform")
		return
	}
	if preset.CredentialMode == "optional" && input.APIKey != nil && strings.TrimSpace(*input.APIKey) != "" {
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
	channel := upstream.ChannelCreate{Name: name, Provider: preset.Provider, Dialect: "generic", Enabled: true, BaseURL: base, Endpoints: endpoints, UseSystemProxy: input.UseSystemProxy, ChannelProxy: strings.TrimSpace(input.ChannelProxy)}
	if err := validateImportedConfig(importedChannelConfig{Name: name, Provider: preset.Provider, Dialect: "generic", BaseURL: base, Endpoints: endpoints, UseSystemProxy: channel.UseSystemProxy, ChannelProxy: channel.ChannelProxy}); err != nil {
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
