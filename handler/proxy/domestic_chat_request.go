package proxyhandler

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// prepareDomesticChatRequest translates only the provider's thinking contract.
// RawMessage keeps tools, schemas, opaque fields and numeric values intact.
func prepareDomesticChatRequest(body []byte, profile string) ([]byte, error) {
	if profile != "deepseek" && profile != "zai" {
		return body, nil
	}
	var request map[string]json.RawMessage
	if json.Unmarshal(body, &request) != nil || request == nil {
		return nil, fmt.Errorf("invalid domestic Chat request")
	}
	var effort string
	if raw, ok := request["reasoning_effort"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if json.Unmarshal(raw, &effort) != nil {
			return nil, fmt.Errorf("reasoning_effort must be a string")
		}
	}
	var thinking map[string]json.RawMessage
	if raw, ok := request["thinking"]; ok && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if json.Unmarshal(raw, &thinking) != nil || thinking == nil {
			return nil, fmt.Errorf("thinking must be an object")
		}
	}
	var mode string
	if raw, ok := thinking["type"]; ok {
		if json.Unmarshal(raw, &mode) != nil {
			return nil, fmt.Errorf("thinking.type must be a string")
		}
		if mode != "enabled" && mode != "disabled" {
			return nil, fmt.Errorf("unsupported domestic thinking.type")
		}
	}
	disabled := effort == "none" || mode == "disabled"
	if profile == "zai" && effort == "" && mode == "" {
		return body, nil
	}
	if thinking == nil {
		thinking = map[string]json.RawMessage{}
	}
	mode = "enabled"
	if disabled {
		mode = "disabled"
	}
	thinking["type"], _ = json.Marshal(mode)
	request["thinking"], _ = json.Marshal(thinking)
	if profile == "deepseek" {
		if disabled {
			delete(request, "reasoning_effort")
		} else {
			var messages []map[string]json.RawMessage
			if json.Unmarshal(request["messages"], &messages) != nil || len(messages) == 0 {
				return nil, fmt.Errorf("DeepSeek messages must be nonempty")
			}
			for _, message := range messages {
				var role string
				_ = json.Unmarshal(message["role"], &role)
				if role != "assistant" {
					continue
				}
				raw, exists := message["reasoning_content"]
				if !exists || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
					message["reasoning_content"] = json.RawMessage(`""`)
				}
			}
			request["messages"], _ = json.Marshal(messages)
		}
	}
	return json.Marshal(request)
}

func nativeChatRequestProfile(site store.Site, path string) string {
	endpoint, ok := proxy.EndpointFromPath(path)
	if !ok || endpoint != proxy.EndpointChat {
		return ""
	}
	preset := service.DetectSiteInitializationPreset(site.URL, site.Platform)
	if preset == nil {
		return ""
	}
	switch preset.ID {
	case "deepseek-openai":
		return "deepseek"
	case "zhipu-openai", "zhipu-coding-plan-openai", "zai-openai", "zai-coding-plan-openai", "xiaomi-openai":
		return "zai"
	default:
		return ""
	}
}
