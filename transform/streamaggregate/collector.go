// Package streamaggregate collects native text streams without translating
// their response schema. Network framing, cancellation and retries belong to
// the caller. No output is available until a verified terminal is received.
package streamaggregate

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/deliciousbuding/metapi-go/transform/openai/responses"
)

type object = map[string]json.RawMessage

type Collector struct {
	protocol     string
	limit, used  int64
	err          error
	root         object
	choices      map[int]object
	messages     map[int]object
	tools        map[int]map[int]object
	blocks       map[int]object
	closed       map[int]bool
	arguments    map[int]string
	terminal     bool
	messageDelta bool
	responses    responses.CodexResponseCollector
}

func New(protocol string, limit int64) (*Collector, error) {
	switch protocol {
	case "chat", "completions", "responses", "messages", "gemini":
	default:
		return nil, fmt.Errorf("unsupported stream aggregation protocol")
	}
	if limit <= 0 {
		return nil, errors.New("stream aggregation requires a byte limit")
	}
	return &Collector{protocol: protocol, limit: limit, root: object{},
		choices: map[int]object{}, messages: map[int]object{}, tools: map[int]map[int]object{},
		blocks: map[int]object{}, closed: map[int]bool{}, arguments: map[int]string{},
		responses: responses.CodexResponseCollector{Limit: limit, AllowIncomplete: true}}, nil
}

func decode(raw []byte) (object, error) {
	var value object
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return nil, errors.New("invalid native stream object")
	}
	return value, nil
}

func text(raw json.RawMessage) (string, error) {
	var value string
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	if json.Unmarshal(raw, &value) != nil {
		return "", errors.New("invalid native stream text")
	}
	return value, nil
}

func number(raw json.RawMessage) (int, error) {
	var value int
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || value < 0 || value > 4095 {
		return 0, errors.New("invalid native stream index")
	}
	return value, nil
}

func appendText(dst object, key string, raw json.RawMessage) error {
	next, err := text(raw)
	if err != nil {
		return err
	}
	previous, err := text(dst[key])
	if err != nil {
		return err
	}
	dst[key], _ = json.Marshal(previous + next)
	return nil
}

func copyFields(dst, src object, excluded ...string) {
	for key, raw := range src {
		skip := false
		for _, exclude := range excluded {
			skip = skip || key == exclude
		}
		if !skip {
			dst[key] = bytes.Clone(raw)
		}
	}
}

