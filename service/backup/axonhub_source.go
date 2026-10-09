package backup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
)

// AxonHubBackupVersion is the only AxonHub backup format this importer has
// audited. A newer format must be reviewed against the provider enum and the
// endpoint contract before it can be assumed to carry the same semantics.
const AxonHubBackupVersion = "1.4"

// AxonHubSource is the parsed, source-side view of an AxonHub backup.
//
// It is not an import plan and it is not a claim of compatibility: an AxonHub
// feature has a Metapi equivalent only after CompileAxonHubPlan says so.
// Sections this importer never executes are counted rather than modeled, so a
// payload cannot smuggle routing semantics through a field nobody reads.
type AxonHubSource struct {
	Version            string
	Timestamp          string
	SystemConfigs      []AxonHubSourceSystemConfig
	Projects           []AxonHubSourceProject
	Channels           []AxonHubSourceChannel
	Models             []AxonHubSourceModel
	ChannelModelPrices []AxonHubSourceModelPrice
	APIKeys            []AxonHubSourceAPIKey
	// UsageRequests/UsageLogs are history, not configuration; only their row
	// counts are retained so a preview can report them honestly.
	UsageRequests int
	UsageLogs     int
	// UnknownSections counts recognized top-level sections outside this
	// importer's contract.
	UnknownSections map[string]int
}

type AxonHubSourceChannel struct {
	ID                      int                          `json:"id"`
	Type                    string                       `json:"type"`
	Name                    string                       `json:"name"`
	Status                  string                       `json:"status"`
	BaseURL                 string                       `json:"base_url"`
	SupportedModels         []string                     `json:"supported_models"`
	ManualModels            []string                     `json:"manual_models"`
	AutoSyncSupportedModels bool                         `json:"auto_sync_supported_models"`
	AutoSyncModelPattern    string                       `json:"auto_sync_model_pattern"`
	Tags                    []string                     `json:"tags"`
	DefaultTestModel        string                       `json:"default_test_model"`
	OrderingWeight          int                          `json:"ordering_weight"`
	Remark                  *string                      `json:"remark"`
	DeletedAt               int                          `json:"deleted_at"`
	UnsupportedFields       []string                     `json:"-"`
	Credentials             AxonHubSourceCredentials     `json:"-"`
	Settings                AxonHubSourceChannelSettings `json:"-"`
	Policies                AxonHubSourceChannelPolicies `json:"-"`
	Endpoints               []AxonHubSourceEndpoint      `json:"-"`
	DisabledAPIKeys         []AxonHubSourceDisabledKey   `json:"-"`
}

// AxonHubSourceCredentials mirrors objects.ChannelCredentials. Every kind is
// modeled even when the importer cannot serve it, because "cannot serve" must
// be a decision made after reading, never a field somebody forgot to parse.
type AxonHubSourceCredentials struct {
	APIKey  string
	APIKeys []string
	OAuth   bool
	Azure   bool
	GCP     bool
	// UnsupportedFields names credential keys outside the audited contract.
	UnsupportedFields []string
}

type AxonHubSourceChannelSettings struct {
	ExtraModelPrefix        string                      `json:"extraModelPrefix"`
	AutoTrimedModelPrefixes []string                    `json:"autoTrimedModelPrefixes"`
	ModelMappings           []AxonHubSourceModelMapping `json:"modelMappings"`
	HideOriginalModels      bool                        `json:"hideOriginalModels"`
	HideMappedModels        bool                        `json:"hideMappedModels"`
	LowercaseModelID        bool                        `json:"lowercaseModelId"`
	OverrideParameters      string                      `json:"overrideParameters"`
	PassThroughUserAgent    *bool                       `json:"passThroughUserAgent"`
	PassThroughBody         *bool                       `json:"passThroughBody"`
	RetryableStatusCodes    []int                       `json:"retryableStatusCodes"`
	// Operation-shaped transforms are retained only as presence flags so the
	// compiler can refuse them instead of half-applying them.
	HasBodyOverrideOperations   bool     `json:"-"`
	HasHeaderOverrideOperations bool     `json:"-"`
	ProxyConfigured             bool     `json:"-"`
	ProxyURL                    string   `json:"-"`
	HasTransformOptions         bool     `json:"-"`
	HasRateLimit                bool     `json:"-"`
	HasRetryableErrorPatterns   bool     `json:"-"`
	HasProviderQuota            bool     `json:"-"`
	UnsupportedFields           []string `json:"-"`
}

type AxonHubSourceChannelPolicies struct {
	Stream               string   `json:"stream"`
	AutoDisableRuleCount int      `json:"-"`
	UnsupportedFields    []string `json:"-"`
}

type AxonHubSourceModelMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type AxonHubSourceEndpoint struct {
	APIFormat string `json:"api_format"`
	Path      string `json:"path"`
	BaseURL   string `json:"base_url"`
	Transport string `json:"transport"`
}

type AxonHubSourceDisabledKey struct {
	Key string `json:"key"`
}

type AxonHubSourceModel struct {
	ID        int                        `json:"id"`
	Developer string                     `json:"developer"`
	ModelID   string                     `json:"model_id"`
	Type      string                     `json:"type"`
	Name      string                     `json:"name"`
	Icon      string                     `json:"icon"`
	Group     string                     `json:"group"`
	Status    string                     `json:"status"`
	DeletedAt int                        `json:"deleted_at"`
	Settings  AxonHubSourceModelSettings `json:"-"`
}

type AxonHubSourceModelSettings struct {
	DisableDeveloperSettingsInheritance bool     `json:"disableDeveloperSettingsInheritance"`
	LoadBalancerStrategy                string   `json:"loadBalancerStrategy"`
	TraceStickyMode                     string   `json:"traceStickyMode"`
	UnsupportedFields                   []string `json:"-"`
	// Associations are decoded separately so an unread association shape is an
	// error rather than a silently empty list.
	Associations []AxonHubSourceAssociation `json:"-"`
}

// AxonHubSourceAssociation covers all six association kinds the source
// documents. The compiler resolves them into concrete channel/model pairs.
type AxonHubSourceAssociation struct {
	Type             string
	Priority         int
	Disabled         bool
	Conditional      bool
	ChannelModel     *AxonHubSourceChannelModel
	ChannelRegex     *AxonHubSourceChannelRegex
	Regex            *AxonHubSourceRegex
	ModelID          *AxonHubSourceModelIDMatch
	ChannelTagsModel *AxonHubSourceTagsModel
	ChannelTagsRegex *AxonHubSourceTagsRegex
	UnsupportedField string
}

type AxonHubSourceChannelModel struct {
	ChannelID int
	ModelID   string
}

type AxonHubSourceChannelRegex struct {
	ChannelID int
	Pattern   string
}

type AxonHubSourceRegex struct {
	Pattern string
	Exclude []AxonHubSourceExclude
}

type AxonHubSourceModelIDMatch struct {
	ModelID string
	Exclude []AxonHubSourceExclude
}

type AxonHubSourceTagsModel struct {
	ChannelTags []string
	ModelID     string
}

type AxonHubSourceTagsRegex struct {
	ChannelTags []string
	Pattern     string
}

type AxonHubSourceExclude struct {
	ChannelNamePattern string
	ChannelIDs         []int
	ChannelTags        []string
}

type AxonHubSourceModelPrice struct {
	ChannelName string
	ModelID     string
	Price       json.RawMessage
	ReferenceID string
}

type AxonHubSourceProject struct {
	ID          int
	Name        string
	Description string
	Status      string
	ProfileRows int
}

type AxonHubSourceAPIKey struct {
	ID          int
	ProjectID   int
	Name        string
	Type        string
	Status      string
	Scopes      []string
	ProfileRows int
	AllowedIPs  int
	HasQuota    bool
	DeletedAt   int
}

type AxonHubSourceSystemConfig struct {
	Key   string
	Value string
}

// AxonHubSourceError describes malformed or unsupported source data. Its text
// never includes a credential value or any other attacker-chosen payload.
type AxonHubSourceError struct{ Message string }

func (e AxonHubSourceError) Error() string { return e.Message }

func axonHubErr(format string, args ...any) error {
	return AxonHubSourceError{Message: fmt.Sprintf(format, args...)}
}

