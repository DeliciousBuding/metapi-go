package admin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// Admin mutations reject ambiguous keys, unknown fields and additional values.
// A caller may allow null only for explicit root-level removal fields; nested
// nulls and all other fields retain the existing rejection contract.
func decodeImportedJSON(r *http.Request, dst any, nullableRootFields ...string) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return fmt.Errorf("invalid request body")
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return fmt.Errorf("invalid JSON object")
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) == 0 {
		return fmt.Errorf("expected a non-empty JSON object")
	}
	var tree any
	if json.Unmarshal(raw, &tree) != nil {
		return fmt.Errorf("invalid JSON object")
	}
	if object, ok := tree.(map[string]any); ok {
		for _, field := range nullableRootFields {
			if value, exists := object[field]; exists && value == nil {
				delete(object, field)
			}
		}
	}
	if importedJSONHasNull(tree) {
		return fmt.Errorf("null is not supported")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid request fields")
	}
	return nil
}

func importedJSONHasNull(value any) bool {
	if value == nil {
		return true
	}
	switch value := value.(type) {
	case map[string]any:
		for _, v := range value {
			if importedJSONHasNull(v) {
				return true
			}
		}
	case []any:
		for _, v := range value {
			if importedJSONHasNull(v) {
				return true
			}
		}
	}
	return false
}

func validateImportedConfig(row importedChannelConfig) error {
	if err := row.RateLimit.Validate(); err != nil {
		return err
	}
	if !store.ValidDirectStreamPolicy(row.StreamPolicy) {
		return fmt.Errorf("Invalid stream policy")
	}
	if err := row.RequestConfig.Validate(); err != nil {
		return fmt.Errorf("Invalid request operations: %s", err)
	}
	if err := row.RequestConfig.ValidateEndpoints(row.Endpoints); err != nil {
		return fmt.Errorf("Endpoints are incompatible with the imported request operations")
	}
	if strings.TrimSpace(row.Name) == "" || len(row.Name) > 200 {
		return fmt.Errorf("Name must contain 1 to 200 characters")
	}
	if !validImportedURL(row.BaseURL, false) {
		return fmt.Errorf("Invalid base URL")
	}
	for _, path := range []string{row.ChatPath, row.ResponsesPath, row.MessagesPath} {
		if path != "" && (!strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "?#\\\r\n") || proxy.ContainsPathTraversal(path)) {
			return fmt.Errorf("Invalid legacy endpoint path")
		}
		decoded, err := url.PathUnescape(path)
		if err != nil || proxy.ContainsPathTraversal(decoded) {
			return fmt.Errorf("Invalid legacy endpoint path")
		}
	}
	raw, _ := row.Endpoints.Value()
	var checked store.DirectEndpoints
	if checked.Scan(raw) != nil {
		return fmt.Errorf("Invalid endpoint URL or authentication")
	}
	for _, entry := range row.Endpoints.Entries() {
		ep := entry.Endpoint
		if ep == nil {
			continue
		}
		for _, address := range ep.URLFields() {
			target := *address
			if entry.Protocol == store.DirectProtocolResponsesWebsocket {
				target = strings.Replace(strings.Replace(target, "wss://", "https://", 1), "ws://", "http://", 1)
			}
			if !validImportedURL(target, false) {
				return fmt.Errorf("Invalid endpoint URL")
			}
		}
		if ep.Profile == "opencode-go" && row.Provider != "opencode_go" {
			return fmt.Errorf("OpenCode Go profile requires its matching provider")
		}
		if ep.Profile == "antigravity" && row.Provider != "antigravity" {
			return fmt.Errorf("Antigravity profile requires its matching provider")
		}
		if (ep.Profile == "codex" || ep.Profile == "codex-image" || ep.Profile == "codex-alpha-search") && row.Provider != "codex" && row.Provider != "fenno" || ep.Profile == "claudecode" && row.Provider != "claudecode" {
			return fmt.Errorf("Endpoint profile does not match the provider and protocol")
		}
	}
	if row.ChannelProxy != "" && !validImportedURL(row.ChannelProxy, true) {
		return fmt.Errorf("Invalid channel proxy URL")
	}
	if row.CustomHeaders != "" {
		if rejectDuplicateJSONKeys([]byte(row.CustomHeaders)) != nil {
			return fmt.Errorf("Invalid custom headers")
		}
		var headers []struct {
			Key   string `json:"header_key"`
			Value string `json:"header_value"`
		}
		decoder := json.NewDecoder(strings.NewReader(row.CustomHeaders))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&headers) != nil || headers == nil {
			return fmt.Errorf("Custom headers must be a JSON array")
		}
		seen := map[string]bool{}
		for _, header := range headers {
			key := strings.ToLower(header.Key)
			if key == "" || seen[key] || platform.IsDeniedCustomHeader(key) || strings.ContainsAny(header.Value, "\r\n") {
				return fmt.Errorf("Unsupported custom header")
			}
			for _, char := range header.Key {
				if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", char)) {
					return fmt.Errorf("Invalid custom header name")
				}
			}
			if _, ok := httpclient.ExpandClientHeaderTemplate(header.Value, nil); !ok {
				return fmt.Errorf("Unsupported client header template")
			}
			seen[key] = true
		}
	}
	if row.ParamOverride != "" {
		var obj map[string]json.RawMessage
		if rejectDuplicateJSONKeys([]byte(row.ParamOverride)) != nil || json.Unmarshal([]byte(row.ParamOverride), &obj) != nil || obj == nil {
			return fmt.Errorf("Parameter override must be a JSON object")
		}
	}
	return nil
}

func validImportedURL(raw string, isProxy bool) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Fragment != "" || u.RawQuery != "" || service.IsForbiddenSiteTargetURL(raw) {
		return false
	}
	if !isProxy && u.User != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https" || isProxy && (u.Scheme == "socks5" || u.Scheme == "socks5h")
}
