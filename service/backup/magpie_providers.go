package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

type magpiePlanProvider struct {
	SourceID      int64
	ID            string
	Name          string
	Enabled       bool
	BaseURL       string
	Endpoints     store.DirectEndpoints
	Protocols     int
	Routing       string
	AccountModels map[string][]string
	UseProxy      bool
	ChannelProxy  string
	CustomHeader  string
	Credentials   []magpiePlanCredential
	Models        []string
}

type magpiePlanCredential struct {
	SourceID   int64
	ProviderID string
	Name       string
	Secret     string
	Protocols  int
	Weight     int
	Enabled    bool
}

// MagpieSkippedProvider names a provider this importer will not write. IDs and
// reasons are source identifiers only; names and URLs stay out of the preview.
type MagpieSkippedProvider struct {
	SourceID string   `json:"sourceId"`
	Type     string   `json:"type"`
	Reasons  []string `json:"reasons"`
}

func compileMagpieProvider(src MagpieSourceProvider, usedNames map[string]int, groupModels []string) (*magpiePlanProvider, []string, []string) {
	var reasons, residuals []string
	if !magpieIdentifier(src.ID) {
		return nil, []string{"provider_id_invalid"}, nil
	}
	if src.HasVertex {
		return nil, []string{"vertex_credentials_unsupported"}, nil
	}
	switch src.Routing {
	case "", "order", "rotate", "usage", "pace", "weight":
	default:
		return nil, []string{"provider_routing_unsupported"}, nil
	}
	switch src.Routing {
	case "":
		residuals = append(residuals, "provider_routing_smart_approximated_as_weighted")
	case "usage":
		residuals = append(residuals, "provider_routing_usage_approximated_as_weighted")
	case "pace":
		residuals = append(residuals, "provider_routing_pace_approximated_as_weighted")
	}
	if affinity := strings.ToLower(strings.TrimSpace(src.Affinity)); affinity != "off" {
		residuals = append(residuals, "provider_affinity_not_imported")
	}
	if src.Sink {
		residuals = append(residuals, "provider_rate_limit_sink_not_imported")
	}
	if src.Decide != "" {
		residuals = append(residuals, "decision_api_not_imported")
	}
	if len(src.AccountProxies) > 0 {
		residuals = append(residuals, "per_key_proxies_not_imported")
	}
	if src.HasControlSecrets {
		residuals = append(residuals, "provider_control_credentials_not_imported")
	}
	if src.HasAccountControls {
		residuals = append(residuals, "subscription_account_controls_not_imported")
	}
	for _, context := range src.Contexts {
		if context == -1 {
			residuals = append(residuals, "provider_smallest_context_not_imported")
			break
		}
	}
	if len(src.UnsupportedFields) > 0 {
		residuals = append(residuals, "provider_fields_not_imported")
	}
	endpoints, protocols, reasons, endpointResiduals := magpieEndpoints(src)
	residuals = append(residuals, endpointResiduals...)
	if len(reasons) > 0 {
		return nil, reasons, residuals
	}
	base := magpieChannelBase(endpoints)
	if base == "" {
		return nil, []string{"provider_endpoint_missing"}, residuals
	}
	useProxy, channelProxy, proxyReason := magpieProxy(src)
	if proxyReason != "" {
		return nil, []string{proxyReason}, residuals
	}
	customHeader, headerResiduals := magpieCustomHeaders(src.Headers)
	residuals = append(residuals, headerResiduals...)

	keys := magpieProviderKeys(src)
	if len(keys) == 0 {
		residuals = append(residuals, "provider_has_no_importable_keys")
	}
	provider := &magpiePlanProvider{
		SourceID: sourceTupleID("magpie-provider", src.ID), ID: src.ID,
		Name: uniqueMagpieName(src.Name, src.ID, usedNames), Enabled: !src.Off,
		BaseURL: base, Endpoints: endpoints, Protocols: protocols, Routing: src.Routing,
		AccountModels: src.AccountModels,
		UseProxy:      useProxy, ChannelProxy: channelProxy, CustomHeader: customHeader,
		Models: magpieServedModels(src.Models, groupModels),
	}
	usedCredentialNames := map[string]int{}
	for index, key := range keys {
		protocolMask, ok := magpieKeyProtocol(key.Protocol, protocols)
		if !ok {
			residuals = append(residuals, "provider_key_protocol_unsupported")
			continue
		}
		name := strings.TrimSpace(key.Name)
		if name == "" {
			if index == 0 {
				name = "default"
			} else {
				name = fmt.Sprintf("key-%d", index+1)
			}
		}
		name = uniqueCredentialName(name, index, usedCredentialNames)
		provider.Credentials = append(provider.Credentials, magpiePlanCredential{
			SourceID: sourceTupleID("magpie-credential", src.ID, name), ProviderID: src.ID,
			Name: name, Secret: key.Key, Protocols: protocolMask, Weight: key.Weight,
			Enabled: !src.Off && !key.Off,
		})
	}
	if len(src.AccountModels) > 0 {
		residuals = append(residuals, "per_key_model_scopes_applied")
	}
	return provider, nil, residuals
}

