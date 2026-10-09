package backup

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// axonHubPlan is the single compiled result shared by the preview and the
// transaction executor. Both paths render exactly what this produced, so a
// preview can never claim a channel the import will not write.
type axonHubPlan struct {
	channels    []axonHubPlanChannel
	credentials []axonHubPlanCredential
	models      []axonHubPlanModel
	grants      []axonHubPlanGrant
	routes      []axonHubPlanRoute

	notImported map[string]int
	skipped     []AxonHubSkippedChannel
	residuals   []string
	// blocking holds reasons this backup cannot be imported at all. Skip
	// reasons stay per channel so a partial import is still reviewable.
	blocking []string
}

type axonHubPlanChannel struct {
	SourceID       int
	Name           string
	Enabled        bool
	BaseURL        string
	ChatPath       string
	ResponsesPath  string
	MessagesPath   string
	Endpoints      store.DirectEndpoints
	Proxy          bool
	ChannelProxy   string
	CustomHeader   string
	ParamOverride  string
	Protocols      int
	Tags           []string
	CredentialKeys []string
	DisabledKeys   map[string]bool
	// Entries maps a request model name onto the upstream model a grant to this
	// channel has to send.
	Entries map[string]string
}

type axonHubPlanCredential struct {
	ChannelSourceID int
	Name            string
	Secret          string
	Enabled         bool
}

type axonHubPlanModel struct {
	ChannelSourceID int
	Name            string
	Enabled         bool
}

type axonHubPlanGrant struct {
	ChannelSourceID int
	ModelName       string
	CredentialName  string
	Protocols       int
	Enabled         bool
}

type axonHubPlanRoute struct {
	SourceModelID int
	Pattern       string
	GroupKey      string
	Enabled       bool
	Strategy      string
	Items         []axonHubPlanRouteItem
}

type axonHubPlanRouteItem struct {
	Key             string
	ChannelSourceID int
	ModelName       string
	CredentialName  string
	Priority        int
}

// AxonHubSkippedChannel is one source channel this importer will not write,
// named by reason. Only sanitized identifiers appear; a channel name is source
// operator text and is deliberately not echoed.
type AxonHubSkippedChannel struct {
	SourceID int      `json:"sourceId"`
	Type     string   `json:"type"`
	Reasons  []string `json:"reasons"`
}