// ParseAxonHubSource parses the configuration-bearing sections of an AxonHub
// backup. Structural problems are errors; a well-formed feature this importer
// cannot execute is reported by the compiler, not here.
func ParseAxonHubSource(raw []byte) (*AxonHubSource, error) {
	if len(raw) > 64<<20 {
		return nil, axonHubErr("AxonHub backup exceeds 64 MiB")
	}
	if err := checkSourceJSONKeys(raw); err != nil {
		return nil, axonHubErr("invalid AxonHub backup JSON: ambiguous or malformed JSON")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	var top map[string]json.RawMessage
	if err := dec.Decode(&top); err != nil {
		return nil, axonHubErr("invalid AxonHub backup JSON: %s", safeJSONDecodeError(err))
	}
	var trailing any
	if err := dec.Decode(&trailing); err == nil {
		return nil, axonHubErr("invalid AxonHub backup JSON: trailing JSON value")
	} else if err != io.EOF {
		return nil, axonHubErr("invalid AxonHub backup JSON: trailing data")
	}
	if top == nil {
		return nil, axonHubErr("invalid AxonHub backup JSON: expected an object")
	}
	if _, ok := top["channels"]; !ok {
		return nil, axonHubErr("invalid AxonHub backup: missing channels field")
	}
	if _, ok := top["models"]; !ok {
		return nil, axonHubErr("invalid AxonHub backup: missing models field")
	}

	src := &AxonHubSource{UnknownSections: map[string]int{}}
	var err error
	if src.Version, err = decodeAxonHubString(top["version"], "version"); err != nil {
		return nil, err
	}
	src.Version = strings.TrimSpace(src.Version)
	if src.Version == "" {
		return nil, axonHubErr("invalid AxonHub backup: missing version")
	}
	if src.Version != AxonHubBackupVersion {
		return nil, axonHubErr("unsupported AxonHub backup version (only %s is supported)", AxonHubBackupVersion)
	}
	if src.Timestamp, err = decodeAxonHubString(top["timestamp"], "timestamp"); err != nil {
		return nil, err
	}
	if src.Channels, err = decodeAxonHubChannels(top["channels"]); err != nil {
		return nil, err
	}
	if src.Models, err = decodeAxonHubModels(top["models"]); err != nil {
		return nil, err
	}
	if src.SystemConfigs, err = decodeAxonHubSystemConfigs(top["system_configs"]); err != nil {
		return nil, err
	}
	if src.Projects, err = decodeAxonHubProjects(top["projects"]); err != nil {
		return nil, err
	}
	if src.ChannelModelPrices, err = decodeAxonHubPrices(top["channel_model_prices"]); err != nil {
		return nil, err
	}
	if src.APIKeys, err = decodeAxonHubAPIKeys(top["api_keys"]); err != nil {
		return nil, err
	}
	if src.UsageRequests, err = sectionRowCount(top["usage_requests"]); err != nil {
		return nil, axonHubErr("invalid AxonHub backup: usage_requests must be an array")
	}
	if src.UsageLogs, err = sectionRowCount(top["usage_logs"]); err != nil {
		return nil, axonHubErr("invalid AxonHub backup: usage_logs must be an array")
	}
	for _, name := range axonHubTopLevelSections {
		delete(top, name)
	}
	for name := range top {
		src.UnknownSections[sanitizeIdentifier(name)] = 1
	}
	if err := validateAxonHubSource(src); err != nil {
		return nil, err
	}
	return src, nil
}

var axonHubTopLevelSections = []string{
	"version", "timestamp", "channels", "models", "system_configs", "projects",
	"channel_model_prices", "api_keys", "usage_requests", "usage_logs",
}

// sanitizeIdentifier keeps preview output free of arbitrary attacker text while
// still naming the item well enough to investigate it.
func sanitizeIdentifier(name string) string {
	if len(name) > 64 {
		name = name[:64]
	}
	out := make([]rune, 0, len(name))
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-', r == '.':
			out = append(out, r)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

func decodeAxonHubString(raw json.RawMessage, name string) (string, error) {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", axonHubErr("invalid AxonHub backup: %s must be a string", name)
	}
	return value, nil
}

// sectionRowCount accepts an absent, null, or array section and returns its row
// count. Anything else is malformed.
func sectionRowCount(raw json.RawMessage) (int, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}

func rawSectionRows(raw json.RawMessage) ([]json.RawMessage, error) {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, axonHubErr("expected an array section")
	}
	return rows, nil
}

// rowObject decodes one section row and rejects duplicate keys: a duplicate key
// is how a crafted payload hides a credential behind a later value.
func rowObject(row json.RawMessage) (map[string]json.RawMessage, error) {
	if err := checkSourceJSONKeys(row); err != nil {
		return nil, axonHubErr("ambiguous JSON object")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(row, &obj); err != nil || obj == nil {
		return nil, axonHubErr("expected an object")
	}
	return obj, nil
}

// present reports whether a JSON field carries a value rather than being absent,
// null, or an empty container.
func present(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	switch string(trimmed) {
	case "null", `""`, "{}", "[]":
		return false
	}
	return true
}

func unknownKeys(obj map[string]json.RawMessage, allowed []string) []string {
	known := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		known[key] = struct{}{}
	}
	var extra []string
	for key := range obj {
		if _, ok := known[key]; !ok {
			extra = append(extra, sanitizeIdentifier(key))
		}
	}
	sort.Strings(extra)
	return extra
}

var axonHubChannelFields = []string{
	"id", "created_at", "updated_at", "deleted_at", "type", "base_url", "name", "status",
	"credentials", "disabled_api_keys", "supported_models", "manual_models",
	"auto_sync_supported_models", "auto_sync_model_pattern", "tags", "default_test_model",
	"policies", "settings", "ordering_weight", "error_message", "auto_disabled_at",
	"auto_disable_expires_at", "remark", "endpoints", "edges",
}

