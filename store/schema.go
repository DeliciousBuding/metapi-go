package store

// ---- Table 1: sites ----
type Site struct {
	ID                 int64   `db:"id" json:"id"`
	Name               string  `db:"name" json:"name"`
	URL                string  `db:"url" json:"url"`
	ExternalCheckinURL *string `db:"external_checkin_url" json:"externalCheckinUrl"`
	Platform           string  `db:"platform" json:"platform"`
	ProxyURL           *string `db:"proxy_url" json:"proxyUrl"`
	UseSystemProxy     bool    `db:"use_system_proxy" json:"useSystemProxy"`
	CustomHeaders      *string `db:"custom_headers" json:"customHeaders"`
	// CustomHeadersOverrideRequestHeaders: when true, site custom headers
	// overwrite same-name request headers (site-wins). Default false =
	// request-wins (only set when the request lacks that header).
	CustomHeadersOverrideRequestHeaders bool   `db:"custom_headers_override_request_headers" json:"customHeadersOverrideRequestHeaders"`
	Status                              string `db:"status" json:"status"`
	// IsPinned/SortOrder/GlobalWeight are nullable in the sites table
	// (DEFAULT without NOT NULL): rows migrated from the TS version can carry
	// NULL. Keep them as pointers so a NULL column scans to nil (JSON null)
	// instead of failing with "converting NULL to ..." in the routing join
	// scans (channelAccountSiteSelect). Readers that need a numeric default
	// use the OrZero helpers below.
	IsPinned     *bool    `db:"is_pinned" json:"isPinned"`
	SortOrder    *int64   `db:"sort_order" json:"sortOrder"`
	GlobalWeight *float64 `db:"global_weight" json:"globalWeight"`
	APIKey       *string  `db:"api_key" json:"apiKey"`
	// MaxConcurrency caps concurrent upstream calls for this site.
	// 0 (default) means unlimited — preserves pre-SC2 behavior.
	MaxConcurrency                     int64  `db:"max_concurrency" json:"maxConcurrency"`
	PostRefreshProbeEnabled            bool   `db:"post_refresh_probe_enabled" json:"postRefreshProbeEnabled"`
	PostRefreshProbeModel              string `db:"post_refresh_probe_model" json:"postRefreshProbeModel"`
	PostRefreshProbeScope              string `db:"post_refresh_probe_scope" json:"postRefreshProbeScope"`
	PostRefreshProbeLatencyThresholdMs int64  `db:"post_refresh_probe_latency_threshold_ms" json:"postRefreshProbeLatencyThresholdMs"`
	CreatedAt                          string `db:"created_at" json:"createdAt"`
	UpdatedAt                          string `db:"updated_at" json:"updatedAt"`
	// Tags holds a JSON array of operator labels.
	Tags *string `db:"tags" json:"tags"`
	// BrowserUA optionally overrides the global browser User-Agent for this
	// site (used to match the UA of a harvested cf_clearance cookie).
	BrowserUA *string `db:"browser_ua" json:"browserUa"`
	// CfClearance is a per-site Cloudflare clearance cookie injected into
	// outbound requests to bypass CF challenges.
	CfClearance *string `db:"cf_clearance" json:"cfClearance"`
	// ResinEnabled is a per-site Resin sticky-proxy override.
	// nil  = inherit global RESIN_ENABLED flag (default, preserves legacy behavior)
	// true = force-enable Resin for this site even when global is disabled
	// false = force-disable Resin for this site even when global is enabled
	ResinEnabled *bool `db:"resin_enabled" json:"resinEnabled"`
	// UseUTLS is a per-site uTLS TLS fingerprint masking override.
	// nil  = inherit global UTLS_ENABLED flag (default, preserves legacy behavior)
	// true = force-enable uTLS Chrome-ClientHello masking for this site
	// false = force-disable uTLS masking even when global is enabled
	UseUTLS *bool `db:"use_utls" json:"useUtls"`
}

// OrZero helpers coerce the nullable numeric site columns for callers that
// need a numeric default (routing weights, ordering). Callers that must
// preserve the unknown/nil semantics (JSON display) read the pointer directly.
func (s Site) IsPinnedOrFalse() bool {
	if s.IsPinned == nil {
		return false
	}
	return *s.IsPinned
}

func (s Site) SortOrderOrZero() int64 {
	if s.SortOrder == nil {
		return 0
	}
	return *s.SortOrder
}

func (s Site) GlobalWeightOrZero() float64 {
	if s.GlobalWeight == nil {
		return 0
	}
	return *s.GlobalWeight
}

// ---- Table 2: site_api_endpoints ----
type SiteAPIEndpoint struct {
	ID                int64   `db:"id" json:"id"`
	SiteID            int64   `db:"site_id" json:"siteId"`
	URL               string  `db:"url" json:"url"`
	Enabled           bool    `db:"enabled" json:"enabled"`
	SortOrder         int64   `db:"sort_order" json:"sortOrder"`
	CooldownUntil     *string `db:"cooldown_until" json:"cooldownUntil"`
	LastSelectedAt    *string `db:"last_selected_at" json:"lastSelectedAt"`
	LastFailedAt      *string `db:"last_failed_at" json:"lastFailedAt"`
	LastFailureReason *string `db:"last_failure_reason" json:"lastFailureReason"`
	CreatedAt         string  `db:"created_at" json:"createdAt"`
	UpdatedAt         string  `db:"updated_at" json:"updatedAt"`
}

// ---- Table 3: site_disabled_models ----
type SiteDisabledModel struct {
	ID        int64  `db:"id" json:"id"`
	SiteID    int64  `db:"site_id" json:"siteId"`
	ModelName string `db:"model_name" json:"modelName"`
	CreatedAt string `db:"created_at" json:"createdAt"`
}

