package app

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
)

// Deleting the empty coordinator must not replace the legacy zero-load signal
// with historical request volume for least_busy session channels.
func TestProxyLoadProviderPreservesSessionRanking(t *testing.T) {
	prev := config.RuntimeSafe()
	t.Cleanup(func() { config.SetRuntime(prev) })
	session := `{"credentialMode":"session"}`
	oauth := "claude"
	var many int64 = 1000
	var few int64 = 1
	candidates := []routing.RouteChannelCandidate{
		{Channel: store.RouteChannel{ID: 1, SuccessCount: &many}, Account: store.Account{ExtraConfig: &session}},
		{Channel: store.RouteChannel{ID: 2, SuccessCount: &few}},
	}
	provider := proxyLoadProvider{}
	for _, limit := range []int{2, 7, 0, -1} {
		config.SetRuntime(&config.RuntimeSettings{ProxySessionChannelConcurrencyLimit: limit})
		for _, params := range []routing.ChannelLoadParams{
			{AccountExtraConfig: &session},
			{AccountOAuthProvider: &oauth},
		} {
			snap := provider.GetChannelLoadSnapshot(params)
			if !snap.SessionScoped || snap.ConcurrencyLimit != max(0, limit) ||
				snap.ActiveLeaseCount != 0 || snap.WaitingCount != 0 || snap.Saturated {
				t.Fatalf("limit=%d: snapshot=%+v", limit, snap)
			}
			if got := routing.ResolveChannelRuntimeLoadMultiplier(snap); got != 1 {
				t.Fatalf("legacy snapshot changed weighted routing: multiplier=%v", got)
			}
		}
		if snap := provider.GetChannelLoadSnapshot(routing.ChannelLoadParams{}); snap != (routing.ChannelLoadSnapshot{}) {
			t.Fatalf("API Key channel acquired a session load signal: %+v", snap)
		}
		wantID := int64(2)
		if limit > 0 {
			wantID = 1
		}
		if got := routing.SelectLeastBusyCandidate(candidates, provider); got == nil || got.Channel.ID != wantID {
			t.Fatalf("limit=%d: selected=%+v, want channel %d", limit, got, wantID)
		}
	}
}
