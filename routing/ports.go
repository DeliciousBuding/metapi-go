// Package routing implements the TokenRouter route selection engine.
// Split from the TS 3800-line monolith into independent modules with
// interface contracts and unidirectional dependencies.
package routing

import (
	"context"

	"github.com/deliciousbuding/metapi-go/store"
)

// ModelProvider supplies model availability data.
type ModelProvider interface {
	GetAvailableModels(ctx context.Context, accountID int64) ([]ModelInfo, error)
	RefreshModelsForAccount(ctx context.Context, accountID int64) error
}

// ModelInfo is a lightweight model availability record.
type ModelInfo struct {
	ModelName string
	Available bool
	LatencyMs *int64
}

// TokenProvider supplies token data.
type TokenProvider interface {
	GetTokens(ctx context.Context, accountID int64) ([]store.AccountToken, error)
	GetDefaultToken(ctx context.Context, accountID int64) (*store.AccountToken, error)
}

// PricingProvider supplies model pricing reference costs.
type PricingProvider interface {
	GetReferenceCost(ctx context.Context, model string, siteID int64, accountID int64) (float64, error)
	RefreshModelPricingCatalog(ctx context.Context, site store.Site, account store.Account, modelName string) error
}

// Catalog pricing provenance labels for cold-start cost routing.
const (
	// CatalogSourceOfficial labels an official vendor list price from the
	// catalog (site points at the vendor's own API host).
	CatalogSourceOfficial = "catalog"
	// CatalogSourceRelayEstimate labels an official list price used only as
	// an estimate for a third-party relay site — never a real payment price.
	CatalogSourceRelayEstimate = "catalog_estimate"
)

// CatalogPricingResolver supplies catalog unit costs for cold-start routing
// (no history, no configured unit_cost yet). Implementations must label
// relay-site estimates CatalogSourceRelayEstimate instead of presenting an
// official list price as a real relay price.
type CatalogPricingResolver interface {
	// ResolveCatalogPricing returns the catalog unit cost for a model on a
	// site/account plus its provenance. (nil, "") declines the query, in
	// which case EffectiveUnitCost falls through to the fallback cost.
	ResolveCatalogPricing(siteID, accountID int64, modelName string) (unitCost *float64, source string)
}

// ChannelLoadSnapshotProvider supplies per-channel concurrency load snapshots.
type ChannelLoadSnapshotProvider interface {
	GetChannelLoadSnapshot(params ChannelLoadParams) ChannelLoadSnapshot
}

// ChannelLoadParams are the parameters for resolving a channel's load snapshot.
type ChannelLoadParams struct {
	ChannelID            int64
	AccountExtraConfig   *string
	AccountOAuthProvider *string
}

// ChannelLoadSnapshot is a snapshot of a channel's concurrency load.
type ChannelLoadSnapshot struct {
	SessionScoped    bool
	ConcurrencyLimit int
	ActiveLeaseCount int
	WaitingCount     int
	Saturated        bool
}

// RouteRebuilder rebuilds token routes from model availability data.
type RouteRebuilder interface {
	RebuildTokenRoutesFromAvailability(ctx context.Context) error
}

// DownstreamRoutingPolicy mirrors TS DownstreamRoutingPolicy.
type DownstreamRoutingPolicy struct {
	ExcludedSiteIDs        []int64
	ExcludedCredentialRefs []CredentialRef
	// AllowedSiteIDs / AllowedCredentialRefs: optional allow-lists.
	// Empty = unrestricted; non-empty = only listed sites/credentials eligible.
	AllowedSiteIDs        []int64
	AllowedCredentialRefs []CredentialRef
	AllowedRouteIDs       []int64
	SupportedModels       []string
	DenyAllWhenEmpty      bool
	SiteWeightMultipliers map[int64]float64
	// KeyWeight multiplies channel.Weight in weighted selection.
	// 0 or negative is treated as 1.0 (no-op).
	KeyWeight float64
	// RequestedContextTokens is a best-effort inbound context estimate for
	// multi-tier route pick. 0 means unknown → first-match honesty.
	RequestedContextTokens int64
}

// CredentialRef identifies a specific credential to exclude.
type CredentialRef struct {
	Kind      string `json:"kind"` // "account_token" or empty
	TokenID   int64  `json:"tokenId"`
	AccountID int64  `json:"accountId"`
	SiteID    int64  `json:"siteId"`
}

