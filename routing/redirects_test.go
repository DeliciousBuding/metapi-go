package routing

import (
	"context"
	"testing"

	"github.com/tokendancelab/metapi-go/store"
)

// ---- K1b (all-api-hub borrow): redirect registry integration ----
//
// End-to-end selector behavior: a channel whose source_model is the upstream
// actual name (claude-3-5-sonnet-20241022) becomes eligible for a canonical
// request (claude-3-5-sonnet) once the registry holds the mapping, and the
// selected channel forwards the actual name (proxy attribution stays on the
// canonical requested name via ctx.RequestedModel).

func TestSelectChannel_RedirectRegistryEnablesActualChannel(t *testing.T) {
	actualSource := "claude-3-5-sonnet-20241022"
	token := store.AccountToken{ID: 1, AccountID: 7, Name: "default", Token: "tok-redirect", Enabled: true, IsDefault: true}

	db := &sourceModelFallbackDB{
		routes: []store.TokenRoute{{
			ID:            1,
			ModelPattern:  "claude-3-5-sonnet",
			RouteMode:     "pattern",
			RoutingStrategy: "round_robin",
			Enabled:       true,
		}},
		channels: []struct {
			Channel store.RouteChannel
			Account store.Account
			Site    store.Site
			Token   *store.AccountToken
		}{
			{
				Channel: store.RouteChannel{
					ID: 1, RouteID: 1, AccountID: 7, TokenID: &token.ID,
					SourceModel: &actualSource, Priority: 0, Weight: 10, Enabled: true,
				},
				Account: store.Account{ID: 7, SiteID: 1, Status: "active"},
				Site:    store.Site{ID: 1, Name: "site-a", Status: "active", GlobalWeight: 1},
				Token:   &token,
			},
		},
	}

	selector := NewChannelSelector(db, NewRouteCache(60_000), 3600, defaultRoutingWeights(), nil, 1, nil)

	// Without the registry: the actual-named channel does not support the
	// canonical request → no selection (K1a/K1b pre-state).
	sel, err := selector.SelectChannel(context.Background(), "claude-3-5-sonnet", DownstreamRoutingPolicy{})
	if err != nil {
		t.Fatalf("select without registry: %v", err)
	}
	if sel != nil {
		t.Fatalf("expected no selection without registry, got %+v", sel)
	}

	// With the registry: channel eligible, outbound model = actual name.
	SetModelRedirects(map[int64]map[string]string{
		7: {"claude-3-5-sonnet": actualSource},
	})
	t.Cleanup(func() { SetModelRedirects(nil) })

	sel, err = selector.SelectChannel(context.Background(), "claude-3-5-sonnet", DownstreamRoutingPolicy{})
	if err != nil {
		t.Fatalf("select with registry: %v", err)
	}
	if sel == nil {
		t.Fatal("expected selection with registry")
	}
	if sel.Channel.ID != 1 {
		t.Fatalf("selected channel %d, want 1", sel.Channel.ID)
	}
	if sel.ActualModel != actualSource {
		t.Fatalf("ActualModel = %q, want %q (forward rewrite)", sel.ActualModel, actualSource)
	}

	// Direct actual-name request does NOT match the canonical route pattern —
	// clients are expected to request canonical names; K1b only forwards
	// canonical → actual on the wire. Verify no crash and no selection.
	sel, err = selector.SelectChannel(context.Background(), actualSource, DownstreamRoutingPolicy{})
	if err != nil {
		t.Fatalf("select exact actual: %v", err)
	}
	if sel != nil {
		t.Fatalf("expected no route match for bare actual name, got %+v", sel)
	}
	// Another account without the mapping: still rejected.
	SetModelRedirects(map[int64]map[string]string{
		99: {"claude-3-5-sonnet": actualSource},
	})
	sel, err = selector.SelectChannel(context.Background(), "claude-3-5-sonnet", DownstreamRoutingPolicy{})
	if err != nil {
		t.Fatalf("select other account: %v", err)
	}
	if sel != nil {
		t.Fatalf("expected no selection for account without mapping, got %+v", sel)
	}
}
