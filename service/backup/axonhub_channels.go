package backup

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/gcpsa"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/service/oauth"
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
	var reasons []string
	provider, known := axonHubProviderTypes[channel.Type]
	if !known {
		return nil, []string{"provider_type_unknown"}, nil
	}
	if !provider.Supported {
		return nil, []string{provider.Reason}, nil
	}
	reasons = append(reasons, channelSkipReasons(channel)...)
	if channel.Credentials.OAuth && !axonHubOAuthProvider(channel.Type) {
		reasons = append(reasons, "oauth_credentials_unsupported")
	}
	if channel.Credentials.OAuth && axonHubOAuthProvider(channel.Type) {
		for _, endpoint := range channel.Endpoints {
			if channel.Type == "codex" && endpoint.APIFormat == "openai/alpha_search" {
				continue
			}
			if (channel.Type == "codex" || channel.Type == "fenno") && (endpoint.APIFormat == "openai/responses" || endpoint.APIFormat == "openai/responses_websocket" || endpoint.APIFormat == "openai/image_generation" || endpoint.APIFormat == "openai/image_edit") {
				continue
			}
			if channel.Type == "github_copilot" {
				if strings.TrimSpace(endpoint.BaseURL) != "" || strings.TrimSpace(endpoint.Path) != "" {
					reasons = append(reasons, "oauth_custom_endpoint_unsupported")
				}
				continue
			}
			if channel.Type == "xai_subscription" {
				if strings.TrimSpace(endpoint.BaseURL) != "" || strings.TrimSpace(endpoint.Path) != "" {
					reasons = append(reasons, "oauth_custom_endpoint_unsupported")
				}
				continue
			}
			// Source custom endpoints use a static API-key provider except the
			// Codex Responses/image wrapper. Do not silently replace that contract.
			reasons = append(reasons, "oauth_custom_endpoint_unsupported")
		}
	}
	if channel.Type == "github_copilot" && !channel.Credentials.OAuth {
		reasons = append(reasons, "copilot_oauth_credential_required")
	}
	if channel.Type == "xai_subscription" {
		if !channel.Credentials.OAuth {
			reasons = append(reasons, "xai_subscription_oauth_credential_required")
		} else if credentials := channel.Credentials.OAuthCredentials; credentials == nil ||
			strings.TrimSpace(credentials.RefreshToken) == "" {
			reasons = append(reasons, "xai_subscription_refresh_token_required")
		} else if clientID := strings.TrimSpace(credentials.ClientID); clientID != "" && clientID != "b1a00492-073a-47ea-816f-4c329264a828" {
			reasons = append(reasons, "xai_subscription_client_id_unsupported")
		}
		if base := strings.TrimSpace(channel.BaseURL); base != "" && base != "https://cli-chat-proxy.grok.com/v1" {
			reasons = append(reasons, "xai_subscription_custom_base_url_unsupported")
		}
	}
	if channel.Credentials.Azure {
		reasons = append(reasons, "azure_credentials_unsupported")
	}
	if channel.Credentials.GCP != nil && channel.Type != "gemini_vertex" && channel.Type != "anthropic_gcp" {
		reasons = append(reasons, "gcp_credentials_unsupported")
	}
	for _, key := range channel.Credentials.UnsupportedFields {
		reasons = append(reasons, "credential_field_unsupported:"+key)
	}
	keys := credentialKeys(channel.Credentials)
	oauthState := axonHubOAuthState(channel.Credentials.OAuthCredentials)
	if channel.Type == "antigravity" {
		if len(channel.Credentials.APIKeys) > 0 || channel.Credentials.OAuth || channel.Credentials.GCP != nil || channel.Credentials.Azure {
			reasons = append(reasons, "antigravity_credential_form_unsupported")
		} else {
			state, err := oauth.ParseAntigravityImportMaterial(channel.Credentials.APIKey)
			if err != nil {
				reasons = append(reasons, "antigravity_refresh_token_project_required")
			} else {
				keys, oauthState = []string{""}, &state
			}
		}
		if len(channel.Endpoints) != 0 {
			reasons = append(reasons, "antigravity_custom_endpoint_unsupported")
		}
	}
	if channel.Credentials.OAuth && axonHubOAuthProvider(channel.Type) && channel.Credentials.OAuthCredentials != nil {
		keys = []string{channel.Credentials.OAuthCredentials.AccessToken}
		if tokenType := strings.ToLower(channel.Credentials.OAuthCredentials.TokenType); tokenType != "" && tokenType != "bearer" {
			reasons = append(reasons, "oauth_token_type_unsupported")
		}
		if channel.Type == "github_copilot" {
			// AxonHub exports the durable GitHub OAuth token. The short-lived
			// Copilot token is exchanged lazily; secret starts empty.
			keys = []string{""}
			oauthState = &store.DirectOAuthState{
				RefreshToken: channel.Credentials.OAuthCredentials.AccessToken,
				ClientID:     channel.Credentials.OAuthCredentials.ClientID,
			}
		}
	}
	if len(keys) == 0 && channel.Credentials.GCP != nil && (channel.Type == "gemini_vertex" || channel.Type == "anthropic_gcp") {
		keys = []string{""}
	}
	if len(keys) == 0 {
		if axonHubOptionalAuth(channel) {
			// An empty secret is the explicit no-auth credential. Never replace
			// configured but disabled keys with an anonymous grant.
			keys = []string{""}
		} else {
			reasons = append(reasons, "credential_missing")
		}
	}
	// gemini_vertex serves bearer tokens exchanged from the SA material only;
	// an API-key-shaped channel would fail at the upstream on every request.
	if (channel.Type == "gemini_vertex" || channel.Type == "anthropic_gcp") && channel.Credentials.GCP == nil {
		reasons = append(reasons, "gcp_credential_missing")
	}
	gcpMaterial := ""
	if gcp := channel.Credentials.GCP; gcp != nil && (channel.Type == "gemini_vertex" || channel.Type == "anthropic_gcp") {
		if _, _, err := gcpsa.Parse(gcp.JSONData); err != nil {
			reasons = append(reasons, "gcp_credential_invalid")
		} else if strings.TrimSpace(gcp.ProjectID) == "" {
			reasons = append(reasons, "gcp_project_missing")
		} else {
			gcpMaterial = gcp.JSONData
		}
	}
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

