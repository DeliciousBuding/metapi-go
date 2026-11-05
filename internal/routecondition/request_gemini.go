package routecondition

import (
	"bytes"
	"encoding/json"
	"math"
	"strconv"
	"strings"
)

type geminiPart struct {
	Text             string
	Thought          bool
	ThoughtSignature string
	InlineData       *struct{ MIMEType, Data string }
	FileData         *struct{ MIMEType, FileURI string }
	FunctionCall     *struct {
		ID, Name string
		Args     json.RawMessage
	}
	FunctionResponse *struct {
		ID, Name string
		Response json.RawMessage
	}
}

func (part *geminiPart) UnmarshalJSON(raw []byte) error {
	type fields geminiPart
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var value fields
	if decoder.Decode(&value) != nil {
		return errRequest
	}
	*part = geminiPart(value)
	return nil
}

type geminiContent struct {
	Role  string
	Parts []geminiPart
}

func describeGemini(object map[string]json.RawMessage, result *Request) error {
	var request struct {
		SystemInstruction *geminiContent
		Contents          []*geminiContent
		Tools             []struct {
			FunctionDeclarations []struct {
				Name, Description                string
				Parameters, ParametersJSONSchema json.RawMessage
			}
			GoogleSearch, CodeExecution, URLContext json.RawMessage
		}
	}
	body, _ := json.Marshal(object)
	if json.Unmarshal(body, &request) != nil || len(request.Contents) == 0 {
		return errRequest
	}
	if request.SystemInstruction != nil {
		var parts []string
		for _, part := range request.SystemInstruction.Parts {
			if part.Text != "" && !part.Thought {
				parts = append(parts, part.Text)
			}
		}
		if text := strings.Join(parts, "\n"); text != "" {
			result.PromptTokens += sourceTokens("system") + sourceTokens(text)
		}
	}
	previousCalls := map[string]string{}
	for _, content := range request.Contents {
		if content == nil || len(content.Parts) == 0 {
			continue
		}
		role := content.Role
		if role == "model" {
			role = "assistant"
		} else if role == "" {
			role = "user"
		}
		var contentTokens, toolTokens int64
		partCount := 0
		onlyText, reasoning := "", ""
		features := &Request{}
		var response *struct {
			ID, Name string
			Response json.RawMessage
		}
		for _, part := range content.Parts {
			switch {
			case part.Text != "":
				if part.Thought {
					reasoning = part.Text // Source keeps the last thought text.
				} else {
					partCount++
					onlyText = part.Text
					contentTokens += sourceTokens("text") + sourceTokens(part.Text)
				}
			case part.InlineData != nil:
				partCount++
				onlyText = ""
				url := "data:" + part.InlineData.MIMEType + ";base64," + part.InlineData.Data
				contentTokens += geminiMediaTokens(part.InlineData.MIMEType, url, features)
			case part.FileData != nil:
				partCount++
				onlyText = ""
				contentTokens += geminiMediaTokens(part.FileData.MIMEType, part.FileData.FileURI, features)
			case part.FunctionCall != nil:
				args := "{}"
				var err error
				if len(part.FunctionCall.Args) > 0 && !bytes.Equal(part.FunctionCall.Args, []byte("null")) {
					args, err = compactJSON(part.FunctionCall.Args, true)
				}
				if err != nil {
					return err
				}
				toolTokens += sourceTokens("function") + sourceTokens(part.FunctionCall.Name) + sourceTokens(args)
			case part.FunctionResponse != nil:
				response = part.FunctionResponse
			default:
				// Empty parts/signature-only parts produce an empty source message.
			}
			if response != nil {
				break
			}
		}
		if response != nil {
			text, err := compactJSON(response.Response, true)
			if err != nil {
				return err
			}
			id := response.ID
			if id == "" {
				id = previousCalls[response.Name]
			}
			result.PromptTokens += sourceTokens("tool") + sourceTokens(id) + sourceTokens(response.Name) + sourceTokens(text)
		} else {
			if partCount == 1 && onlyText != "" {
				contentTokens = sourceTokens(onlyText)
			}
			result.PromptTokens += sourceTokens(role) + contentTokens + toolTokens + sourceTokens(reasoning)
			result.HasImage = result.HasImage || features.HasImage
			result.HasVideo = result.HasVideo || features.HasVideo
			result.HasDocument = result.HasDocument || features.HasDocument
			result.HasAudio = result.HasAudio || features.HasAudio
		}
		for _, part := range content.Parts {
			if part.FunctionCall != nil && part.FunctionCall.ID != "" {
				previousCalls[part.FunctionCall.Name] = part.FunctionCall.ID
			}
		}
	}
	for _, tool := range request.Tools {
		for _, function := range tool.FunctionDeclarations {
			var schema string
			if len(function.Parameters) > 0 {
				var err error
				schema, err = geminiSchema(function.Parameters)
				if err != nil {
					return err
				}
			}
			// ParametersJSONSchema is kept in a different source field, which
			// its conditional estimator does not count.
			result.PromptTokens += sourceTokens("function") + sourceTokens(function.Name) + sourceTokens(function.Description) + sourceTokens(schema)
		}
		for _, builtin := range []struct {
			raw  json.RawMessage
			name string
		}{
			{tool.GoogleSearch, "google_search"}, {tool.CodeExecution, "google_code_execution"}, {tool.URLContext, "google_url_context"},
		} {
			if len(builtin.raw) > 0 && !bytes.Equal(builtin.raw, []byte("null")) {
				result.PromptTokens += sourceTokens(builtin.name)
			}
		}
	}
	return nil
}