// ---- Table 4: accounts ----
type Account struct {
	ID          int64   `db:"id" json:"id"`
	SiteID      int64   `db:"site_id" json:"siteId"`
	Username    *string `db:"username" json:"username"`
	AccessToken string  `db:"access_token" json:"accessToken"`
	APIToken    *string `db:"api_token" json:"apiToken"`
	// Balance/BalanceUsed/Quota/ValueScore are nullable in the accounts table
	// (DEFAULT 0 without NOT NULL): rows migrated from the TS version, or never
	// refreshed, carry NULL. Keep them as pointers so a NULL column scans to nil
	// (JSON null) instead of failing with "converting NULL to float64". Readers
	// that need a numeric default use the OrZero helpers below.
	Balance            *float64 `db:"balance" json:"balance"`
	BalanceUsed        *float64 `db:"balance_used" json:"balanceUsed"`
	Quota              *float64 `db:"quota" json:"quota"`
	UnitCost           *float64 `db:"unit_cost" json:"unitCost"`
	ValueScore         *float64 `db:"value_score" json:"valueScore"`
	Status             string   `db:"status" json:"status"`
	IsPinned           bool     `db:"is_pinned" json:"isPinned"`
	SortOrder          int64    `db:"sort_order" json:"sortOrder"`
	CheckinEnabled     bool     `db:"checkin_enabled" json:"checkinEnabled"`
	LastCheckinAt      *string  `db:"last_checkin_at" json:"lastCheckinAt"`
	LastBalanceRefresh *string  `db:"last_balance_refresh" json:"lastBalanceRefresh"`
	OAuthProvider      *string  `db:"oauth_provider" json:"oauthProvider"`
	OAuthAccountKey    *string  `db:"oauth_account_key" json:"oauthAccountKey"`
	OAuthProjectID     *string  `db:"oauth_project_id" json:"oauthProjectId"`
	ExtraConfig        *string  `db:"extra_config" json:"extraConfig"`
	CreatedAt          string   `db:"created_at" json:"createdAt"`
	UpdatedAt          string   `db:"updated_at" json:"updatedAt"`
	// Tags holds a JSON array of operator labels.
	// NULL/empty means no tags; JSON array text like ["prod","priority"].
	Tags *string `db:"tags" json:"tags"`
	// Remark holds a free-form human-readable note (e.g. account source,
	// expiry, donor). NULL means no remark. Complements the machine-readable
	// Tags column; surfaced via PUT /api/accounts/{id}.
	//
	// Do not store access tokens, passwords, or other secrets here. Unlike
	// accessToken/apiToken (masked in list views), remark is returned in
	// plaintext on admin list/search responses.
	Remark *string `db:"remark" json:"remark"`
}

// OrZero helpers coerce the nullable numeric account columns for callers that
// need a numeric default (routing weights, balance aggregation, refresh
// results). Callers that must preserve the unknown/nil semantics (e.g. the
// daily low-balance check, JSON display) should read the pointer directly.
func (a Account) BalanceOrZero() float64 {
	if a.Balance == nil {
		return 0
	}
	return *a.Balance
}

func (a Account) BalanceUsedOrZero() float64 {
	if a.BalanceUsed == nil {
		return 0
	}
	return *a.BalanceUsed
}

func (a Account) QuotaOrZero() float64 {
	if a.Quota == nil {
		return 0
	}
	return *a.Quota
}

func (a Account) ValueScoreOrZero() float64 {
	if a.ValueScore == nil {
		return 0
	}
	return *a.ValueScore
}

// ---- Table 5: account_tokens ----
type AccountToken struct {
	ID          int64   `db:"id" json:"id"`
	AccountID   int64   `db:"account_id" json:"accountId"`
	Name        string  `db:"name" json:"name"`
	Token       string  `db:"token" json:"token"`
	TokenGroup  *string `db:"token_group" json:"tokenGroup"`
	ValueStatus string  `db:"value_status" json:"valueStatus"`
	Source      string  `db:"source" json:"source"`
	Enabled     bool    `db:"enabled" json:"enabled"`
	IsDefault   bool    `db:"is_default" json:"isDefault"`
	CreatedAt   string  `db:"created_at" json:"createdAt"`
	UpdatedAt   string  `db:"updated_at" json:"updatedAt"`
}

// ---- Table 6: checkin_logs ----
// FailureReason holds the JSON-serialized FailureReason struct produced by
// service/checkin/failure_reason.go ClassifyFailureReason. NULL for success
// or unclassified rows. Surfaced to the API as a parsed `failureReason`
// object by handler/admin/checkin_routes.go getLogs.
type CheckinLog struct {
	ID            int64   `db:"id" json:"id"`
	AccountID     int64   `db:"account_id" json:"accountId"`
	Status        string  `db:"status" json:"status"`
	Message       *string `db:"message" json:"message"`
	Reward        *string `db:"reward" json:"reward"`
	FailureReason *string `db:"failure_reason" json:"failureReason"`
	CreatedAt     string  `db:"created_at" json:"createdAt"`
}

// ---- Table 7: model_availability ----
type ModelAvailability struct {
	ID        int64  `db:"id" json:"id"`
	AccountID int64  `db:"account_id" json:"accountId"`
	ModelName string `db:"model_name" json:"modelName"`
	Available *bool  `db:"available" json:"available"`
	IsManual  bool   `db:"is_manual" json:"isManual"`
	LatencyMs *int64 `db:"latency_ms" json:"latencyMs"`
	CheckedAt string `db:"checked_at" json:"checkedAt"`
}

// ---- Table 8: token_model_availability ----
type TokenModelAvailability struct {
	ID        int64  `db:"id" json:"id"`
	TokenID   int64  `db:"token_id" json:"tokenId"`
	ModelName string `db:"model_name" json:"modelName"`
	Available *bool  `db:"available" json:"available"`
	LatencyMs *int64 `db:"latency_ms" json:"latencyMs"`
	CheckedAt string `db:"checked_at" json:"checkedAt"`
}

