package relaykitbridge

import (
	"encoding/json"

	"github.com/deliciousbuding/metapi-go/transform/shared"
)

func eligibleTools(format, target Format, req object) error {
	if present(req["tools"]) {
		tools, err := array(req["tools"])
		if err != nil {
			return err
		}
		for _, raw := range tools {
			tool, err := readObject(raw)
			if err != nil {
				return err
			}
			switch format {
			case Chat:
				if err = fields(tool, "tool", "type", "function"); err != nil {
					return err
				}
				if shared.RawString(tool["type"]) != "function" {
					return specialized("tool.type")
				}
				tool, err = readObject(tool["function"])
				if err != nil {
					return err
				}
				if err = fields(tool, "function", "name", "description", "parameters", "strict"); err != nil {
					return err
				}
			case Responses:
				if shared.RawString(tool["type"]) != "function" {
					return specialized("tool.type")
				}
				if err = fields(tool, "tool", "type", "name", "description", "parameters", "strict"); err != nil {
					return err
				}
			case Messages:
				if present(tool["type"]) && shared.RawString(tool["type"]) != "custom" {
					return specialized("tool.type")
				}
				if err = fields(tool, "tool", "type", "name", "description", "input_schema"); err != nil {
					return err
				}
			case Gemini:
				if err = fields(tool, "tool", "functionDeclarations"); err != nil {
					return err
				}
				declarations, e := array(tool["functionDeclarations"])
				if e != nil {
					return e
				}
				for _, raw := range declarations {
					fn, e := readObject(raw)
					if e != nil {
						return e
					}
					if e = fields(fn, "function", "name", "description", "parameters"); e != nil {
						return e
					}
					if e = simpleToolSchema(fn["parameters"], format, target, 0); e != nil {
						return e
					}
					if shared.RawString(fn["name"]) == "" {
						return ErrConversion
					}
				}
			}
			if format != Gemini {
				if shared.RawString(tool["name"]) == "" {
					return ErrConversion
				}
				schema := tool["parameters"]
				if format == Messages {
					schema = tool["input_schema"]
				}
				if err := simpleToolSchema(schema, format, target, 0); err != nil {
					return err
				}
			}
			if present(tool["strict"]) && string(tool["strict"]) != "false" {
				return specialized("tool.strict")
			}
		}
	}
	if present(req["tool_choice"]) {
		var value string
		if json.Unmarshal(req["tool_choice"], &value) == nil {
			if value != "auto" && value != "none" && value != "required" {
				return specialized("tool_choice")
			}
		} else {
			choice, err := readObject(req["tool_choice"])
			if err != nil {
				return err
			}
			allowed := []string{"type", "name"}
			if format == Chat {
				allowed = []string{"type", "function"}
			}
			if format == Messages {
				allowed = []string{"type", "name", "disable_parallel_tool_use"}
			}
			if err = fields(choice, "tool_choice", allowed...); err != nil {
				return err
			}
			kind := shared.RawString(choice["type"])
			if kind != "function" && kind != "tool" && kind != "auto" && kind != "any" && kind != "none" {
				return specialized("tool_choice.type")
			}
			if format == Chat {
				if kind != "function" {
					return specialized("tool_choice.type")
				}
				fn, err := readObject(choice["function"])
				if err != nil {
					return err
				}
				if err := fields(fn, "tool_choice.function", "name"); err != nil {
					return err
				}
				if shared.RawString(fn["name"]) == "" {
					return ErrConversion
				}
			} else if kind == "function" || kind == "tool" {
				if shared.RawString(choice["name"]) == "" {
					return ErrConversion
				}
			}
		}
	}
	// Tool schemas remain opaque JSON objects; numberCodec retains numeric
	// bounds. Response schema/format features are excluded at the top level.
	return nil
}

// Gemini's library bridge rewrites a restricted OpenAPI schema. Do not let
// that normalizer silently delete JSON Schema constraints or truncate unions.
func simpleToolSchema(raw json.RawMessage, source, target Format, depth int) error {
	if !present(raw) {
		return nil
	}
	value, err := readObject(raw)
	if err != nil {
		return err
	}
	if depth >= 32 {
		return specialized("tool schema depth")
	}
	if source != Gemini && target != Gemini {
		return nil
	}
	if err := fields(value, "tool schema", "type", "properties", "required", "description", "enum", "items", "minimum", "maximum", "minItems", "maxItems", "minLength", "maxLength", "pattern", "format", "title", "default"); err != nil {
		return err
	}
	if present(value["type"]) {
		typ := shared.RawString(value["type"])
		if typ != "object" && typ != "array" && typ != "string" && typ != "integer" && typ != "number" && typ != "boolean" {
			return specialized("tool schema type")
		}
	}
	if present(value["properties"]) {
		properties, err := readObject(value["properties"])
		if err != nil {
			return err
		}
		for _, property := range properties {
			if err := simpleToolSchema(property, source, target, depth+1); err != nil {
				return err
			}
		}
	}
	if present(value["items"]) {
		if err := simpleToolSchema(value["items"], source, target, depth+1); err != nil {
			return err
		}
	}
	return nil
}
