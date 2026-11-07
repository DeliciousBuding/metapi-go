package channelplugin

import (
	"bytes"
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/tidwall/sjson"
)

// Observation paths select original JSON values, independently of executable
// decoders. The host retains token interpretation, pricing and settlement.
type Observation struct {
	UsagePaths map[string]string `json:"usagePaths,omitempty"`
	ModelPath  string            `json:"modelPath,omitempty"`
	ErrorPath  string            `json:"errorPath,omitempty"`
}

type ProtocolObservations struct {
	Response *Observation `json:"response,omitempty"`
	Event    *Observation `json:"event,omitempty"`
}

var usageObservationFields = []string{
	"prompt_tokens", "completion_tokens", "total_tokens", "input_tokens", "output_tokens",
	"cache_read_input_tokens", "cache_creation_input_tokens",
	"prompt_tokens_details.cached_tokens", "input_tokens_details.cached_tokens",
	"completion_tokens_details.reasoning_tokens", "output_tokens_details.reasoning_tokens",
	"promptTokenCount", "candidatesTokenCount", "totalTokenCount", "thoughtsTokenCount",
}

func (o ProtocolObservations) Validate() error {
	if o.Response == nil && o.Event == nil {
		return ErrManifest
	}
	for _, observation := range []*Observation{o.Response, o.Event} {
		if observation == nil {
			continue
		}
		if len(observation.UsagePaths) == 0 && observation.ModelPath == "" && observation.ErrorPath == "" {
			return ErrManifest
		}
		for field, path := range observation.UsagePaths {
			if !slices.Contains(usageObservationFields, field) || !validObservationPointer(path) {
				return ErrManifest
			}
		}
		for _, path := range []string{observation.ModelPath, observation.ErrorPath} {
			if path != "" && !validObservationPointer(path) {
				return ErrManifest
			}
		}
	}
	return nil
}

func validObservationPointer(path string) bool {
	if len(path) == 0 || len(path) > 1024 || path[0] != '/' || strings.Count(path, "/") > 64 || strings.ContainsAny(path, "\r\n\x00") {
		return false
	}
	for i := 0; i < len(path); i++ {
		if path[i] == '~' {
			if i+1 >= len(path) || path[i+1] != '0' && path[i+1] != '1' {
				return false
			}
			i++
		}
	}
	return true
}

type ObservationResult struct {
	Usage  json.RawMessage
	Model  json.RawMessage
	Failed bool
}

func (p *Program) HasDriverHook(hook string) bool {
	return p != nil && p.manifest.IsProvider() && slices.Contains(p.manifest.Hooks, hook)
}

// Observe uses captured metadata only: even a throwing decoder cannot remove
// already received usage. Missing pointers are normal for partial SSE events;
// malformed/ambiguous JSON is an error, never an unknown successful document.
func (p *Program) Observe(protocol string, event bool, body []byte) (ObservationResult, error) {
	if p == nil || !p.manifest.IsProvider() || !slices.Contains(p.manifest.Protocols, protocol) {
		return ObservationResult{}, ErrManifest
	}
	if len(body) > MaxInputBytes || requestoverride.RejectDuplicateJSONKeys(body) != nil {
		return ObservationResult{}, ErrResult
	}
	spec := p.manifest.Observations[protocol]
	observation := spec.Response
	if event {
		observation = spec.Event
	}
	if observation == nil {
		return ObservationResult{}, nil
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return ObservationResult{}, ErrResult
	}
	usage := []byte(`{"usage":{}}`)
	for _, field := range usageObservationFields {
		pointer, configured := observation.UsagePaths[field]
		if !configured {
			continue
		}
		selected, present := observationValue(value, pointer)
		if !present {
			continue
		}
		raw, err := json.Marshal(selected)
		if err != nil {
			return ObservationResult{}, ErrResult
		}
		// Only fixed host-owned usage paths are writable here. Values retain
		// their original numeric/type representation for host validation.
		usage, err = sjson.SetRawBytes(usage, "usage."+field, raw)
		if err != nil {
			return ObservationResult{}, ErrResult
		}
	}
	result := ObservationResult{Usage: usage}
	if observation.ModelPath != "" {
		if selected, present := observationValue(value, observation.ModelPath); present {
			result.Model, _ = json.Marshal(selected)
		}
	}
	if observation.ErrorPath != "" {
		if selected, present := observationValue(value, observation.ErrorPath); present {
			switch selected := selected.(type) {
			case nil:
			case bool:
				result.Failed = selected
			case string:
				result.Failed = selected != ""
			case json.Number:
				number, err := selected.Float64()
				result.Failed = err != nil || number != 0
			default:
				result.Failed = true
			}
		}
	}
	return result, nil
}

// RFC 6901 keys are literal, including dots, wildcards and prototype names;
// there are no gjson queries, filters or executable expressions in pointers.
func observationValue(value any, pointer string) (any, bool) {
	for part := range strings.SplitSeq(strings.TrimPrefix(pointer, "/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch current := value.(type) {
		case map[string]any:
			var present bool
			value, present = current[part]
			if !present {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(current) || strconv.Itoa(index) != part {
				return nil, false
			}
			value = current[index]
		default:
			return nil, false
		}
	}
	return value, true
}