var axonHubChannelSettingsFields = []string{
	"extraModelPrefix", "autoTrimedModelPrefixes", "modelMappings", "hideOriginalModels",
	"hideMappedModels", "lowercaseModelId", "overrideParameters", "bodyOverrideOperations",
	"overrideHeaders", "headerOverrideOperations", "proxy", "transformOptions",
	"passThroughUserAgent", "passThroughBody", "rateLimit", "retryableStatusCodes",
	"retryableErrorPatterns", "providerQuota",
}

var axonHubChannelPolicyFields = []string{"stream", "apiKeyAutoDisableRules"}

var axonHubCredentialFields = []string{"apiKey", "apiKeys", "oauth", "azure", "gcp"}

var axonHubEndpointFields = []string{"api_format", "path", "base_url", "transport"}

var axonHubModelFields = []string{
	"id", "created_at", "updated_at", "deleted_at", "developer", "model_id", "type", "name",
	"icon", "group", "model_card", "status", "settings", "remark", "edges",
}

var axonHubModelSettingsFields = []string{
	"disableDeveloperSettingsInheritance", "associations", "loadBalancerStrategy", "traceStickyMode",
}

var axonHubAssociationFields = []string{
	"type", "priority", "disabled", "when", "channelModel", "channelRegex", "regex", "modelId",
	"channelTagsModel", "channelTagsRegex",
}

var axonHubExcludeFields = []string{"channelNamePattern", "channelIds", "channelTags"}

func decodeAxonHubChannels(raw json.RawMessage) ([]AxonHubSourceChannel, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: channels must be an array")
	}
	out := make([]AxonHubSourceChannel, 0, len(rows))
	for i, row := range rows {
		obj, err := rowObject(row)
		if err != nil {
			return nil, axonHubErr("invalid AxonHub backup: channels[%d] is not an object", i)
		}
		extra := unknownKeys(obj, axonHubChannelFields)
		var ch AxonHubSourceChannel
		if err := json.Unmarshal(row, &ch); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: channels[%d] has an invalid identity or field type", i)
		}
		if len(extra) > 0 {
			ch.UnsupportedFields = extra
		}
		if err := decodeAxonHubCredentials(obj["credentials"], &ch.Credentials); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: channels[%d].credentials is invalid", i)
		}
		if err := decodeAxonHubEndpoints(obj["endpoints"], &ch.Endpoints); err != nil {
			return nil, err
		}
		if err := decodeAxonHubSettings(obj["settings"], &ch.Settings); err != nil {
			return nil, err
		}
		if err := decodeAxonHubPolicies(obj["policies"], &ch.Policies); err != nil {
			return nil, err
		}
		if err := decodeAxonHubDisabledKeys(obj["disabled_api_keys"], &ch.DisabledAPIKeys); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: channels[%d].disabled_api_keys is invalid", i)
		}
		out = append(out, ch)
	}
	return out, nil
}

func decodeAxonHubCredentials(raw json.RawMessage, dst *AxonHubSourceCredentials) error {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	obj, err := rowObject(raw)
	if err != nil {
		return err
	}
	if dst.APIKey, err = decodeAxonHubString(obj["apiKey"], "credentials.apiKey"); err != nil {
		return err
	}
	if rawKeys, ok := obj["apiKeys"]; ok {
		if err := json.Unmarshal(rawKeys, &dst.APIKeys); err != nil {
			return axonHubErr("credentials.apiKeys must be an array")
		}
	}
	dst.OAuth = present(obj["oauth"])
	dst.Azure = present(obj["azure"])
	dst.GCP = present(obj["gcp"])
	dst.UnsupportedFields = unknownKeys(obj, axonHubCredentialFields)
	return nil
}

func decodeAxonHubEndpoints(raw json.RawMessage, dst *[]AxonHubSourceEndpoint) error {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return axonHubErr("invalid AxonHub backup: endpoints must be an array")
	}
	out := make([]AxonHubSourceEndpoint, 0, len(rows))
	for i, row := range rows {
		obj, err := rowObject(row)
		if err != nil {
			return axonHubErr("invalid AxonHub backup: endpoints[%d] is not an object", i)
		}
		if extra := unknownKeys(obj, axonHubEndpointFields); len(extra) > 0 {
			return axonHubErr("invalid AxonHub backup: endpoints[%d] has unsupported fields", i)
		}
		var ep AxonHubSourceEndpoint
		if err := json.Unmarshal(row, &ep); err != nil {
			return axonHubErr("invalid AxonHub backup: endpoints[%d] has an invalid field type", i)
		}
		out = append(out, ep)
	}
	*dst = out
	return nil
}

