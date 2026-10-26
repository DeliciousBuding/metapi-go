package upstreamadapter

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
type NanoGPTTools struct {
	names map[string]bool
	scope [sha256.Size]byte
}

func NewNanoGPTTools(body []byte) (*NanoGPTTools, error) {
	request, err := NanoGPTObject(body)
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
	tools := &NanoGPTTools{names: make(map[string]bool), scope: sha256.Sum256(canonical)}
	var declared []struct {
		Type     string `json:"type"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if raw := request["tools"]; NanoGPTPresent(raw) {
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
	if NanoGPTPresent(rawChoice) && json.Unmarshal(rawChoice, &selection) == nil {
		if selection != "auto" && selection != "required" {
			clear(tools.names)
		}
	} else if raw := rawChoice; NanoGPTPresent(raw) && bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
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
	} else if NanoGPTPresent(rawChoice) {
		clear(tools.names)
	}
	return tools, nil
}

func prepareDirectNanoGPTRequest(body []byte) ([]byte, error) {
	request, err := NanoGPTObject(body)
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
		if err := MoveNanoGPTReasoning(message, "reasoning_content", "reasoning"); err != nil {
			return nil, err
		}
	}
	request["messages"], err = json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	return json.Marshal(request)
}

func MoveNanoGPTReasoning(message map[string]json.RawMessage, source, target string) error {
	raw := message[source]
	if !NanoGPTPresent(raw) {
		return nil
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return fmt.Errorf("NanoGPT reasoning must be a string")
	}
	if existing := message[target]; NanoGPTPresent(existing) {
		var other string
		if json.Unmarshal(existing, &other) != nil || text != other {
			return fmt.Errorf("NanoGPT reasoning fields conflict")
		}
	}
	message[target] = raw
	delete(message, source)
	return nil
}

func NanoGPTObject(raw []byte) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if !utf8.Valid(raw) || json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, fmt.Errorf("NanoGPT requires a JSON object")
	}
	return object, nil
}
func NanoGPTPresent(raw []byte) bool {
	return len(raw) > 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// Empty and Allows expose only the request-scoped tool authorization decision.
func (t *NanoGPTTools) Empty() bool             { return t == nil || len(t.names) == 0 }
func (t *NanoGPTTools) Allows(name string) bool { return t != nil && t.names[name] }

// IdentitySeed keeps synthesized tool IDs stable across buffered/streaming wire modes.
func (t *NanoGPTTools) IdentitySeed() string {
	if t == nil {
		return ""
	}
	return hex.EncodeToString(t.scope[:])
}