func geminiMediaTokens(mime, url string, result *Request) int64 {
	lower := strings.ToLower(mime)
	switch {
	case strings.HasPrefix(lower, "application/pdf"), strings.HasPrefix(lower, "application/msword"),
		strings.HasPrefix(lower, "application/vnd.openxmlformats-officedocument"),
		strings.HasPrefix(lower, "application/vnd.ms-"), strings.HasPrefix(lower, "text/"):
		result.HasDocument = true
		return sourceTokens("document") + min(sourceTokens(url), 512) + sourceTokens(mime)
	case strings.HasPrefix(lower, "video/"):
		result.HasVideo = true
		return sourceTokens("video_url") + 128
	case strings.HasPrefix(lower, "audio/"):
		result.HasAudio = true
		format := "mp3"
		switch lower {
		case "audio/wav", "audio/wave", "audio/x-wav":
			format = "wav"
		case "audio/webm":
			format = "webm"
		case "audio/ogg":
			format = "ogg"
		case "audio/flac":
			format = "flac"
		case "audio/aac":
			format = "aac"
		case "audio/mp4", "audio/m4a":
			format = "mp4"
		}
		return sourceTokens("input_audio") + 128 + sourceTokens(format)
	default:
		result.HasImage = true
		return sourceTokens("image_url") + 128
	}
}

func geminiSchema(raw json.RawMessage) (string, error) {
	if !geminiSchemaValid(raw) {
		return string(raw), nil
	}
	return normalizeGeminiSchema(raw, false)
}