// ---- Table 9: token_routes ----
type TokenRoute struct {
	ID                  int64   `db:"id" json:"id"`
	ModelPattern        string  `db:"model_pattern" json:"modelPattern"`
	DisplayName         *string `db:"display_name" json:"displayName"`
	DisplayIcon         *string `db:"display_icon" json:"displayIcon"`
	RouteMode           string  `db:"route_mode" json:"routeMode"`
	ModelMapping        *string `db:"model_mapping" json:"modelMapping"`
	DecisionSnapshot    *string `db:"decision_snapshot" json:"decisionSnapshot"`
	DecisionRefreshedAt *string `db:"decision_refreshed_at" json:"decisionRefreshedAt"`
	RoutingStrategy     string  `db:"routing_strategy" json:"routingStrategy"`
	// ContextLength is optional route-level context window metadata (tokens).
	// NULL means unknown / no enforcement — preserves pre-SC2 behavior.
	// No dedicated model_catalog table exists; token_routes is the SC0 Option A home.
	ContextLength *int64 `db:"context_length" json:"contextLength"`
	// SortOrder controls admin list order (lower first, then id). Default 0.
	// Nullable in the DDL (DEFAULT 0 without NOT NULL): TS-migrated rows can
	// carry NULL, so keep a pointer — NULL scans to nil instead of failing
	// LoadEnabledRoutes with "converting NULL to int64".
	SortOrder *int64 `db:"sort_order" json:"sortOrder"`
	Enabled   bool   `db:"enabled" json:"enabled"`
	CreatedAt string `db:"created_at" json:"createdAt"`
	UpdatedAt string `db:"updated_at" json:"updatedAt"`
}

// SortOrderOrZero coerces the nullable sort_order for ordering callers.
func (r TokenRoute) SortOrderOrZero() int64 {
	if r.SortOrder == nil {
		return 0
	}
	return *r.SortOrder
}

// ---- Table 10: route_group_sources ----
type RouteGroupSource struct {
	ID            int64 `db:"id" json:"id"`
	GroupRouteID  int64 `db:"group_route_id" json:"groupRouteId"`
	SourceRouteID int64 `db:"source_route_id" json:"sourceRouteId"`
}

// ---- Table 11: oauth_route_units ----
type OAuthRouteUnit struct {
	ID        int64  `db:"id" json:"id"`
	SiteID    int64  `db:"site_id" json:"siteId"`
	Provider  string `db:"provider" json:"provider"`
	Name      string `db:"name" json:"name"`
	Strategy  string `db:"strategy" json:"strategy"`
	Enabled   bool   `db:"enabled" json:"enabled"`
	CreatedAt string `db:"created_at" json:"createdAt"`
	UpdatedAt string `db:"updated_at" json:"updatedAt"`
}

// ---- Table 12: oauth_route_unit_members ----
type OAuthRouteUnitMember struct {
	ID        int64 `db:"id" json:"id"`
	UnitID    int64 `db:"unit_id" json:"unitId"`
	AccountID int64 `db:"account_id" json:"accountId"`
	// SortOrder/SuccessCount/FailCount/TotalLatencyMs/TotalCost are nullable
	// in the DDL (DEFAULT 0 without NOT NULL): TS-migrated rows can carry
	// NULL. Keep pointers so NULL scans to nil instead of failing
	// LoadOAuthRouteUnitMembers with "converting NULL to int64". Numeric
	// callers use the OrZero helpers below.
	SortOrder            *int64   `db:"sort_order" json:"sortOrder"`
	SuccessCount         *int64   `db:"success_count" json:"successCount"`
	FailCount            *int64   `db:"fail_count" json:"failCount"`
	TotalLatencyMs       *int64   `db:"total_latency_ms" json:"totalLatencyMs"`
	TotalCost            *float64 `db:"total_cost" json:"totalCost"`
	LastUsedAt           *string  `db:"last_used_at" json:"lastUsedAt"`
	LastSelectedAt       *string  `db:"last_selected_at" json:"lastSelectedAt"`
	LastFailAt           *string  `db:"last_fail_at" json:"lastFailAt"`
	ConsecutiveFailCount int64    `db:"consecutive_fail_count" json:"consecutiveFailCount"`
	CooldownLevel        int64    `db:"cooldown_level" json:"cooldownLevel"`
	CooldownUntil        *string  `db:"cooldown_until" json:"cooldownUntil"`
	// Structured cooldown reason — same contract as RouteChannel (P0-3).
	CooldownReasonCode *string `db:"cooldown_reason_code" json:"cooldownReasonCode"`
	CooldownReason     *string `db:"cooldown_reason" json:"cooldownReason"`
	CooldownReasonAt   *string `db:"cooldown_reason_at" json:"cooldownReasonAt"`
	CreatedAt          string  `db:"created_at" json:"createdAt"`
	UpdatedAt          string  `db:"updated_at" json:"updatedAt"`
}

// OrZero helpers coerce the nullable numeric member columns for callers that
// need a numeric default (ordering, success/failure recording).
func (m OAuthRouteUnitMember) SortOrderOrZero() int64 {
	if m.SortOrder == nil {
		return 0
	}
	return *m.SortOrder
}

func (m OAuthRouteUnitMember) SuccessCountOrZero() int64 {
	if m.SuccessCount == nil {
		return 0
	}
	return *m.SuccessCount
}

func (m OAuthRouteUnitMember) FailCountOrZero() int64 {
	if m.FailCount == nil {
		return 0
	}
	return *m.FailCount
}

func (m OAuthRouteUnitMember) TotalLatencyMsOrZero() int64 {
	if m.TotalLatencyMs == nil {
		return 0
	}
	return *m.TotalLatencyMs
}

