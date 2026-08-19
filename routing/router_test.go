package routing

// TokenRouter public-surface tests. The deep-test coverage audit found the
// entire TokenRouter API at 0%: NewTokenRouter defaults, GetAvailableModels /
// GetAvailableModelContextLengths (route → exposed-model mapping), error
// propagation, and the SelectChannel delegation path.

import (
	"context"
	"errors"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

// routerTestDB is a minimal ChannelSelectorDB: FindAllEnabledRoutes and
// LoadEnabledRoutes serve the configured route list; every other method is a
// no-op (only exercised by selector flows this test file does not drive).
type routerTestDB struct {
	routes  []store.TokenRoute
	routes2 []store.TokenRoute // LoadEnabledRoutes view (selector path)
	err     error
}

func (db *routerTestDB) LoadEnabledRoutes(ctx context.Context) ([]store.TokenRoute, error) {
	if db.err != nil {
		return nil, db.err
	}
	return db.routes2, nil
}
func (db *routerTestDB) FindAllEnabledRoutes(ctx context.Context) ([]store.TokenRoute, error) {
	if db.err != nil {
		return nil, db.err
	}
	return db.routes, nil
}
func (db *routerTestDB) LoadRouteGroupSources(ctx context.Context, groupRouteIDs []int64) (map[int64][]int64, error) {
	return nil, nil
}
func (db *routerTestDB) LoadRouteChannels(ctx context.Context, routeIDs []int64) ([]struct {
	Channel store.RouteChannel
	Account store.Account
	Site    store.Site
	Token   *store.AccountToken
}, error) {
	return nil, nil
}
func (db *routerTestDB) LoadOAuthRouteUnitSummaries(ctx context.Context, unitIDs []int64) (map[int64]OAuthRouteUnitSummary, error) {
	return nil, nil
}
func (db *routerTestDB) LoadOAuthRouteUnitMembers(ctx context.Context, unitIDs []int64) (map[int64][]OAuthRouteUnitMemberCandidate, error) {
	return nil, nil
}
func (db *routerTestDB) UpdateChannelLastSelectedAt(ctx context.Context, channelID int64, lastSelectedAt string) error {
	return nil
}
func (db *routerTestDB) UpdateRouteUnitMemberLastSelectedAt(ctx context.Context, unitID, accountID int64, lastSelectedAt string) error {
	return nil
}
func (db *routerTestDB) FindRouteIDsByOAuthRouteUnitID(ctx context.Context, unitID int64) ([]int64, error) {
	return nil, nil
}
func (db *routerTestDB) LoadCredentialScopedChannelIDs(ctx context.Context, channel store.RouteChannel, accountID int64) ([]int64, error) {
	return []int64{channel.ID}, nil
}
func (db *routerTestDB) LoadChannelWithAccount(ctx context.Context, channelID int64) (*struct {
	Channel store.RouteChannel
	Account store.Account
}, error) {
	return nil, nil
}
func (db *routerTestDB) LoadChannelWithAccountAndRoute(ctx context.Context, channelID int64) (*struct {
	Channel store.RouteChannel
	Account store.Account
	Route   store.TokenRoute
}, error) {
	return nil, nil
}
func (db *routerTestDB) UpdateChannelCooldownFields(ctx context.Context, channelIDs []int64, updates map[string]interface{}) error {
	return nil
}
func (db *routerTestDB) UpdateChannelSuccessFields(ctx context.Context, channelID int64, updates map[string]interface{}) error {
	return nil
}
func (db *routerTestDB) UpdateRouteUnitMemberCooldownFields(ctx context.Context, memberID int64, updates map[string]interface{}) error {
	return nil
}
func (db *routerTestDB) UpdateRouteUnitMemberSuccessFields(ctx context.Context, memberID int64, updates map[string]interface{}) error {
	return nil
}
func (db *routerTestDB) LoadRouteUnitMemberWithAccount(ctx context.Context, unitID, accountID int64) (*struct {
	Member  store.OAuthRouteUnitMember
	Account store.Account
	Unit    store.OAuthRouteUnit
}, error) {
	return nil, nil
}
func (db *routerTestDB) LoadChannelsByTokenID(ctx context.Context, tokenID int64) ([]store.RouteChannel, error) {
	return nil, nil
}
func (db *routerTestDB) LoadChannelsByAccountIDWithoutToken(ctx context.Context, accountID int64) ([]store.RouteChannel, error) {
	return nil, nil
}
func (db *routerTestDB) LoadRuntimeHealthChannelRows(ctx context.Context, channelIDs []int64) ([]struct {
	SiteID            int64
	SourceModel       *string
	RouteModelPattern string
}, error) {
	return nil, nil
}
func (db *routerTestDB) ClearChannelFailureStates(ctx context.Context, channelIDs []int64) error {
	return nil
}

func newTestTokenRouter(db *routerTestDB) *TokenRouter {
	cfg := &config.Config{}
	return NewTokenRouter(db, cfg, nil, nil)
}

func TestNewTokenRouter_AppliesDefaults(t *testing.T) {
	db := &routerTestDB{}
	tr := NewTokenRouter(db, &config.Config{}, nil, nil)
	if tr.configuredMaxSec != TokenRouterFailureCooldownMaxSecCeiling {
		t.Fatalf("configuredMaxSec = %d, want ceiling %d when config is 0", tr.configuredMaxSec, TokenRouterFailureCooldownMaxSecCeiling)
	}
	if tr.fallbackUnitCost != 1 {
		t.Fatalf("fallbackUnitCost = %v, want 1 when config is 0", tr.fallbackUnitCost)
	}
}

func TestTokenRouter_GetAvailableModels(t *testing.T) {
	// FindAllEnabledRoutes (the DB side) already filters disabled routes;
	// the router maps whatever it is given.
	db := &routerTestDB{routes: []store.TokenRoute{
		{ID: 1, ModelPattern: "gpt-4o", Enabled: true},
		{ID: 2, ModelPattern: "gpt-4o-mini", Enabled: true},
		{ID: 3, ModelPattern: "gpt-4o", Enabled: true}, // duplicate exposes once
	}}
	tr := newTestTokenRouter(db)

	names, err := tr.GetAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("GetAvailableModels: %v", err)
	}
	if len(names) != 2 {
		t.Fatalf("exposed names = %v, want exactly {gpt-4o, gpt-4o-mini}", names)
	}
	set := map[string]bool{}
	for _, name := range names {
		set[name] = true
	}
	if !set["gpt-4o"] || !set["gpt-4o-mini"] {
		t.Fatalf("exposed names = %v, want gpt-4o and gpt-4o-mini (disabled + duplicates excluded)", names)
	}
}