func axonHubNativeChatMapping(channel AxonHubSourceChannel) bool {
	if len(channel.Settings.TransformOptions.ReasoningEffortMapping) == 0 {
		return false
	}
	for _, endpoint := range channel.Endpoints {
		if endpoint.APIFormat == "openai/chat_completions" {
			return false
		}
	}
	switch channel.Type {
	case "openai", "atlascloud", "deepinfra", "qiniu", "minimax", "ppio", "siliconflow", "vercel",
		"aihubmix", "burncloud", "github", "evolink", "groq":
		return true
	default:
		return false
	}
}

// channelSkipReasons are the source features that change which requests reach
// the provider. Reproducing a channel without them would make its traffic
// behave differently than it did in AxonHub, so the channel is left behind
// under a named reason instead.
func channelSkipReasons(channel AxonHubSourceChannel) []string {
	var reasons []string
	for _, field := range channel.UnsupportedFields {
		reasons = append(reasons, "channel_field_unsupported:"+field)
	}
	for _, field := range channel.Settings.UnsupportedFields {
		reasons = append(reasons, "channel_settings_unsupported:"+field)
	}
	for _, field := range channel.Policies.UnsupportedFields {
		reasons = append(reasons, "channel_policy_unsupported:"+field)
	}
	switch channel.Policies.Stream {
	case "", "unlimited", "require", "forbid":
	default:
		reasons = append(reasons, "channel_stream_policy_unsupported")
	}
	if channel.Settings.HasUnsupportedRateLimit {
		reasons = append(reasons, "channel_rate_limit_unsupported")
	}
	return reasons
}

