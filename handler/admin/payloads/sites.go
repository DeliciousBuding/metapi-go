package payloads

// SiteCreatePayload mirrors TS SiteCreatePayload (siteRoutePayloads.ts).
type SiteCreatePayload struct {
	Name                   string  `json:"name"`
	URL                    string  `json:"url"`
	Platform               *string `json:"platform,omitempty"`
	InitializationPresetID *string `json:"initializationPresetId,omitempty"`
	ProxyURL               *string `json:"proxyUrl,omitempty"`
	UseSystemProxy         *bool   `json:"useSystemProxy,omitempty"`
	CustomHeaders          *string `json:"customHeaders,omitempty"`
	// CustomHeadersOverrideRequestHeaders: when true, site custom headers
	// overwrite same-name request headers (site-wins). Default false = request-wins.
	CustomHeadersOverrideRequestHeaders *bool    `json:"customHeadersOverrideRequestHeaders,omitempty"`
	ExternalCheckinURL                  *string  `json:"externalCheckinUrl,omitempty"`
	Status                              *string  `json:"status,omitempty"`
	IsPinned                            *bool    `json:"isPinned,omitempty"`
	SortOrder                           *int     `json:"sortOrder,omitempty"`
	GlobalWeight                        *float64 `json:"globalWeight,omitempty"`
	// MaxConcurrency caps concurrent upstream calls for this site (0 = unlimited).
	MaxConcurrency *int64                 `json:"maxConcurrency,omitempty"`
	APIEndpoints   []SiteAPIEndpointInput `json:"apiEndpoints,omitempty"`
	// ResinEnabled / UseUTLS are nullable per-site overrides: true/false set
	// the override, null clears it back to inheriting the global flag. Value
	// types (not pointers) so a JSON null reaches UnmarshalJSON — a *T field
	// would be left nil by encoding/json and null/absent would be
	// indistinguishable.
	ResinEnabled NullableBool `json:"resinEnabled"`
	UseUTLS      NullableBool `json:"useUtls"`
}

// SiteAPIEndpointInput is an embedded sub-resource input for apiEndpoints.
type SiteAPIEndpointInput struct {
	URL       string `json:"url"`
	Enabled   bool   `json:"enabled"`
	SortOrder int    `json:"sortOrder"`
}

// SiteUpdatePayload mirrors TS SiteUpdatePayload.
type SiteUpdatePayload struct {
	Name           *string `json:"name,omitempty"`
	URL            *string `json:"url,omitempty"`
	Platform       *string `json:"platform,omitempty"`
	ProxyURL       *string `json:"proxyUrl,omitempty"`
	UseSystemProxy *bool   `json:"useSystemProxy,omitempty"`
	CustomHeaders  *string `json:"customHeaders,omitempty"`
	// CustomHeadersOverrideRequestHeaders: opt-in site-wins for same-name headers.
	CustomHeadersOverrideRequestHeaders *bool    `json:"customHeadersOverrideRequestHeaders,omitempty"`
	ExternalCheckinURL                  *string  `json:"externalCheckinUrl,omitempty"`
	Status                              *string  `json:"status,omitempty"`
	IsPinned                            *bool    `json:"isPinned,omitempty"`
	SortOrder                           *int     `json:"sortOrder,omitempty"`
	GlobalWeight                        *float64 `json:"globalWeight,omitempty"`
	// MaxConcurrency caps concurrent upstream calls for this site (0 = unlimited).
	MaxConcurrency                     *int64                 `json:"maxConcurrency,omitempty"`
	APIEndpoints                       []SiteAPIEndpointInput `json:"apiEndpoints,omitempty"`
	PostRefreshProbeEnabled            *bool                  `json:"postRefreshProbeEnabled,omitempty"`
	PostRefreshProbeModel              *string                `json:"postRefreshProbeModel,omitempty"`
	PostRefreshProbeScope              *string                `json:"postRefreshProbeScope,omitempty"`
	PostRefreshProbeLatencyThresholdMs *int                   `json:"postRefreshProbeLatencyThresholdMs,omitempty"`
	// ResinEnabled / UseUTLS are nullable per-site overrides: true/false set
	// the override, null clears it back to inheriting the global flag. Value
	// types (not pointers) so a JSON null reaches UnmarshalJSON.
	ResinEnabled NullableBool `json:"resinEnabled"`
	UseUTLS      NullableBool `json:"useUtls"`
}

// SiteBatchPayload mirrors TS SiteBatchPayload.
type SiteBatchPayload struct {
	IDs    []int  `json:"ids"`
	Action string `json:"action"`
}

// SiteDetectPayload mirrors TS SiteDetectPayload.
type SiteDetectPayload struct {
	URL string `json:"url"`
}

// SiteDisabledModelsPayload mirrors TS SiteDisabledModelsPayload.
type SiteDisabledModelsPayload struct {
	Models []string `json:"models"`
}

// SiteImportAccount mirrors an account to attach during batch import.
type SiteImportAccount struct {
	Username    *string `json:"username,omitempty"`
	AccessToken string  `json:"accessToken,omitempty"`
	APIToken    string  `json:"apiToken,omitempty"`
}

// SiteImportItem is one candidate site in POST /api/sites/import.
type SiteImportItem struct {
	Name              string              `json:"name"`
	URL               string              `json:"url"`
	Platform          *string             `json:"platform,omitempty"`
	GlobalWeight      *float64            `json:"globalWeight,omitempty"`
	MaxConcurrency    *int64              `json:"maxConcurrency,omitempty"`
	DuplicateStrategy string              `json:"duplicateStrategy,omitempty"`
	Accounts          []SiteImportAccount `json:"accounts,omitempty"`
}

// SiteImportPayload is the JSON body for POST /api/sites/import.
type SiteImportPayload struct {
	Items             []SiteImportItem `json:"items"`
	DuplicateStrategy string           `json:"duplicateStrategy,omitempty"`
}

// ProbeNowBody is the JSON body for POST /api/sites/:id/probe-now.
type ProbeNowBody struct {
	Scope              *string `json:"scope,omitempty"`
	ModelName          *string `json:"modelName,omitempty"`
	LatencyThresholdMs *int    `json:"latencyThresholdMs,omitempty"`
}
