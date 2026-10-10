package backup

import (
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

// Native endpoints own their URL and authentication rules. The returned URL is
// the collection URL; only a model or task identity is appended at execution.
func resolveAxonHubNativeEndpoint(ch AxonHubSourceChannel, protocol int, base, path string, custom bool) (*store.DirectEndpoint, bool, string) {
	ep := &store.DirectEndpoint{Auth: store.DirectAuthBearer}
	switch protocol {
	case protoOllama:
		if strings.Contains(base, "#") {
			return nil, true, "endpoint_url_mode_unsupported"
		}
		ep.URL = strings.TrimSuffix(base, "/") + "/api/chat"
		ep.Profile = "ollama"
		if axonHubOptionalAuth(ch) {
			ep.Auth = store.DirectAuthNone
		}
	case protoMessages:
		if custom || (ch.Type != "anthropic_aws" && ch.Type != "ollama_anthropic") {
			return nil, false, ""
		}
		if ch.Type == "anthropic_aws" {
			ep.URL = normalizeAxonHubBaseURL(base, "") + "/model"
			ep.Profile, ep.ModelPath = "bedrock", true
		} else {
			ep.URL = normalizeAxonHubBaseURL(base, "v1") + "/messages"
			ep.Profile = "ollama-messages"
			if axonHubOptionalAuth(ch) {
				ep.Auth = store.DirectAuthNone
			}
		}
	case protoSeedanceVideo:
		if strings.HasSuffix(base, "##") {
			base = strings.TrimRight(strings.TrimSuffix(base, "##"), "/")
		} else {
			base = normalizeAxonHubBaseURL(base, "v3")
		}
		ep.URL = base + "/contents/generations/tasks"
		ep.Profile = "seedance-video"
	case protoZenmuxVideo:
		if path == "" {
			path = "/videos"
		}
		path = "/" + strings.Trim(path, "/")
		if strings.HasSuffix(base, "##") {
			base = strings.TrimSuffix(base, "##")
		} else if path == "/videos" {
			base = normalizeAxonHubBaseURL(base, "v1")
		} else {
			base = normalizeAxonHubBaseURL(base, "")
		}
		ep.URL, ep.Profile = base+path, "zenmux-video"
	case protoSystemOne:
		version := "v1"
		if path != "" {
			version = ""
		} else {
			path = "/systemone"
		}
		ep.URL = normalizeAxonHubBaseURL(base, version) + path
	case protoAlphaSearch:
		version := "v1"
		if path != "" {
			version = ""
		} else {
			path = "/alpha/search"
		}
		if ch.Type == "codex" {
			if base == "https://api.openai.com/v1" {
				base = "https://chatgpt.com/backend-api/codex#"
			}
			ep.URL, ep.Profile = strings.TrimRight(base, "#/")+path, "codex-alpha-search"
		} else if strings.HasSuffix(base, "##") {
			ep.URL = strings.TrimSuffix(base, "##") + path
		} else {
			ep.URL = normalizeAxonHubBaseURL(base, version) + path
		}
	default:
		return nil, false, ""
	}
	return ep, true, ""
}
