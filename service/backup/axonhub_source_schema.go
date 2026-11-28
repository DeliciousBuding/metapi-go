package backup

import (
	"encoding/json"

	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/internal/routecondition"
	"github.com/deliciousbuding/metapi-go/store"
)

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