// channelResiduals are source features this importer deliberately does not
// carry because Metapi owns an equivalent workflow. They are reported so the
// operator knows the channel is not a byte-for-byte copy, but they do not
// change which requests reach the provider, so the channel still imports.
func channelResiduals(channel AxonHubSourceChannel) []string {
	prefix := "channel-" + strconv.Itoa(channel.ID) + ":"
	var notes []string
	add := func(reason string) { notes = append(notes, prefix+reason) }
	if channel.Type == "antigravity" {
		add("source_endpoint_fallback_not_imported")
	}
	if channel.Policies.AutoDisableRuleCount > 0 {
		add("source_auto_disable_rules_replaced_by_grant_cooldown")
	}
	if len(channel.Settings.RetryableStatusCodes) > 0 || channel.Settings.HasRetryableErrorPatterns {
		add("source_retry_policy_replaced_by_gateway_retry_policy")
	}
	if channel.Settings.HasProviderQuota {
		add("provider_quota_polling_not_imported")
	}
	// The management key only feeds provider quota polling, which is already a
	// residual; it is never attached to inference requests, so leaving the
	// channel behind would lose its traffic for no routing benefit.
	if channel.Credentials.HasManagementKey {
		add("management_api_key_not_imported")
	}
	if channel.Settings.PassThroughUserAgent != nil {
		add("pass_through_user_agent_not_configurable")
	}
	if channel.Settings.PassThroughBody != nil {
		add("pass_through_body_not_configurable")
	}
	if channel.AutoSyncSupportedModels || strings.TrimSpace(channel.AutoSyncModelPattern) != "" {
		add("model_auto_sync_replaced_by_model_refresh")
	}
	if len(channel.ManualModels) > 0 {
		add("manual_models_not_imported_until_synced")
	}
	if strings.TrimSpace(channel.DefaultTestModel) != "" && !axonHubUsesCodexImage(channel) {
		add("default_test_model_not_imported")
	}
	return notes
}

// axonHubChannelEntries reproduces AxonHub's Channel.GetModelEntries: the
// request-model names a channel can accept, and the upstream model each one
// resolves to. Re-imports depend on this being the same walk.
func axonHubChannelEntries(channel AxonHubSourceChannel) map[string]string {
	type entry struct {
		actual string
		source string
	}
	entries := map[string]entry{}
	add := func(request, actual, source string) {
		if request == "" {
			return
		}
		if _, exists := entries[request]; exists {
			return
		}
		entries[request] = entry{actual: actual, source: source}
	}
	for _, model := range channel.SupportedModels {
		add(model, model, "direct")
	}
	if prefix := channel.Settings.ExtraModelPrefix; prefix != "" {
		for _, model := range channel.SupportedModels {
			add(prefix+"/"+model, model, "prefix")
		}
	}
	for _, prefix := range channel.Settings.AutoTrimedModelPrefixes {
		if prefix == "" {
			continue
		}
		prefixed := prefix + "/"
		for _, model := range channel.SupportedModels {
			trimmed, ok := strings.CutPrefix(model, prefixed)
			if !ok {
				continue
			}
			add(trimmed, model, "auto_trim")
		}
	}
	supported := make(map[string]bool, len(channel.SupportedModels))
	for _, model := range channel.SupportedModels {
		supported[model] = true
	}
	for _, mapping := range channel.Settings.ModelMappings {
		if mapping.From == "" || !supported[mapping.To] {
			continue
		}
		if _, exists := entries[mapping.From]; exists {
			continue
		}
		add(mapping.From, mapping.To, "mapping")
		if channel.Settings.HideMappedModels {
			for key, value := range entries {
				if value.actual == mapping.To && value.source != "mapping" {
					delete(entries, key)
				}
			}
		}
	}
	if channel.Settings.HideOriginalModels {
		for key, value := range entries {
			if value.source == "direct" {
				delete(entries, key)
			}
		}
	}
	out := make(map[string]string, len(entries))
	if channel.Settings.LowercaseModelID {
		rank := map[string]int{"direct": 4, "auto_trim": 3, "mapping": 2, "prefix": 1}
		best := map[string]entry{}
		for key, value := range entries {
			lower := strings.ToLower(key)
			current, exists := best[lower]
			if !exists || rank[value.source] > rank[current.source] {
				best[lower] = value
			}
		}
		for key, value := range best {
			out[key] = value.actual
		}
		return out
	}
	for key, value := range entries {
		out[key] = value.actual
	}
	return out
}

func credentialKeys(credentials AxonHubSourceCredentials) []string {
	var keys []string
	seen := map[string]bool{}
	add := func(key string) {
		key = strings.TrimSpace(key)
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		keys = append(keys, key)
	}
	if credentials.OAuth || credentials.Azure || credentials.GCP != nil {
		return nil
	}
	add(credentials.APIKey)
	for _, key := range credentials.APIKeys {
		add(key)
	}
	return keys
}
