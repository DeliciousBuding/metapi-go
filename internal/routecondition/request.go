package routecondition

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"time"
	"unicode"
)

// The source-compatibility model is pinned to AxonHub d96748f1. Existing
// relay transforms intentionally preserve a different native wire contract;
// they cannot replace these field counts without changing routing thresholds.
// These readers produce only scalar routing facts and never a relay payload.

var errRequest = errors.New("association request description unavailable")

// BuildRequest describes the request as the source's inbound transformer would
// present it to conditional routing. It does not rewrite or retain the payload.
// Token counts intentionally follow the source heuristic, not quota accounting.
func BuildRequest(body []byte, format string, headers http.Header, stream bool, now time.Time) (*Request, error) {
	var object map[string]json.RawMessage
	if json.Unmarshal(body, &object) != nil || object == nil {
		return nil, errRequest
	}
	result := &Request{Stream: stream, RequestFormat: format, Now: now, Headers: map[string]string{}}
	for name, values := range headers {
		if len(values) == 0 || sensitiveHeader(name) {
			continue
		}
		result.Headers[http.CanonicalHeaderKey(name)] = values[0]
	}
	var err error
	switch format {
	case "openai/chat_completions":
		err = describeChat(object, result)
	case "openai/responses":
		err = describeResponses(object, result)
	case "anthropic/messages":
		err = describeMessages(object, result)
	case "gemini/contents":
		err = describeGemini(object, result)
	case "openai/completions":
		// Completion retains its stream flag but does not populate Messages.
	case "openai/embeddings", "openai/moderations",
		"jina/rerank", "jina/embeddings", "gemini/embeddings":
		// These source inbound adapters populate dedicated request fields,
		// not the Messages/Tools used by its conditional prompt estimator.
		result.Stream = false
	default:
		err = errRequest
	}
	if err != nil {
		return nil, errRequest
	}
	return result, nil
}

func sourceTokens(value string) int64 {
	if value == "" {
		return 0
	}
	var cjk, other int
	for _, r := range value {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
			unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			cjk++
		} else if !unicode.IsSpace(r) {
			other++
		}
	}
	return max(1, int64(float64(cjk)/1.5+float64(other)/4))
}

func rawString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return "", nil
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return "", errRequest
	}
	return value, nil
}

func rawObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil || object == nil {
		return nil, errRequest
	}
	return object, nil
}

func rawArray(raw json.RawMessage) ([]json.RawMessage, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, nil
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return nil, errRequest
	}
	return values, nil
}

// Source fields represented as RawMessage keep their spelling; fields decoded
// into maps are marshaled back to JSON before the source counts them.
func compactJSON(raw json.RawMessage, mapRoundTrip bool) (string, error) {
	if len(raw) == 0 {
		return "null", nil
	}
	if !mapRoundTrip {
		var compact bytes.Buffer
		if json.Compact(&compact, raw) != nil {
			return "", errRequest
		}
		return compact.String(), nil
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return "", errRequest
	}
	encoded, err := json.Marshal(value)
	return string(encoded), err
}

func addFunctionTool(object map[string]json.RawMessage, schemaField, prefix string, normalize bool, result *Request) error {
	name, err := rawString(object["name"])
	if err != nil {
		return err
	}
	description, err := rawString(object["description"])
	if err != nil {
		return err
	}
	schema := string(object[schemaField])
	if normalize {
		schema, err = compactJSON(object[schemaField], true)
		if err != nil {
			return err
		}
	}
	result.PromptTokens += sourceTokens("function") + sourceTokens(prefix+name) + sourceTokens(description) + sourceTokens(schema)
	return nil
}

func addStringList(raw json.RawMessage, result *Request) error {
	var values []string
	if len(raw) != 0 && json.Unmarshal(raw, &values) != nil {
		return errRequest
	}
	for _, value := range values {
		result.PromptTokens += sourceTokens(value)
	}
	return nil
}

func addLocation(raw json.RawMessage, defaultType bool, result *Request) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	var location struct{ Type, City, Country, Region, Timezone string }
	if json.Unmarshal(raw, &location) != nil {
		return errRequest
	}
	if defaultType && location.Type == "" {
		location.Type = "approximate"
	}
	for _, value := range []string{location.Type, location.City, location.Country, location.Region, location.Timezone} {
		result.PromptTokens += sourceTokens(value)
	}
	return nil
}