// CompileAxonHubPlan turns a parsed backup into the exact channel graph the
// executor will persist. It never guesses: a feature it cannot reproduce
// excludes the channel that carries it and says why.
func CompileAxonHubPlan(src *AxonHubSource) (*axonHubPlan, error) {
	if src == nil {
		return nil, axonHubErr("AxonHub source is missing")
	}
	plan := &axonHubPlan{
		notImported: map[string]int{},
	}
	channels := make([]*axonHubPlanChannel, 0, len(src.Channels))
	bySourceID := make(map[int]*axonHubPlanChannel, len(src.Channels))
	usedChannelNames := map[string]int{}

	for _, channel := range src.Channels {
		if channel.DeletedAt != 0 {
			plan.notImported["deletedChannels"]++
			continue
		}
		compiled, reasons, protocolResiduals := compileAxonHubChannel(channel)
		plan.residuals = append(plan.residuals, protocolResiduals...)
		if len(reasons) > 0 {
			plan.skipped = append(plan.skipped, AxonHubSkippedChannel{
				SourceID: channel.ID,
				Type:     sanitizeIdentifier(channel.Type),
				Reasons:  reasons,
			})
			continue
		}
		compiled.Name = uniqueChannelName(compiled.Name, channel.ID, usedChannelNames)
		channels = append(channels, compiled)
		bySourceID[channel.ID] = compiled
	}
	if len(channels) == 0 {
		plan.blocking = append(plan.blocking, "no_servable_channels")
	}

	plan.channels = derefChannels(channels)
	for _, channel := range plan.channels {
		for i, secret := range channel.CredentialKeys {
			plan.credentials = append(plan.credentials, axonHubPlanCredential{
				ChannelSourceID: channel.SourceID,
				Name:            credentialName(i, len(channel.CredentialKeys)),
				Secret:          secret,
				Enabled:         !channel.DisabledKeys[secret],
			})
		}
	}

	routes, residuals := compileAxonHubRoutes(src, channels, bySourceID)
	plan.routes = routes
	plan.residuals = append(plan.residuals, residuals...)
	sort.Strings(plan.residuals)

	usedModels := map[string]bool{}
	// A grant is one (channel, model, credential) triple: upstream_grants is
	// unique on (model_id, credential_id), so two models on one channel each
	// need their own row even when they share a key.
	usedGrants := map[string]bool{}
	for _, route := range plan.routes {
		if !route.Enabled {
			plan.notImported["disabledModelRoutes"]++
		}
		for _, item := range route.Items {
			modelKey := fmt.Sprintf("%d\x00%s", item.ChannelSourceID, item.ModelName)
			if !usedModels[modelKey] {
				usedModels[modelKey] = true
				channel := bySourceID[item.ChannelSourceID]
				plan.models = append(plan.models, axonHubPlanModel{
					ChannelSourceID: item.ChannelSourceID,
					Name:            item.ModelName,
					Enabled:         channel.Enabled,
				})
			}
			grantKey := fmt.Sprintf("%d\x00%s\x00%s", item.ChannelSourceID, item.ModelName, item.CredentialName)
			if usedGrants[grantKey] {
				continue
			}
			usedGrants[grantKey] = true
			channel := bySourceID[item.ChannelSourceID]
			plan.grants = append(plan.grants, axonHubPlanGrant{
				ChannelSourceID: item.ChannelSourceID,
				ModelName:       item.ModelName,
				CredentialName:  item.CredentialName,
				Protocols:       channel.Protocols,
				Enabled:         true,
			})
		}
	}

	plan.notImported["projects"] = len(src.Projects)
	plan.notImported["apiKeys"] = len(src.APIKeys)
	plan.notImported["projectsWithProfiles"] = countProjectsWithProfiles(src.Projects)
	plan.notImported["apiKeysWithProfiles"] = countAPIKeysWithProfiles(src.APIKeys)
	plan.notImported["apiKeysWithQuota"] = countAPIKeysWithQuota(src.APIKeys)
	plan.notImported["modelPrices"] = len(src.ChannelModelPrices)
	plan.notImported["systemConfigs"] = len(src.SystemConfigs)
	plan.notImported["usageRequests"] = src.UsageRequests
	plan.notImported["usageLogs"] = src.UsageLogs
	for section, count := range src.UnknownSections {
		plan.notImported["section:"+section] += count
	}
	dropZeroCounts(plan.notImported)
	return plan, nil
}

func derefChannels(channels []*axonHubPlanChannel) []axonHubPlanChannel {
	out := make([]axonHubPlanChannel, 0, len(channels))
	for _, channel := range channels {
		out = append(out, *channel)
	}
	return out
}