func (m OAuthRouteUnitMember) TotalCostOrZero() float64 {
	if m.TotalCost == nil {
		return 0
	}
	return *m.TotalCost
}

// ---- Table 13: route_channels ----
type RouteChannel struct {
	ID               int64   `db:"id" json:"id"`
	RouteID          int64   `db:"route_id" json:"routeId"`
	AccountID        int64   `db:"account_id" json:"accountId"`
	TokenID          *int64  `db:"token_id" json:"tokenId"`
	OAuthRouteUnitID *int64  `db:"oauth_route_unit_id" json:"oauthRouteUnitId"`
	SourceModel      *string `db:"source_model" json:"sourceModel"`
	// Priority/Weight/SuccessCount/FailCount/TotalLatencyMs/TotalCost are
	// nullable in the DDL (DEFAULT without NOT NULL): TS-migrated rows can
	// carry NULL. Keep pointers so NULL scans to nil instead of failing
	// LoadRouteChannels with "converting NULL to int64" (a single NULL row
	// used to take down the whole routing load). Numeric callers use the
	// OrZero helpers below.
	Priority             *int64   `db:"priority" json:"priority"`
	Weight               *int64   `db:"weight" json:"weight"`
	Enabled              bool     `db:"enabled" json:"enabled"`
	ManualOverride       bool     `db:"manual_override" json:"manualOverride"`
	SuccessCount         *int64   `db:"success_count" json:"successCount"`
	FailCount            *int64   `db:"fail_count" json:"failCount"`
	TotalLatencyMs       *int64   `db:"total_latency_ms" json:"totalLatencyMs"`
	TotalCost            *float64 `db:"total_cost" json:"totalCost"`
	LastUsedAt           *string  `db:"last_used_at" json:"lastUsedAt"`
	LastSelectedAt       *string  `db:"last_selected_at" json:"lastSelectedAt"`
	LastFailAt           *string  `db:"last_fail_at" json:"lastFailAt"`
	ConsecutiveFailCount int64    `db:"consecutive_fail_count" json:"consecutiveFailCount"`
	CooldownLevel        int64    `db:"cooldown_level" json:"cooldownLevel"`
	CooldownUntil        *string  `db:"cooldown_until" json:"cooldownUntil"`
	// Structured reason for the active cooldown (P0-3): classification code,
	// truncated upstream error summary, and the time the triggering failure was
	// recorded. All NULL for rows that cooled down before this schema existed
	// — the UI renders that honestly as "reason not recorded".
	CooldownReasonCode *string `db:"cooldown_reason_code" json:"cooldownReasonCode"`
	CooldownReason     *string `db:"cooldown_reason" json:"cooldownReason"`
	CooldownReasonAt   *string `db:"cooldown_reason_at" json:"cooldownReasonAt"`
}

// OrZero helpers coerce the nullable numeric channel columns for callers that
// need a numeric default (priority layers, weighted selection, success/failure
// recording). Callers that must preserve unknown/nil semantics (JSON display,
// IsChannelRecentlyFailed) read the pointer directly.
func (c RouteChannel) PriorityOrZero() int64 {
	if c.Priority == nil {
		return 0
	}
	return *c.Priority
}

func (c RouteChannel) WeightOrZero() int64 {
	if c.Weight == nil {
		return 0
	}
	return *c.Weight
}

func (c RouteChannel) SuccessCountOrZero() int64 {
	if c.SuccessCount == nil {
		return 0
	}
	return *c.SuccessCount
}

func (c RouteChannel) FailCountOrZero() int64 {
	if c.FailCount == nil {
		return 0
	}
	return *c.FailCount
}

func (c RouteChannel) TotalLatencyMsOrZero() int64 {
	if c.TotalLatencyMs == nil {
		return 0
	}
	return *c.TotalLatencyMs
}

func (c RouteChannel) TotalCostOrZero() float64 {
	if c.TotalCost == nil {
		return 0
	}
	return *c.TotalCost
}

// ---- Table 14: proxy_logs ----
type ProxyLog struct {
	ID                 int64    `db:"id" json:"id"`
	RouteID            *int64   `db:"route_id" json:"routeId"`
	ChannelID          *int64   `db:"channel_id" json:"channelId"`
	AccountID          *int64   `db:"account_id" json:"accountId"`
	DownstreamAPIKeyID *int64   `db:"downstream_api_key_id" json:"downstreamApiKeyId"`
	ModelRequested     *string  `db:"model_requested" json:"modelRequested"`
	ModelActual        *string  `db:"model_actual" json:"modelActual"`
	Status             *string  `db:"status" json:"status"`
	HTTPStatus         *int64   `db:"http_status" json:"httpStatus"`
	IsStream           *bool    `db:"is_stream" json:"isStream"`
	FirstByteLatencyMs *int64   `db:"first_byte_latency_ms" json:"firstByteLatencyMs"`
	LatencyMs          *int64   `db:"latency_ms" json:"latencyMs"`
	PromptTokens       *int64   `db:"prompt_tokens" json:"promptTokens"`
	CompletionTokens   *int64   `db:"completion_tokens" json:"completionTokens"`
	TotalTokens        *int64   `db:"total_tokens" json:"totalTokens"`
	EstimatedCost      *float64 `db:"estimated_cost" json:"estimatedCost"`
	BillingDetails     *string  `db:"billing_details" json:"billingDetails"`
	ClientFamily       *string  `db:"client_family" json:"clientFamily"`
	ClientAppID        *string  `db:"client_app_id" json:"clientAppId"`
	ClientAppName      *string  `db:"client_app_name" json:"clientAppName"`
	ClientConfidence   *string  `db:"client_confidence" json:"clientConfidence"`
	ErrorMessage       *string  `db:"error_message" json:"errorMessage"`
	RetryCount         int64    `db:"retry_count" json:"retryCount"`
	// RequestID is the ingress X-Request-Id / chi RequestID shared across retries.
	RequestID *string `db:"request_id" json:"requestId"`
	CreatedAt string  `db:"created_at" json:"createdAt"`
}

