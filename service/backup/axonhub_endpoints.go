package backup

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func axonHubChannelBaseURL(channel AxonHubSourceChannel) string {
	if base := strings.TrimSpace(channel.BaseURL); base != "" {
		if (channel.Type == "codex" || channel.Type == "fenno") && base == "https://api.openai.com/v1" {
			return "https://chatgpt.com/backend-api/codex#"
		}
		return base
	}
	// Only defaults actually supplied by channel_llm.go / the primary
	// transformer are used. Custom endpoint overrides retain this base.
	switch channel.Type {
	case "gemini", "zenmux_gemini":
		return "https://generativelanguage.googleapis.com"
	case "codex", "fenno":
		return "https://chatgpt.com/backend-api/codex#"
	case "claudecode":
		return "https://api.anthropic.com/v1"
	case "zenmux", "zenmux_responses":
		return "https://zenmux.ai/api/v1"
	case "zenmux_anthropic":
		return "https://zenmux.ai/api/anthropic"
	case "fireworks":
		return "https://api.fireworks.ai/inference/v1"
	case "xai":
		return "https://api.x.ai/v1"
	default:
		return ""
	}
}

func validateAxonHubEndpointBase(base string) string {
	// These are AxonHub URL-mode suffixes, not URL fragments.
	if strings.HasSuffix(base, "##") {
		base = strings.TrimSuffix(base, "##")
	} else {
		base = strings.TrimSuffix(base, "#")
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || strings.ContainsAny(base, "?#") {
		return "invalid"
	}
	if service.IsForbiddenSiteTargetURL(base) {
		return "target_forbidden"
	}
	return ""
}

// resolveChannelEndpoints mirrors ResolveEndpoints: defaults survive and each
// custom api_format replaces only its own endpoint. Resolve URLs here, once,
// using the actual outbound constructors at the audited source revision.
func resolveChannelEndpoints(channel AxonHubSourceChannel, provider axonHubProviderType) (int, store.DirectEndpoints, []string, []string) {
	var endpoints store.DirectEndpoints
	resolvedEndpoints := map[string]*store.DirectEndpoint{}
	endpointKeys := map[int]string{}
	for _, entry := range endpoints.Entries() {
		endpointKeys[entry.Protocol] = entry.Key
	}
	custom := map[string]bool{}
	for _, endpoint := range channel.Endpoints {
		format := strings.TrimSpace(endpoint.APIFormat)
		custom[format] = true
	}
	protocols := 0
	var residuals []string
	for _, ep := range axonHubMergedEndpoints(channel, provider) {
		format := ep.APIFormat
		protocol, servable := axonHubServableFormats[format]
		if !servable {
			if reason, known := axonHubResidualProtocolFormats[format]; known {
				residuals = append(residuals, "declared_protocol_not_servable:"+format+" ("+reason+")")
				continue
			}
			return 0, endpoints, []string{"api_format_unsupported"}, nil
		}
		if endpointKeys[protocol] == "" {
			return 0, endpoints, []string{"api_format_unsupported"}, nil
		}
		base := strings.TrimSpace(ep.BaseURL)
		if base == "" {
			base = channel.BaseURL
		}
		if ep.Transport == "websocket" || strings.HasPrefix(strings.ToLower(base), "ws://") || strings.HasPrefix(strings.ToLower(base), "wss://") {
			return 0, endpoints, []string{"websocket_transport_unsupported"}, nil
		}
		if problem := validateAxonHubEndpointBase(base); problem != "" {
			return 0, endpoints, []string{"endpoint_base_url_" + problem}, nil
		}
		if (channel.Type == "commandcode" || channel.Type == "commandcode_anthropic") && !strings.HasPrefix(base, "https://") {
			return 0, endpoints, []string{"provider_requires_https"}, nil
		}
		path := strings.TrimSpace(ep.Path)
		if !safeDirectEndpointPath(path) {
			return 0, endpoints, []string{"endpoint_path_invalid"}, nil
		}
		if protocol&store.DirectGenerationProtocols == 0 {
			endpoint, problem := resolveAxonHubMediaEndpoint(channel, protocol, base, path, custom[format])
			if problem != "" {
				return 0, endpoints, []string{problem}, nil
			}
			if problem := validateAxonHubEndpointBase(endpoint.URL); problem != "" {
				return 0, endpoints, []string{"endpoint_url_" + problem}, nil
			}
			resolvedEndpoints[endpointKeys[protocol]] = endpoint
			protocols |= protocol
			continue
		}
		profile := ""
		if protocol == protoChat && !custom[format] {
			switch channel.Type {
			case "deepseek":
				profile = "deepseek"
			case "zai", "zhipu", "xiaomi":
				profile = "zai"
			}
		}
		if protocol == protoResponses && (channel.Type == "codex" || channel.Type == "fenno") {
			if base == "https://api.openai.com/v1" {
				base = "https://chatgpt.com/backend-api/codex#"
			}
			profile = "codex"
			path = ""
		}
		if protocol == protoMessages && channel.Type == "claudecode" {
			if !custom[format] {
				profile = "claudecode"
			} else if channel.Credentials.OAuth {
				return 0, endpoints, []string{"claudecode_oauth_custom_endpoint_unsupported"}, nil
			}
		}
		if !safeDirectEndpointPath(path) {
			return 0, endpoints, []string{"endpoint_path_invalid"}, nil
		}
		resolved, problem := resolveAxonHubEndpointURL(channel.Type, protocol, base, path, custom[format])
		if problem != "" {
			return 0, endpoints, []string{problem}, nil
		}
		if problem := validateAxonHubEndpointBase(resolved); problem != "" {
			return 0, endpoints, []string{"endpoint_url_" + problem}, nil
		}
		auth := store.DirectAuthBearer
		if protocol == protoMessages {
			auth = store.DirectAuthAPIKey
			// Custom Messages endpoints use PlatformDirect, except Command Code.
			if channel.Type == "commandcode" || channel.Type == "commandcode_anthropic" ||
				profile == "claudecode" ||
				(!custom[format] && (channel.Type == "ollama_anthropic" || channel.Type == "longcat_anthropic")) {
				auth = store.DirectAuthBearer
			}
		}
		endpoint := &store.DirectEndpoint{URL: resolved, Auth: auth, Profile: profile}
		if protocol == protoGemini {
			endpoint.Auth = store.DirectAuthGoogle
			endpoint.ModelPath = path == ""
		}
		resolvedEndpoints[endpointKeys[protocol]] = endpoint
		protocols |= protocol
	}
	if protocols == 0 {
		return 0, endpoints, []string{"no_servable_protocol"}, nil
	}
	raw, err := json.Marshal(resolvedEndpoints)
	if err != nil || endpoints.Scan(raw) != nil {
		return 0, store.DirectEndpoints{}, []string{"endpoint_config_invalid"}, nil
	}
	return protocols, endpoints, nil, residuals
}

func resolveAxonHubEndpointURL(provider string, protocol int, base, path string, custom bool) (string, string) {
	if protocol == protoGemini {
		if strings.Contains(base, "#") {
			return "", "endpoint_url_mode_unsupported"
		}
		version := "v1beta"
		if strings.HasSuffix(base, "/v1beta") {
			base = strings.TrimSuffix(base, "/v1beta")
		} else if strings.HasSuffix(base, "/v1") {
			version = "v1"
			base = strings.TrimSuffix(base, "/v1")
		}
		if path != "" {
			return strings.TrimSuffix(base, "/") + path, ""
		}
		return strings.TrimSuffix(base, "/") + "/" + version + "/models", ""
	}
	version := "v1"
	if protocol == protoChat {
		switch provider {
		case "zai", "zhipu", "zai_anthropic", "zhipu_anthropic":
			if !custom || strings.Contains(strings.TrimSuffix(base, "##")+"/", "/v4/") {
				version = "v4"
			}
		case "doubao", "volcengine", "doubao_anthropic", "volcengine_anthropic":
			if !custom || strings.Contains(strings.TrimSuffix(base, "##")+"/", "/v3/") {
				version = "v3"
			}
		case "gemini_openai":
			if !custom {
				version = "v1beta/openai"
			}
		case "openrouter":
			if !custom {
				// The primary transformer appends directly to the supplied base.
				if strings.HasSuffix(base, "#") {
					return "", "endpoint_url_mode_unsupported"
				}
				return strings.TrimSuffix(base, "/") + "/chat/completions", ""
			}
		}
	}
	if strings.HasSuffix(base, "##") {
		// Anthropic has no raw-URL mode; the Gemini/DeepSeek/Moonshot primary
		// wrappers own their URLs and do not implement the OpenAI raw mode.
		if protocol == protoMessages || (!custom && protocol == protoChat &&
			(provider == "gemini_openai" || provider == "deepseek" || provider == "moonshot")) {
			return "", "endpoint_url_mode_unsupported"
		}
		return strings.TrimRight(strings.TrimSuffix(base, "##"), "/"), ""
	}
	if path != "" {
		version = ""
	}
	base = normalizeAxonHubBaseURL(base, version)
	if path == "" {
		switch protocol {
		case protoChat:
			path = "/chat/completions"
		case protoResponses:
			path = "/responses"
		case protoMessages:
			path = "/messages"
		}
	}
	return base + path, ""
}

// Mirror the source's version-segment and trailing marker rules, rather than
// applying Metapi's independent base/path joining conventions.
func normalizeAxonHubBaseURL(base, version string) string {
	if strings.HasSuffix(base, "#") {
		return strings.TrimRight(strings.TrimSuffix(base, "#"), "/")
	}
	if version == "" || strings.HasSuffix(base, "/"+version) || strings.Contains(base, "/"+version+"/") {
		return strings.TrimRight(base, "/")
	}
	return strings.TrimRight(base, "/") + "/" + version
}
