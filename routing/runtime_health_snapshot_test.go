package routing

import "testing"

func TestSnapshotRuntimeHealth(t *testing.T) {
	ResetSiteRuntimeHealthState()
	defer ResetSiteRuntimeHealthState()

	empty := SnapshotRuntimeHealth()
	if empty.SitesTracked != 0 || empty.ModelsTracked != 0 {
		t.Fatalf("expected empty snapshot, got sites=%d models=%d", empty.SitesTracked, empty.ModelsTracked)
	}
	if empty.OpenBreakers == nil {
		t.Fatalf("expected non-nil OpenBreakers slice for stable JSON []")
	}

	until := nowMs() + 60_000
	healthStateMu.Lock()
	siteRuntimeHealthStates[7] = &SiteRuntimeHealthState{
		BreakerLevel:   1,
		BreakerUntilMs: &until,
		PenaltyScore:   3,
	}
	if siteModelRuntimeHealthStates[7] == nil {
		siteModelRuntimeHealthStates[7] = map[string]*SiteRuntimeHealthState{}
	}
	siteModelRuntimeHealthStates[7]["gpt-4"] = &SiteRuntimeHealthState{BreakerLevel: 0}
	healthStateMu.Unlock()

	snap := SnapshotRuntimeHealth()
	if snap.SitesTracked != 1 {
		t.Fatalf("expected 1 tracked site, got %d", snap.SitesTracked)
	}
	if snap.SitesBreakerOpen != 1 {
		t.Fatalf("expected 1 open site breaker, got %d", snap.SitesBreakerOpen)
	}
	if snap.ModelsTracked != 1 {
		t.Fatalf("expected 1 tracked model, got %d", snap.ModelsTracked)
	}
	if snap.ModelsBreakerOpen != 0 {
		t.Fatalf("expected 0 open model breakers, got %d", snap.ModelsBreakerOpen)
	}
	if len(snap.OpenBreakers) != 1 || snap.OpenBreakers[0].SiteID != 7 {
		t.Fatalf("unexpected open breakers: %+v", snap.OpenBreakers)
	}
}