// ---- Table 15: proxy_debug_traces ----
type ProxyDebugTrace struct {
	ID                       int64   `db:"id" json:"id"`
	DownstreamPath           string  `db:"downstream_path" json:"downstreamPath"`
	ClientKind               *string `db:"client_kind" json:"clientKind"`
	SessionID                *string `db:"session_id" json:"sessionId"`
	TraceHint                *string `db:"trace_hint" json:"traceHint"`
	RequestedModel           *string `db:"requested_model" json:"requestedModel"`
	DownstreamAPIKeyID       *int64  `db:"downstream_api_key_id" json:"downstreamApiKeyId"`
	RequestHeadersJSON       *string `db:"request_headers_json" json:"requestHeadersJson"`
	RequestBodyJSON          *string `db:"request_body_json" json:"requestBodyJson"`
	StickySessionKey         *string `db:"sticky_session_key" json:"stickySessionKey"`
	StickyHitChannelID       *int64  `db:"sticky_hit_channel_id" json:"stickyHitChannelId"`
	SelectedChannelID        *int64  `db:"selected_channel_id" json:"selectedChannelId"`
	SelectedRouteID          *int64  `db:"selected_route_id" json:"selectedRouteId"`
	SelectedAccountID        *int64  `db:"selected_account_id" json:"selectedAccountId"`
	SelectedSiteID           *int64  `db:"selected_site_id" json:"selectedSiteId"`
	SelectedSitePlatform     *string `db:"selected_site_platform" json:"selectedSitePlatform"`
	EndpointCandidatesJSON   *string `db:"endpoint_candidates_json" json:"endpointCandidatesJson"`
	EndpointRuntimeStateJSON *string `db:"endpoint_runtime_state_json" json:"endpointRuntimeStateJson"`
	DecisionSummaryJSON      *string `db:"decision_summary_json" json:"decisionSummaryJson"`
	FinalStatus              *string `db:"final_status" json:"finalStatus"`
	FinalHTTPStatus          *int64  `db:"final_http_status" json:"finalHttpStatus"`
	FinalUpstreamPath        *string `db:"final_upstream_path" json:"finalUpstreamPath"`
	FinalResponseHeadersJSON *string `db:"final_response_headers_json" json:"finalResponseHeadersJson"`
	FinalResponseBodyJSON    *string `db:"final_response_body_json" json:"finalResponseBodyJson"`
	CreatedAt                string  `db:"created_at" json:"createdAt"`
	UpdatedAt                string  `db:"updated_at" json:"updatedAt"`
}

// ---- Table 16: proxy_debug_attempts ----
type ProxyDebugAttempt struct {
	ID                  int64   `db:"id" json:"id"`
	TraceID             int64   `db:"trace_id" json:"traceId"`
	AttemptIndex        int64   `db:"attempt_index" json:"attemptIndex"`
	Endpoint            string  `db:"endpoint" json:"endpoint"`
	RequestPath         string  `db:"request_path" json:"requestPath"`
	TargetURL           string  `db:"target_url" json:"targetUrl"`
	RuntimeExecutor     *string `db:"runtime_executor" json:"runtimeExecutor"`
	RequestHeadersJSON  *string `db:"request_headers_json" json:"requestHeadersJson"`
	RequestBodyJSON     *string `db:"request_body_json" json:"requestBodyJson"`
	ResponseStatus      *int64  `db:"response_status" json:"responseStatus"`
	ResponseHeadersJSON *string `db:"response_headers_json" json:"responseHeadersJson"`
	ResponseBodyJSON    *string `db:"response_body_json" json:"responseBodyJson"`
	RawErrorText        *string `db:"raw_error_text" json:"rawErrorText"`
	RecoverApplied      bool    `db:"recover_applied" json:"recoverApplied"`
	DowngradeDecision   bool    `db:"downgrade_decision" json:"downgradeDecision"`
	DowngradeReason     *string `db:"downgrade_reason" json:"downgradeReason"`
	MemoryWriteJSON     *string `db:"memory_write_json" json:"memoryWriteJson"`
	CreatedAt           string  `db:"created_at" json:"createdAt"`
}

// ---- Table 17: proxy_video_tasks ----
type ProxyVideoTask struct {
	ID                   int64   `db:"id" json:"id"`
	PublicID             string  `db:"public_id" json:"publicId"`
	UpstreamVideoID      string  `db:"upstream_video_id" json:"upstreamVideoId"`
	SiteURL              string  `db:"site_url" json:"siteUrl"`
	TokenValue           string  `db:"token_value" json:"tokenValue"`
	RequestedModel       *string `db:"requested_model" json:"requestedModel"`
	ActualModel          *string `db:"actual_model" json:"actualModel"`
	ChannelID            *int64  `db:"channel_id" json:"channelId"`
	AccountID            *int64  `db:"account_id" json:"accountId"`
	StatusSnapshot       *string `db:"status_snapshot" json:"statusSnapshot"`
	UpstreamResponseMeta *string `db:"upstream_response_meta" json:"upstreamResponseMeta"`
	LastUpstreamStatus   *int64  `db:"last_upstream_status" json:"lastUpstreamStatus"`
	LastPolledAt         *string `db:"last_polled_at" json:"lastPolledAt"`
	CreatedAt            string  `db:"created_at" json:"createdAt"`
	UpdatedAt            string  `db:"updated_at" json:"updatedAt"`
}