// EmptyDownstreamRoutingPolicy is the default allow-all policy.
var EmptyDownstreamRoutingPolicy = DownstreamRoutingPolicy{
	SiteWeightMultipliers: map[int64]float64{},
}

// SelectedChannel is the result of a successful channel selection.
type SelectedChannel struct {
	Channel     store.RouteChannel
	Account     store.Account
	Site        store.Site
	Token       *store.AccountToken
	TokenValue  string
	TokenName   string
	ActualModel string
	// ContextLength is the matched token_routes.context_length (tokens).
	// nil or <=0 means unknown / no max_tokens enforcement on the proxy path.
	ContextLength *int64
}

// RouteDecisionExplanation mirrors TS RouteDecisionExplanation.
type RouteDecisionExplanation struct {
	RequestedModel    string                   `json:"requestedModel"`
	ActualModel       string                   `json:"actualModel"`
	Matched           bool                     `json:"matched"`
	RouteID           *int64                   `json:"routeId,omitempty"`
	ModelPattern      string                   `json:"modelPattern,omitempty"`
	SelectedChannelID *int64                   `json:"selectedChannelId,omitempty"`
	SelectedAccountID *int64                   `json:"selectedAccountId,omitempty"`
	SelectedLabel     string                   `json:"selectedLabel,omitempty"`
	Summary           []string                 `json:"summary"`
	Candidates        []RouteDecisionCandidate `json:"candidates"`
}

// RouteDecisionCandidate mirrors TS RouteDecisionCandidate.
type RouteDecisionCandidate struct {
	ChannelID              int64   `json:"channelId"`
	AccountID              int64   `json:"accountId"`
	Username               string  `json:"username"`
	SiteName               string  `json:"siteName"`
	TokenName              string  `json:"tokenName"`
	Priority               int64   `json:"priority"`
	Weight                 int64   `json:"weight"`
	Eligible               bool    `json:"eligible"`
	RecentlyFailed         bool    `json:"recentlyFailed"`
	AvoidedByRecentFailure bool    `json:"avoidedByRecentFailure"`
	Probability            float64 `json:"probability"`
	Reason                 string  `json:"reason"`
}

// RouteRoutingStrategy is the strategy for a route.
type RouteRoutingStrategy string

const (
	StrategyWeighted      RouteRoutingStrategy = "weighted"
	StrategyRoundRobin    RouteRoutingStrategy = "round_robin"
	StrategyStableFirst   RouteRoutingStrategy = "stable_first"
	StrategyLeastBusy     RouteRoutingStrategy = "least_busy"
	StrategyLowestLatency RouteRoutingStrategy = "lowest_latency"
	StrategyLowestCost    RouteRoutingStrategy = "lowest_cost"
)

// KnownRouteRoutingStrategies lists operator-selectable strategies.
var KnownRouteRoutingStrategies = []RouteRoutingStrategy{
	StrategyWeighted,
	StrategyRoundRobin,
	StrategyStableFirst,
	StrategyLeastBusy,
	StrategyLowestLatency,
	StrategyLowestCost,
}

// NormalizeRouteRoutingStrategy normalizes a strategy string.
func NormalizeRouteRoutingStrategy(value string) RouteRoutingStrategy {
	switch value {
	case "round_robin":
		return StrategyRoundRobin
	case "stable_first":
		return StrategyStableFirst
	case "least_busy":
		return StrategyLeastBusy
	case "lowest_latency", "latency":
		return StrategyLowestLatency
	case "lowest_cost", "cost":
		return StrategyLowestCost
	default:
		return StrategyWeighted
	}
}

// RouteMatch holds a matched route with its resolved channels.
type RouteMatch struct {
	Route    store.TokenRoute
	Channels []RouteChannelCandidate
}

// RouteChannelCandidate is a channel joined with account, site, token, and optional OAuth route unit.
type RouteChannelCandidate struct {
	Channel          store.RouteChannel
	Account          store.Account
	Site             store.Site
	Token            *store.AccountToken
	RouteUnit        *OAuthRouteUnitSummary
	RouteUnitMembers []OAuthRouteUnitMemberCandidate
}

// OAuthRouteUnitSummary is a light summary of an OAuth route unit.
type OAuthRouteUnitSummary struct {
	ID       int64
	SiteID   int64
	Provider string
	Name     string
	Strategy string
	Enabled  bool
}

