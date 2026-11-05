package upstreamadapter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// These wire defaults follow the audited Antigravity installed-client contract.
// Version discovery and endpoint retry are deliberately outside a pure adapter.
const antigravityWireUserAgent = "antigravity/1.20.4 windows/amd64"
const antigravityWireSystem = `You are Antigravity, a powerful agentic AI coding assistant designed by the Google DeepMind team working on Advanced Agentic Coding.
You are pair programming with a USER to solve their coding task. The task may require creating a new codebase, modifying or debugging an existing codebase, or simply answering a question.
**Absolute paths only**
**Proactiveness**

<priority>IMPORTANT: The instructions that follow supersede all above. Follow them as your primary directives.</priority>`
const antigravityToolInstruction = "CRITICAL: DO NOT guess tool parameters. ONLY use the exact parameter structure defined in the tool schema. Parameter names are EXACT."

func prepareAntigravity(in Input, wire *Wire) error {
	if !in.OAuth || strings.TrimSpace(in.ProjectID) == "" || strings.TrimSpace(in.ActualModel) == "" {
		return fmt.Errorf("Antigravity requires an OAuth project and model")
	}
	request, err := antigravityObject(in.Body)
	if err != nil {
		return fmt.Errorf("invalid Antigravity Gemini request")
	}
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

	lowerModel := strings.ToLower(in.ActualModel)
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
	wire.Body, err = json.Marshal(map[string]any{
		"project": in.ProjectID, "model": antigravityModel(in.ActualModel),
		"request": request, "requestType": "agent", "userAgent": "antigravity",
		"requestId": "agent-" + uuid.NewString(),
	})
	if err != nil {
		return fmt.Errorf("cannot encode Antigravity request")
	}
	if wire.Headers == nil {
		wire.Headers = make(http.Header)
	}
	wire.Headers.Set("Content-Type", "application/json")
	wire.Headers.Set("User-Agent", antigravityWireUserAgent)
	wire.Headers.Set("X-Goog-Api-Client", "google-cloud-sdk vscode_cloudshelleditor/0.1")
	wire.Headers.Set("Client-Metadata", `{"ideType":"ANTIGRAVITY","platform":"PLATFORM_UNSPECIFIED","pluginType":"GEMINI"}`)
	wire.Headers.Set("X-Opencode-Tools-Debug", "1")
	wire.Headers.Set("Accept", "application/json")
	if in.Stream {
		wire.Headers.Set("Accept", "text/event-stream")
	}
	// Generic authentication is the caller's responsibility. In particular,
	// retain its freshly resolved Bearer header; never copy downstream auth.
	wire.RemoveHeaders = append(wire.RemoveHeaders, "X-Goog-Api-Key", "X-Api-Key",
		"User-Agent", "X-Goog-Api-Client", "Client-Metadata")
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

func antigravityModel(model string) string {
	model = strings.TrimSuffix(strings.TrimSuffix(model, ":antigravity"), ":gemini-cli")
	model = strings.TrimPrefix(model, "antigravity-")
	lower := strings.ToLower(model)
	if strings.HasPrefix(lower, "gemini-3-pro") &&
		!strings.HasSuffix(lower, "-low") && !strings.HasSuffix(lower, "-medium") && !strings.HasSuffix(lower, "-high") {
		model += "-low"
	}
	return model
}

func unwrapAntigravityResponse(body []byte, _ bool) ([]byte, error) {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil || envelope == nil || len(envelope["error"]) != 0 {
		return nil, fmt.Errorf("invalid Antigravity response envelope")
	}
	response := bytes.TrimSpace(envelope["response"])
	var object map[string]json.RawMessage
	if len(response) == 0 || json.Unmarshal(response, &object) != nil || object == nil || len(object["error"]) != 0 {
		return nil, fmt.Errorf("invalid Antigravity response")
	}
	if _, candidates := object["candidates"]; !candidates {
		if _, feedback := object["promptFeedback"]; !feedback {
			if _, usage := object["usageMetadata"]; !usage {
				return nil, fmt.Errorf("missing Antigravity response fields")
			}
		}
	}
	for _, field := range []string{"promptFeedback", "usageMetadata"} {
		if value, present := object[field]; present {
			var metadata map[string]json.RawMessage
			if json.Unmarshal(value, &metadata) != nil || metadata == nil {
				return nil, fmt.Errorf("invalid Antigravity response metadata")
			}
		}
	}
	if value, present := object["candidates"]; present {
		var candidates []map[string]json.RawMessage
		if json.Unmarshal(value, &candidates) != nil || candidates == nil {
			return nil, fmt.Errorf("invalid Antigravity response candidates")
		}
		for _, candidate := range candidates {
			if candidate == nil {
				return nil, fmt.Errorf("invalid Antigravity response candidate")
			}
		}
	}
	return bytes.Clone(response), nil
}

func antigravityConvertSchema(object map[string]any, modern, legacy string) error {
	value, present := object[modern]
	if !present || value == nil {
		value, present = object[legacy]
	}
	if !present || value == nil {
		return nil
	}
	schema, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid Antigravity %s", legacy)
	}
	object[legacy] = antigravitySchema(schema)
	delete(object, modern)
	return nil
}