func decodeAxonHubSettings(raw json.RawMessage, dst *AxonHubSourceChannelSettings) error {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	obj, err := rowObject(raw)
	if err != nil {
		return axonHubErr("invalid AxonHub backup: channel settings must be an object")
	}
	dst.UnsupportedFields = unknownKeys(obj, axonHubChannelSettingsFields)
	if err := json.Unmarshal(raw, dst); err != nil {
		return axonHubErr("invalid AxonHub backup: channel settings has an invalid field type")
	}
	// The source gives a non-nil operations slice precedence over the legacy
	// settings, including an explicit empty array that clears the old override.
	var bodyOps, headerOps []json.RawMessage
	if rawOps, ok := obj["bodyOverrideOperations"]; ok {
		if err := json.Unmarshal(rawOps, &bodyOps); err != nil {
			return axonHubErr("invalid AxonHub backup: body override operations must be an array")
		}
	}
	if rawOps, ok := obj["headerOverrideOperations"]; ok {
		if err := json.Unmarshal(rawOps, &headerOps); err != nil {
			return axonHubErr("invalid AxonHub backup: header override operations must be an array")
		}
	}
	dst.HasBodyOverrideOperations = len(bodyOps) > 0
	if bodyOps != nil {
		dst.OverrideParameters = ""
	}
	dst.HasHeaderOverrideOperations = len(headerOps) > 0 || (headerOps == nil && present(obj["overrideHeaders"]))
	dst.HasTransformOptions, err = axonHubActiveTransformOptions(obj["transformOptions"])
	if err != nil {
		return err
	}
	dst.HasRateLimit = present(obj["rateLimit"])
	dst.HasRetryableErrorPatterns = present(obj["retryableErrorPatterns"])
	dst.HasProviderQuota = present(obj["providerQuota"])
	dst.ProxyConfigured = present(obj["proxy"])
	if dst.ProxyConfigured {
		var proxy struct {
			Type                   string `json:"type"`
			URL                    string `json:"url"`
			Username               string `json:"username"`
			Password               string `json:"password"`
			DisableConnectionReuse bool   `json:"disableConnectionReuse"`
		}
		if err := json.Unmarshal(obj["proxy"], &proxy); err != nil {
			return axonHubErr("invalid AxonHub backup: channel proxy configuration is invalid")
		}
		proxyObj, err := rowObject(obj["proxy"])
		if err != nil {
			return axonHubErr("invalid AxonHub backup: channel proxy configuration is invalid")
		}
		for _, field := range unknownKeys(proxyObj, []string{"type", "url", "username", "password", "disableConnectionReuse"}) {
			dst.UnsupportedFields = append(dst.UnsupportedFields, "proxy."+field)
		}
		if proxy.DisableConnectionReuse {
			dst.UnsupportedFields = append(dst.UnsupportedFields, "proxy.disableConnectionReuse")
		}
		switch proxy.Type {
		case "", "environment":
			// The source ignores any stale URL/auth when using the environment.
			dst.ProxyConfigured = false
		case "url":
			dst.ProxyURL = strings.TrimSpace(proxy.URL)
			if proxy.Username != "" && proxy.Password != "" {
				parsed, err := url.Parse(dst.ProxyURL)
				if err != nil {
					return axonHubErr("invalid AxonHub backup: channel proxy URL is invalid")
				}
				parsed.User = url.UserPassword(proxy.Username, proxy.Password)
				dst.ProxyURL = parsed.String()
			}
		case "disabled":
			// Direct grants currently inherit the process environment. Refuse
			// an explicit no-proxy contract instead of using a leftover URL.
		default:
			dst.UnsupportedFields = append(dst.UnsupportedFields, "proxy.type")
		}
	}
	return nil
}

