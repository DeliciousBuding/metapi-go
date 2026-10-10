package proxyhandler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/platform"
)

func directProxyConfig(raw, channelProxy string, useSystemProxy bool, client http.Header) *platform.ProxyConfig {
	var headers []struct {
		Key   string `json:"header_key"`
		Value string `json:"header_value"`
	}
	if raw != "" && json.Unmarshal([]byte(raw), &headers) != nil {
		return &platform.ProxyConfig{}
	}
	proxyCfg := &platform.ProxyConfig{CustomHeaders: map[string]string{}}
	proxyCfg.ProxyURL = channelProxy
	if proxyCfg.ProxyURL == "" && useSystemProxy {
		if rt := config.RuntimeSafe(); rt != nil {
			proxyCfg.ProxyURL = rt.SystemProxyUrl
			proxyCfg.UseSystemProxy = proxyCfg.ProxyURL != ""
		}
	}
	for _, header := range headers {
		if header.Key == "" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(header.Key)) {
		case "authorization", "x-api-key", "x-goog-api-key":
			// Proxy transports apply custom headers again. Authentication has
			// one owner: the selected endpoint and freshly resolved credential.
			continue
		}
		value, ok := httpclient.ExpandClientHeaderTemplate(header.Value, client)
		if !ok {
			continue
		}
		proxyCfg.CustomHeaders[header.Key] = value
	}
	return proxyCfg
}

func applyDirectParamOverrides(body []byte, raw string) ([]byte, error) {
	if raw == "" {
		return body, nil
	}
	var overrides map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &overrides); err != nil || overrides == nil {
		return nil, fmt.Errorf("invalid parameter overrides")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return nil, fmt.Errorf("invalid request body")
	}
	for key, value := range overrides {
		if key == "model" || key == "stream" {
			continue
		}
		payload[key] = value
	}
	return json.Marshal(payload)
}
