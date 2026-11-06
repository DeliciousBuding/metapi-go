package store

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
)

// DirectRequestConfig belongs to the selected channel, independently from its
// endpoint permissions. A zero value keeps pre-existing request behavior.
type DirectRequestConfig struct {
	requestoverride.Config
}

func (c DirectRequestConfig) Value() (driver.Value, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	value, err := json.Marshal(c.Config)
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
	var config requestoverride.Config
	if err := requestoverride.Decode(raw, &config); err != nil {
		return err
	}
	if err := config.Validate(); err != nil {
		return err
	}
	c.Config = config
	return nil
}

func (c DirectRequestConfig) ValidateEndpoints(endpoints DirectEndpoints) error {
	if !c.Active() {
		return nil
	}
	for _, entry := range endpoints.Entries() {
		if entry.Endpoint == nil {
			continue
		}
		if len(c.Body) > 0 && entry.Protocol&(DirectProtocolChat|DirectProtocolResponses|DirectProtocolResponsesWebsocket|DirectProtocolMessages|DirectProtocolGemini|DirectProtocolCompletions|DirectProtocolEmbeddings|DirectProtocolRerank|DirectProtocolModerations|DirectProtocolImageGeneration|DirectProtocolAudioSpeech) == 0 {
			return fmt.Errorf("channel_request_operation_nonjson_unsupported")
		}
		switch entry.Endpoint.Profile {
		case "", "moonshot", "longcat", "openrouter", "cerebras", "bailian", "cline",
			"github-copilot", "xai-subscription", "deepseek", "zai", "codex", "claudecode",
			"bedrock", "vertex", "vertex-anthropic", "ollama-messages":
			// Audited single-request JSON/native header profiles.
		default:
			return fmt.Errorf("channel_request_operation_profile_unsupported")
		}
	}
	return nil
}