// ---- Table 18: proxy_files ----
type ProxyFile struct {
	ID            int64   `db:"id" json:"id"`
	PublicID      string  `db:"public_id" json:"publicId"`
	OwnerType     string  `db:"owner_type" json:"ownerType"`
	OwnerID       string  `db:"owner_id" json:"ownerId"`
	Filename      string  `db:"filename" json:"filename"`
	MimeType      string  `db:"mime_type" json:"mimeType"`
	Purpose       *string `db:"purpose" json:"purpose"`
	ByteSize      int64   `db:"byte_size" json:"byteSize"`
	SHA256        string  `db:"sha256" json:"sha256"`
	ContentBase64 string  `db:"content_base64" json:"contentBase64"`
	CreatedAt     string  `db:"created_at" json:"createdAt"`
	UpdatedAt     string  `db:"updated_at" json:"updatedAt"`
	DeletedAt     *string `db:"deleted_at" json:"deletedAt"`
}

// ---- Table 19: settings (text PK) ----
type Setting struct {
	Key   string  `db:"key" json:"key"`
	Value *string `db:"value" json:"value"`
}

// ---- Table 20: admin_snapshots ----
type AdminSnapshot struct {
	ID          int64  `db:"id" json:"id"`
	Namespace   string `db:"namespace" json:"namespace"`
	SnapshotKey string `db:"snapshot_key" json:"snapshotKey"`
	Payload     string `db:"payload" json:"payload"`
	GeneratedAt string `db:"generated_at" json:"generatedAt"`
	ExpiresAt   string `db:"expires_at" json:"expiresAt"`
	StaleUntil  string `db:"stale_until" json:"staleUntil"`
	CreatedAt   string `db:"created_at" json:"createdAt"`
	UpdatedAt   string `db:"updated_at" json:"updatedAt"`
}

// ---- Table 21: analytics_projection_checkpoints (text PK) ----
type AnalyticsProjectionCheckpoint struct {
	ProjectorKey         string  `db:"projector_key" json:"projectorKey"`
	TimeZone             string  `db:"time_zone" json:"timeZone"`
	LastProxyLogID       int64   `db:"last_proxy_log_id" json:"lastProxyLogId"`
	WatermarkCreatedAt   *string `db:"watermark_created_at" json:"watermarkCreatedAt"`
	LeaseOwner           *string `db:"lease_owner" json:"leaseOwner"`
	LeaseToken           *string `db:"lease_token" json:"leaseToken"`
	LeaseExpiresAt       *string `db:"lease_expires_at" json:"leaseExpiresAt"`
	RecomputeFromID      *int64  `db:"recompute_from_id" json:"recomputeFromId"`
	RecomputeRequestedAt *string `db:"recompute_requested_at" json:"recomputeRequestedAt"`
	RecomputeReason      *string `db:"recompute_reason" json:"recomputeReason"`
	RecomputeStartedAt   *string `db:"recompute_started_at" json:"recomputeStartedAt"`
	RecomputeCompletedAt *string `db:"recompute_completed_at" json:"recomputeCompletedAt"`
	LastProjectedAt      *string `db:"last_projected_at" json:"lastProjectedAt"`
	LastSuccessfulAt     *string `db:"last_successful_at" json:"lastSuccessfulAt"`
	LastError            *string `db:"last_error" json:"lastError"`
	CreatedAt            string  `db:"created_at" json:"createdAt"`
	UpdatedAt            string  `db:"updated_at" json:"updatedAt"`
}

// ---- Table 22: site_day_usage ----
type SiteDayUsage struct {
	ID                int64   `db:"id" json:"id"`
	LocalDay          string  `db:"local_day" json:"localDay"`
	SiteID            int64   `db:"site_id" json:"siteId"`
	TotalCalls        int64   `db:"total_calls" json:"totalCalls"`
	SuccessCalls      int64   `db:"success_calls" json:"successCalls"`
	FailedCalls       int64   `db:"failed_calls" json:"failedCalls"`
	TotalTokens       int64   `db:"total_tokens" json:"totalTokens"`
	TotalSummarySpend float64 `db:"total_summary_spend" json:"totalSummarySpend"`
	TotalSiteSpend    float64 `db:"total_site_spend" json:"totalSiteSpend"`
	TotalLatencyMs    int64   `db:"total_latency_ms" json:"totalLatencyMs"`
	LatencyCount      int64   `db:"latency_count" json:"latencyCount"`
	CreatedAt         string  `db:"created_at" json:"createdAt"`
	UpdatedAt         string  `db:"updated_at" json:"updatedAt"`
}

// ---- Table 23: site_hour_usage ----
type SiteHourUsage struct {
	ID                int64   `db:"id" json:"id"`
	BucketStartUTC    string  `db:"bucket_start_utc" json:"bucketStartUtc"`
	SiteID            int64   `db:"site_id" json:"siteId"`
	TotalCalls        int64   `db:"total_calls" json:"totalCalls"`
	SuccessCalls      int64   `db:"success_calls" json:"successCalls"`
	FailedCalls       int64   `db:"failed_calls" json:"failedCalls"`
	TotalTokens       int64   `db:"total_tokens" json:"totalTokens"`
	TotalSummarySpend float64 `db:"total_summary_spend" json:"totalSummarySpend"`
	TotalSiteSpend    float64 `db:"total_site_spend" json:"totalSiteSpend"`
	TotalLatencyMs    int64   `db:"total_latency_ms" json:"totalLatencyMs"`
	LatencyCount      int64   `db:"latency_count" json:"latencyCount"`
	CreatedAt         string  `db:"created_at" json:"createdAt"`
	UpdatedAt         string  `db:"updated_at" json:"updatedAt"`
}

