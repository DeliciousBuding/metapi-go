package upstreamadapter

import (
	"bytes"
	"encoding/json"
	"fmt"
)

func unwrapAntigravityResponse(body []byte, _ bool) ([]byte, error) {
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil || envelope == nil || len(envelope["error"]) != 0 {
		return nil, fmt.Errorf("invalid Antigravity response envelope")
	}
	response := bytes.TrimSpace(envelope["response"])
	var object map[string]json.RawMessage
	if len(response) == 0 || json.Unmarshal(response, &object) != nil || object == nil || len(object["error"]) != 0 {
		return nil, fmt.Errorf("invalid Antigravity response")
	}
	if _, candidates := object["candidates"]; !candidates {
		if _, feedback := object["promptFeedback"]; !feedback {
			if _, usage := object["usageMetadata"]; !usage {
				return nil, fmt.Errorf("missing Antigravity response fields")
			}
		}
	}
	for _, field := range []string{"promptFeedback", "usageMetadata"} {
		if value, present := object[field]; present {
			var metadata map[string]json.RawMessage
			if json.Unmarshal(value, &metadata) != nil || metadata == nil {
				return nil, fmt.Errorf("invalid Antigravity response metadata")
			}
		}
	}
	if value, present := object["candidates"]; present {
		var candidates []map[string]json.RawMessage
		if json.Unmarshal(value, &candidates) != nil || candidates == nil {
			return nil, fmt.Errorf("invalid Antigravity response candidates")
		}
		for _, candidate := range candidates {
			if candidate == nil {
				return nil, fmt.Errorf("invalid Antigravity response candidate")
			}
		}
	}
	return bytes.Clone(response), nil
}
