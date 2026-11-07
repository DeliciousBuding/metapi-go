package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"net/url"
	"slices"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
)

// DirectRequestConfig belongs to the selected channel, independently from its
// endpoint permissions. A zero value keeps pre-existing request behavior.
type DirectRequestConfig struct {
	requestoverride.Config
	Plugins  []DirectPluginBinding  `json:"plugins,omitempty"`
	Provider *DirectProviderBinding `json:"provider,omitempty"`
}

// Binding selects hooks only for explicitly named native protocol endpoints;
// it grants no protocol, credential, destination or transport permission.
type DirectPluginBinding struct {
	Key       string                     `json:"key"`
	Protocols []string                   `json:"protocols"`
	Options   map[string]json.RawMessage `json:"options,omitempty"`
}

// A single driver may serve explicitly selected slots alongside independent
// wire hooks. Its declaration is not an endpoint or model authorization.
type DirectProviderBinding struct {
	Key          string                              `json:"key"`
	Protocols    []string                            `json:"protocols"`
	Options      map[string]json.RawMessage          `json:"options,omitempty"`
	Catalog      *DirectCatalogEndpoint              `json:"catalog,omitempty"`
	ImageQueries map[string]DirectImageQueryEndpoint `json:"imageQueries,omitempty"`
}

// Each saved query target belongs to one selected image protocol endpoint.
// The only dynamic component is an escaped task id, never an arbitrary URL.
type DirectImageQueryEndpoint struct {
	URL  string `json:"url"`
	Auth string `json:"auth"`
}

func (c DirectImageQueryEndpoint) Validate() error {
	if (DirectCatalogEndpoint{URL: c.URL, Auth: c.Auth}).Validate() != nil {
		return fmt.Errorf("invalid_provider_image_query_endpoint")
	}
	if _, err := contract.ImageQueryURL(c.URL, "validation"); err != nil {
		return fmt.Errorf("invalid_provider_image_query_endpoint")
	}
	return nil
}

type DirectCatalogEndpoint struct {
	URL             string `json:"url"`
	Auth            string `json:"auth"`
	CursorParameter string `json:"cursorParameter,omitempty"`
}

func (c DirectCatalogEndpoint) Validate() error {
	u, err := url.Parse(c.URL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || !contract.ValidCursorParameter(c.CursorParameter) || (c.Auth != DirectAuthBearer && c.Auth != DirectAuthAPIKey && c.Auth != DirectAuthGoogle && c.Auth != DirectAuthNone) {
		return fmt.Errorf("invalid_provider_catalog_endpoint")
	}
	return nil
}

func catalogOrigin(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return ""
	}
	scheme := u.Scheme
	if scheme == "ws" {
		scheme = "http"
	}
	if scheme == "wss" {
		scheme = "https"
	}
	port := u.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return scheme + "://" + strings.ToLower(u.Hostname()) + ":" + port
}

func (c DirectRequestConfig) Active() bool {
	return c.Config.Active() || len(c.Plugins) > 0 || c.Provider != nil
}

func (c DirectRequestConfig) Validate() error {
	if err := c.Config.Validate(); err != nil {
		return err
	}
	if len(c.Plugins) > 8 {
		return fmt.Errorf("too_many_channel_plugins")
	}
	seen := map[string]bool{}
	if c.Provider != nil {
		if err := validateDirectPluginBinding(DirectPluginBinding{Key: c.Provider.Key, Protocols: c.Provider.Protocols, Options: c.Provider.Options}, contract.MaxProviderProtocols, contract.ValidProviderProtocol); err != nil {
			return err
		}
		if c.Provider.Catalog != nil {
			if err := c.Provider.Catalog.Validate(); err != nil {
				return err
			}
		}
		if len(c.Provider.ImageQueries) > 4 {
			return fmt.Errorf("invalid_provider_image_query_endpoint")
		}
		for protocol, query := range c.Provider.ImageQueries {
			if !contract.ValidImageTaskProtocol(protocol) || !slices.Contains(c.Provider.Protocols, protocol) || query.Validate() != nil {
				return fmt.Errorf("invalid_provider_image_query_endpoint")
			}
		}
		seen[c.Provider.Key] = true
	}
	for _, binding := range c.Plugins {
		if seen[binding.Key] {
			return fmt.Errorf("invalid_channel_plugin_binding")
		}
		seen[binding.Key] = true
		if err := validateDirectPluginBinding(binding, contract.MaxProtocols, contract.ValidProtocol); err != nil {
			return err
		}
	}
	return nil
}

