package streamaggregate

import (
	"encoding/json"
	"errors"
)

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

func (c *Collector) finishMessages() error {
	if !c.terminal {
		return errors.New("message stream ended without message_stop")
	}
	blocks, err := array(c.blocks)
	if err != nil {
		return err
	}
	for _, block := range blocks {
		kind, _ := text(block["type"])
		if kind == "tool_use" || kind == "server_tool_use" {
			id, idErr := text(block["id"])
			name, nameErr := text(block["name"])
			if idErr != nil || nameErr != nil || id == "" || name == "" {
				return errors.New("incomplete message tool identity")
			}
			if _, err := decode(block["input"]); err != nil {
				return errors.New("incomplete message tool input")
			}
		}
	}
	c.root["content"], _ = json.Marshal(blocks)
	return nil
}
