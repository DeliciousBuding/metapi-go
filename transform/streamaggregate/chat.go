package streamaggregate

import (
	"bytes"
	"encoding/json"
	"errors"
)

func (c *Collector) addChat(value object) error {
	copyFields(c.root, value, "choices", "object")
	if c.protocol == "chat" {
		c.root["object"] = json.RawMessage(`"chat.completion"`)
	} else {
		c.root["object"] = json.RawMessage(`"text_completion"`)
	}
	var choices []object
	if json.Unmarshal(value["choices"], &choices) != nil {
		return errors.New("invalid stream choices")
	}
	for _, delta := range choices {
		index, err := number(delta["index"])
		if err != nil {
			return err
		}
		choice := c.choices[index]
		if choice == nil {
			choice = object{}
			c.choices[index] = choice
		}
		if len(choice["finish_reason"]) > 0 {
			return errors.New("native choice continued after finish reason")
		}
		copyFields(choice, delta, "delta", "text", "finish_reason", "logprobs")
		if raw := delta["logprobs"]; len(raw) > 0 && string(raw) != "null" {
			current := object{}
			if len(choice["logprobs"]) > 0 {
				if json.Unmarshal(choice["logprobs"], &current) != nil {
					return errors.New("invalid stream logprobs")
				}
			}
			next, err := decode(raw)
			if err != nil {
				return err
			}
			for key, raw := range next {
				var tail, prior []json.RawMessage
				if json.Unmarshal(raw, &tail) == nil {
					if len(current[key]) > 0 && json.Unmarshal(current[key], &prior) != nil {
						return errors.New("invalid stream logprobs")
					}
					current[key], _ = json.Marshal(append(prior, tail...))
				} else {
					current[key] = raw
				}
			}
			choice["logprobs"], _ = json.Marshal(current)
		}
		if raw := delta["finish_reason"]; len(raw) > 0 && string(raw) != "null" {
			reason, err := text(raw)
			if err != nil || reason == "" {
				return errors.New("invalid finish reason")
			}
			choice["finish_reason"] = bytes.Clone(raw)
		}
		if c.protocol == "completions" {
			if raw, ok := delta["text"]; ok {
				if err := appendText(choice, "text", raw); err != nil {
					return err
				}
			}
			continue
		}
		message := c.messages[index]
		if message == nil {
			message = object{}
			c.messages[index] = message
		}
		if len(delta["delta"]) == 0 {
			continue
		}
		fields, err := decode(delta["delta"])
		if err != nil {
			return err
		}
		for key, raw := range fields {
			switch key {
			case "content", "reasoning_content", "reasoning", "refusal":
				if string(raw) == "null" {
					if _, ok := message[key]; !ok {
						message[key] = raw
					}
					continue
				}
				if err := appendText(message, key, raw); err != nil {
					return err
				}
			case "tool_calls":
				if err := c.addTools(index, raw); err != nil {
					return err
				}
			case "function_call":
				call := object{}
				if previous := message[key]; len(previous) > 0 {
					if json.Unmarshal(previous, &call) != nil {
						return errors.New("invalid function call")
					}
				}
				next, err := decode(raw)
				if err != nil {
					return err
				}
				for name, part := range next {
					if name == "arguments" || name == "name" {
						if err := appendText(call, name, part); err != nil {
							return err
						}
					} else {
						call[name] = part
					}
				}
				message[key], _ = json.Marshal(call)
			default:
				message[key] = bytes.Clone(raw)
			}
		}
	}
	return nil
}

func (c *Collector) addTools(choice int, raw json.RawMessage) error {
	var deltas []object
	if json.Unmarshal(raw, &deltas) != nil {
		return errors.New("invalid tool call deltas")
	}
	if c.tools[choice] == nil {
		c.tools[choice] = map[int]object{}
	}
	for _, delta := range deltas {
		index, err := number(delta["index"])
		if err != nil {
			return err
		}
		call := c.tools[choice][index]
		if call == nil {
			call = object{}
			c.tools[choice][index] = call
		}
		for _, key := range []string{"id", "type"} {
			if incoming, ok := delta[key]; ok {
				if previous, exists := call[key]; exists && !bytes.Equal(previous, incoming) {
					return errors.New("native tool identity changed")
				}
			}
		}
		copyFields(call, delta, "index", "function")
		if len(delta["function"]) > 0 {
			function := object{}
			if len(call["function"]) > 0 {
				if json.Unmarshal(call["function"], &function) != nil {
					return errors.New("invalid tool function")
				}
			}
			next, err := decode(delta["function"])
			if err != nil {
				return err
			}
			for key, part := range next {
				if key == "arguments" || key == "name" {
					if err := appendText(function, key, part); err != nil {
						return err
					}
				} else {
					function[key] = part
				}
			}
			call["function"], _ = json.Marshal(function)
		}
	}
	return nil
}

func (c *Collector) finishChat() error {
	if !c.terminal || len(c.choices) == 0 {
		return errors.New("stream ended without a terminal")
	}
	for index, choice := range c.choices {
		reason, _ := text(choice["finish_reason"])
		if reason == "" {
			return errors.New("stream ended without a finish reason")
		}
		if c.protocol == "chat" {
			message := c.messages[index]
			if len(message) == 0 {
				return errors.New("stream has no message")
			}
			if reason == "tool_calls" && len(c.tools[index]) == 0 {
				return errors.New("tool finish without tool calls")
			}
			if reason == "function_call" && len(message["function_call"]) == 0 {
				return errors.New("function finish without function call")
			}
			if len(c.tools[index]) > 0 {
				tools, err := array(c.tools[index])
				if err != nil {
					return err
				}
				ids := map[string]bool{}
				for _, call := range tools {
					id, idErr := text(call["id"])
					kind, kindErr := text(call["type"])
					if idErr != nil || kindErr != nil || id == "" || kind != "function" {
						return errors.New("incomplete native tool identity")
					}
					if ids[id] {
						return errors.New("duplicate native tool identity")
					}
					ids[id] = true
					if err := validateFunction(call["function"]); err != nil {
						return err
					}
				}
				message["tool_calls"], _ = json.Marshal(tools)
			}
			if function := message["function_call"]; len(function) > 0 {
				if err := validateFunction(function); err != nil {
					return err
				}
			}
			choice["message"], _ = json.Marshal(message)
		}
	}
	choices, err := array(c.choices)
	if err != nil {
		return err
	}
	c.root["choices"], _ = json.Marshal(choices)
	return nil
}

func validateFunction(raw json.RawMessage) error {
	function, err := decode(raw)
	if err != nil {
		return err
	}
	name, err := text(function["name"])
	if err != nil || name == "" {
		return errors.New("incomplete native tool name")
	}
	arguments, err := text(function["arguments"])
	if err != nil {
		return err
	}
	if _, err := decode([]byte(arguments)); err != nil {
		return errors.New("incomplete native tool arguments")
	}
	return nil
}