func magpieEndpoints(p MagpieSourceProvider) (store.DirectEndpoints, int, []string, []string) {
	var out store.DirectEndpoints
	mask := 0
	var reasons, residuals []string
	add := func(base, suffix, auth string, modelPath bool, protocol int) {
		if base == "" {
			return
		}
		endpoint, err := buildMagpieEndpoint(base, suffix, auth, modelPath)
		if err != nil {
			reasons = append(reasons, "endpoint_url_invalid")
			return
		}
		switch protocol {
		case store.DirectProtocolChat:
			out.Chat = endpoint
		case store.DirectProtocolResponses:
			out.Responses = endpoint
		case store.DirectProtocolMessages:
			out.Messages = endpoint
		case store.DirectProtocolGemini:
			out.Gemini = endpoint
		}
		mask |= protocol
	}
	add(strings.TrimRight(p.Chat, "/"), "/chat/completions", store.DirectAuthBearer, false, store.DirectProtocolChat)
	add(strings.TrimRight(p.Responses, "/"), "/responses", store.DirectAuthBearer, false, store.DirectProtocolResponses)
	add(magpieAnthropicBase(p.Anthropic), "/v1/messages", store.DirectAuthAPIKey, false, store.DirectProtocolMessages)
	add(magpieGeminiBase(p.Gemini), "", store.DirectAuthGoogle, true, store.DirectProtocolGemini)
	if mask == 0 {
		reasons = append(reasons, "provider_endpoint_missing")
	}
	if p.Anthropic != "" && !strings.HasSuffix(strings.ToLower(hostOf(p.Anthropic)), "anthropic.com") {
		residuals = append(residuals, "anthropic_relay_bearer_header_not_imported")
	}
	var checked store.DirectEndpoints
	encoded, _ := json.Marshal(out)
	if err := checked.Scan(encoded); err != nil {
		return store.DirectEndpoints{}, 0, []string{"endpoint_configuration_invalid"}, residuals
	}
	return checked, mask, uniquifyReasons(reasons), residuals
}

func buildMagpieEndpoint(base, suffix, auth string, modelPath bool) (*store.DirectEndpoint, error) {
	full := strings.TrimRight(strings.TrimSpace(base), "/") + suffix
	u, err := url.Parse(full)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
		return nil, errors.New("invalid endpoint URL")
	}
	if service.IsForbiddenSiteTargetURL(full) {
		return nil, errors.New("forbidden endpoint target")
	}
	return &store.DirectEndpoint{URL: full, Auth: auth, ModelPath: modelPath}, nil
}

func magpieChannelBase(endpoints store.DirectEndpoints) string {
	for _, endpoint := range []*store.DirectEndpoint{endpoints.Chat, endpoints.Responses, endpoints.Messages, endpoints.Gemini} {
		if endpoint == nil || endpoint.URL == "" {
			continue
		}
		base := endpoint.URL
		for _, suffix := range []string{"/chat/completions", "/responses", "/v1/messages"} {
			if cut, ok := strings.CutSuffix(base, suffix); ok {
				base = cut
				break
			}
		}
		return strings.TrimRight(base, "/")
	}
	return ""
}

func magpieAnthropicBase(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	for _, suffix := range []string{"/v1/messages", "/v1"} {
		if cut, ok := strings.CutSuffix(base, suffix); ok && strings.Contains(cut, "://") && len(cut) > len("https://") {
			return cut
		}
	}
	return base
}