func TestTokenRouter_GetAvailableModels_WildcardDisplayNameCoversExact(t *testing.T) {
	wildcardDisplay := "gpt-all" // must differ from the pattern to count as a custom display name
	db := &routerTestDB{routes: []store.TokenRoute{
		{ID: 1, ModelPattern: "gpt-*", DisplayName: &wildcardDisplay, Enabled: true},
		{ID: 2, ModelPattern: "gpt-4o", Enabled: true}, // covered by the wildcard display route
		{ID: 3, ModelPattern: "claude-3-5-sonnet", Enabled: true},
	}}
	tr := newTestTokenRouter(db)

	names, err := tr.GetAvailableModels(context.Background())
	if err != nil {
		t.Fatalf("GetAvailableModels: %v", err)
	}
	set := map[string]bool{}
	for _, name := range names {
		set[name] = true
	}
	if set["gpt-4o"] {
		t.Fatalf("gpt-4o should be covered by the gpt-* wildcard display route, names=%v", names)
	}
	if !set["gpt-all"] || !set["claude-3-5-sonnet"] {
		t.Fatalf("exposed names = %v, want the wildcard display name and the uncovered exact model", names)
	}
}

func TestTokenRouter_GetAvailableModelContextLengths_MaxWins(t *testing.T) {
	db := &routerTestDB{routes: []store.TokenRoute{
		{ID: 1, ModelPattern: "gpt-4o", Enabled: true, ContextLength: int64Ptr(64000)},
		{ID: 2, ModelPattern: "gpt-4o", Enabled: true, ContextLength: int64Ptr(128000)},
		{ID: 3, ModelPattern: "gpt-4o", Enabled: true, ContextLength: nil},
	}}
	tr := newTestTokenRouter(db)

	lengths, err := tr.GetAvailableModelContextLengths(context.Background())
	if err != nil {
		t.Fatalf("GetAvailableModelContextLengths: %v", err)
	}
	if lengths["gpt-4o"] != 128000 {
		t.Fatalf("gpt-4o = %d, want max positive 128000", lengths["gpt-4o"])
	}
}

func TestTokenRouter_GetAvailableModels_DBErrorPropagates(t *testing.T) {
	db := &routerTestDB{err: errors.New("boom")}
	tr := newTestTokenRouter(db)

	if _, err := tr.GetAvailableModels(context.Background()); err == nil {
		t.Fatal("GetAvailableModels should surface the DB error")
	}
	if _, err := tr.GetAvailableModelContextLengths(context.Background()); err == nil {
		t.Fatal("GetAvailableModelContextLengths should surface the DB error")
	}
}

func TestTokenRouter_SelectChannel_NoRoutes(t *testing.T) {
	db := &routerTestDB{}
	tr := newTestTokenRouter(db)

	// With no routes the composed ChannelSelector returns a nil channel
	// without error ("no route matched" is a normal routing outcome; the
	// caller treats nil as no-route-available). This asserts the delegation
	// path runs through NewTokenRouter's composed selector.
	channel, err := tr.SelectChannel(context.Background(), "gpt-4o", DownstreamRoutingPolicy{})
	if err != nil {
		t.Fatalf("SelectChannel with no routes should not error, got %v", err)
	}
	if channel != nil {
		t.Fatalf("SelectChannel with no routes should return nil channel, got %+v", channel)
	}
}