// ---- Table 24: model_day_usage ----
type ModelDayUsage struct {
	ID             int64   `db:"id" json:"id"`
	LocalDay       string  `db:"local_day" json:"localDay"`
	SiteID         int64   `db:"site_id" json:"siteId"`
	Model          string  `db:"model" json:"model"`
	TotalCalls     int64   `db:"total_calls" json:"totalCalls"`
	SuccessCalls   int64   `db:"success_calls" json:"successCalls"`
	FailedCalls    int64   `db:"failed_calls" json:"failedCalls"`
	TotalTokens    int64   `db:"total_tokens" json:"totalTokens"`
	TotalSpend     float64 `db:"total_spend" json:"totalSpend"`
	TotalLatencyMs int64   `db:"total_latency_ms" json:"totalLatencyMs"`
	LatencyCount   int64   `db:"latency_count" json:"latencyCount"`
	CreatedAt      string  `db:"created_at" json:"createdAt"`
	UpdatedAt      string  `db:"updated_at" json:"updatedAt"`
}

// ---- Table 29: balance_history ----
// One snapshot per account per UTC day (UPSERT on local_day+account_id).
// Captured by RefreshBalance on success so operators get a balance trend
// without a separate capture job. Replaces the "current balance only" gap.
type BalanceHistory struct {
	ID          int64   `db:"id" json:"id"`
	AccountID   int64   `db:"account_id" json:"accountId"`
	Balance     float64 `db:"balance" json:"balance"`
	BalanceUsed float64 `db:"balance_used" json:"balanceUsed"`
	Quota       float64 `db:"quota" json:"quota"`
	LocalDay    string  `db:"local_day" json:"localDay"`
	CapturedAt  string  `db:"captured_at" json:"capturedAt"`
	CreatedAt   string  `db:"created_at" json:"createdAt"`
}

// ---- Table 30: model_verify_history ----
type ModelVerifyRecord struct {
	ID         int64    `db:"id" json:"id"`
	BatchID    string   `db:"batch_id" json:"batchId"`
	ModelName  string   `db:"model_name" json:"modelName"`
	ChannelID  *int64   `db:"channel_id" json:"channelId"`
	AccountID  *int64   `db:"account_id" json:"accountId"`
	SiteID     *int64   `db:"site_id" json:"siteId"`
	Status     string   `db:"status" json:"status"`
	LatencyMs  *float64 `db:"latency_ms" json:"latencyMs"`
	HTTPStatus *int64   `db:"http_status" json:"httpStatus"`
	ErrorText  *string  `db:"error_text" json:"errorText"`
	CreatedAt  string   `db:"created_at" json:"createdAt"`
}

// ---- Table 35: model_probe_results ----
type ModelProbeResult struct {
	ID         int64    `db:"id" json:"id"`
	ChannelID  *int64   `db:"channel_id" json:"channelId"`
	AccountID  int64    `db:"account_id" json:"accountId"`
	SiteID     int64    `db:"site_id" json:"siteId"`
	ModelName  string   `db:"model_name" json:"modelName"`
	Status     string   `db:"status" json:"status"` // success | failure | inconclusive | skipped
	LatencyMs  *float64 `db:"latency_ms" json:"latencyMs"`
	HTTPStatus *int64   `db:"http_status" json:"httpStatus"`
	ErrorText  *string  `db:"error_text" json:"errorText"`
	CreatedAt  string   `db:"created_at" json:"createdAt"`
}

// ---- Table 31: product_announcements ----
type ProductAnnouncement struct {
	ID        int64   `db:"id" json:"id"`
	Title     string  `db:"title" json:"title"`
	Message   string  `db:"message" json:"message"`
	Severity  string  `db:"severity" json:"severity"` // info | warning | critical
	Link      *string `db:"link" json:"link"`
	Enabled   bool    `db:"enabled" json:"enabled"`
	CreatedAt string  `db:"created_at" json:"createdAt"`
	UpdatedAt string  `db:"updated_at" json:"updatedAt"`
}

// ---- Table 32: announcement_dismissals ----
type AnnouncementDismissal struct {
	AnnouncementID int64  `db:"announcement_id" json:"announcementId"`
	DismissedAt    string `db:"dismissed_at" json:"dismissedAt"`
}

// ---- Table 33: model_name_redirects ----
// Maps a canonical route model name to the actual upstream name per account
// (e.g. claude-3-5-sonnet → claude-3-5-sonnet-20241022). source is sync
// (auto-generated) or manual (operator-authored, never overwritten).
type ModelNameRedirect struct {
	ID         int64   `db:"id" json:"id"`
	AccountID  int64   `db:"account_id" json:"accountId"`
	Canonical  string  `db:"canonical" json:"canonical"`
	Actual     string  `db:"actual" json:"actual"`
	Source     string  `db:"source" json:"source"` // sync | manual
	LastSeenAt *string `db:"last_seen_at" json:"lastSeenAt"`
	CreatedAt  string  `db:"created_at" json:"createdAt"`
	UpdatedAt  string  `db:"updated_at" json:"updatedAt"`
}

// ---- Table 34: admin_audit_logs ----
// One row per authenticated admin write operation (POST/PUT/PATCH/DELETE).
// actor is a sha256 prefix of the admin bearer token; the raw token is never
// stored. status 0 means the handler panicked before writing (recoverer
// catches it — the row is still useful as evidence the request arrived).
type AdminAuditLog struct {
	ID        int64  `db:"id" json:"id"`
	Actor     string `db:"actor" json:"actor"`
	Method    string `db:"method" json:"method"`
	Path      string `db:"path" json:"path"`
	Status    int    `db:"status" json:"status"`
	RequestID string `db:"request_id" json:"requestId"`
	RemoteIP  string `db:"remote_ip" json:"remoteIp"`
	CreatedAt string `db:"created_at" json:"createdAt"`
}

