package responses

import (
	"bytes"
	"encoding/json"
	"errors"
	"sort"
)

// PrepareCodexRequest applies the Codex Responses contract after protocol
// conversion. Raw items, encrypted reasoning and provider extensions survive.
func PrepareCodexRequest(raw []byte) ([]byte, error) {
	var body map[string]json.RawMessage
	if json.Unmarshal(raw, &body) != nil || body == nil {
		return nil, errors.New("invalid Codex request")
	}
	for _, key := range []string{"max_tokens", "max_output_tokens", "max_completion_tokens", "metadata", "user"} {
		delete(body, key)
	}
	body["stream"] = json.RawMessage(`true`)
	body["store"] = json.RawMessage(`false`)
	body["parallel_tool_calls"] = json.RawMessage(`true`)
	if _, ok := body["instructions"]; !ok {
		body["instructions"] = json.RawMessage(`""`)
	}
	if _, ok := body["include"]; !ok {
		body["include"] = json.RawMessage(`["reasoning.encrypted_content"]`)
	}
	var reasoning map[string]json.RawMessage
	if raw, ok := body["reasoning"]; ok && string(raw) != "null" {
		if json.Unmarshal(raw, &reasoning) != nil {
			return nil, errors.New("invalid Codex reasoning")
		}
	}
	if reasoning == nil {
		reasoning = map[string]json.RawMessage{}
	}
	if raw, ok := reasoning["summary"]; !ok || string(raw) == `""` || string(raw) == "null" {
		reasoning["summary"] = json.RawMessage(`"auto"`)
	}
	body["reasoning"], _ = json.Marshal(reasoning)
	var input string
	if json.Unmarshal(body["input"], &input) == nil {
		body["input"], _ = json.Marshal([]any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "input_text", "text": input}}}})
	}
	return json.Marshal(body)
}

var ErrCodexStreamIncomplete = errors.New("Codex response stream ended without a completed response")

// CodexResponseCollector consumes framed SSE data, not network I/O. The caller
// owns idle timeouts and framing. A real terminal response is mandatory; EOF,
// [DONE], errors, and partial deltas never manufacture a successful document.
type CodexResponseCollector struct {
	Limit    int64
	used     int64
	items    map[int]json.RawMessage
	response json.RawMessage
	err      error
	terminal bool
}

func (c *CodexResponseCollector) AddData(raw []byte) error {
	if c.err != nil {
		return c.err
	}
	if c.Limit <= 0 {
		c.Limit = 16 << 20
	}
	c.used += int64(len(raw))
	if c.used > c.Limit {
		c.err = errors.New("Codex response exceeds byte limit")
		return c.err
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("[DONE]")) {
		return nil
	}
	var event struct {
		Type        string          `json:"type"`
		OutputIndex int             `json:"output_index"`
		Item        json.RawMessage `json:"item"`
		Response    json.RawMessage `json:"response"`
	}
	if json.Unmarshal(raw, &event) != nil {
		c.err = errors.New("invalid Codex response event")
		return c.err
	}
	switch event.Type {
	case "error", "response.failed", "response.incomplete":
		c.err = errors.New("Codex upstream did not complete the response")
		return c.err
	case "response.output_item.done":
		if c.terminal || event.OutputIndex < 0 || event.OutputIndex > 4095 || len(event.Item) == 0 {
			c.err = ErrCodexStreamIncomplete
			return c.err
		}
		if c.items == nil {
			c.items = map[int]json.RawMessage{}
		}
		c.items[event.OutputIndex] = bytes.Clone(event.Item)
	case "response.completed":
		if c.terminal {
			c.err = errors.New("duplicate Codex terminal response")
			return c.err
		}
		var response map[string]json.RawMessage
		if json.Unmarshal(event.Response, &response) != nil || response == nil {
			c.err = ErrCodexStreamIncomplete
			return c.err
		}
		var status string
		_ = json.Unmarshal(response["status"], &status)
		if status != "completed" || (len(response["error"]) > 0 && string(response["error"]) != "null") {
			c.err = ErrCodexStreamIncomplete
			return c.err
		}
		if _, ok := response["output"]; !ok && len(c.items) > 0 {
			indices := make([]int, 0, len(c.items))
			for index := range c.items {
				indices = append(indices, index)
			}
			sort.Ints(indices)
			items := make([]json.RawMessage, 0, len(indices))
			for expected, index := range indices {
				if expected != index {
					c.err = ErrCodexStreamIncomplete
					return c.err
				}
				items = append(items, c.items[index])
			}
			response["output"], _ = json.Marshal(items)
		}
		if len(response["output"]) == 0 {
			c.err = ErrCodexStreamIncomplete
			return c.err
		}
		c.response, _ = json.Marshal(response)
		c.terminal = true
	}
	return nil
}

func (c *CodexResponseCollector) Result() ([]byte, error) {
	if c.err != nil {
		return nil, c.err
	}
	if !c.terminal {
		return nil, ErrCodexStreamIncomplete
	}
	return bytes.Clone(c.response), nil
}
