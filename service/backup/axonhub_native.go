package backup

import (
	"github.com/deliciousbuding/metapi-go/service"
	"strings"

	"github.com/deliciousbuding/metapi-go/store"
)

// Native endpoints own their URL and authentication rules. The returned URL is
// the collection URL; only a model or task identity is appended at execution.
func resolveAxonHubNativeEndpoint(ch AxonHubSourceChannel, protocol int, base, path string, custom bool) (*store.DirectEndpoint, bool, string) {
	ep := &store.DirectEndpoint{Auth: store.DirectAuthBearer}
	switch protocol {
	case protoDecisions:
		// This custom format was audited at AxonHub 8d3c3865; the provider
		// default-format snapshot remains independently frozen in its owner.
		// The source's Decisions constructor is generic JSON, not the Codex
		// generation wrapper. Raw URL mode ignores even an explicit path.
		if strings.HasSuffix(base, "##") {
			ep.URL = strings.TrimRight(strings.TrimSuffix(base, "##"), "/")
		} else if path != "" {
			ep.URL = normalizeAxonHubBaseURL(base, "") + path
		} else {
			ep.URL = normalizeAxonHubBaseURL(base, "v1") + "/decisions"
		}
	case protoResponsesCompact:
		// The source uses the generic Responses compact constructor, including
		// for custom compact endpoints on Codex. It does not use the Codex
		// generation wrapper or silently inherit its OAuth wire profile.
		if strings.HasSuffix(base, "##") {
			ep.URL = strings.TrimSuffix(base, "##")
		} else {
			version := "v1"
			if path != "" {
				version = ""
			}
			ep.URL = normalizeAxonHubBaseURL(base, version) + "/responses/compact"
		}
	case protoGemini, protoGeminiEmbeddings:
		if ch.Type == "antigravity" {
			if custom || protocol != protoGemini || path != "" {
				return nil, true, "antigravity_custom_endpoint_unsupported"
			}
			switch strings.TrimSuffix(base, "/") {
			case "https://daily-cloudcode-pa.sandbox.googleapis.com", "https://autopush-cloudcode-pa.sandbox.googleapis.com", "https://cloudcode-pa.googleapis.com":
			default:
				return nil, true, "antigravity_origin_unsupported"
			}
			ep.URL, ep.Profile = strings.TrimSuffix(base, "/")+"/v1internal", "antigravity"
			return ep, true, ""
		}
		if ch.Type != "gemini_vertex" || custom {
			return nil, false, ""
		}
		if ch.Credentials.GCP == nil {
			return nil, true, "gcp_credentials_unsupported"
		}
		url, err := service.VertexEndpointURL(base, ch.Credentials.GCP.ProjectID, "google")
		if err != nil {
			return nil, true, "gcp_endpoint_underivable"
		}
		ep.URL = url
		ep.Profile = "vertex"
		ep.ModelPath = true
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
		if ch.Type == "anthropic_gcp" && !custom {
			if ch.Credentials.GCP == nil {
				return nil, true, "gcp_credentials_unsupported"
			}
			url, err := service.VertexEndpointURL(base, ch.Credentials.GCP.ProjectID, "anthropic")
			if err != nil {
				return nil, true, "gcp_endpoint_underivable"
			}
			ep.URL = url
			ep.Profile = "vertex-anthropic"
			ep.ModelPath = true
			return ep, true, ""
		}
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