// ---- Table 25: downstream_api_keys ----
type DownstreamAPIKey struct {
	ID           int64    `db:"id" json:"id"`
	Name         string   `db:"name" json:"name"`
	Key          string   `db:"key" json:"key"`
	Description  *string  `db:"description" json:"description"`
	GroupName    *string  `db:"group_name" json:"groupName"`
	Tags         *string  `db:"tags" json:"tags"`
	Enabled      bool     `db:"enabled" json:"enabled"`
	ExpiresAt    *string  `db:"expires_at" json:"expiresAt"`
	MaxCost      *float64 `db:"max_cost" json:"maxCost"`
	UsedCost     float64  `db:"used_cost" json:"usedCost"`
	MaxRequests  *int64   `db:"max_requests" json:"maxRequests"`
	UsedRequests int64    `db:"used_requests" json:"usedRequests"`
	// MaxRPM / MaxTPM optional soft rate windows. nil = unlimited.
	MaxRPM                 *int64  `db:"max_rpm" json:"maxRpm"`
	MaxTPM                 *int64  `db:"max_tpm" json:"maxTpm"`
	SupportedModels        *string `db:"supported_models" json:"supportedModels"`
	AllowedRouteIDs        *string `db:"allowed_route_ids" json:"allowedRouteIds"`
	SiteWeightMultipliers  *string `db:"site_weight_multipliers" json:"siteWeightMultipliers"`
	ExcludedSiteIDs        *string `db:"excluded_site_ids" json:"excludedSiteIds"`
	ExcludedCredentialRefs *string `db:"excluded_credential_refs" json:"excludedCredentialRefs"`
	// AllowedSiteIDs / AllowedCredentialRefs are optional allow-lists.
	// Empty/NULL means unrestricted (legacy exclude-only semantics).
	// When non-empty, only listed sites/credentials are eligible; exclusions still apply.
	AllowedSiteIDs        *string `db:"allowed_site_ids" json:"allowedSiteIds"`
	AllowedCredentialRefs *string `db:"allowed_credential_refs" json:"allowedCredentialRefs"`
	// KeyWeight is an optional per-downstream-key scalar.
	// NULL means 1.0 (no change). Positive values amplify channel.Weight
	// differences in weighted selection: (channel.weight*keyWeight + 10)*...
	KeyWeight *float64 `db:"key_weight" json:"keyWeight"`
	// ProxyURL is an optional per-key egress proxy override.
	// NULL falls back to site / system proxy — preserves pre-SC2 behavior.
	ProxyURL   *string `db:"proxy_url" json:"proxyUrl"`
	LastUsedAt *string `db:"last_used_at" json:"lastUsedAt"`
	CreatedAt  string  `db:"created_at" json:"createdAt"`
	// IPAllowlist / IPBlocklist: newline/comma-
	// separated CIDR or exact IPs. Empty/NULL = unrestricted. Blocklist wins.
	IPAllowlist *string `db:"ip_allowlist" json:"ipAllowlist"`
	IPBlocklist *string `db:"ip_blocklist" json:"ipBlocklist"`
	UpdatedAt   string  `db:"updated_at" json:"updatedAt"`
}

// ---- Table 26: site_announcements ----
type SiteAnnouncement struct {
	ID                int64   `db:"id" json:"id"`
	SiteID            int64   `db:"site_id" json:"siteId"`
	Platform          string  `db:"platform" json:"platform"`
	SourceKey         string  `db:"source_key" json:"sourceKey"`
	Title             string  `db:"title" json:"title"`
	Content           string  `db:"content" json:"content"`
	Level             string  `db:"level" json:"level"`
	SourceURL         *string `db:"source_url" json:"sourceUrl"`
	StartsAt          *string `db:"starts_at" json:"startsAt"`
	EndsAt            *string `db:"ends_at" json:"endsAt"`
	UpstreamCreatedAt *string `db:"upstream_created_at" json:"upstreamCreatedAt"`
	UpstreamUpdatedAt *string `db:"upstream_updated_at" json:"upstreamUpdatedAt"`
	FirstSeenAt       string  `db:"first_seen_at" json:"firstSeenAt"`
	LastSeenAt        string  `db:"last_seen_at" json:"lastSeenAt"`
	ReadAt            *string `db:"read_at" json:"readAt"`
	DismissedAt       *string `db:"dismissed_at" json:"dismissedAt"`
	RawPayload        *string `db:"raw_payload" json:"rawPayload"`
}

// ---- Table 27: events ----
// ---- Table 28: admin_background_tasks ----
type AdminBackgroundTask struct {
	ID         int64   `db:"id" json:"id"`
	TaskID     string  `db:"task_id" json:"taskId"`
	Type       string  `db:"type" json:"type"`
	Title      string  `db:"title" json:"title"`
	Status     string  `db:"status" json:"status"`
	Message    *string `db:"message" json:"message"`
	Error      *string `db:"error" json:"error"`
	ResultJSON *string `db:"result_json" json:"resultJson"`
	DedupeKey  *string `db:"dedupe_key" json:"dedupeKey"`
	CreatedAt  string  `db:"created_at" json:"createdAt"`
	UpdatedAt  string  `db:"updated_at" json:"updatedAt"`
	StartedAt  *string `db:"started_at" json:"startedAt"`
	FinishedAt *string `db:"finished_at" json:"finishedAt"`
	LogsJSON   *string `db:"logs_json" json:"logsJson"`
}

type Event struct {
	ID          int64   `db:"id" json:"id"`
	Type        string  `db:"type" json:"type"`
	Title       string  `db:"title" json:"title"`
	Message     *string `db:"message" json:"message"`
	Level       string  `db:"level" json:"level"`
	Read        bool    `db:"read" json:"read"`
	RelatedID   *int64  `db:"related_id" json:"relatedId"`
	RelatedType *string `db:"related_type" json:"relatedType"`
	CreatedAt   string  `db:"created_at" json:"createdAt"`
}