// OAuthRouteUnitMemberCandidate is a member candidate with account and site info.
type OAuthRouteUnitMemberCandidate struct {
	Member  store.OAuthRouteUnitMember
	Account store.Account
	Site    store.Site
}

// SiteRuntimeFailureContext describes a failure event for runtime health tracking.
type SiteRuntimeFailureContext struct {
	Status    *int
	ErrorText *string
	ModelName *string
}

// CostSignal describes the unit cost and its provenance.
type CostSignal struct {
	UnitCost float64
	Source   string // "observed", "configured", "catalog", "catalog_estimate", "fallback"
}

// PricingReferenceRefreshOptions configures pricing refresh behavior.
type PricingReferenceRefreshOptions struct {
	UseChannelSourceModelForCost bool
	DownstreamPolicy             DownstreamRoutingPolicy
	RefreshedKeys                *map[string]struct{}
}

// ChannelSelectorDB defines the DB operations needed by the selector.
type ChannelSelectorDB interface {
	// Route operations
	LoadEnabledRoutes(ctx context.Context) ([]store.TokenRoute, error)
	LoadRouteGroupSources(ctx context.Context, groupRouteIDs []int64) (map[int64][]int64, error)

	// Channel operations
	LoadRouteChannels(ctx context.Context, routeIDs []int64) ([]struct {
		Channel store.RouteChannel
		Account store.Account
		Site    store.Site
		Token   *store.AccountToken
	}, error)

	// OAuth route unit operations
	LoadOAuthRouteUnitSummaries(ctx context.Context, unitIDs []int64) (map[int64]OAuthRouteUnitSummary, error)
	LoadOAuthRouteUnitMembers(ctx context.Context, unitIDs []int64) (map[int64][]OAuthRouteUnitMemberCandidate, error)

	// Channel mutation
	UpdateChannelLastSelectedAt(ctx context.Context, channelID int64, lastSelectedAt string) error
	UpdateRouteUnitMemberLastSelectedAt(ctx context.Context, unitID, accountID int64, lastSelectedAt string) error

	// Route unit member routes
	FindRouteIDsByOAuthRouteUnitID(ctx context.Context, unitID int64) ([]int64, error)

	// Load credential-scoped channel IDs
	LoadCredentialScopedChannelIDs(ctx context.Context, channel store.RouteChannel, accountID int64) ([]int64, error)

	// Load channel by ID with joins
	LoadChannelWithAccount(ctx context.Context, channelID int64) (*struct {
		Channel store.RouteChannel
		Account store.Account
	}, error)

	LoadChannelWithAccountAndRoute(ctx context.Context, channelID int64) (*struct {
		Channel store.RouteChannel
		Account store.Account
		Route   store.TokenRoute
	}, error)

	// Batch updates
	UpdateChannelCooldownFields(ctx context.Context, channelIDs []int64, updates map[string]interface{}) error
	UpdateChannelSuccessFields(ctx context.Context, channelID int64, updates map[string]interface{}) error

	// Route unit member updates
	UpdateRouteUnitMemberCooldownFields(ctx context.Context, memberID int64, updates map[string]interface{}) error
	UpdateRouteUnitMemberSuccessFields(ctx context.Context, memberID int64, updates map[string]interface{}) error

	// Load member with account+unit
	LoadRouteUnitMemberWithAccount(ctx context.Context, unitID, accountID int64) (*struct {
		Member  store.OAuthRouteUnitMember
		Account store.Account
		Unit    store.OAuthRouteUnit
	}, error)

	// Find all routes
	FindAllEnabledRoutes(ctx context.Context) ([]store.TokenRoute, error)

	// Credential scoping
	LoadChannelsByTokenID(ctx context.Context, tokenID int64) ([]store.RouteChannel, error)
	LoadChannelsByAccountIDWithoutToken(ctx context.Context, accountID int64) ([]store.RouteChannel, error)

	// Runtime health
	LoadRuntimeHealthChannelRows(ctx context.Context, channelIDs []int64) ([]struct {
		SiteID            int64
		SourceModel       *string
		RouteModelPattern string
	}, error)

	// Clear channel failure states
	ClearChannelFailureStates(ctx context.Context, channelIDs []int64) error
}


