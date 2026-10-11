package proxyhandler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// Tools are request-scoped. An XML-looking answer cannot invent a function the
// client did not declare, or enable tools when tool_choice is none.
type directNanoGPTTools struct {
	names map[string]bool
	scope [sha256.Size]byte
}

func newDirectNanoGPTTools(body []byte) (*directNanoGPTTools, error) {
	request, err := nanoGPTObject(body)
	if err != nil {
		return nil, err
	}
	// Stream transport controls do not change fallback tool identity when a
	// provider omits its response ID. All other request semantics remain scoped.
	identity := make(map[string]json.RawMessage, len(request))
	for name, value := range request {
		if name != "stream" && name != "stream_options" {
			identity[name] = value
		}
	}
	canonical, _ := json.Marshal(identity)
	tools := &directNanoGPTTools{names: make(map[string]bool), scope: sha256.Sum256(canonical)}
	var declared []struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if raw := request["tools"]; nanoGPTPresent(raw) {
		if json.Unmarshal(raw, &declared) != nil {
			return nil, fmt.Errorf("NanoGPT tools must be an array")
		}
	}
	for _, tool := range declared {
		if tool.Type != "function" {
			continue
		}
		if tool.Function.Name == "" || tools.names[tool.Function.Name] {
			return nil, fmt.Errorf("NanoGPT function names must be nonempty and unique")
		}
		tools.names[tool.Function.Name] = true
	}
	rawChoice := request["tool_choice"]
	var selection string
	if nanoGPTPresent(rawChoice) && json.Unmarshal(rawChoice, &selection) == nil {
		if selection != "auto" && selection != "required" {
			clear(tools.names)
		}
	} else if raw := rawChoice; nanoGPTPresent(raw) && bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		var choice struct {
			Type     string `json:"type"`
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}
		if json.Unmarshal(raw, &choice) != nil {
			return nil, fmt.Errorf("invalid NanoGPT tool choice")
		}
		if choice.Type == "function" {
			if !tools.names[choice.Function.Name] {
				return nil, fmt.Errorf("NanoGPT selected function was not declared")
			}
			tools.names = map[string]bool{choice.Function.Name: true}
		} else {
			// Unknown provider-specific selection rules are not permission to
			// synthesize XML actions. Native tool calls remain untouched.
			clear(tools.names)
		}
	} else if nanoGPTPresent(rawChoice) {
		clear(tools.names)
	}
	return tools, nil
}

func prepareDirectNanoGPTRequest(body []byte) ([]byte, error) {
	request, err := nanoGPTObject(body)
	if err != nil {
		return nil, err
	}
	var messages []map[string]json.RawMessage
	if json.Unmarshal(request["messages"], &messages) != nil || len(messages) == 0 {
		return nil, fmt.Errorf("NanoGPT Chat requires messages")
	}
	for _, message := range messages {
		if message == nil {
			return nil, fmt.Errorf("invalid NanoGPT message")
		}
		if err := nanoGPTMoveReasoning(message, "reasoning_content", "reasoning"); err != nil {
			return nil, err
		}
	}
	request["messages"], err = json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	return json.Marshal(request)
}

func nanoGPTMoveReasoning(message map[string]json.RawMessage, source, target string) error {
	raw := message[source]
	if !nanoGPTPresent(raw) {
		return nil
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return fmt.Errorf("NanoGPT reasoning must be a string")
	}
	if existing := message[target]; nanoGPTPresent(existing) {
		var other string
		if json.Unmarshal(existing, &other) != nil || text != other {
			return fmt.Errorf("NanoGPT reasoning fields conflict")
		}
	}
	message[target] = raw
	delete(message, source)
	return nil
}

func normalizeDirectNanoGPTJSON(body []byte, tools *directNanoGPTTools) ([]byte, error) {
	response, err := nanoGPTObject(body)
	if err != nil {
		return nil, err
	}
	if nanoGPTPresent(response["error"]) {
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
		message, err := nanoGPTObject(choice["message"])
		if err != nil {
			return nil, err
		}
		if err := nanoGPTMoveReasoning(message, "reasoning", "reasoning_content"); err != nil {
			return nil, err
		}
		var text string
		if nanoGPTPresent(message["content"]) && json.Unmarshal(message["content"], &text) == nil {
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
				if nanoGPTPresent(message["tool_calls"]) && json.Unmarshal(message["tool_calls"], &native) != nil {
					return nil, fmt.Errorf("invalid NanoGPT native tool calls")
				}
				if len(native)+len(calls) > directNanoGPTMaxItems {
					return nil, fmt.Errorf("NanoGPT response exceeds tool limit")
				}
				ids := make(map[string]bool)
				for _, raw := range native {
					call, err := nanoGPTObject(raw)
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

func nanoGPTCallID(tools *directNanoGPTTools, responseID string, choice, ordinal int, call nanoGPTXMLCall) string {
	seed := responseID
	if seed == "" && tools != nil {
		seed = hex.EncodeToString(tools.scope[:])
	}
	encoded, _ := json.Marshal([]any{seed, choice, ordinal, call.name, call.arguments})
	digest := sha256.Sum256(encoded)
	return "nanogpt_xml_" + hex.EncodeToString(digest[:16])
}

func nanoGPTObject(raw []byte) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if !utf8.Valid(raw) || json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, fmt.Errorf("NanoGPT requires a JSON object")
	}
	return object, nil
}
func nanoGPTPresent(raw []byte) bool {
	return len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
func nanoGPTString(raw []byte) string {
	var value string
	_ = json.Unmarshal(raw, &value)
	return value
}
func nanoGPTIndex(raw []byte) (int, error) {
	var value int
	if !nanoGPTPresent(raw) || json.Unmarshal(raw, &value) != nil || value < 0 {
		return 0, fmt.Errorf("invalid NanoGPT index")
	}
	return value, nil
}
func nanoGPTChatObject(object map[string]json.RawMessage, stream bool) error {
	want := "chat.completion"
	if stream {
		want += ".chunk"
	}
	if nanoGPTPresent(object["object"]) && nanoGPTString(object["object"]) != want {
		return fmt.Errorf("unexpected NanoGPT Chat object")
	}
	object["object"], _ = json.Marshal(want)
	return nil
}