func dropZeroCounts(counts map[string]int) {
	for key, count := range counts {
		if count == 0 {
			delete(counts, key)
		}
	}
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

func credentialName(index, total int) string {
	if total <= 1 {
		return "default"
	}
	return fmt.Sprintf("key-%d", index+1)
}

func countProjectsWithProfiles(projects []AxonHubSourceProject) int {
	count := 0
	for _, project := range projects {
		if project.ProfileRows > 0 {
			count++
		}
	}
	return count
}

func countAPIKeysWithProfiles(keys []AxonHubSourceAPIKey) int {
	count := 0
	for _, key := range keys {
		if key.ProfileRows > 0 || len(key.Scopes) > 0 || key.AllowedIPs > 0 {
			count++
		}
	}
	return count
}

func countAPIKeysWithQuota(keys []AxonHubSourceAPIKey) int {
	count := 0
	for _, key := range keys {
		if key.HasQuota {
			count++
		}
	}
	return count
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
	if channel.Credentials.OAuth {
		reasons = append(reasons, "oauth_credentials_unsupported")
	}
	if channel.Credentials.Azure {
		reasons = append(reasons, "azure_credentials_unsupported")
	}
	if channel.Credentials.GCP {
		reasons = append(reasons, "gcp_credentials_unsupported")
	}
	for _, key := range channel.Credentials.UnsupportedFields {
		reasons = append(reasons, "credential_field_unsupported:"+key)
	}
	keys := credentialKeys(channel.Credentials)
	if len(keys) == 0 {
		reasons = append(reasons, "credential_missing")
	}
	if len(reasons) > 0 {
		return nil, uniquifyReasons(reasons), nil
	}

	baseURL := axonHubChannelBaseURL(channel)
	if problem := validateAxonHubEndpointBase(baseURL); problem != "" {
		return nil, []string{"base_url_" + problem}, nil
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
		if !validAxonHubProxyURL(channel.Settings.ProxyURL) {
			return nil, []string{"proxy_url_invalid"}, nil
		}
		channelProxy = channel.Settings.ProxyURL
	}

	compiled := &axonHubPlanChannel{
		SourceID: channel.ID,
		Name:     channel.Name,
		Enabled:  channel.Status == "" || channel.Status == "enabled",
		BaseURL:  baseURL,
		// The dispatcher uses these as protocol identities. The exact URL is
		// owned by Endpoints, so custom paths cannot accidentally trigger a
		// different protocol's request/response conversion.
		ChatPath:       "/v1/chat/completions",
		ResponsesPath:  "/v1/responses",
		MessagesPath:   "/v1/messages",
		Endpoints:      endpoints,
		ChannelProxy:   channelProxy,
		ParamOverride:  strings.TrimSpace(channel.Settings.OverrideParameters),
		Protocols:      protocols,
		Tags:           channel.Tags,
		CredentialKeys: keys,
		DisabledKeys:   map[string]bool{},
		Entries:        axonHubChannelEntries(channel),
	}
	for _, disabled := range channel.DisabledAPIKeys {
		if disabled.Key != "" {
			compiled.DisabledKeys[disabled.Key] = true
		}
	}
	if compiled.ParamOverride != "" {
		if !validParamOverride(compiled.ParamOverride) {
			return nil, []string{"param_override_invalid"}, nil
		}
	}
	return compiled, nil, append(protocolResiduals, channelResiduals(channel)...)
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
	case "", "unlimited":
	default:
		reasons = append(reasons, "channel_stream_policy_unsupported")
	}
	if channel.Settings.HasBodyOverrideOperations {
		reasons = append(reasons, "channel_body_override_unsupported")
	}
	if channel.Settings.HasHeaderOverrideOperations {
		reasons = append(reasons, "channel_header_override_unsupported")
	}
	if channel.Settings.HasTransformOptions {
		reasons = append(reasons, "channel_transform_options_unsupported")
	}
	if channel.Settings.HasRateLimit {
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
	if channel.Policies.AutoDisableRuleCount > 0 {
		add("source_auto_disable_rules_replaced_by_grant_cooldown")
	}
	if len(channel.Settings.RetryableStatusCodes) > 0 || channel.Settings.HasRetryableErrorPatterns {
		add("source_retry_policy_replaced_by_gateway_retry_policy")
	}
	if channel.Settings.HasProviderQuota {
		add("provider_quota_polling_not_imported")
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
	if strings.TrimSpace(channel.DefaultTestModel) != "" {
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
	if credentials.OAuth || credentials.Azure || credentials.GCP {
		return nil
	}
	add(credentials.APIKey)
	for _, key := range credentials.APIKeys {
		add(key)
	}
	return keys
}

func validParamOverride(raw string) bool {
	var value map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &value); err != nil || value == nil {
		return false
	}
	return true
}

func validAxonHubProxyURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.Fragment != "" || parsed.RawQuery != "" {
		return false
	}
	switch parsed.Scheme {
	case "http", "https", "socks5", "socks5h":
	default:
		return false
	}
	return !service.IsForbiddenSiteTargetURL(raw)
}

func safeDirectEndpointPath(path string) bool { return safeOctopusEndpointPath(path) }

func uniquifyReasons(reasons []string) []string {
	seen := map[string]bool{}
	out := reasons[:0]
	for _, reason := range reasons {
		if seen[reason] {
			continue
		}
		seen[reason] = true
		out = append(out, reason)
	}
	return out
}

// compileAxonHubRoutes resolves every model's associations into concrete
// channel/model candidates and emits one exact route per requested model name.
func compileAxonHubRoutes(src *AxonHubSource, channels []*axonHubPlanChannel, bySourceID map[int]*axonHubPlanChannel) ([]axonHubPlanRoute, []string) {
	var routes []axonHubPlanRoute
	var residuals []string
	for _, model := range src.Models {
		if model.DeletedAt != 0 {
			continue
		}
		pattern := strings.TrimSpace(model.ModelID)
		if pattern == "" {
			continue
		}
		if !routing.IsExactRouteModelPattern(pattern) {
			residuals = append(residuals, "model_pattern_not_exact:"+sanitizeIdentifier(pattern))
			continue
		}
		if len(model.Settings.UnsupportedFields) > 0 {
			residuals = append(residuals, "model_settings_unsupported:"+sanitizeIdentifier(pattern))
			continue
		}
		switch model.Type {
		case "", "chat":
		default:
			residuals = append(residuals, "model_type_not_servable:"+sanitizeIdentifier(model.Type))
			continue
		}

		items := map[string]axonHubPlanRouteItem{}
		var order []string
		for _, assoc := range axonHubEffectiveAssociations(src, model) {
			if assoc.Disabled {
				continue
			}
			if assoc.UnsupportedField != "" {
				residuals = append(residuals, "model_association_field_unsupported:"+assoc.UnsupportedField)
				continue
			}
			if assoc.Conditional {
				residuals = append(residuals, "conditional_model_association_not_imported:"+sanitizeIdentifier(pattern))
				continue
			}
			connections, ok := matchAxonHubAssociation(assoc, channels, bySourceID)
			if !ok {
				residuals = append(residuals, "model_association_invalid:"+sanitizeIdentifier(pattern))
				continue
			}
			for _, connection := range connections {
				channel := bySourceID[connection.channelSourceID]
				if channel == nil {
					continue
				}
				for i := range channel.CredentialKeys {
					name := credentialName(i, len(channel.CredentialKeys))
					if channel.DisabledKeys[channel.CredentialKeys[i]] {
						continue
					}
					key := fmt.Sprintf("%d\x00%s\x00%s", connection.channelSourceID, name, connection.modelName)
					if previous, exists := items[key]; exists {
						if previous.Priority <= assoc.Priority {
							continue
						}
					} else {
						order = append(order, key)
					}
					items[key] = axonHubPlanRouteItem{
						Key:             key,
						ChannelSourceID: connection.channelSourceID,
						ModelName:       connection.modelName,
						CredentialName:  name,
						Priority:        assoc.Priority,
					}
				}
			}
		}
		if len(items) == 0 {
			residuals = append(residuals, "model_has_no_importable_channel:"+sanitizeIdentifier(pattern))
			continue
		}
		route := axonHubPlanRoute{
			SourceModelID: model.ID,
			Pattern:       pattern,
			GroupKey:      pattern,
			Enabled:       model.Status == "" || model.Status == "enabled",
			Strategy:      axonHubRouteStrategy(model.Settings.LoadBalancerStrategy, &residuals, pattern),
		}
		sort.Strings(order)
		for _, key := range order {
			route.Items = append(route.Items, items[key])
		}
		if model.Settings.TraceStickyMode != "" && model.Settings.TraceStickyMode != "default" {
			residuals = append(residuals, "model_trace_sticky_mode_not_imported:"+sanitizeIdentifier(pattern))
		}
		routes = append(routes, route)
	}
	return routes, residuals
}

func axonHubRouteStrategy(strategy string, residuals *[]string, pattern string) string {
	switch strategy {
	case "", "default", "adaptive":
		if strategy == "adaptive" {
			*residuals = append(*residuals, "model_strategy_approximated:adaptive:"+sanitizeIdentifier(pattern))
		}
		return "weighted"
	case "round-robin":
		return "round_robin"
	default:
		*residuals = append(*residuals, "model_strategy_approximated:"+sanitizeIdentifier(strategy)+":"+sanitizeIdentifier(pattern))
		return "weighted"
	}
}

type axonHubConnection struct {
	channelSourceID int
	modelName       string
}

// matchAxonHubAssociation is a faithful port of AxonHub's six association
// matchers. The second result reports a structural error (an unknown type or an
// unusable regex) rather than an empty match.
func matchAxonHubAssociation(assoc AxonHubSourceAssociation, channels []*axonHubPlanChannel, bySourceID map[int]*axonHubPlanChannel) ([]axonHubConnection, bool) {
	switch assoc.Type {
	case "channel_model":
		if assoc.ChannelModel == nil {
			return nil, false
		}
		channel := bySourceID[assoc.ChannelModel.ChannelID]
		if channel == nil {
			return nil, true
		}
		if _, ok := channel.Entries[assoc.ChannelModel.ModelID]; !ok {
			return nil, true
		}
		return []axonHubConnection{{assoc.ChannelModel.ChannelID, channel.Entries[assoc.ChannelModel.ModelID]}}, true
	case "channel_regex":
		if assoc.ChannelRegex == nil {
			return nil, false
		}
		channel := bySourceID[assoc.ChannelRegex.ChannelID]
		if channel == nil {
			return nil, true
		}
		matcher, err := compileAxonHubPattern(assoc.ChannelRegex.Pattern)
		if err != nil {
			return nil, false
		}
		return matchEntries(channel.SourceID, channel.Entries, matcher), true
	case "regex":
		if assoc.Regex == nil {
			return nil, false
		}
		matcher, err := compileAxonHubPattern(assoc.Regex.Pattern)
		if err != nil {
			return nil, false
		}
		var out []axonHubConnection
		for _, channel := range channels {
			if axonHubExcluded(channel, assoc.Regex.Exclude) {
				continue
			}
			out = append(out, matchEntries(channel.SourceID, channel.Entries, matcher)...)
		}
		return out, true
	case "model":
		if assoc.ModelID == nil {
			return nil, false
		}
		var out []axonHubConnection
		for _, channel := range channels {
			if axonHubExcluded(channel, assoc.ModelID.Exclude) {
				continue
			}
			if actual, ok := channel.Entries[assoc.ModelID.ModelID]; ok {
				out = append(out, axonHubConnection{channel.SourceID, actual})
			}
		}
		return out, true
	case "channel_tags_model":
		if assoc.ChannelTagsModel == nil || len(assoc.ChannelTagsModel.ChannelTags) == 0 {
			return nil, false
		}
		var out []axonHubConnection
		for _, channel := range channels {
			if !channelHasAnyTag(channel, assoc.ChannelTagsModel.ChannelTags) {
				continue
			}
			if actual, ok := channel.Entries[assoc.ChannelTagsModel.ModelID]; ok {
				out = append(out, axonHubConnection{channel.SourceID, actual})
			}
		}
		return out, true
	case "channel_tags_regex":
		if assoc.ChannelTagsRegex == nil || len(assoc.ChannelTagsRegex.ChannelTags) == 0 {
			return nil, false
		}
		matcher, err := compileAxonHubPattern(assoc.ChannelTagsRegex.Pattern)
		if err != nil {
			return nil, false
		}
		var out []axonHubConnection
		for _, channel := range channels {
			if !channelHasAnyTag(channel, assoc.ChannelTagsRegex.ChannelTags) {
				continue
			}
			out = append(out, matchEntries(channel.SourceID, channel.Entries, matcher)...)
		}
		return out, true
	default:
		return nil, false
	}
}

func matchEntries(channelSourceID int, entries map[string]string, matcher func(string) bool) []axonHubConnection {
	names := make([]string, 0, len(entries))
	for requestModel := range entries {
		if matcher(requestModel) {
			names = append(names, requestModel)
		}
	}
	sort.Strings(names)
	out := make([]axonHubConnection, 0, len(names))
	for _, name := range names {
		out = append(out, axonHubConnection{channelSourceID, entries[name]})
	}
	return out
}

// compileAxonHubPattern uses Go's regexp engine. AxonHub's xregexp wrapper is
// RE2-compatible, so a pattern that fails to compile here is refused rather
// than approximated.
func compileAxonHubPattern(pattern string) (func(string) bool, error) {
	trimmed := strings.TrimSpace(pattern)
	if trimmed == "" {
		return nil, fmt.Errorf("empty pattern")
	}
	expr, err := regexp.Compile(trimmed)
	if err != nil {
		return nil, err
	}
	return expr.MatchString, nil
}

func channelHasAnyTag(channel *axonHubPlanChannel, tags []string) bool {
	for _, tag := range tags {
		for _, candidate := range channel.Tags {
			if candidate == tag {
				return true
			}
		}
	}
	return false
}

func axonHubExcluded(channel *axonHubPlanChannel, excludes []AxonHubSourceExclude) bool {
	for _, exclude := range excludes {
		if exclude.ChannelNamePattern != "" {
			if matcher, err := compileAxonHubPattern(exclude.ChannelNamePattern); err == nil && matcher(channel.Name) {
				return true
			}
		}
		for _, id := range exclude.ChannelIDs {
			if id == channel.SourceID {
				return true
			}
		}
		for _, tag := range exclude.ChannelTags {
			for _, candidate := range channel.Tags {
				if candidate == tag {
					return true
				}
			}
		}
	}
	return false
}