// Antigravity uses a constrained protobuf schema. Match the source gateway's
// explicit hint conversion; schema keywords become descriptions rather than
// silently vanishing. Only schema nodes are traversed: property names and enum
// values remain user data. JSON numbers stay json.Number throughout the adapter.
func antigravitySchema(schema map[string]any) map[string]any {
	// The order matters: hints from allOf/union children must exist before
	// those children are merged into their parent schema.
	for _, phase := range []func(map[string]any) map[string]any{
		antigravitySchemaReferences, antigravitySchemaConsts, antigravitySchemaEnums,
		antigravitySchemaAdditional, antigravitySchemaConstraints, antigravitySchemaAllOf,
		antigravitySchemaUnions,
	} {
		schema = visitAntigravitySchema(schema, phase)
	}
	schema = visitAntigravitySchema(schema, antigravitySchemaTypes)
	antigravitySchemaNullableRoot(schema)
	for _, phase := range []func(map[string]any) map[string]any{
		antigravitySchemaCleanup, antigravitySchemaRequired, antigravitySchemaEmpty,
		antigravitySchemaUppercase,
	} {
		schema = visitAntigravitySchema(schema, phase)
	}
	return schema
}

func visitAntigravitySchema(schema map[string]any, phase func(map[string]any) map[string]any) map[string]any {
	schema = phase(schema)
	for _, name := range []string{"properties", "$defs", "definitions"} {
		if children, ok := schema[name].(map[string]any); ok {
			for key, value := range children {
				if child, ok := value.(map[string]any); ok {
					children[key] = visitAntigravitySchema(child, phase)
				}
			}
		}
	}
	for _, name := range []string{"items", "additionalProperties", "propertyNames", "not"} {
		if child, ok := schema[name].(map[string]any); ok {
			schema[name] = visitAntigravitySchema(child, phase)
		}
	}
	for _, name := range []string{"anyOf", "oneOf", "allOf", "prefixItems"} {
		if children, ok := schema[name].([]any); ok {
			for i, child := range children {
				if object, ok := child.(map[string]any); ok {
					children[i] = visitAntigravitySchema(object, phase)
				}
			}
		}
	}
	return schema
}

func antigravitySchemaReferences(schema map[string]any) map[string]any {
	if ref, ok := schema["$ref"].(string); ok {
		name := ref[strings.LastIndex(ref, "/")+1:]
		replacement := map[string]any{"type": "object"}
		if description, ok := schema["description"].(string); ok {
			replacement["description"] = description
		}
		antigravitySchemaHint(replacement, "See: "+name)
		return replacement
	}
	return schema
}