// Validate only the schema decoding contract used before the source heuristic.
// When its typed schema decoder rejects a document, the source counts the
// original JSON instead of a partially normalized tree.
func geminiSchemaValid(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if bytes.Equal(raw, []byte("true")) || bytes.Equal(raw, []byte("false")) || bytes.Equal(raw, []byte("null")) {
		return true
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	if isNonNull(fields["$defs"]) && isNonNull(fields["definitions"]) {
		return false
	}
	for key, value := range fields {
		if bytes.Equal(value, []byte("null")) {
			if key == "type" {
				return false
			}
			continue
		}
		switch key {
		case "type":
			var one string
			var many []string
			if json.Unmarshal(value, &one) != nil && json.Unmarshal(value, &many) != nil {
				return false
			}
		case "$id", "$schema", "$ref", "$comment", "$anchor", "$dynamicAnchor", "$dynamicRef",
			"title", "description", "pattern", "format", "contentEncoding", "contentMediaType":
			var text string
			if json.Unmarshal(value, &text) != nil {
				return false
			}
		case "deprecated", "readOnly", "writeOnly", "uniqueItems":
			var flag bool
			if json.Unmarshal(value, &flag) != nil {
				return false
			}
		case "minLength", "maxLength", "minItems", "maxItems", "minContains", "maxContains", "minProperties", "maxProperties":
			var number float64
			if json.Unmarshal(value, &number) != nil || number != math.Trunc(number) || number < math.MinInt32 || number > math.MaxInt32 {
				return false
			}
			if !bytes.ContainsRune(value, '.') {
				if _, err := strconv.ParseInt(string(value), 10, 64); err != nil {
					return false
				}
			}
		case "multipleOf", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum":
			var number float64
			if json.Unmarshal(value, &number) != nil {
				return false
			}
		case "enum", "examples":
			var array []any
			if json.Unmarshal(value, &array) != nil {
				return false
			}
		case "required":
			var array []string
			if json.Unmarshal(value, &array) != nil {
				return false
			}
		case "dependentRequired":
			var entries map[string][]string
			if json.Unmarshal(value, &entries) != nil {
				return false
			}
		case "$vocabulary":
			var entries map[string]bool
			if json.Unmarshal(value, &entries) != nil {
				return false
			}
		case "$defs", "definitions", "properties", "patternProperties", "dependentSchemas", "dependencies":
			var children map[string]json.RawMessage
			if json.Unmarshal(value, &children) != nil {
				return false
			}
			for _, child := range children {
				if key == "dependencies" && len(child) > 0 && child[0] == '[' {
					var names []string
					if json.Unmarshal(child, &names) != nil {
						return false
					}
				} else if !geminiSchemaValid(child) {
					return false
				}
			}
		case "prefixItems", "allOf", "anyOf", "oneOf":
			var children []json.RawMessage
			if json.Unmarshal(value, &children) != nil {
				return false
			}
			for _, child := range children {
				if !geminiSchemaValid(child) {
					return false
				}
			}
		case "items":
			if len(value) > 0 && value[0] == '[' {
				var children []json.RawMessage
				if json.Unmarshal(value, &children) != nil {
					return false
				}
				for _, child := range children {
					if !geminiSchemaValid(child) {
						return false
					}
				}
			} else if !geminiSchemaValid(value) {
				return false
			}
		case "additionalItems", "contains", "not", "if", "then", "else", "propertyNames", "unevaluatedProperties", "unevaluatedItems", "contentSchema", "additionalProperties":
			if !geminiSchemaValid(value) {
				return false
			}
		}
	}
	return true
}

func normalizeGeminiSchema(raw json.RawMessage, pointer bool) (string, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		if pointer {
			return "null", nil
		}
		// json.Unmarshal(null, *bool) succeeds with false in the source's
		// Schema.UnmarshalJSON first branch.
		return "false", nil
	}
	var schema map[string]json.RawMessage
	if json.Unmarshal(raw, &schema) != nil || schema == nil {
		// The source preserves an unparseable schema instead of normalizing it.
		return string(raw), nil
	}
	if value, ok := schema["type"]; ok {
		var kind string
		if len(value) > 0 && value[0] == '"' && json.Unmarshal(value, &kind) == nil {
			if kind == "" {
				delete(schema, "type")
			} else {
				schema["type"], _ = json.Marshal(strings.ToLower(kind))
			}
		} else if len(value) == 0 || value[0] != '[' {
			return string(raw), nil
		}
	}
	for _, key := range []string{"properties", "patternProperties", "$defs", "definitions", "dependentSchemas"} {
		var children map[string]json.RawMessage
		if value, ok := schema[key]; ok {
			if json.Unmarshal(value, &children) != nil {
				return string(raw), nil
			}
			for name, child := range children {
				normalized, err := normalizeGeminiSchema(child, true)
				if err != nil {
					return "", err
				}
				children[name] = json.RawMessage(normalized)
			}
			schema[key], _ = json.Marshal(children)
		}
	}
	if value, ok := schema["dependencies"]; ok {
		var children map[string]json.RawMessage
		if json.Unmarshal(value, &children) != nil {
			return string(raw), nil
		}
		for name, child := range children {
			if len(child) != 0 && child[0] != '[' {
				normalized, err := normalizeGeminiSchema(child, false)
				if err != nil {
					return "", err
				}
				children[name] = json.RawMessage(normalized)
			}
		}
		schema["dependencies"], _ = json.Marshal(children)
	}
	for _, key := range []string{"items", "additionalItems", "contains", "not", "if", "then", "else", "propertyNames", "unevaluatedProperties", "unevaluatedItems", "contentSchema", "additionalProperties"} {
		if value, ok := schema[key]; ok && len(value) > 0 && value[0] == '{' {
			normalized, err := normalizeGeminiSchema(value, key != "items")
			if err != nil {
				return "", err
			}
			schema[key] = json.RawMessage(normalized)
		}
	}
	for _, key := range []string{"prefixItems", "allOf", "anyOf", "oneOf", "items"} {
		var children []json.RawMessage
		if value, ok := schema[key]; ok {
			if key == "items" && (len(value) == 0 || value[0] != '[') {
				continue
			}
			if json.Unmarshal(value, &children) != nil {
				return string(raw), nil
			}
			for i, child := range children {
				normalized, err := normalizeGeminiSchema(child, true)
				if err != nil {
					return "", err
				}
				children[i] = json.RawMessage(normalized)
			}
			schema[key], _ = json.Marshal(children)
		}
	}
	// The source schema representation omits these false/empty metadata values.
	for _, key := range []string{"title", "description", "$id", "$schema", "$ref", "$comment", "$anchor", "$dynamicAnchor", "$dynamicRef", "pattern", "format", "contentEncoding", "contentMediaType"} {
		if value, ok := schema[key]; ok && bytes.Equal(value, []byte(`""`)) {
			delete(schema, key)
		}
	}
	for _, key := range []string{"deprecated", "readOnly", "writeOnly", "uniqueItems"} {
		if value, ok := schema[key]; ok && bytes.Equal(value, []byte("false")) {
			delete(schema, key)
		}
	}
	// The source's typed Schema uses omitempty for every slice/map keyword.
	for _, key := range []string{"$defs", "definitions", "dependencies", "$vocabulary",
		"examples", "enum", "prefixItems", "required", "dependentRequired", "properties",
		"patternProperties", "allOf", "anyOf", "oneOf", "dependentSchemas"} {
		collectionRaw, ok := schema[key]
		if !ok {
			continue
		}
		var collection any
		if json.Unmarshal(collectionRaw, &collection) != nil {
			return string(raw), nil
		}
		switch value := collection.(type) {
		case nil:
			delete(schema, key)
		case []any:
			if len(value) == 0 {
				delete(schema, key)
			}
		case map[string]any:
			if len(value) == 0 {
				delete(schema, key)
			}
		}
	}
	for _, key := range []string{"multipleOf", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum",
		"minLength", "maxLength", "minItems", "maxItems", "minContains", "maxContains", "minProperties", "maxProperties",
		"enum", "const", "examples"} {
		if raw, ok := schema[key]; ok && !bytes.Equal(raw, []byte("null")) {
			if key == "enum" || key == "const" || key == "examples" {
				normalized, err := compactJSON(raw, true)
				if err != nil {
					return "", err
				}
				schema[key] = json.RawMessage(normalized)
				continue
			}
			var number float64
			if json.Unmarshal(raw, &number) != nil {
				return string(raw), nil
			}
			schema[key], _ = json.Marshal(number)
		}
	}
	for key, value := range schema {
		if !bytes.Equal(value, []byte("null")) || !geminiKnownSchemaField(key) || key == "default" || key == "const" {
			continue
		}
		if key == "items" {
			schema[key] = json.RawMessage("false")
		} else {
			delete(schema, key)
		}
	}
	// Unknown keywords are stored in map[string]any by the source schema
	// library, while default is RawMessage and must keep its number spelling.
	for key, value := range schema {
		if geminiKnownSchemaField(key) {
			continue
		}
		normalized, err := compactJSON(value, true)
		if err != nil {
			return "", err
		}
		schema[key] = json.RawMessage(normalized)
	}
	if len(schema) == 0 {
		return "true", nil
	}
	if len(schema) == 1 && bytes.Equal(schema["not"], []byte("true")) {
		return "false", nil
	}
	encoded, err := json.Marshal(schema)
	return string(encoded), err
}

func geminiKnownSchemaField(key string) bool {
	switch key {
	case "$id", "$schema", "$ref", "$comment", "$defs", "definitions", "dependencies",
		"$anchor", "$dynamicAnchor", "$dynamicRef", "$vocabulary", "title", "description",
		"default", "deprecated", "readOnly", "writeOnly", "examples", "type", "enum", "const",
		"multipleOf", "minimum", "maximum", "exclusiveMinimum", "exclusiveMaximum", "minLength",
		"maxLength", "pattern", "prefixItems", "items", "minItems", "maxItems", "additionalItems",
		"uniqueItems", "contains", "minContains", "maxContains", "unevaluatedItems", "minProperties",
		"maxProperties", "required", "dependentRequired", "properties", "patternProperties",
		"additionalProperties", "propertyNames", "unevaluatedProperties", "allOf", "anyOf",
		"oneOf", "not", "if", "then", "else", "dependentSchemas", "contentEncoding",
		"contentMediaType", "contentSchema", "format":
		return true
	}
	return false
}