// ChannelSettings exports its zero-valued TransformOptions object. Only
// enabled known options require an implementation; unknown keys remain a
// refusal even when false so future semantics cannot be silently discarded.
func axonHubActiveTransformOptions(raw json.RawMessage) (bool, error) {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return false, nil
	}
	obj, err := rowObject(raw)
	if err != nil {
		return false, axonHubErr("invalid AxonHub backup: transform options must be an object")
	}
	var shape struct {
		ForceArrayInstructions         bool              `json:"forceArrayInstructions"`
		ForceArrayInputs               bool              `json:"forceArrayInputs"`
		ReplaceDeveloperRoleWithSystem bool              `json:"replaceDeveloperRoleWithSystem"`
		ReasoningEffortMapping         []json.RawMessage `json:"reasoningEffortMapping"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		return false, axonHubErr("invalid AxonHub backup: transform options has an invalid field type")
	}
	unknown := unknownKeys(obj, []string{"forceArrayInstructions", "forceArrayInputs", "replaceDeveloperRoleWithSystem", "reasoningEffortMapping"})
	return shape.ForceArrayInstructions || shape.ForceArrayInputs || shape.ReplaceDeveloperRoleWithSystem || len(shape.ReasoningEffortMapping) > 0 || len(unknown) > 0, nil
}

func decodeAxonHubPolicies(raw json.RawMessage, dst *AxonHubSourceChannelPolicies) error {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	obj, err := rowObject(raw)
	if err != nil {
		return axonHubErr("invalid AxonHub backup: channel policies must be an object")
	}
	dst.UnsupportedFields = unknownKeys(obj, axonHubChannelPolicyFields)
	if err := json.Unmarshal(raw, dst); err != nil {
		return axonHubErr("invalid AxonHub backup: channel policies has an invalid field type")
	}
	var rules []json.RawMessage
	if err := json.Unmarshal(obj["apiKeyAutoDisableRules"], &rules); err == nil {
		dst.AutoDisableRuleCount = len(rules)
	}
	return nil
}

func decodeAxonHubDisabledKeys(raw json.RawMessage, dst *[]AxonHubSourceDisabledKey) error {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return err
	}
	out := make([]AxonHubSourceDisabledKey, 0, len(rows))
	for i, row := range rows {
		obj, err := rowObject(row)
		if err != nil {
			return axonHubErr("disabled_api_keys[%d] is not an object", i)
		}
		key, err := decodeAxonHubString(obj["key"], "disabled_api_keys.key")
		if err != nil {
			return err
		}
		out = append(out, AxonHubSourceDisabledKey{Key: key})
	}
	*dst = out
	return nil
}

func decodeAxonHubModels(raw json.RawMessage) ([]AxonHubSourceModel, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: models must be an array")
	}
	out := make([]AxonHubSourceModel, 0, len(rows))
	for i, row := range rows {
		obj, err := rowObject(row)
		if err != nil {
			return nil, axonHubErr("invalid AxonHub backup: models[%d] is not an object", i)
		}
		if extra := unknownKeys(obj, axonHubModelFields); len(extra) > 0 {
			return nil, axonHubErr("invalid AxonHub backup: models[%d] has unsupported fields", i)
		}
		var model AxonHubSourceModel
		if err := json.Unmarshal(row, &model); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: models[%d] has an invalid field type", i)
		}
		if err := decodeAxonHubModelSettings(obj["settings"], &model.Settings); err != nil {
			return nil, err
		}
		out = append(out, model)
	}
	return out, nil
}

func decodeAxonHubModelSettings(raw json.RawMessage, dst *AxonHubSourceModelSettings) error {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	obj, err := rowObject(raw)
	if err != nil {
		return axonHubErr("invalid AxonHub backup: model settings must be an object")
	}
	dst.UnsupportedFields = unknownKeys(obj, axonHubModelSettingsFields)
	if err := json.Unmarshal(raw, dst); err != nil {
		return axonHubErr("invalid AxonHub backup: model settings has an invalid field type")
	}
	rows, err := rawSectionRows(obj["associations"])
	if err != nil {
		return axonHubErr("invalid AxonHub backup: model associations must be an array")
	}
	for i, row := range rows {
		assoc, err := decodeAxonHubAssociation(row)
		if err != nil {
			return axonHubErr("invalid AxonHub backup: model associations[%d] is invalid", i)
		}
		dst.Associations = append(dst.Associations, assoc)
	}
	return nil
}

func decodeAxonHubAssociation(raw json.RawMessage) (AxonHubSourceAssociation, error) {
	var assoc AxonHubSourceAssociation
	obj, err := rowObject(raw)
	if err != nil {
		return assoc, err
	}
	if extra := unknownKeys(obj, axonHubAssociationFields); len(extra) > 0 {
		assoc.UnsupportedField = extra[0]
	}
	var shape struct {
		Type         string                     `json:"type"`
		Priority     int                        `json:"priority"`
		Disabled     bool                       `json:"disabled"`
		When         json.RawMessage            `json:"when"`
		ChannelModel *AxonHubSourceChannelModel `json:"channelModel"`
		ChannelRegex *AxonHubSourceChannelRegex `json:"channelRegex"`
		Regex        *struct {
			Pattern string            `json:"pattern"`
			Exclude []json.RawMessage `json:"exclude"`
		} `json:"regex"`
		ModelID *struct {
			ModelID string            `json:"modelId"`
			Exclude []json.RawMessage `json:"exclude"`
		} `json:"modelId"`
		ChannelTagsModel *AxonHubSourceTagsModel `json:"channelTagsModel"`
		ChannelTagsRegex *AxonHubSourceTagsRegex `json:"channelTagsRegex"`
	}
	if err := json.Unmarshal(raw, &shape); err != nil {
		return assoc, err
	}
	assoc.Type = shape.Type
	assoc.Priority = shape.Priority
	assoc.Disabled = shape.Disabled
	if present(shape.When) {
		var when struct {
			Enabled bool `json:"enabled"`
		}
		if err := json.Unmarshal(shape.When, &when); err != nil {
			return assoc, axonHubErr("invalid association condition")
		}
		assoc.Conditional = when.Enabled
	}
	assoc.ChannelModel = shape.ChannelModel
	assoc.ChannelRegex = shape.ChannelRegex
	assoc.ChannelTagsModel = shape.ChannelTagsModel
	assoc.ChannelTagsRegex = shape.ChannelTagsRegex
	if shape.Regex != nil {
		excludes, err := decodeAxonHubExcludes(shape.Regex.Exclude)
		if err != nil {
			return assoc, err
		}
		assoc.Regex = &AxonHubSourceRegex{Pattern: shape.Regex.Pattern, Exclude: excludes}
	}
	if shape.ModelID != nil {
		excludes, err := decodeAxonHubExcludes(shape.ModelID.Exclude)
		if err != nil {
			return assoc, err
		}
		assoc.ModelID = &AxonHubSourceModelIDMatch{ModelID: shape.ModelID.ModelID, Exclude: excludes}
	}
	return assoc, nil
}

func decodeAxonHubExcludes(rows []json.RawMessage) ([]AxonHubSourceExclude, error) {
	out := make([]AxonHubSourceExclude, 0, len(rows))
	for _, row := range rows {
		obj, err := rowObject(row)
		if err != nil {
			return nil, err
		}
		if extra := unknownKeys(obj, axonHubExcludeFields); len(extra) > 0 {
			return nil, axonHubErr("unsupported exclude field")
		}
		var exclude AxonHubSourceExclude
		if err := json.Unmarshal(row, &exclude); err != nil {
			return nil, err
		}
		out = append(out, exclude)
	}
	return out, nil
}

func decodeAxonHubSystemConfigs(raw json.RawMessage) ([]AxonHubSourceSystemConfig, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: system_configs must be an array")
	}
	out := make([]AxonHubSourceSystemConfig, 0, len(rows))
	for i, row := range rows {
		var entry AxonHubSourceSystemConfig
		if err := json.Unmarshal(row, &entry); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: system_configs[%d] is invalid", i)
		}
		out = append(out, entry)
	}
	return out, nil
}

func decodeAxonHubProjects(raw json.RawMessage) ([]AxonHubSourceProject, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: projects must be an array")
	}
	out := make([]AxonHubSourceProject, 0, len(rows))
	for i, row := range rows {
		var shape struct {
			ID          int             `json:"id"`
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Status      string          `json:"status"`
			Profiles    json.RawMessage `json:"profiles"`
		}
		if err := json.Unmarshal(row, &shape); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: projects[%d] is invalid", i)
		}
		out = append(out, AxonHubSourceProject{
			ID: shape.ID, Name: shape.Name, Description: shape.Description,
			Status: shape.Status, ProfileRows: profileRowCount(shape.Profiles),
		})
	}
	return out, nil
}

func profileRowCount(raw json.RawMessage) int {
	var shape struct {
		Profiles []json.RawMessage `json:"profiles"`
	}
	if json.Unmarshal(raw, &shape) != nil {
		return 0
	}
	return len(shape.Profiles)
}

func decodeAxonHubPrices(raw json.RawMessage) ([]AxonHubSourceModelPrice, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: channel_model_prices must be an array")
	}
	out := make([]AxonHubSourceModelPrice, 0, len(rows))
	for i, row := range rows {
		var shape struct {
			ChannelName string          `json:"channel_name"`
			ModelID     string          `json:"model_id"`
			Price       json.RawMessage `json:"price"`
			ReferenceID string          `json:"reference_id"`
		}
		if err := json.Unmarshal(row, &shape); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: channel_model_prices[%d] is invalid", i)
		}
		out = append(out, AxonHubSourceModelPrice{
			ChannelName: shape.ChannelName, ModelID: shape.ModelID,
			Price: shape.Price, ReferenceID: shape.ReferenceID,
		})
	}
	return out, nil
}

func decodeAxonHubAPIKeys(raw json.RawMessage) ([]AxonHubSourceAPIKey, error) {
	rows, err := rawSectionRows(raw)
	if err != nil {
		return nil, axonHubErr("invalid AxonHub backup: api_keys must be an array")
	}
	out := make([]AxonHubSourceAPIKey, 0, len(rows))
	for i, row := range rows {
		var shape struct {
			ID         int             `json:"id"`
			ProjectID  int             `json:"project_id"`
			Name       string          `json:"name"`
			Type       string          `json:"type"`
			Status     string          `json:"status"`
			DeletedAt  int             `json:"deleted_at"`
			Scopes     []string        `json:"scopes"`
			Profiles   json.RawMessage `json:"profiles"`
			AllowedIPs []string        `json:"allowed_ips"`
		}
		if err := json.Unmarshal(row, &shape); err != nil {
			return nil, axonHubErr("invalid AxonHub backup: api_keys[%d] is invalid", i)
		}
		out = append(out, AxonHubSourceAPIKey{
			ID: shape.ID, ProjectID: shape.ProjectID, Name: shape.Name, Type: shape.Type,
			Status: shape.Status, DeletedAt: shape.DeletedAt, Scopes: shape.Scopes,
			ProfileRows: profileRowCount(shape.Profiles), AllowedIPs: len(shape.AllowedIPs),
			HasQuota: apiKeyProfilesHaveQuota(shape.Profiles),
		})
	}
	return out, nil
}

func apiKeyProfilesHaveQuota(raw json.RawMessage) bool {
	var shape struct {
		Profiles []struct {
			Quota json.RawMessage `json:"quota"`
		} `json:"profiles"`
	}
	if json.Unmarshal(raw, &shape) != nil {
		return false
	}
	for _, profile := range shape.Profiles {
		if present(profile.Quota) {
			return true
		}
	}
	return false
}

func validateAxonHubSource(src *AxonHubSource) error {
	channelIDs := make(map[int]struct{}, len(src.Channels))
	for i, ch := range src.Channels {
		if ch.ID <= 0 {
			return axonHubErr("invalid AxonHub backup: channels[%d] has a non-positive id", i)
		}
		if _, exists := channelIDs[ch.ID]; exists {
			return axonHubErr("invalid AxonHub backup: duplicate channel id %d", ch.ID)
		}
		channelIDs[ch.ID] = struct{}{}
		if strings.TrimSpace(ch.Type) == "" {
			return axonHubErr("invalid AxonHub backup: channels[%d] has no provider type", i)
		}
		switch ch.Status {
		case "", "enabled", "disabled", "archived":
		default:
			return axonHubErr("invalid AxonHub backup: channels[%d] has an unknown status", i)
		}
		formats := make(map[string]struct{}, len(ch.Endpoints))
		for j, ep := range ch.Endpoints {
			format := strings.TrimSpace(ep.APIFormat)
			if format == "" {
				return axonHubErr("invalid AxonHub backup: channel[%d].endpoints[%d] missing api_format", i, j)
			}
			if _, exists := formats[format]; exists {
				return axonHubErr("invalid AxonHub backup: duplicate endpoint format at channel[%d].endpoints[%d]", i, j)
			}
			formats[format] = struct{}{}
			switch ep.Transport {
			case "", "http", "websocket":
			default:
				return axonHubErr("invalid AxonHub backup: channel[%d].endpoints[%d] has an unknown transport", i, j)
			}
			if ep.Path != "" && !safeDirectEndpointPath(ep.Path) {
				return axonHubErr("invalid AxonHub backup: channel[%d].endpoints[%d] has an invalid path", i, j)
			}
		}
	}
	seenModelIDs := make(map[string]struct{}, len(src.Models))
	modelSourceIDs := make(map[int]struct{}, len(src.Models))
	for i, model := range src.Models {
		if model.ID <= 0 {
			return axonHubErr("invalid AxonHub backup: models[%d] has a non-positive id", i)
		}
		if _, exists := modelSourceIDs[model.ID]; exists {
			return axonHubErr("invalid AxonHub backup: duplicate model source id %d", model.ID)
		}
		modelSourceIDs[model.ID] = struct{}{}
		switch model.Status {
		case "", "enabled", "disabled", "archived":
		default:
			return axonHubErr("invalid AxonHub backup: models[%d] has an unknown status", i)
		}
		modelID := strings.TrimSpace(model.ModelID)
		if modelID == "" {
			return axonHubErr("invalid AxonHub backup: model[%d] has no model_id", i)
		}
		if model.DeletedAt != 0 {
			continue
		}
		if _, exists := seenModelIDs[modelID]; exists {
			return axonHubErr("invalid AxonHub backup: duplicate model_id %q", sanitizeIdentifier(modelID))
		}
		seenModelIDs[modelID] = struct{}{}
	}
	return nil
}

func safeJSONDecodeError(err error) string {
	// encoding/json errors identify a field/offset but never echo the document.
	return err.Error()
}
