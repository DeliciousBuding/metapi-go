package generate_content

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/deliciousbuding/metapi-go/transform/shared"
)

// ToChatRequest maps native generation input to the shared Chat representation.
// Provider-only resources and safety policies require a native endpoint.
func ToChatRequest(body []byte, model string) ([]byte, error) {
	in, err := bridgeObject(body)
	if err != nil {
		return nil, err
	}
	if err := bridgeKeys(in, "contents", "systemInstruction", "generationConfig", "tools", "toolConfig", "model", "stream"); err != nil {
		return nil, err
	}
	out := map[string]any{"model": model}
	if in["stream"] != nil {
		out["stream"] = in["stream"]
	}
	messages, err := geminiChatMessages(in)
	if err != nil {
		return nil, err
	}
	out["messages"] = messages
	if raw := in["generationConfig"]; raw != nil {
		gc, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Gemini generationConfig must be an object")
		}
		if err := bridgeKeys(gc, "temperature", "topP", "maxOutputTokens", "stopSequences", "candidateCount", "responseMimeType", "responseSchema", "responseJsonSchema", "thinkingConfig"); err != nil {
			return nil, err
		}
		for from, to := range map[string]string{"temperature": "temperature", "topP": "top_p", "maxOutputTokens": "max_tokens", "stopSequences": "stop"} {
			if gc[from] != nil {
				out[to] = gc[from]
			}
		}
		if n, ok := gc["candidateCount"]; ok && bridgeNumber(n) != 1 {
			return nil, fmt.Errorf("Gemini candidateCount must be one for conversion")
		}
		if gc["thinkingConfig"] != nil {
			return nil, fmt.Errorf("Gemini thinkingConfig requires a native endpoint")
		}
		format, err := geminiResponseFormat(gc, body)
		if err != nil {
			return nil, err
		}
		if format != nil {
			out["response_format"] = format
		}
	}
	if raw, ok := in["tools"].([]any); ok {
		var tools []any
		for _, t := range raw {
			tool, ok := t.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid Gemini tool")
			}
			if err := bridgeKeys(tool, "functionDeclarations"); err != nil {
				return nil, err
			}
			decls, ok := tool["functionDeclarations"].([]any)
			if !ok {
				return nil, fmt.Errorf("Gemini tool requires functionDeclarations")
			}
			for _, d := range decls {
				fn, ok := d.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid Gemini function declaration")
				}
				if err := bridgeKeys(fn, "name", "description", "parameters", "parametersJsonSchema"); err != nil {
					return nil, err
				}
				copy := map[string]any{}
				for k, v := range fn {
					copy[k] = v
				}
				if v := copy["parametersJsonSchema"]; v != nil {
					copy["parameters"] = v
					delete(copy, "parametersJsonSchema")
				} else if schema := copy["parameters"]; schema != nil {
					converted, err := geminiSchemaToJSON(schema)
					if err != nil {
						return nil, err
					}
					copy["parameters"] = converted
				}
				tools = append(tools, map[string]any{"type": "function", "function": copy})
			}
		}
		out["tools"] = tools
	}
	if tc, ok := in["toolConfig"].(map[string]any); ok {
		if err := bridgeKeys(tc, "functionCallingConfig"); err != nil {
			return nil, err
		}
		fc, ok := tc["functionCallingConfig"].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid Gemini functionCallingConfig")
		}
		if err := bridgeKeys(fc, "mode", "allowedFunctionNames"); err != nil {
			return nil, err
		}
		mode, _ := fc["mode"].(string)
		names, _ := fc["allowedFunctionNames"].([]any)
		if len(names) > 1 {
			return nil, fmt.Errorf("Gemini allowedFunctionNames subset requires native endpoint")
		}
		switch mode {
		case "", "AUTO":
			out["tool_choice"] = "auto"
		case "NONE":
			out["tool_choice"] = "none"
		case "ANY":
			out["tool_choice"] = "required"
		default:
			return nil, fmt.Errorf("unsupported Gemini function calling mode")
		}
		if len(names) == 1 {
			if mode != "ANY" {
				return nil, fmt.Errorf("Gemini allowedFunctionNames requires ANY mode")
			}
			out["tool_choice"] = map[string]any{"type": "function", "function": map[string]any{"name": names[0]}}
		}
	}
	return json.Marshal(out)
}

func bridgeObject(body []byte) (map[string]any, error) {
	var out map[string]any
	if shared.DecodeJSON(bytes.NewReader(body), &out) != nil || out == nil {
		return nil, fmt.Errorf("protocol body must be a JSON object")
	}
	return out, nil
}

func bridgeKeys(obj map[string]any, allowed ...string) error {
	if key, ok := shared.UnsupportedField(obj, allowed...); ok {
		return fmt.Errorf("field %s requires a native protocol endpoint", key)
	}
	return nil
}

func bridgeID(prefix string) string {
	var data [12]byte
	if _, err := rand.Read(data[:]); err != nil {
		panic(err)
	}
	return prefix + hex.EncodeToString(data[:])
}
