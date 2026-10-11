package responses

import "fmt"

// Responses flattens JSON Schema metadata into text.format; Chat nests it in
// response_format.json_schema. The schema remains an opaque JSON object so
// integer precision, references and provider-supported keywords survive.
func bridgeTextOptions(req, out bridgeObject, toChat bool) error {
	config := bridgeObject{"format": req["response_format"], "verbosity": req["verbosity"]}
	if toChat {
		config = bridgeMap(req["text"])
		if req["text"] != nil && config == nil {
			return fmt.Errorf("Responses/Chat bridge: text must be an object")
		}
		if err := bridgeFields(config, "format", "verbosity"); err != nil {
			return err
		}
	}
	textConfig := bridgeObject{}
	if raw := config["format"]; raw != nil {
		format := bridgeMap(raw)
		if format == nil {
			return fmt.Errorf("Responses/Chat bridge: response format must be an object")
		}
		typ := bridgeString(format["type"])
		converted := bridgeObject{"type": typ}
		switch typ {
		case "text", "json_object":
			if err := bridgeFields(format, "type"); err != nil {
				return err
			}
		case "json_schema":
			definition := format
			if !toChat {
				if err := bridgeFields(format, "type", "json_schema"); err != nil {
					return err
				}
				definition = bridgeMap(format["json_schema"])
				if definition == nil {
					return fmt.Errorf("Responses/Chat bridge: json_schema must be an object")
				}
			}
			fields := []string{"name", "schema", "description", "strict"}
			if toChat {
				fields = append(fields, "type")
			}
			if err := bridgeFields(definition, fields...); err != nil {
				return err
			}
			name, err := bridgeID(definition["name"], "JSON Schema name")
			if err != nil {
				return err
			}
			if !bridgeSchemaName(name) || bridgeMap(definition["schema"]) == nil {
				return fmt.Errorf("Responses/Chat bridge: JSON Schema needs a valid name and schema object")
			}
			if definition["description"] != nil {
				if _, err := bridgeText(definition["description"], "JSON Schema description"); err != nil {
					return err
				}
			}
			if definition["strict"] != nil {
				if _, ok := definition["strict"].(bool); !ok {
					return fmt.Errorf("Responses/Chat bridge: JSON Schema strict must be boolean")
				}
			}
			schema := bridgeObject{}
			for _, key := range []string{"name", "schema", "description", "strict"} {
				if value, ok := definition[key]; ok {
					schema[key] = value
				}
			}
			if toChat {
				converted["json_schema"] = schema
			} else {
				converted = schema
				converted["type"] = typ
			}
		default:
			return fmt.Errorf("Responses/Chat bridge: unsupported response format type %q", typ)
		}
		if toChat {
			out["response_format"] = converted
		} else {
			textConfig["format"] = converted
		}
	}
	if config["verbosity"] != nil {
		verbosity, err := bridgeText(config["verbosity"], "verbosity")
		if err != nil {
			return err
		}
		if verbosity != "low" && verbosity != "medium" && verbosity != "high" {
			return fmt.Errorf("Responses/Chat bridge: unsupported verbosity")
		}
		if toChat {
			out["verbosity"] = verbosity
		} else {
			textConfig["verbosity"] = verbosity
		}
	}
	if !toChat && len(textConfig) != 0 {
		out["text"] = textConfig
	}
	return nil
}

func bridgeSchemaName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	for _, c := range name {
		if c != '_' && c != '-' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
