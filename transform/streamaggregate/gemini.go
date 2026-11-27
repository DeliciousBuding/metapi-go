package streamaggregate

import (
	"bytes"
	"encoding/json"
	"errors"
)

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

func (c *Collector) finishGemini() error {
	if len(c.choices) == 0 && !c.terminal {
		return errors.New("Gemini stream has no completed candidate")
	}
	for index := range c.choices {
		if !c.closed[index] {
			return errors.New("Gemini stream ended without finishReason")
		}
	}
	candidates, err := array(c.choices)
	if err != nil {
		return err
	}
	c.root["candidates"], _ = json.Marshal(candidates)
	return nil
}
