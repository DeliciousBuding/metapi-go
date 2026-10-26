package proxyhandler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/deliciousbuding/metapi-go/internal/upstreamadapter"
)

func normalizeDirectNanoGPTJSON(body []byte, tools *upstreamadapter.NanoGPTTools) ([]byte, error) {
	response, err := upstreamadapter.NanoGPTObject(body)
	if err != nil {
		return nil, err
	}
	if upstreamadapter.NanoGPTPresent(response["error"]) {
		return nil, fmt.Errorf("NanoGPT returned an error")
	}
	var choices []map[string]json.RawMessage
	if json.Unmarshal(response["choices"], &choices) != nil || len(choices) == 0 {
		return nil, fmt.Errorf("NanoGPT response requires Chat choices")
	}
	if len(choices) > directNanoGPTMaxItems {
		return nil, fmt.Errorf("NanoGPT response exceeds choice limit")
	}
	if err := nanoGPTChatObject(response, false); err != nil {
		return nil, err
	}
	seen := make(map[int]bool)
	for _, choice := range choices {
		index, err := nanoGPTIndex(choice["index"])
		if err != nil || seen[index] {
			return nil, fmt.Errorf("NanoGPT choice indices must be unique nonnegative integers")
		}
		seen[index] = true
		message, err := upstreamadapter.NanoGPTObject(choice["message"])
		if err != nil {
			return nil, err
		}
		if err := upstreamadapter.MoveNanoGPTReasoning(message, "reasoning", "reasoning_content"); err != nil {
			return nil, err
		}
		var text string
		if upstreamadapter.NanoGPTPresent(message["content"]) && json.Unmarshal(message["content"], &text) == nil {
			parser := newNanoGPTText(tools)
			remaining, calls, err := parser.feed(text, true)
			if err != nil {
				return nil, err
			}
			if len(calls) > 0 {
				finish := nanoGPTString(choice["finish_reason"])
				if finish != "stop" && finish != "tool_calls" {
					return nil, fmt.Errorf("NanoGPT XML tools require a successful choice terminal")
				}
				var native []json.RawMessage
				if upstreamadapter.NanoGPTPresent(message["tool_calls"]) && json.Unmarshal(message["tool_calls"], &native) != nil {
					return nil, fmt.Errorf("invalid NanoGPT native tool calls")
				}
				if len(native)+len(calls) > directNanoGPTMaxItems {
					return nil, fmt.Errorf("NanoGPT response exceeds tool limit")
				}
				ids := make(map[string]bool)
				for _, raw := range native {
					call, err := upstreamadapter.NanoGPTObject(raw)
					if err != nil {
						return nil, err
					}
					id := nanoGPTString(call["id"])
					if id == "" || ids[id] {
						return nil, fmt.Errorf("invalid NanoGPT native tool identity")
					}
					ids[id] = true
				}
				for ordinal, call := range calls {
					id := nanoGPTCallID(tools, nanoGPTString(response["id"]), index, ordinal, call)
					if ids[id] {
						return nil, fmt.Errorf("NanoGPT XML/native tool identity collision")
					}
					ids[id] = true
					encoded, _ := json.Marshal(map[string]any{"id": id, "type": "function", "function": map[string]string{"name": call.name, "arguments": call.arguments}})
					native = append(native, encoded)
				}
				message["tool_calls"], _ = json.Marshal(native)
				message["content"] = json.RawMessage("null")
				if remaining != "" {
					message["content"], _ = json.Marshal(remaining)
				}
				choice["finish_reason"] = json.RawMessage(`"tool_calls"`)
			}
		}
		choice["message"], err = json.Marshal(message)
		if err != nil {
			return nil, err
		}
	}
	response["choices"], err = json.Marshal(choices)
	if err != nil {
		return nil, err
	}
	return json.Marshal(response)
}

func nanoGPTCallID(tools *upstreamadapter.NanoGPTTools, responseID string, choice, ordinal int, call nanoGPTXMLCall) string {
	seed := responseID
	if seed == "" && tools != nil {
		seed = tools.IdentitySeed()
	}
	encoded, _ := json.Marshal([]any{seed, choice, ordinal, call.name, call.arguments})
	digest := sha256.Sum256(encoded)
	return "nanogpt_xml_" + hex.EncodeToString(digest[:16])
}

func nanoGPTString(raw []byte) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}
func nanoGPTIndex(raw []byte) (int, error) {
	var value int
	if !upstreamadapter.NanoGPTPresent(raw) || json.Unmarshal(raw, &value) != nil || value < 0 {
		return 0, fmt.Errorf("invalid NanoGPT index")
	}
	return value, nil
}
func nanoGPTChatObject(object map[string]json.RawMessage, stream bool) error {
	want := "chat.completion"
	if stream {
		want += ".chunk"
	}
	if upstreamadapter.NanoGPTPresent(object["object"]) && nanoGPTString(object["object"]) != want {
		return fmt.Errorf("unexpected NanoGPT Chat object")
	}
	object["object"], _ = json.Marshal(want)
	return nil
}
