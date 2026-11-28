package messages

import "fmt"

func fromChatTools(req rawObject, out wireObject) error {
	names := make(map[string]bool)
	if !absent(req["tools"]) {
		tools, err := array(req["tools"], "Chat tools")
		if err != nil {
			return err
		}
		converted := make([]wireObject, 0, len(tools))
		for i, raw := range tools {
			path := fmt.Sprintf("Chat tools[%d]", i)
			tool, err := object(raw, path)
			if err != nil {
				return err
			}
			if err := allowOnly(tool, path, "type", "function", "cache_control"); err != nil {
				return err
			}
			if string(tool["type"]) != `"function"` {
				return unsupported(path + ".type")
			}
			function, err := object(tool["function"], path+".function")
			if err != nil {
				return err
			}
			if err := allowOnly(function, path+".function", "name", "description", "parameters", "strict"); err != nil {
				return err
			}
			name, err := identity(function["name"], path+".function.name")
			if err != nil || names[name] {
				return invalid(path+".function.name", "must be nonempty and unique")
			}
			names[name] = true
			entry := wireObject{"name": name, "input_schema": wireObject{"type": "object"}}
			if !absent(function["parameters"]) {
				if _, err := object(function["parameters"], path+".function.parameters"); err != nil {
					return err
				}
				entry["input_schema"] = function["parameters"]
			}
			if !absent(function["description"]) {
				value, err := text(function["description"], path+".function.description")
				if err != nil {
					return err
				}
				entry["description"] = value
			}
			if !absent(function["strict"]) {
				strict, err := boolean(function["strict"], path+".function.strict")
				if err != nil || strict {
					return invalid(path+".function.strict", "cannot guarantee strict schema enforcement")
				}
			}
			if err := copyCacheControl(tool, entry, path); err != nil {
				return err
			}
			converted = append(converted, entry)
		}
		out["tools"] = converted
	}
	choice := wireObject{"type": "auto"}
	if !absent(req["tool_choice"]) {
		if value, err := text(req["tool_choice"], "Chat tool_choice"); err == nil {
			switch value {
			case "auto", "none":
				choice["type"] = value
			case "required":
				choice["type"] = "any"
			default:
				return unsupported("Chat tool_choice")
			}
		} else {
			value, err := object(req["tool_choice"], "Chat tool_choice")
			if err != nil {
				return err
			}
			if err := allowOnly(value, "Chat tool_choice", "type", "function"); err != nil {
				return err
			}
			function, err := object(value["function"], "Chat tool_choice.function")
			if err != nil {
				return err
			}
			if err := allowOnly(function, "Chat tool_choice.function", "name"); err != nil {
				return err
			}
			name, err := identity(function["name"], "Chat tool_choice.function.name")
			if err != nil || string(value["type"]) != `"function"` || !names[name] {
				return invalid("Chat tool_choice", "must name a declared function tool")
			}
			choice = wireObject{"type": "tool", "name": name}
		}
	}
	if !absent(req["parallel_tool_calls"]) {
		parallel, err := boolean(req["parallel_tool_calls"], "Chat parallel_tool_calls")
		if err != nil {
			return err
		}
		if choice["type"] != "none" {
			choice["disable_parallel_tool_use"] = !parallel
		}
	}
	if len(names) != 0 {
		out["tool_choice"] = choice
	} else if choice["type"] == "any" || choice["type"] == "tool" {
		return invalid("Chat tool_choice", "requires tools")
	}
	return nil
}
