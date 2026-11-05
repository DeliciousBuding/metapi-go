// Package routecondition owns the validated predicates on imported association
// edges. It does not select routes or read request bodies.
package routecondition

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Condition struct {
	Type       string          `json:"type"`
	Logic      string          `json:"logic,omitempty"`
	Conditions []Condition     `json:"conditions,omitempty"`
	Field      string          `json:"field,omitempty"`
	Operator   string          `json:"operator,omitempty"`
	Value      json.RawMessage `json:"value,omitempty"`
}

// Request is the immutable description constructed before candidate selection.
// Now carries the explicitly chosen clock/location for daily-time predicates.
type Request struct {
	PromptTokens  int64
	Stream        bool
	RequestFormat string
	HasImage      bool
	HasVideo      bool
	HasDocument   bool
	HasAudio      bool
	Headers       map[string]string
	Now           time.Time
}

var errCondition = errors.New("invalid association condition")

// UnmarshalJSON rejects unknown, duplicate and incorrectly cased fields at
// every level. Silently ignoring one can broaden an authorization condition.
func (c *Condition) UnmarshalJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return errCondition
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return errCondition
		}
		name, ok := token.(string)
		if !ok {
			return errCondition
		}
		switch name {
		case "type", "logic", "conditions", "field", "operator", "value":
		default:
			return errCondition
		}
		if _, duplicate := fields[name]; duplicate {
			return errCondition
		}
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return errCondition
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil || decoder.Decode(new(any)) != io.EOF {
		return errCondition
	}
	var next Condition
	for name, raw := range fields {
		var target any
		switch name {
		case "type":
			target = &next.Type
		case "logic":
			target = &next.Logic
		case "conditions":
			target = &next.Conditions
		case "field":
			target = &next.Field
		case "operator":
			target = &next.Operator
		case "value":
			next.Value = append(json.RawMessage(nil), raw...)
			continue
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) || json.Unmarshal(raw, target) != nil {
			return errCondition
		}
	}
	*c = next
	return nil
}

func Decode(raw []byte) (*Condition, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var condition Condition
	if json.Unmarshal(raw, &condition) != nil || condition.Validate() != nil {
		return nil, errCondition
	}
	return &condition, nil
}

func (c Condition) Validate() error {
	return c.validate(1, true)
}

func (c Condition) validate(depth int, root bool) error {
	group := c.Type == "group" || root && c.Type == ""
	if root && !group {
		return errCondition
	}
	if group {
		logic := strings.ToUpper(strings.TrimSpace(c.Logic))
		if depth > 3 || len(c.Conditions) == 0 || c.Field != "" || c.Operator != "" ||
			len(c.Value) != 0 || logic != "" && logic != "AND" && logic != "OR" {
			return errCondition
		}
		for _, child := range c.Conditions {
			if err := child.validate(depth+1, false); err != nil {
				return err
			}
		}
		return nil
	}
	// The source's evaluator treats an omitted child type as a leaf.
	if c.Type != "" && c.Type != "condition" || len(c.Conditions) != 0 || c.Logic != "" {
		return errCondition
	}
	op := normalizeOperator(c.Operator)
	switch c.Field {
	case "prompt_tokens":
		if op != "lt" && op != "lte" && op != "gt" && op != "gte" {
			return errCondition
		}
		_, err := integerValue(c.Value)
		return err
	case "stream", "has_image", "has_video", "has_document", "has_audio":
		var value bool
		if !sourceEqualityOperator(c.Operator) || !isNonNull(c.Value) || json.Unmarshal(c.Value, &value) != nil {
			return errCondition
		}
		return nil
	case "request_format":
		if !sourceEqualityOperator(c.Operator) {
			return errCondition
		}
	case "daily_time":
		value, ok := stringValue(c.Value)
		if !ok || (op != "within" && op != "not_within") {
			return errCondition
		}
		_, _, err := dailyRange(value)
		return err
	default:
		if !strings.HasPrefix(c.Field, "request_header.") {
			return errCondition
		}
		name := strings.TrimSpace(strings.TrimPrefix(c.Field, "request_header."))
		if !validHeader(name) || sensitiveHeader(name) ||
			(!equalityOperator(op) && op != "contains" && op != "not_contains" && op != "start_with" && op != "end_with") {
			return errCondition
		}
	}
	if _, ok := stringValue(c.Value); !ok {
		return errCondition
	}
	return nil
}

// Match reports whether a condition could be evaluated as well as its result.
// Missing request context and invalid predicates never grant a candidate.
func (c *Condition) Match(input *Request) (matches, evaluated bool) {
	if c == nil {
		return true, true
	}
	if c.Validate() != nil || input == nil {
		return false, false
	}
	return c.match(input, true)
}

