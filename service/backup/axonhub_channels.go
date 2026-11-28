package backup

import (
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

type axonHubPlanChannel struct {
	ProtocolOrder   store.DirectProtocolOrder
	ModelProtocols  []AxonHubSourceModelProtocol
	DeclaredFormats map[string]bool
	SourceID        int
	Name            string
	Enabled         bool
	BaseURL         string
	ChatPath        string
	ResponsesPath   string
	MessagesPath    string
	Endpoints       store.DirectEndpoints
	Proxy           bool
	ChannelProxy    string
	CustomHeader    string
	ParamOverride   string
	RequestConfig   store.DirectRequestConfig
	RateLimit       store.DirectRateLimit
	StreamPolicy    string
	Protocols       int
	Tags            []string
	CredentialKeys  []string
	DisabledKeys    map[string]bool
	Provider        string
	OAuthState      *store.DirectOAuthState
	// GCPMaterial is the validated service-account JSON for gemini_vertex.
	GCPMaterial string
	// Entries maps a request model name onto the upstream model a grant to this
	// channel has to send.
	Entries map[string]string
}

// AxonHubSkippedChannel is one source channel this importer will not write,
// named by reason. Only sanitized identifiers appear; a channel name is source
// operator text and is deliberately not echoed.
type AxonHubSkippedChannel struct {
	SourceID int      `json:"sourceId"`
	Type     string   `json:"type"`
	Reasons  []string `json:"reasons"`
}

// uniqueChannelName keeps re-imports stable: the source name is used verbatim
// unless two live channels collide, in which case the source id disambiguates.
func uniqueChannelName(name string, sourceID int, used map[string]int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = fmt.Sprintf("channel-%d", sourceID)
	}
	used[name]++
	if used[name] == 1 {
		return name
	}
	return fmt.Sprintf("%s#%d", name, sourceID)
}

