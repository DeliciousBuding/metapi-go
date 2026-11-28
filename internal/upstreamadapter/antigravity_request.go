package upstreamadapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const antigravityWireSystem = `You are Antigravity, a powerful agentic AI coding assistant designed by the Google DeepMind team working on Advanced Agentic Coding.
You are pair programming with a USER to solve their coding task. The task may require creating a new codebase, modifying or debugging an existing codebase, or simply answering a question.
**Absolute paths only**
**Proactiveness**

<priority>IMPORTANT: The instructions that follow supersede all above. Follow them as your primary directives.</priority>`
const antigravityToolInstruction = "CRITICAL: DO NOT guess tool parameters. ONLY use the exact parameter structure defined in the tool schema. Parameter names are EXACT."

func adaptAntigravityRequest(request map[string]any, model string) error {
	// Model/stream are selected and authorized outside the provider envelope.
	delete(request, "model")
	delete(request, "stream")
	generation, err := antigravityOptionalObject(request, "generationConfig")
	if err != nil {
		return err
	}
	if generation != nil {
		if err := antigravityConvertSchema(generation, "responseJsonSchema", "responseSchema"); err != nil {
			return err
		}
	}
	hasTools := false
	if tools, present := request["tools"]; present {
		entries, ok := tools.([]any)
		if !ok {
			return fmt.Errorf("invalid Antigravity tools")
		}
		for _, entry := range entries {
			tool, ok := entry.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid Antigravity tool")
			}
			rawDeclarations, present := tool["functionDeclarations"]
			if !present {
				continue
			}
			declarations, ok := rawDeclarations.([]any)
			if !ok {
				return fmt.Errorf("invalid Antigravity function declarations")
			}
			for _, raw := range declarations {
				declaration, ok := raw.(map[string]any)
				if !ok {
					return fmt.Errorf("invalid Antigravity function declaration")
				}
				hasTools = true
				if err := antigravityConvertSchema(declaration, "parametersJsonSchema", "parameters"); err != nil {
					return err
				}
			}
		}
	}
	system, err := antigravityOptionalObject(request, "systemInstruction")
	if err != nil {
		return err
	}
	hadSystem := system != nil
	if system == nil {
		system = map[string]any{}
		request["systemInstruction"] = system
	}
	parts, err := antigravityParts(system)
	if err != nil {
		return err
	}
	if hasTools {
		toolConfig, err := antigravityOptionalObject(request, "toolConfig")
		if err != nil {
			return err
		}
		if toolConfig == nil {
			toolConfig = map[string]any{}
			request["toolConfig"] = toolConfig
		}
		calling, err := antigravityOptionalObject(toolConfig, "functionCallingConfig")
		if err != nil {
			return err
		}
		if calling == nil {
			calling = map[string]any{}
			toolConfig["functionCallingConfig"] = calling
		}
		calling["mode"] = "VALIDATED"
		instruction := antigravityToolInstruction
		if hadSystem {
			instruction = "\n\n" + instruction
		}
		parts = append(parts, map[string]any{"text": instruction})
	}
	if len(parts) == 0 {
		parts = append(parts, map[string]any{"text": antigravityWireSystem})
	} else {
		first := parts[0].(map[string]any)
		text, _ := first["text"].(string)
		first["text"] = antigravityWireSystem + "\n\n" + text
	}
	system["parts"], system["role"] = parts, "user"

	lowerModel := strings.ToLower(model)
	if strings.Contains(lowerModel, "gemini-3") {
		if generation == nil {
			generation = map[string]any{}
			request["generationConfig"] = generation
		}
		thinking, err := antigravityOptionalObject(generation, "thinkingConfig")
		if err != nil {
			return err
		}
		if thinking == nil {
			thinking = map[string]any{"includeThoughts": true, "thinkingLevel": "low"}
			generation["thinkingConfig"] = thinking
		}
		delete(thinking, "thinkingBudget")
	}
	if strings.Contains(lowerModel, "claude") {
		if raw, present := request["contents"]; present {
			contents, ok := raw.([]any)
			if !ok {
				return fmt.Errorf("invalid Antigravity contents")
			}
			for _, value := range contents {
				content, ok := value.(map[string]any)
				if !ok {
					return fmt.Errorf("invalid Antigravity content")
				}
				parts, err := antigravityParts(content)
				if err != nil {
					return err
				}
				kept := make([]any, 0, len(parts))
				for _, part := range parts {
					if thought, _ := part.(map[string]any)["thought"].(bool); !thought {
						kept = append(kept, part)
					}
				}
				content["parts"] = kept
			}
		}
	}
	return nil
}

func antigravityObject(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var result map[string]any
	if err := decoder.Decode(&result); err != nil || result == nil {
		return nil, fmt.Errorf("expected JSON object")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return nil, fmt.Errorf("unexpected trailing JSON")
	}
	return result, nil
}

func antigravityOptionalObject(object map[string]any, field string) (map[string]any, error) {
	value, present := object[field]
	if !present || value == nil {
		return nil, nil
	}
	child, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid Antigravity %s", field)
	}
	return child, nil
}

func antigravityParts(content map[string]any) ([]any, error) {
	if content["parts"] == nil {
		return nil, nil
	}
	parts, ok := content["parts"].([]any)
	if !ok {
		return nil, fmt.Errorf("invalid Antigravity content parts")
	}
	for _, part := range parts {
		if _, ok := part.(map[string]any); !ok {
			return nil, fmt.Errorf("invalid Antigravity content part")
		}
	}
	return parts, nil
}
