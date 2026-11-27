package backup

import (
	"encoding/json"
	"net/url"
	"strings"
)

func validParamOverride(raw string) bool {
	var value map[string]json.RawMessage
	return json.Unmarshal([]byte(raw), &value) == nil && value != nil
}

func safeDirectEndpointPath(path string) bool {
	if strings.TrimSpace(path) == "" {
		return true
	}
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "\\") || strings.Contains(path, "..") {
		return false
	}
	u, err := url.Parse(path)
	return err == nil && !u.IsAbs() && u.Host == "" && u.RawQuery == "" && u.Fragment == ""
}

func validImportProxyURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	switch u.Scheme {
	case "http", "https", "socks5", "socks5h":
		return true
	}
	return false
}

type importHeader struct {
	Key   string `json:"header_key"`
	Value string `json:"header_value"`
}