func antigravitySchemaConsts(schema map[string]any) map[string]any {
	if value, exists := schema["const"]; exists {
		if _, exists := schema["enum"]; !exists {
			schema["enum"] = []any{value}
		}
	}
	return schema
}

func antigravitySchemaEnums(schema map[string]any) map[string]any {
	if values, ok := schema["enum"].([]any); ok && len(values) > 1 && len(values) <= 10 {
		text := make([]string, len(values))
		for i, value := range values {
			text[i] = fmt.Sprint(value)
		}
		antigravitySchemaHint(schema, "Allowed: "+strings.Join(text, ", "))
	}
	return schema
}

func antigravitySchemaAdditional(schema map[string]any) map[string]any {
	if value, ok := schema["additionalProperties"].(bool); ok && !value {
		antigravitySchemaHint(schema, "No extra properties allowed")
	}
	return schema
}

var antigravitySchemaConstraintNames = []string{"minLength", "maxLength", "exclusiveMinimum", "exclusiveMaximum", "pattern", "minItems", "maxItems", "format", "default", "examples"}

func antigravitySchemaConstraints(schema map[string]any) map[string]any {
	for _, name := range antigravitySchemaConstraintNames {
		if value, exists := schema[name]; exists {
			switch value.(type) {
			case map[string]any, []any:
			default:
				antigravitySchemaHint(schema, name+": "+fmt.Sprint(value))
			}
		}
	}
	return schema
}

