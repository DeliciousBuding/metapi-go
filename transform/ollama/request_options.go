package ollama

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func applyChatOptions(in, out object) error {
	opts := object{}
	if raw, ok := in["options"]; ok {
		var err error
		opts, err = decodeObject(raw)
		if err != nil {
			return fmt.Errorf("options: %w", err)
		}
		// Native options stay byte-preserving; unlike unknown Chat fields, their
		// explicit namespace is already the target API's contract.
	}
	for _, key := range []string{"temperature", "top_p", "top_k", "seed", "frequency_penalty", "presence_penalty"} {
		if raw, ok := in[key]; ok {
			var number json.Number
			if string(raw) == "null" || json.Unmarshal(raw, &number) != nil || len(raw) == 0 || raw[0] == '"' {
				return fmt.Errorf("%s must be a number", key)
			}
			if _, exists := opts[key]; exists {
				return fmt.Errorf("%s conflicts with options", key)
			}
			opts[key] = raw
		}
	}
	for _, key := range []string{"max_tokens", "max_completion_tokens"} {
		if raw, ok := in[key]; ok {
			var count int64
			if json.Unmarshal(raw, &count) != nil || count <= 0 {
				return fmt.Errorf("%s must be a positive integer", key)
			}
			if previous, ok := opts["num_predict"]; ok && !bytes.Equal(previous, raw) {
				return fmt.Errorf("conflicting output token limits")
			}
			opts["num_predict"] = raw
		}
	}
	if raw, ok := in["stop"]; ok && string(raw) != "null" {
		var stops []string
		if err := json.Unmarshal(raw, &stops); err != nil {
			value, err := stringValue(raw)
			if err != nil {
				return fmt.Errorf("invalid stop")
			}
			stops = []string{value}
		}
		if _, ok := opts["stop"]; ok {
			return fmt.Errorf("stop conflicts with options")
		}
		opts["stop"] = rawValue(stops)
	}
	if len(opts) > 0 {
		out["options"] = rawValue(opts)
	}
	if raw, ok := in["response_format"]; ok {
		format, err := decodeObject(raw)
		if err != nil {
			return err
		}
		if err = known(format, "type", "json_schema"); err != nil {
			return err
		}
		switch string(format["type"]) {
		case `"text"`:
			if len(format) != 1 {
				return fmt.Errorf("text response_format cannot include a schema")
			}
		case `"json_object"`:
			if len(format) != 1 {
				return fmt.Errorf("json_object cannot include a schema")
			}
			out["format"] = rawValue("json")
		case `"json_schema"`:
			schema, err := decodeObject(format["json_schema"])
			if err != nil {
				return err
			}
			if err = known(schema, "name", "description", "schema", "strict"); err != nil {
				return err
			}
			if _, err := decodeObject(schema["schema"]); err != nil {
				return err
			}
			if raw, ok := schema["strict"]; ok && string(raw) != "false" {
				return fmt.Errorf("Ollama cannot promise strict JSON schema semantics")
			}
			out["format"] = schema["schema"]
		default:
			return fmt.Errorf("unsupported response_format")
		}
	}
	if raw, ok := in["think"]; ok {
		if string(raw) != "true" && string(raw) != "false" && string(raw) != `"low"` && string(raw) != `"medium"` && string(raw) != `"high"` {
			return fmt.Errorf("invalid think option")
		}
		out["think"] = raw
	}
	if raw, ok := in["reasoning_effort"]; ok {
		if _, exists := out["think"]; exists {
			return fmt.Errorf("reasoning_effort conflicts with think")
		}
		if string(raw) == `"none"` {
			out["think"] = rawValue(false)
		} else if string(raw) == `"low"` || string(raw) == `"medium"` || string(raw) == `"high"` {
			out["think"] = raw
		} else {
			return fmt.Errorf("unsupported reasoning_effort")
		}
	}
	if raw, ok := in["keep_alive"]; ok {
		out["keep_alive"] = raw
	}
	return nil
}
