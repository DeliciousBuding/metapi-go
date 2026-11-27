package responses

import (
	"encoding/json"
	"fmt"
	"strings"
)

func bridgeRequestOptions(req, out bridgeObject, toChat bool) error {
	for _, key := range []string{"model", "stream", "temperature", "top_p", "parallel_tool_calls", "metadata", "user"} {
		if v, ok := req[key]; ok {
			out[key] = v
		}
	}
	if _, err := bridgeID(out["model"], "model"); err != nil {
		return err
	}
	for _, key := range []string{"stream", "parallel_tool_calls"} {
		if v := out[key]; v != nil {
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("Responses/Chat bridge: %s must be boolean", key)
			}
		}
	}
	for _, key := range []string{"temperature", "top_p"} {
		if v := out[key]; v != nil {
			if _, ok := v.(json.Number); !ok {
				return fmt.Errorf("Responses/Chat bridge: %s must be numeric", key)
			}
		}
	}
	from, to := "max_output_tokens", "max_completion_tokens"
	if !toChat {
		from, to = "max_completion_tokens", "max_output_tokens"
		if req[from] == nil {
			from = "max_tokens"
		}
	}
	if v := req[from]; v != nil {
		n, ok := v.(json.Number)
		if !ok {
			return fmt.Errorf("Responses/Chat bridge: %s must be a positive integer", from)
		}
		i, err := n.Int64()
		if err != nil || i <= 0 {
			return fmt.Errorf("Responses/Chat bridge: %s must be a positive integer", from)
		}
		out[to] = v
	}
	if !toChat && req["max_tokens"] != nil && req["max_completion_tokens"] != nil {
		return fmt.Errorf("Responses/Chat bridge: conflicting token limits")
	}
	if v := req["store"]; v != nil && v != false {
		return fmt.Errorf("Responses/Chat bridge: stored response semantics are unsupported")
	}
	out["store"] = false
	if toChat && req["stream"] == true {
		out["stream_options"] = bridgeObject{"include_usage": true}
	}
	if v := req["stream_options"]; v != nil {
		opts := bridgeMap(v)
		if opts == nil {
			return fmt.Errorf("Responses/Chat bridge: invalid stream_options")
		}
		if err := bridgeFields(opts, "include_usage"); err != nil {
			return err
		}
		if value := opts["include_usage"]; value != nil {
			if _, ok := value.(bool); !ok {
				return fmt.Errorf("Responses/Chat bridge: invalid include_usage")
			}
		}
	}
	if err := bridgeReasoningOptions(req, out, toChat); err != nil {
		return err
	}
	if err := bridgeTextOptions(req, out, toChat); err != nil {
		return err
	}
	return bridgeRequestTools(req, out, toChat)
}

func bridgeRequestTools(req, out bridgeObject, toChat bool) error {
	if raw := req["tools"]; raw != nil {
		tools, ok := raw.([]any)
		if !ok {
			return fmt.Errorf("Responses/Chat bridge: tools must be an array")
		}
		converted := make([]any, 0, len(tools))
		for _, raw := range tools {
			tool := bridgeMap(raw)
			if bridgeString(tool["type"]) != "function" {
				return fmt.Errorf("Responses/Chat bridge: only function tools are supported")
			}
			fn := tool
			if !toChat {
				if err := bridgeFields(tool, "type", "function"); err != nil {
					return err
				}
				fn = bridgeMap(tool["function"])
			}
			if err := bridgeFields(fn, "type", "name", "description", "parameters", "strict"); err != nil {
				return err
			}
			if _, err := bridgeID(fn["name"], "function.name"); err != nil {
				return err
			}
			if fn["parameters"] != nil && bridgeMap(fn["parameters"]) == nil {
				return fmt.Errorf("Responses/Chat bridge: function parameters must be an object")
			}
			if fn["description"] != nil {
				if _, err := bridgeText(fn["description"], "function.description"); err != nil {
					return err
				}
			}
			if fn["strict"] != nil {
				if _, ok := fn["strict"].(bool); !ok {
					return fmt.Errorf("Responses/Chat bridge: function strict must be boolean")
				}
			}
			copy := bridgeObject{}
			for _, key := range []string{"name", "description", "parameters", "strict"} {
				if v, ok := fn[key]; ok {
					copy[key] = v
				}
			}
			// Chat function tools default to non-strict schemas. Responses may
			// normalize an omitted strict flag, so make the source default explicit.
			if !toChat && copy["strict"] == nil {
				copy["strict"] = false
			}
			if toChat {
				converted = append(converted, bridgeObject{"type": "function", "function": copy})
			} else {
				copy["type"] = "function"
				converted = append(converted, copy)
			}
		}
		out["tools"] = converted
	}
	if choice := req["tool_choice"]; choice != nil {
		if text, ok := choice.(string); ok {
			if text != "auto" && text != "none" && text != "required" {
				return fmt.Errorf("Responses/Chat bridge: unsupported tool_choice")
			}
			out["tool_choice"] = text
		} else {
			obj := bridgeMap(choice)
			if bridgeString(obj["type"]) != "function" {
				return fmt.Errorf("Responses/Chat bridge: only function tool_choice is supported")
			}
			var name string
			var err error
			if toChat {
				err = bridgeFields(obj, "type", "name")
				name = bridgeString(obj["name"])
			} else {
				err = bridgeFields(obj, "type", "function")
				fn := bridgeMap(obj["function"])
				if err == nil {
					err = bridgeFields(fn, "name")
				}
				name = bridgeString(fn["name"])
			}
			if err != nil {
				return err
			}
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("Responses/Chat bridge: tool_choice needs a function name")
			}
			if toChat {
				out["tool_choice"] = bridgeObject{"type": "function", "function": bridgeObject{"name": name}}
			} else {
				out["tool_choice"] = bridgeObject{"type": "function", "name": name}
			}
		}
	}
	return nil
}