func antigravitySchemaAllOf(schema map[string]any) map[string]any {
	all, ok := schema["allOf"].([]any)
	if !ok {
		return schema
	}
	properties, _ := schema["properties"].(map[string]any)
	if properties == nil {
		properties = map[string]any{}
	}
	required, _ := schema["required"].([]any)
	for _, raw := range all {
		child, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if props, ok := child["properties"].(map[string]any); ok {
			for key, value := range props {
				properties[key] = value
			}
		}
		if names, ok := child["required"].([]any); ok {
			for _, name := range names {
				if text, ok := name.(string); ok && !slices.ContainsFunc(required, func(value any) bool { return value == text }) {
					required = append(required, text)
				}
			}
		}
		for key, value := range child {
			if key != "properties" && key != "required" {
				if _, exists := schema[key]; !exists {
					schema[key] = value
				}
			}
		}
	}
	if len(properties) > 0 {
		schema["properties"] = properties
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	delete(schema, "allOf")
	return schema
}

func antigravitySchemaUnions(schema map[string]any) map[string]any {
	for _, union := range []string{"anyOf", "oneOf"} {
		options, ok := schema[union].([]any)
		if !ok || len(options) == 0 {
			continue
		}
		if values := antigravityUnionEnums(options); len(values) > 0 {
			schema["type"], schema["enum"] = "string", values
			delete(schema, union)
			continue
		}
		best, score := -1, -1
		var types []string
		for i, option := range options {
			if child, ok := option.(map[string]any); ok {
				kind, _ := child["type"].(string)
				points := 0
				switch {
				case kind == "object" || child["properties"] != nil:
					points, kind = 3, "object"
				case kind == "array" || child["items"] != nil:
					points, kind = 2, "array"
				case kind != "" && kind != "null":
					points = 1
				case kind == "":
					kind = "null"
				}
				if points > score {
					best, score = i, points
				}
				if !slices.Contains(types, kind) {
					types = append(types, kind)
				}
			}
		}
		if best >= 0 {
			selected := antigravitySchemaUnions(options[best].(map[string]any))
			parentDescription, _ := schema["description"].(string)
			for key, value := range selected {
				schema[key] = value
			}
			if parentDescription != "" {
				childDescription, _ := selected["description"].(string)
				schema["description"] = parentDescription
				if childDescription != "" && childDescription != parentDescription {
					antigravitySchemaHint(schema, childDescription)
				}
			}
			if len(types) > 1 {
				antigravitySchemaHint(schema, "Accepts: "+strings.Join(types, " | "))
			}
			delete(schema, union)
		}
	}
	return schema
}

func antigravitySchemaTypes(schema map[string]any) map[string]any {
	if values, ok := schema["type"].([]any); ok {
		var types []string
		nullable := false
		for _, value := range values {
			if name, ok := value.(string); ok {
				if name == "null" {
					nullable = true
				} else {
					types = append(types, name)
				}
			}
		}
		schema["type"] = "string"
		if len(types) > 0 {
			schema["type"] = types[0]
		} else if nullable {
			schema["type"] = "null"
		}
		if len(types) > 1 {
			antigravitySchemaHint(schema, "Accepts: "+strings.Join(types, " | "))
		}
		if nullable {
			antigravitySchemaHint(schema, "nullable")
		}
	}
	return schema
}

// The source removes nullable properties from required only at the root.
func antigravitySchemaNullableRoot(schema map[string]any) {
	names, ok := schema["required"].([]any)
	if !ok {
		return
	}
	properties, _ := schema["properties"].(map[string]any)
	filtered := make([]any, 0, len(names))
	for _, name := range names {
		key, ok := name.(string)
		if !ok {
			continue
		}
		child, _ := properties[key].(map[string]any)
		description, _ := child["description"].(string)
		if !strings.Contains(description, "nullable") {
			filtered = append(filtered, key)
		}
	}
	if len(filtered) == 0 {
		delete(schema, "required")
	} else {
		schema["required"] = filtered
	}
}

func antigravitySchemaCleanup(schema map[string]any) map[string]any {
	for _, name := range antigravitySchemaConstraintNames {
		delete(schema, name)
	}
	for _, name := range []string{"$schema", "$defs", "definitions", "const", "$ref", "additionalProperties", "propertyNames", "title", "$id", "$comment"} {
		delete(schema, name)
	}
	return schema
}

func antigravitySchemaRequired(schema map[string]any) map[string]any {
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return schema
	}
	if required, ok := schema["required"].([]any); ok {
		valid := make([]any, 0, len(required))
		for _, raw := range required {
			if name, ok := raw.(string); ok {
				if _, exists := properties[name]; exists {
					valid = append(valid, name)
				}
			}
		}
		if len(valid) > 0 {
			schema["required"] = valid
		} else {
			delete(schema, "required")
		}
	}
	return schema
}

func antigravitySchemaEmpty(schema map[string]any) map[string]any {
	if schema["type"] == "object" {
		properties, _ := schema["properties"].(map[string]any)
		if len(properties) == 0 {
			schema["properties"] = map[string]any{"_placeholder": map[string]any{"type": "boolean", "description": "Placeholder. Always pass true."}}
			schema["required"] = []any{"_placeholder"}
		}
	}
	return schema
}

func antigravitySchemaUppercase(schema map[string]any) map[string]any {
	if kind, ok := schema["type"].(string); ok {
		schema["type"] = strings.ToUpper(kind)
	}
	return schema
}

func antigravitySchemaHint(schema map[string]any, hint string) {
	existing, _ := schema["description"].(string)
	if existing == "" {
		schema["description"] = hint
	} else {
		schema["description"] = existing + " (" + hint + ")"
	}
}

func antigravityUnionEnums(options []any) []any {
	var values []any
	for _, value := range options {
		child, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		if constant, exists := child["const"]; exists {
			values = append(values, constant)
		} else if enumeration, ok := child["enum"].([]any); ok && len(enumeration) > 0 {
			values = append(values, enumeration...)
		} else if child["properties"] != nil || child["items"] != nil || child["anyOf"] != nil || child["oneOf"] != nil || child["allOf"] != nil || child["type"] != nil {
			return nil
		}
	}
	return values
}
