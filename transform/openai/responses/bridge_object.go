package responses

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/transform/shared"
)

type bridgeObject map[string]any

func bridgeDecode(raw []byte) (bridgeObject, error) {
	var obj bridgeObject
	err := shared.DecodeJSON(bytes.NewReader(raw), &obj)
	if obj != nil && err == shared.ErrTrailingJSON {
		return nil, fmt.Errorf("Responses/Chat bridge: trailing JSON data")
	}
	if err != nil || obj == nil {
		return nil, fmt.Errorf("Responses/Chat bridge: expected a JSON object")
	}
	return obj, nil
}

func bridgeMap(v any) bridgeObject {
	if obj, ok := v.(bridgeObject); ok {
		return obj
	}
	obj, _ := v.(map[string]any)
	return bridgeObject(obj)
}

func bridgeString(v any) string { s, _ := v.(string); return s }

func bridgeFields(obj bridgeObject, fields ...string) error {
	if key, ok := shared.UnsupportedField(obj, fields...); ok {
		return fmt.Errorf("Responses/Chat bridge: unsupported field %q", key)
	}
	return nil
}

func bridgeText(v any, path string) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("Responses/Chat bridge: %s must be a string", path)
	}
	return s, nil
}

func bridgeID(v any, path string) (string, error) {
	s, err := bridgeText(v, path)
	if err != nil || strings.TrimSpace(s) == "" {
		return "", fmt.Errorf("Responses/Chat bridge: %s must be nonempty", path)
	}
	return s, nil
}

func bridgeOptionalString(obj bridgeObject, key string) (string, error) {
	if obj[key] == nil {
		return "", nil
	}
	return bridgeText(obj[key], key)
}

func bridgeContent(v any, responses bool) (string, error) {
	if s, ok := v.(string); ok {
		return s, nil
	}
	parts, ok := v.([]any)
	if !ok {
		return "", fmt.Errorf("Responses/Chat bridge: content must be text or a text part array")
	}
	var out strings.Builder
	for _, raw := range parts {
		part := bridgeMap(raw)
		if err := bridgeFields(part, "type", "text", "annotations"); err != nil {
			return "", err
		}
		typ := bridgeString(part["type"])
		if (responses && typ != "input_text" && typ != "output_text") || (!responses && typ != "text") {
			return "", fmt.Errorf("Responses/Chat bridge: unsupported content type %q", typ)
		}
		if a, ok := part["annotations"].([]any); part["annotations"] != nil && (!ok || len(a) != 0) {
			return "", fmt.Errorf("Responses/Chat bridge: annotations cannot be represented")
		}
		s, err := bridgeText(part["text"], "content.text")
		if err != nil {
			return "", err
		}
		out.WriteString(s)
	}
	return out.String(), nil
}

func bridgeArguments(v any) (string, error) {
	s, err := bridgeText(v, "function arguments")
	if err != nil {
		return "", err
	}
	if _, err = bridgeDecode([]byte(s)); err != nil {
		return "", fmt.Errorf("Responses/Chat bridge: function arguments must contain a complete JSON object")
	}
	return s, nil
}