func magpieGeminiBase(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base == "" {
		return ""
	}
	if !strings.Contains(base, "://") {
		base = "https://" + base
	}
	if u, err := url.Parse(base); err == nil && u.Host != "" && strings.Trim(u.Path, "/") == "" {
		return base + "/v1beta"
	}
	return base
}

func magpieProxy(p MagpieSourceProvider) (bool, string, string) {
	proxy := strings.TrimSpace(p.Proxy)
	if proxy == "" {
		return true, "", ""
	}
	if proxy == "direct" {
		return false, "", ""
	}
	if !strings.Contains(proxy, "://") {
		proxy = "http://" + proxy
	}
	u, err := url.Parse(proxy)
	if err != nil || u.Host == "" {
		return false, "", "proxy_url_invalid"
	}
	switch u.Scheme {
	case "http", "https", "socks5":
	default:
		return false, "", "proxy_url_invalid"
	}
	return false, proxy, ""
}

func magpieCustomHeaders(headers map[string]string) (string, []string) {
	var out []importHeader
	var residuals []string
	for _, key := range sortedStringKeys(headers) {
		value := headers[key]
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "authorization", "x-api-key", "x-goog-api-key":
			residuals = append(residuals, "auth_override_headers_not_imported")
			continue
		}
		if !validHTTPHeaderName(key) || strings.ContainsAny(value, "\r\n") {
			residuals = append(residuals, "invalid_custom_headers_not_imported")
			continue
		}
		out = append(out, importHeader{Key: key, Value: value})
	}
	if len(out) == 0 {
		return "[]", residuals
	}
	encoded, _ := json.Marshal(out)
	return string(encoded), residuals
}

func magpieProviderKeys(p MagpieSourceProvider) []MagpieSourceKey {
	var out []MagpieSourceKey
	if p.Key != "" {
		out = append(out, MagpieSourceKey{Name: p.KeyName, Key: p.Key, Protocol: p.KeyProtocol, Weight: p.KeyWeight})
	}
	for _, key := range p.Keys {
		if key.Key != "" {
			out = append(out, key)
		}
	}
	return out
}

func magpieKeyProtocol(protocol string, channelMask int) (int, bool) {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	if protocol == "" {
		return channelMask, true
	}
	bit := map[string]int{
		"chat": store.DirectProtocolChat, "responses": store.DirectProtocolResponses,
		"anthropic": store.DirectProtocolMessages, "gemini": store.DirectProtocolGemini,
	}[protocol]
	if bit == 0 || channelMask&bit == 0 {
		return 0, false
	}
	return bit, true
}

func magpieCredentialServes(credential magpiePlanCredential, model string, accountModels map[string][]string) bool {
	allowed := accountModels[magpieKeyFingerprint(credential.Secret)]
	if len(allowed) == 0 {
		return true
	}
	for _, pattern := range allowed {
		if pattern == model || magpieGlobMatch(pattern, model) {
			return true
		}
	}
	return false
}

func magpieContext(p MagpieSourceProvider, model string) *int {
	if p.Contexts == nil {
		return nil
	}
	if value := p.Contexts[model]; value > 0 {
		return &value
	}
	if value := p.Contexts["*"]; value > 0 {
		return &value
	}
	return nil
}

// magpieServedModels keeps the provider's exposed models in their declared
// order and appends the models its groups name that the list left out.
func magpieServedModels(exposed, fromGroups []string) []string {
	seen := make(map[string]bool, len(exposed)+len(fromGroups))
	models := make([]string, 0, len(exposed)+len(fromGroups))
	for _, model := range exposed {
		model = strings.TrimSpace(model)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		models = append(models, model)
	}
	for _, model := range fromGroups {
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		models = append(models, model)
	}
	return models
}

func uniqueCredentialName(name string, index int, used map[string]int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = fmt.Sprintf("key-%d", index+1)
	}
	if used[name] == 0 {
		used[name] = 1
		return name
	}
	for suffix := index + 1; ; suffix++ {
		candidate := fmt.Sprintf("%s#%d", name, suffix)
		if used[candidate] == 0 {
			used[candidate] = 1
			return candidate
		}
	}
}

func validHTTPHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r > 127 || !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", r)) {
			return false
		}
	}
	return true
}

func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func magpieKeyFingerprint(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:5])
}
