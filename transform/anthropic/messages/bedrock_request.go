package messages

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PrepareBedrockRequest preserves native Messages fields while moving the
// model/stream selectors to the URL and beta flags into the Bedrock body.
func PrepareBedrockRequest(body []byte, betaHeaders []string) ([]byte, error) {
	value, err := object(body, "Bedrock request")
	if err != nil {
		return nil, err
	}
	delete(value, "model")
	delete(value, "stream")
	value["anthropic_version"] = json.RawMessage(`"bedrock-2023-05-31"`)
	var beta []string
	if raw := value["anthropic_beta"]; !absent(raw) {
		if err := json.Unmarshal(raw, &beta); err != nil {
			return nil, fmt.Errorf("Bedrock anthropic_beta must be an array of strings")
		}
	}
	for _, header := range betaHeaders {
		beta = append(beta, strings.Split(header, ",")...)
	}
	var tools []struct {
		Type string `json:"type"`
	}
	if raw := value["tools"]; !absent(raw) {
		if err := json.Unmarshal(raw, &tools); err != nil {
			return nil, fmt.Errorf("Bedrock tools must be an array of tool objects")
		}
		for _, tool := range tools {
			if tool.Type == "web_search_20250305" {
				beta = append(beta, "web-search-2025-03-05")
			}
		}
	}
	seen := make(map[string]bool, len(beta))
	unique := make([]string, 0, len(beta))
	for _, flag := range beta {
		flag = strings.TrimSpace(flag)
		if flag == "" || seen[flag] {
			continue
		}
		seen[flag] = true
		unique = append(unique, flag)
	}
	delete(value, "anthropic_beta")
	if len(unique) > 0 {
		value["anthropic_beta"], err = json.Marshal(unique)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(value)
}