// compileAxonHubChannel returns the direct-grant shape of one source channel,
// the reasons it cannot be represented, and the declared protocols that are
// deliberately left behind.
func compileAxonHubChannel(channel AxonHubSourceChannel) (*axonHubPlanChannel, []string, []string) {
	provider, known := axonHubProviderTypes[channel.Type]
	if !known {
		return nil, []string{"provider_type_unknown"}, nil
	}
	if !provider.Supported {
		return nil, []string{provider.Reason}, nil
	}
	reasons := channelSkipReasons(channel)
	keys, oauthState, gcpMaterial, credentialReasons := compileAxonHubChannelCredentials(channel)
	reasons = append(reasons, credentialReasons...)
	if len(reasons) > 0 {
		return nil, uniquifyReasons(reasons), nil
	}

	baseURL := axonHubChannelBaseURL(channel)
	if strings.HasPrefix(baseURL, "ws://") || strings.HasPrefix(baseURL, "wss://") {
		wsOnly := true
		for _, endpoint := range axonHubMergedEndpoints(channel, provider) {
			wsOnly = wsOnly && (endpoint.APIFormat == "openai/responses_websocket" || endpoint.APIFormat == "openai/responses" && endpoint.Transport == "websocket")
		}
		if !wsOnly {
			return nil, []string{"websocket_base_requires_explicit_transport"}, nil
		}
		// The legacy channel base remains HTTP-shaped; only the explicit WS
		// endpoint can authorize the scheme conversion back at execution.
		baseURL = responsesHTTPAddress(baseURL)
	}
	if problem := validateAxonHubEndpointBase(baseURL); problem != "" {
		return nil, []string{"base_url_" + problem}, nil
	}
	if channel.Type == "cerebras" && strings.TrimSpace(channel.BaseURL) == "" {
		// The Cerebras constructor supplies its primary default internally; the
		// source custom-outbound builder still receives the empty channel base.
		channel.Endpoints = append([]AxonHubSourceEndpoint(nil), channel.Endpoints...)
		for i, endpoint := range channel.Endpoints {
			if strings.TrimSpace(endpoint.BaseURL) != "" {
				continue
			}
			switch endpoint.APIFormat {
			case "gemini/contents", "gemini/embeddings":
				channel.Endpoints[i].BaseURL = "https://generativelanguage.googleapis.com"
			default:
				return nil, []string{"source_custom_endpoint_requires_base_url"}, nil
			}
		}
	}
	channel.BaseURL = baseURL
	protocols, endpoints, problems, protocolResiduals := resolveChannelEndpoints(channel, provider)
	if len(problems) > 0 {
		return nil, problems, nil
	}

	channelProxy := ""
	if channel.Settings.ProxyConfigured {
		if channel.Settings.ProxyURL == "" {
			return nil, []string{"proxy_configuration_unsupported"}, nil
		}
		if !validImportProxyURL(channel.Settings.ProxyURL) || service.IsForbiddenSiteTargetURL(channel.Settings.ProxyURL) {
			return nil, []string{"proxy_url_invalid"}, nil
		}
		channelProxy = channel.Settings.ProxyURL
	}

	compiled := &axonHubPlanChannel{
		ProtocolOrder:   axonHubEndpointOrder(channel, provider),
		ModelProtocols:  channel.Settings.ModelProtocols,
		DeclaredFormats: axonHubDeclaredFormats(channel, provider),
		SourceID:        channel.ID,
		Name:            channel.Name,
		Enabled:         channel.Status == "" || channel.Status == "enabled",
		BaseURL:         baseURL,
		Provider:        channel.Type,
		// The dispatcher uses these as protocol identities. The exact URL is
		// owned by Endpoints, so custom paths cannot accidentally trigger a
		// different protocol's request/response conversion.
		ChatPath:      "/v1/chat/completions",
		ResponsesPath: "/v1/responses",
		MessagesPath:  "/v1/messages",
		Endpoints:     endpoints,
		ChannelProxy:  channelProxy,
		ParamOverride: strings.TrimSpace(channel.Settings.OverrideParameters),
		StreamPolicy:  store.NormalizeDirectStreamPolicy(channel.Policies.Stream),
		RequestConfig: store.DirectRequestConfig{Config: requestoverride.Config{
			Body:              channel.Settings.BodyOverrideOperations,
			Headers:           channel.Settings.HeaderOverrideOperations,
			Options:           channel.Settings.TransformOptions,
			NativeChatMapping: axonHubNativeChatMapping(channel),
		}},
		RateLimit:      channel.Settings.RateLimit,
		Protocols:      protocols,
		Tags:           channel.Tags,
		CredentialKeys: keys,
		OAuthState:     oauthState,
		GCPMaterial:    gcpMaterial,

		DisabledKeys: map[string]bool{},
		Entries:      axonHubChannelEntries(channel),
	}
	for _, disabled := range channel.DisabledAPIKeys {
		if channel.Type == "antigravity" && disabled.Key == channel.Credentials.APIKey {
			compiled.DisabledKeys[""] = true
			continue
		}
		if disabled.Key == "__oauth__" && channel.Credentials.OAuth && len(keys) > 0 {
			compiled.DisabledKeys[keys[0]] = true
			continue
		}
		if disabled.Key != "" {
			compiled.DisabledKeys[disabled.Key] = true
		}
	}
	if compiled.ParamOverride != "" {
		if !validParamOverride(compiled.ParamOverride) {
			return nil, []string{"param_override_invalid"}, nil
		}
	}
	if err := compiled.RequestConfig.Validate(); err != nil {
		return nil, []string{"channel_request_operation_unsupported:" + err.Error()}, nil
	}
	if err := compiled.RequestConfig.ValidateEndpoints(compiled.Endpoints); err != nil {
		return nil, []string{err.Error()}, nil
	}
	return compiled, nil, append(protocolResiduals, channelResiduals(channel)...)
}
