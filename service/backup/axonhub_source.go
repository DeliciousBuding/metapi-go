package backup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/internal/routecondition"
	"github.com/deliciousbuding/metapi-go/store"
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
	Version               string
	Timestamp             string
	SystemConfigs         []AxonHubSourceSystemConfig
	DeveloperAssociations map[string][]AxonHubSourceAssociation
	Projects              []AxonHubSourceProject
	Channels              []AxonHubSourceChannel
	Models                []AxonHubSourceModel
	ChannelModelPrices    []AxonHubSourceModelPrice
	APIKeys               []AxonHubSourceAPIKey
	// UsageRequests/UsageLogs are history, not configuration; only their row
	// counts are retained so a preview can report them honestly.
	UsageRequests       int
	UsageLogs           int
	QuotaUsage          []AxonHubQuotaUsage
	QuotaHistoryPresent bool
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
	APIKey           string
	APIKeys          []string
	OAuth            bool
	OAuthCredentials *AxonHubSourceOAuth
	Azure            bool
	// HasManagementKey reports whether a provider console key was present. The
	// source uses it only for server-side quota checks and never attaches it to
	// inference requests, so only its presence is retained.
	HasManagementKey bool
	// GCP carries the Vertex service-account payload (region, project and the
	// raw key JSON) when the channel authenticates with a GCP credential.
	GCP *AxonHubSourceGCP
	// UnsupportedFields names credential keys outside the audited contract.
	UnsupportedFields []string
}

// AxonHubSourceGCP mirrors objects.GCPCredential.
type AxonHubSourceGCP struct {
	Region    string `json:"region"`
	ProjectID string `json:"projectID"`
	JSONData  string `json:"jsonData"`
}

type AxonHubSourceChannelSettings struct {
	ModelProtocols            []AxonHubSourceModelProtocol     `json:"modelProtocols"`
	ExtraModelPrefix          string                           `json:"extraModelPrefix"`
	AutoTrimedModelPrefixes   []string                         `json:"autoTrimedModelPrefixes"`
	ModelMappings             []AxonHubSourceModelMapping      `json:"modelMappings"`
	HideOriginalModels        bool                             `json:"hideOriginalModels"`
	HideMappedModels          bool                             `json:"hideMappedModels"`
	LowercaseModelID          bool                             `json:"lowercaseModelId"`
	OverrideParameters        string                           `json:"overrideParameters"`
	PassThroughUserAgent      *bool                            `json:"passThroughUserAgent"`
	PassThroughBody           *bool                            `json:"passThroughBody"`
	RetryableStatusCodes      []int                            `json:"retryableStatusCodes"`
	BodyOverrideOperations    []requestoverride.Operation      `json:"-"`
	HeaderOverrideOperations  []requestoverride.Operation      `json:"-"`
	TransformOptions          requestoverride.TransformOptions `json:"-"`
	ProxyConfigured           bool                             `json:"-"`
	ProxyURL                  string                           `json:"-"`
	RateLimit                 store.DirectRateLimit            `json:"-"`
	HasUnsupportedRateLimit   bool                             `json:"-"`
	HasRetryableErrorPatterns bool                             `json:"-"`
	HasProviderQuota          bool                             `json:"-"`
	UnsupportedFields         []string                         `json:"-"`
}

type AxonHubSourceModelProtocol struct {
	Model      string   `json:"model"`
	APIFormats []string `json:"apiFormats"`
	Enabled    *bool    `json:"enabled,omitempty"`
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
	Condition        *routecondition.Condition
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
	Profiles    *AxonHubAccessProfiles
	DeletedAt   int
	ID          int
	Name        string
	Description string
	Status      string
	ProfileRows int
}

type AxonHubSourceAPIKey struct {
	Key         string
	Profiles    *AxonHubAccessProfiles
	IPAllowlist []string
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
	if src.DeveloperAssociations, err = decodeAxonHubDeveloperSettings(src.SystemConfigs); err != nil {
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
	if src.QuotaUsage, src.QuotaHistoryPresent, err = decodeAxonHubQuotaUsage(top["usage_logs"]); err != nil {
		return nil, err
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