func (c Condition) match(input *Request, root bool) (bool, bool) {
	if c.Type == "group" || root && c.Type == "" {
		or := strings.EqualFold(strings.TrimSpace(c.Logic), "OR")
		result := !or
		for _, child := range c.Conditions {
			value, evaluated := child.match(input, false)
			if !evaluated {
				return false, false
			}
			if or {
				result = result || value
			} else {
				result = result && value
			}
		}
		return result, true
	}
	op := normalizeOperator(c.Operator)
	switch c.Field {
	case "prompt_tokens":
		if input.PromptTokens < 0 {
			return false, false
		}
		value, _ := integerValue(c.Value)
		switch op {
		case "lt":
			return input.PromptTokens < value, true
		case "lte":
			return input.PromptTokens <= value, true
		case "gt":
			return input.PromptTokens > value, true
		case "gte":
			return input.PromptTokens >= value, true
		}
	case "daily_time":
		if input.Now.IsZero() {
			return false, false
		}
		value, _ := stringValue(c.Value)
		start, end, _ := dailyRange(value)
		now := input.Now.Hour()*60 + input.Now.Minute()
		inside := now >= start && now < end
		if start > end {
			inside = now >= start || now < end
		}
		if op == "not_within" {
			inside = !inside
		}
		return inside, true
	case "stream", "has_image", "has_video", "has_document", "has_audio":
		actual := input.Stream
		switch c.Field {
		case "has_image":
			actual = input.HasImage
		case "has_video":
			actual = input.HasVideo
		case "has_document":
			actual = input.HasDocument
		case "has_audio":
			actual = input.HasAudio
		}
		var expected bool
		_ = json.Unmarshal(c.Value, &expected)
		return (actual == expected) != (op == "ne"), true
	default:
		actual := input.RequestFormat
		if c.Field == "request_format" && actual == "" {
			return false, false
		}
		if c.Field != "request_format" {
			name := strings.TrimSpace(strings.TrimPrefix(c.Field, "request_header."))
			// BuildRequest stores canonical keys. A missing source map key is
			// the empty string, including for ne/not_contains predicates.
			actual = input.Headers[http.CanonicalHeaderKey(name)]
		}
		value, _ := stringValue(c.Value)
		switch op {
		case "eq":
			return actual == value, true
		case "ne":
			return actual != value, true
		case "contains":
			return strings.Contains(actual, value), true
		case "not_contains":
			return !strings.Contains(actual, value), true
		case "start_with":
			return strings.HasPrefix(actual, value), true
		case "end_with":
			return strings.HasSuffix(actual, value), true
		}
	}
	return false, false
}

func normalizeOperator(op string) string {
	switch strings.ToLower(strings.TrimSpace(op)) {
	case "<", "lt":
		return "lt"
	case "<=", "lte":
		return "lte"
	case ">", "gt":
		return "gt"
	case ">=", "gte":
		return "gte"
	case "=", "==", "eq":
		return "eq"
	case "!=", "<>", "ne":
		return "ne"
	default:
		return strings.ToLower(strings.TrimSpace(op))
	}
}

func equalityOperator(op string) bool { return op == "eq" || op == "ne" }

func sourceEqualityOperator(op string) bool {
	switch op {
	case "eq", "ne", "=", "==", "!=":
		return true
	}
	return false
}

func isNonNull(raw json.RawMessage) bool {
	return len(raw) != 0 && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func stringValue(raw json.RawMessage) (string, bool) {
	var value string
	if !isNonNull(raw) || json.Unmarshal(raw, &value) != nil || value == "" {
		return "", false
	}
	return value, true
}

func integerValue(raw json.RawMessage) (int64, error) {
	if !json.Valid(raw) {
		return 0, errCondition
	}
	text := string(bytes.TrimSpace(raw))
	if integer, err := strconv.ParseInt(text, 10, 64); err == nil && integer >= 0 {
		return integer, nil
	}
	// The source normalizes JSON numbers to int64 first, then accepts an
	// integral float representation such as 100.0 or 1e2.
	value, err := strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 ||
		value >= 9223372036854775808.0 || math.Trunc(value) != value {
		return 0, errCondition
	}
	return int64(value), nil
}

func dailyRange(value string) (int, int, error) {
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return 0, 0, errCondition
	}
	start, firstErr := time.Parse("15:04", strings.TrimSpace(parts[0]))
	end, lastErr := time.Parse("15:04", strings.TrimSpace(parts[1]))
	startMinute, endMinute := start.Hour()*60+start.Minute(), end.Hour()*60+end.Minute()
	if firstErr != nil || lastErr != nil || startMinute == endMinute {
		return 0, 0, errCondition
	}
	return startMinute, endMinute, nil
}

func validHeader(name string) bool {
	if name == "" || len(name) > 256 {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r) {
			continue
		}
		return false
	}
	return true
}

func sensitiveHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case "Authorization", "Api-Key", "X-Api-Key", "X-Api-Secret", "X-Api-Token", "X-Goog-Api-Key", "X-Google-Api-Key",
		"Cookie", "Set-Cookie", "Proxy-Authorization", "Www-Authenticate",
		"X-Admin-Confirm-Token":
		return true
	}
	return strings.HasPrefix(strings.ToLower(name), "x-metapi-")
}
