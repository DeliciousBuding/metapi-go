package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"slices"

	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
)

// DirectRequestConfig belongs to the selected channel, independently from its
// endpoint permissions. A zero value keeps pre-existing request behavior.
type DirectRequestConfig struct {
	requestoverride.Config
	Plugins []DirectPluginBinding `json:"plugins,omitempty"`
}

// Binding selects hooks only for explicitly named native protocol endpoints;
// it grants no protocol, credential, destination or transport permission.
type DirectPluginBinding struct {
	Key       string                     `json:"key"`
	Protocols []string                   `json:"protocols"`
	Options   map[string]json.RawMessage `json:"options,omitempty"`
}

func (c DirectRequestConfig) Active() bool { return c.Config.Active() || len(c.Plugins) > 0 }

func (c DirectRequestConfig) Validate() error {
	if err := c.Config.Validate(); err != nil {
		return err
	}
	if len(c.Plugins) > 8 {
		return fmt.Errorf("too_many_channel_plugins")
	}
	seen := map[string]bool{}
	for _, binding := range c.Plugins {
		if !contract.ValidKey(binding.Key) || seen[binding.Key] || len(binding.Protocols) == 0 || len(binding.Protocols) > 4 || len(binding.Options) > 32 {
			return fmt.Errorf("invalid_channel_plugin_binding")
		}
		seen[binding.Key] = true
		protocols := map[string]bool{}
		for _, protocol := range binding.Protocols {
			if protocols[protocol] || !slices.Contains([]string{"chat", "responses", "messages", "gemini"}, protocol) {
				return fmt.Errorf("invalid_channel_plugin_protocol")
			}
			protocols[protocol] = true
		}
		for key, value := range binding.Options {
			if !contract.ValidKey(key) || len(value) > 4096 || !json.Valid(value) {
				return fmt.Errorf("invalid_channel_plugin_option")
			}
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
		if len(c.Body) > 0 && entry.Protocol&(DirectProtocolChat|DirectProtocolResponses|DirectProtocolResponsesWebsocket|DirectProtocolMessages|DirectProtocolGemini|DirectProtocolCompletions|DirectProtocolEmbeddings|DirectProtocolRerank|DirectProtocolModerations|DirectProtocolImageGeneration|DirectProtocolAudioSpeech) == 0 {
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