func validateDirectPluginBinding(binding DirectPluginBinding, limit int, validProtocol func(string) bool) error {
	if !contract.ValidKey(binding.Key) || len(binding.Protocols) == 0 || len(binding.Protocols) > limit || len(binding.Options) > 32 {
		return fmt.Errorf("invalid_channel_plugin_binding")
	}
	protocols := map[string]bool{}
	for _, protocol := range binding.Protocols {
		if protocols[protocol] || !validProtocol(protocol) {
			return fmt.Errorf("invalid_channel_plugin_protocol")
		}
		protocols[protocol] = true
	}
	for key, value := range binding.Options {
		if !contract.ValidKey(key) || len(value) > 4096 || !json.Valid(value) {
			return fmt.Errorf("invalid_channel_plugin_option")
		}
	}
	return nil
}

func (c DirectRequestConfig) Value() (driver.Value, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	value, err := json.Marshal(c)
	return string(value), err
}

func (c *DirectRequestConfig) Scan(value any) error {
	var raw []byte
	switch value := value.(type) {
	case string:
		raw = []byte(value)
	case []byte:
		raw = value
	case nil:
		*c = DirectRequestConfig{}
		return nil
	default:
		return fmt.Errorf("invalid direct request configuration")
	}
	var config DirectRequestConfig
	if err := requestoverride.Decode(raw, &config); err != nil {
		return err
	}
	if err := config.Validate(); err != nil {
		return err
	}
	*c = config
	return nil
}

func (c DirectRequestConfig) ValidateEndpoints(endpoints DirectEndpoints) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if c.Provider != nil {
		catalogAllowed := c.Provider.Catalog == nil
		for _, protocol := range c.Provider.Protocols {
			found := false
			for _, entry := range endpoints.Entries() {
				if entry.Key == protocol && entry.Endpoint != nil {
					found = true
					if c.Provider.Catalog != nil && catalogOrigin(entry.Endpoint.URL) == catalogOrigin(c.Provider.Catalog.URL) {
						catalogAllowed = true
					}
				}
			}
			if !found {
				return fmt.Errorf("channel_provider_endpoint_unsupported")
			}
		}
		if !catalogAllowed {
			return fmt.Errorf("provider_catalog_origin_not_bound")
		}
		for protocol, query := range c.Provider.ImageQueries {
			bound := false
			for _, entry := range endpoints.Entries() {
				if entry.Key == protocol && entry.Endpoint != nil && catalogOrigin(entry.Endpoint.URL) == catalogOrigin(query.URL) {
					bound = true
				}
			}
			if !bound {
				return fmt.Errorf("provider_image_query_origin_not_bound")
			}
		}
	}
	for _, binding := range c.Plugins {
		for _, protocol := range binding.Protocols {
			found := false
			for _, entry := range endpoints.Entries() {
				if entry.Key == protocol && entry.Endpoint != nil && requestOperationsProfileSupported(entry.Endpoint.Profile) {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("channel_plugin_endpoint_unsupported")
			}
		}
	}
	if !c.Config.Active() {
		return nil
	}
	for _, entry := range endpoints.Entries() {
		if entry.Endpoint == nil {
			continue
		}
		if len(c.Body) > 0 && entry.Protocol&(DirectProtocolChat|DirectProtocolResponses|DirectProtocolResponsesCompact|DirectProtocolResponsesWebsocket|DirectProtocolMessages|DirectProtocolGemini|DirectProtocolCompletions|DirectProtocolEmbeddings|DirectProtocolRerank|DirectProtocolModerations|DirectProtocolImageGeneration|DirectProtocolAudioSpeech|DirectProtocolDecisions) == 0 {
			return fmt.Errorf("channel_request_operation_nonjson_unsupported")
		}
		if !requestOperationsProfileSupported(entry.Endpoint.Profile) {
			return fmt.Errorf("channel_request_operation_profile_unsupported")
		}
	}
	return nil
}

func requestOperationsProfileSupported(profile string) bool {
	switch profile {
	case "", "moonshot", "longcat", "openrouter", "cerebras", "bailian", "cline", "github-copilot", "xai-subscription", "deepseek", "zai", "codex", "claudecode", "bedrock", "vertex", "vertex-anthropic", "ollama-messages":
		return true
	default:
		return false
	}
}
