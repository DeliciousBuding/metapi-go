package proxyhandler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"strings"
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
		value, ok := expandClientHeaderTemplate(header.Value, client)
		if !ok {
			continue
		}
		proxyCfg.CustomHeaders[header.Key] = value
	}
	return proxyCfg
}

func expandClientHeaderTemplate(value string, client http.Header) (string, bool) {
	const marker = "{client_header:"
	var out strings.Builder
	for {
		relative := strings.Index(strings.ToLower(value), marker)
		if relative < 0 {
			out.WriteString(value)
			return out.String(), true
		}
		out.WriteString(value[:relative])
		nameStart := relative + len(marker)
		endRel := strings.IndexByte(value[nameStart:], '}')
		if endRel < 0 {
			return "", false
		}
		name := strings.TrimSpace(value[nameStart : nameStart+endRel])
		if !safeDirectClientHeaderName(name) {
			return "", false
		}
		out.WriteString(client.Get(name))
		value = value[nameStart+endRel+1:]
	}
}

func safeDirectClientHeaderName(name string) bool {
	switch strings.ToLower(name) {
	case "idempotency-key", "openai-beta", "x-request-id", "x-correlation-id", "traceparent", "tracestate":
		return true
	default:
		return false
	}
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
