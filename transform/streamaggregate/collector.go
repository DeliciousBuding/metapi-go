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

func (c *Collector) Result() ([]byte, error) {
	if c.err != nil {
		return nil, c.err
	}
	if c.protocol == "responses" {
		return c.responses.Result()
	}
	var err error
	switch c.protocol {
	case "chat", "completions":
		err = c.finishChat()
	case "messages":
		err = c.finishMessages()
	case "gemini":
		err = c.finishGemini()
	}
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(c.root)
	if int64(len(raw)) > c.limit {
		return nil, errors.New("aggregated response exceeds byte limit")
	}
	return raw, err
}