func array(values map[int]object) ([]object, error) {
	indices := make([]int, 0, len(values))
	for index := range values {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	out := make([]object, 0, len(indices))
	for expected, index := range indices {
		if index != expected {
			return nil, errors.New("native stream has missing indices")
		}
		out = append(out, values[index])
	}
	return out, nil
}

func (c *Collector) Add(event string, raw []byte) error {
	if c.err != nil {
		return c.err
	}
	c.used += int64(len(raw))
	if c.used > c.limit {
		c.err = errors.New("upstream stream exceeds byte limit")
		return c.err
	}
	if event == "error" {
		c.err = errors.New("upstream stream returned an error")
		return c.err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	if c.protocol == "responses" {
		c.err = c.responses.AddData(raw)
		return c.err
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("[DONE]")) {
		if c.protocol != "chat" && c.protocol != "completions" {
			c.err = errors.New("unexpected stream terminal")
		} else if c.terminal {
			c.err = errors.New("duplicate stream terminal")
		} else {
			c.terminal = true
		}
		return c.err
	}
	if c.terminal {
		c.err = errors.New("data after stream terminal")
		return c.err
	}
	value, err := decode(raw)
	if err != nil {
		c.err = err
		return err
	}
	if errorValue, exists := value["error"]; exists && string(errorValue) != "null" {
		c.err = errors.New("upstream stream returned an error")
		return c.err
	}
	switch c.protocol {
	case "chat", "completions":
		err = c.addChat(value)
	case "messages":
		err = c.addMessages(value)
	case "gemini":
		err = c.addGemini(value)
	}
	c.err = err
	return err
}

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

func (c *Collector) addMessages(value object) error {
	kind, err := text(value["type"])
	if err != nil {
		return err
	}
	switch kind {
	case "ping":
		return nil
	case "message_start":
		if len(c.root) > 0 {
			return errors.New("duplicate message start")
		}
		message, err := decode(value["message"])
		if err != nil {
			return err
		}
		copyFields(c.root, message)
		var content []object
		if len(message["content"]) > 0 && json.Unmarshal(message["content"], &content) != nil {
			return errors.New("invalid initial message content")
		}
		for index, block := range content {
			c.blocks[index] = block
			c.closed[index] = true
		}
	case "content_block_start":
		if len(c.root) == 0 {
			return errors.New("content before message start")
		}
		index, err := number(value["index"])
		if err != nil {
			return err
		}
		if c.blocks[index] != nil {
			return errors.New("duplicate content block")
		}
		block, err := decode(value["content_block"])
		if err != nil {
			return err
		}
		c.blocks[index] = block
	case "content_block_delta":
		index, err := number(value["index"])
		if err != nil {
			return err
		}
		block := c.blocks[index]
		if block == nil || c.closed[index] {
			return errors.New("delta outside content block")
		}
		delta, err := decode(value["delta"])
		if err != nil {
			return err
		}
		kind, err := text(delta["type"])
		if err != nil {
			return err
		}
		switch kind {
		case "text_delta":
			return appendText(block, "text", delta["text"])
		case "thinking_delta":
			return appendText(block, "thinking", delta["thinking"])
		case "signature_delta":
			return appendText(block, "signature", delta["signature"])
		case "input_json_delta":
			part, err := text(delta["partial_json"])
			if err != nil {
				return err
			}
			c.arguments[index] += part
		case "citations_delta":
			var citations []json.RawMessage
			if len(block["citations"]) > 0 && json.Unmarshal(block["citations"], &citations) != nil {
				return errors.New("invalid citations")
			}
			if len(delta["citation"]) == 0 {
				return errors.New("missing citation")
			}
			block["citations"], _ = json.Marshal(append(citations, delta["citation"]))
		default:
			return errors.New("unsupported native content delta")
		}
	case "content_block_stop":
		index, err := number(value["index"])
		if err != nil {
			return err
		}
		if c.blocks[index] == nil || c.closed[index] {
			return errors.New("invalid content block stop")
		}
		if args, ok := c.arguments[index]; ok {
			if _, err := decode([]byte(args)); err != nil {
				return errors.New("incomplete tool input")
			}
			c.blocks[index]["input"] = json.RawMessage(args)
		}
		c.closed[index] = true
	case "message_delta":
		if len(c.root) == 0 {
			return errors.New("delta before message start")
		}
		delta, err := decode(value["delta"])
		if err != nil {
			return err
		}
		copyFields(c.root, delta)
		if len(value["usage"]) > 0 {
			usage := object{}
			if len(c.root["usage"]) > 0 {
				if json.Unmarshal(c.root["usage"], &usage) != nil {
					return errors.New("invalid message usage")
				}
			}
			next, err := decode(value["usage"])
			if err != nil {
				return err
			}
			copyFields(usage, next)
			c.root["usage"], _ = json.Marshal(usage)
		}
		c.messageDelta = true
	case "message_stop":
		reason, err := text(c.root["stop_reason"])
		if err != nil || reason == "" || !c.messageDelta {
			return errors.New("message stopped without a finish reason")
		}
		for index := range c.blocks {
			if !c.closed[index] {
				return errors.New("message stopped with incomplete content")
			}
		}
		c.terminal = true
	default:
		return errors.New("unsupported native message event")
	}
	return nil
}

func (c *Collector) addGemini(value object) error {
	copyFields(c.root, value, "candidates")
	var candidates []object
	if raw, ok := value["candidates"]; ok && json.Unmarshal(raw, &candidates) != nil {
		return errors.New("invalid Gemini candidates")
	}
	if len(candidates) == 0 && len(value["promptFeedback"]) > 0 {
		feedback, err := decode(value["promptFeedback"])
		if err != nil {
			return err
		}
		reason, err := text(feedback["blockReason"])
		if err != nil {
			return err
		}
		if reason != "" && reason != "BLOCK_REASON_UNSPECIFIED" {
			if len(c.choices) != 0 {
				return errors.New("Gemini prompt blocked after candidate output")
			}
			c.terminal = true
		}
	}
	for position, delta := range candidates {
		index := position
		if raw, ok := delta["index"]; ok {
			var err error
			index, err = number(raw)
			if err != nil {
				return err
			}
		}
		candidate := c.choices[index]
		if candidate == nil {
			candidate = object{}
			c.choices[index] = candidate
		}
		if c.closed[index] && len(delta["content"]) > 0 {
			return errors.New("Gemini content after finish")
		}
		copyFields(candidate, delta, "content")
		if len(delta["content"]) > 0 {
			content, err := decode(delta["content"])
			if err != nil {
				return err
			}
			current := object{}
			if len(candidate["content"]) > 0 {
				if json.Unmarshal(candidate["content"], &current) != nil {
					return errors.New("invalid Gemini content")
				}
			}
			var previous, parts []object
			if len(current["parts"]) > 0 && json.Unmarshal(current["parts"], &previous) != nil {
				return errors.New("invalid Gemini parts")
			}
			if json.Unmarshal(content["parts"], &parts) != nil {
				return errors.New("invalid Gemini parts")
			}
			for _, part := range parts {
				// Adjacent text fragments with equal thought identity belong to
				// one part. Opaque signatures and function payloads stay native.
				if len(previous) > 0 && len(part["text"]) > 0 {
					last := previous[len(previous)-1]
					if len(last["text"]) > 0 && bytes.Equal(last["thought"], part["thought"]) && len(last["thoughtSignature"]) == 0 {
						if err := appendText(last, "text", part["text"]); err != nil {
							return err
						}
						copyFields(last, part, "text")
						continue
					}
				}
				previous = append(previous, part)
			}
			copyFields(current, content, "parts")
			current["parts"], _ = json.Marshal(previous)
			candidate["content"], _ = json.Marshal(current)
		}
		if raw := delta["finishReason"]; len(raw) > 0 {
			reason, err := text(raw)
			if err != nil || reason == "" || reason == "FINISH_REASON_UNSPECIFIED" {
				return errors.New("invalid Gemini finish reason")
			}
			c.closed[index] = true
		}
	}
	return nil
}

func (c *Collector) Result() ([]byte, error) {
	if c.err != nil {
		return nil, c.err
	}
	if c.protocol == "responses" {
		return c.responses.Result()
	}
	switch c.protocol {
	case "chat", "completions":
		if !c.terminal || len(c.choices) == 0 {
			return nil, errors.New("stream ended without a terminal")
		}
		for index, choice := range c.choices {
			reason, _ := text(choice["finish_reason"])
			if reason == "" {
				return nil, errors.New("stream ended without a finish reason")
			}
			if c.protocol == "chat" {
				message := c.messages[index]
				if len(message) == 0 {
					return nil, errors.New("stream has no message")
				}
				if reason == "tool_calls" && len(c.tools[index]) == 0 {
					return nil, errors.New("tool finish without tool calls")
				}
				if reason == "function_call" && len(message["function_call"]) == 0 {
					return nil, errors.New("function finish without function call")
				}
				if len(c.tools[index]) > 0 {
					tools, err := array(c.tools[index])
					if err != nil {
						return nil, err
					}
					ids := map[string]bool{}
					for _, call := range tools {
						id, idErr := text(call["id"])
						kind, kindErr := text(call["type"])
						if idErr != nil || kindErr != nil || id == "" || kind != "function" {
							return nil, errors.New("incomplete native tool identity")
						}
						if ids[id] {
							return nil, errors.New("duplicate native tool identity")
						}
						ids[id] = true
						if err := validateFunction(call["function"]); err != nil {
							return nil, err
						}
					}
					message["tool_calls"], _ = json.Marshal(tools)
				}
				if function := message["function_call"]; len(function) > 0 {
					if err := validateFunction(function); err != nil {
						return nil, err
					}
				}
				choice["message"], _ = json.Marshal(message)
			}
		}
		choices, err := array(c.choices)
		if err != nil {
			return nil, err
		}
		c.root["choices"], _ = json.Marshal(choices)
	case "messages":
		if !c.terminal {
			return nil, errors.New("message stream ended without message_stop")
		}
		blocks, err := array(c.blocks)
		if err != nil {
			return nil, err
		}
		for _, block := range blocks {
			kind, _ := text(block["type"])
			if kind == "tool_use" || kind == "server_tool_use" {
				id, idErr := text(block["id"])
				name, nameErr := text(block["name"])
				if idErr != nil || nameErr != nil || id == "" || name == "" {
					return nil, errors.New("incomplete message tool identity")
				}
				if _, err := decode(block["input"]); err != nil {
					return nil, errors.New("incomplete message tool input")
				}
			}
		}
		c.root["content"], _ = json.Marshal(blocks)
	case "gemini":
		if len(c.choices) == 0 && !c.terminal {
			return nil, errors.New("Gemini stream has no completed candidate")
		}
		for index := range c.choices {
			if !c.closed[index] {
				return nil, errors.New("Gemini stream ended without finishReason")
			}
		}
		candidates, err := array(c.choices)
		if err != nil {
			return nil, err
		}
		c.root["candidates"], _ = json.Marshal(candidates)
	}
	raw, err := json.Marshal(c.root)
	if int64(len(raw)) > c.limit {
		return nil, errors.New("aggregated response exceeds byte limit")
	}
	return raw, err
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
