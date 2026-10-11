package proxyhandler

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
)

// Direct attempts deliberately keep the shared transport/content fallback
// disabled. Only a complete HTTP rejection enters this gate. The candidate
// list already applies the operator's disable switch and grant/order limits.
func shouldContinueEndpointResponseFallback(selected *routing.SelectedChannel, status int, body string, isLastEndpoint, disableCrossProtocolFallback, readable bool) bool {
	if selected.Direct == nil {
		return shouldContinueEndpointFallback(status, body, isLastEndpoint, disableCrossProtocolFallback, endpointFailureResponse)
	}
	if isLastEndpoint || !readable {
		return false
	}
	return directProtocolRejection(status, body)
}

var directProtocolMiss = regexp.MustCompile(`(?i)(?:unsupported|unknown|unrecognized)\s+(?:endpoint|path|protocol|model)|(?:endpoint|protocol)\s+(?:not\s+found|not\s+supported)|unrecognized\s+request\s+url|please\s+use\s+/(?:v1/)?(?:chat/completions|responses|messages)|model[^\r\n]{0,120}?(?:not\s+supported|is\s+not\s+supported|does\s+not\s+exist|not\s+found)|does\s+not\s+support(?:\s+the)?\s+model|no\s+such\s+model|不支持[^\r\n]{0,80}模型|模型[^\r\n]{0,80}不支持`)
var directProtocolAbort = regexp.MustCompile(`(?i)authenticat|authoriz|api\s*key|access\s*token|permission|forbidden|access\s+denied|do\s+not\s+have\s+access|quota|rate\s*limit|too\s+many\s+requests|余额|额度|限流|无权限`)

// New API's RelayNotFound identifies the rejected method and API path. Do not
// treat an unrelated 404 (such as a reverse proxy's HTML page) as this signal.
var directNewAPIPathMiss = regexp.MustCompile(`(?i)invalid\s+url\s*\(post\s+/(?:v1/(?:chat/completions|responses|messages)|v1beta/models/[^\s)]+:(?:streamGenerateContent|generateContent))\)`)

func directProtocolRejection(status int, body string) bool {
	switch status {
	case 400, 404, 405, 415, 422, 501:
	default:
		return false
	}
	text := strings.TrimSpace(body)
	if text == "" || strings.HasPrefix(text, "<") {
		return false
	}
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		var payload map[string]any
		decoder := json.NewDecoder(strings.NewReader(text))
		decoder.UseNumber()
		if !json.Valid([]byte(text)) || decoder.Decode(&payload) != nil || payload == nil || directResponseHasActivity(payload) {
			return false
		}
		var hints []string
		for _, key := range []string{"error", "errors", "code", "type", "message", "detail"} {
			directErrorHints(payload[key], &hints, 0)
		}
		text = strings.Join(hints, " ")
	}
	pathMiss := directNewAPIPathMiss.MatchString(text)
	text = strings.NewReplacer("_", " ", "-", " ").Replace(text)
	return !directProtocolAbort.MatchString(text) && (directProtocolMiss.MatchString(text) || pathMiss)
}

func directErrorHints(value any, hints *[]string, depth int) {
	if depth > 4 {
		return
	}
	switch v := value.(type) {
	case string:
		*hints = append(*hints, v)
	case map[string]any:
		for _, key := range []string{"code", "type", "message", "detail", "error"} {
			directErrorHints(v[key], hints, depth+1)
		}
	case []any:
		for _, item := range v {
			directErrorHints(item, hints, depth+1)
		}
	}
}

func directResponseHasActivity(payload map[string]any) bool {
	for _, key := range []string{"usage", "usageMetadata", "cost", "total_cost"} {
		if directUsageHasActivity(payload[key]) {
			return true
		}
	}
	for _, key := range []string{"choices", "output", "candidates", "content"} {
		switch value := payload[key].(type) {
		case nil:
		case []any:
			if len(value) != 0 {
				return true
			}
		case string:
			if value != "" {
				return true
			}
		default:
			return true
		}
	}
	for _, key := range []string{"response", "message", "data"} {
		if nested, ok := payload[key].(map[string]any); ok && directResponseHasActivity(nested) {
			return true
		}
	}
	return false
}

func directUsageHasActivity(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case json.Number:
		// The decoder already validated the number. Its mantissa is zero iff
		// every digit is zero, regardless of exponent; avoid float underflow
		// and unbounded big-number allocation for untrusted exponents.
		number := string(v)
		if exponent := strings.IndexAny(number, "eE"); exponent >= 0 {
			number = number[:exponent]
		}
		return strings.ContainsAny(number, "123456789")
	case map[string]any:
		for _, child := range v {
			if directUsageHasActivity(child) {
				return true
			}
		}
		return false
	default:
		// An unknown/malformed usage value is not evidence of zero cost.
		return true
	}
}
