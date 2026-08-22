package routing

import (
	"context"
	"encoding/json"
	"math"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// tripAndExpireBreaker drives a site through the transient-failure streak so the
// breaker opens at level 1, then rewinds BreakerUntilMs into the past to
// simulate cooldown expiry (the precondition for half-open probing).
func tripAndExpireBreaker(t *testing.T, siteID int64) *SiteRuntimeHealthState {
	t.Helper()
	status500 := 500
	for i := 0; i < 3; i++ {
		RecordSiteRuntimeFailure(siteID, SiteRuntimeFailureContext{Status: &status500})
	}
	state := siteRuntimeHealthStates[siteID]
	if state == nil {
		t.Fatal("expected site runtime health state to exist")
	}
	if state.BreakerLevel < 1 || state.BreakerUntilMs == nil {
		t.Fatalf("expected open breaker after streak, level=%d until=%v", state.BreakerLevel, state.BreakerUntilMs)
	}
	healthStateMu.Lock()
	expiredUntil := nowMs() - 1
	state.BreakerUntilMs = &expiredUntil
	healthStateMu.Unlock()
	return state
}

func TestHalfOpenProbe_GrantsSingleProbeAfterExpiry(t *testing.T) {
	ResetSiteRuntimeHealthState()
	t.Cleanup(ResetSiteRuntimeHealthState)

	siteID := int64(32001)
	tripAndExpireBreaker(t, siteID)

	if !TryAdmitSiteModelRuntimeRequest(siteID, "") {
		t.Fatal("first request after cooldown expiry should be admitted as the probe")
	}
	healthStateMu.RLock()
	halfOpenSince := stateHalfOpenSinceMs(siteRuntimeHealthStates[siteID])
	healthStateMu.RUnlock()
	if halfOpenSince == nil {
		t.Fatal("expected half-open probe window to be granted")
	}

	if TryAdmitSiteModelRuntimeRequest(siteID, "") {
		t.Fatal("second request must be rejected while the probe is in flight")
	}

	details := GetSiteRuntimeHealthDetails(siteID, "")
	if !details.GlobalHalfOpen {
		t.Error("expected GlobalHalfOpen while the probe is in flight")
	}
	if details.GlobalBreakerOpen {
		t.Error("expired breaker must not report hard-open")
	}
}

func stateHalfOpenSinceMs(state *SiteRuntimeHealthState) *int64 {
	if state == nil {
		return nil
	}
	return state.HalfOpenSinceMs
}

func TestHalfOpenProbe_SuccessResetsToClosed(t *testing.T) {
	ResetSiteRuntimeHealthState()
	t.Cleanup(ResetSiteRuntimeHealthState)

	siteID := int64(32002)
	tripAndExpireBreaker(t, siteID)
	if !TryAdmitSiteModelRuntimeRequest(siteID, "") {
		t.Fatal("expected probe grant after expiry")
	}

	model := "gpt-half-open"
	RecordSiteRuntimeSuccess(siteID, 300.0, &model)

	state := siteRuntimeHealthStates[siteID]
	if state.HalfOpenSinceMs != nil {
		t.Error("probe success must clear the half-open marker")
	}
	if state.BreakerLevel != 0 {
		t.Errorf("probe success must reset breaker level to 0, got %d", state.BreakerLevel)
	}
	if state.BreakerUntilMs != nil {
		t.Error("probe success must clear breakerUntilMs")
	}

	// After reset, normal traffic is admitted without granting a new probe.
	if !TryAdmitSiteModelRuntimeRequest(siteID, "") {
		t.Error("closed breaker must admit requests")
	}
	if state.HalfOpenSinceMs != nil {
		t.Error("closed breaker must not grant a new probe")
	}
}

func TestHalfOpenProbe_TransientFailureReopensExtended(t *testing.T) {
	ResetSiteRuntimeHealthState()
	t.Cleanup(ResetSiteRuntimeHealthState)

	siteID := int64(32003)
	tripAndExpireBreaker(t, siteID) // breaker tripped at level 1 (60s cooldown)
	if !TryAdmitSiteModelRuntimeRequest(siteID, "") {
		t.Fatal("expected probe grant after expiry")
	}

	status500 := 500
	RecordSiteRuntimeFailure(siteID, SiteRuntimeFailureContext{Status: &status500})

	state := siteRuntimeHealthStates[siteID]
	if state.HalfOpenSinceMs != nil {
		t.Error("probe failure must clear the half-open marker")
	}
	if state.BreakerLevel != 2 {
		t.Errorf("probe failure must extend cooldown to level 2, got %d", state.BreakerLevel)
	}
	if state.BreakerUntilMs == nil {
		t.Fatal("probe failure must re-open the breaker")
	}
	expectedCooldownMs := resolveSiteRuntimeBreakerMs(2)
	delta := *state.BreakerUntilMs - nowMs()
	if delta < expectedCooldownMs-5000 || delta > expectedCooldownMs+5000 {
		t.Errorf("expected re-open cooldown ~%dms, got until-now=%dms", expectedCooldownMs, delta)
	}
	if !isRuntimeHealthBreakerOpen(state) {
		t.Error("breaker must be hard-open after probe failure")
	}
	if TryAdmitSiteModelRuntimeRequest(siteID, "") {
		t.Error("hard-open breaker must reject admission")
	}
}

func TestHalfOpenProbe_NonTransientFailureAllowsRetry(t *testing.T) {
	ResetSiteRuntimeHealthState()
	t.Cleanup(ResetSiteRuntimeHealthState)

	siteID := int64(32004)
	state := tripAndExpireBreaker(t, siteID)
	if !TryAdmitSiteModelRuntimeRequest(siteID, "") {
		t.Fatal("expected probe grant after expiry")
	}

	// A client-caused failure proves nothing about upstream health: the probe
	// window must release so the next request retries the probe.
	status400 := 400
	RecordSiteRuntimeFailure(siteID, SiteRuntimeFailureContext{Status: &status400})

	if state.HalfOpenSinceMs != nil {
		t.Error("inconclusive probe failure must clear the half-open marker")
	}
	if state.BreakerLevel != 1 {
		t.Errorf("inconclusive probe failure must not extend the level, got %d", state.BreakerLevel)
	}
	if isRuntimeHealthBreakerOpen(state) {
		t.Error("inconclusive probe failure must not hard-reopen the breaker")
	}

	// The breaker is still expired → the next request becomes a fresh probe.
	if !TryAdmitSiteModelRuntimeRequest(siteID, "") {
		t.Error("expired breaker must admit the next request as a new probe")
	}
	if state.HalfOpenSinceMs == nil {
		t.Error("expected a fresh probe grant")
	}
}

func TestHalfOpenProbe_TimeoutReleasesProbeWindow(t *testing.T) {
	ResetSiteRuntimeHealthState()
	t.Cleanup(ResetSiteRuntimeHealthState)

	siteID := int64(32005)
	state := tripAndExpireBreaker(t, siteID)
	if !TryAdmitSiteModelRuntimeRequest(siteID, "") {
		t.Fatal("expected probe grant after expiry")
	}

	// Simulate a probe whose outcome never arrived: rewind the probe window
	// past its timeout so the admission gate lazily releases it.
	healthStateMu.Lock()
	staleSince := nowMs() - SiteRuntimeHalfOpenProbeTimeoutMs - 1
	state.HalfOpenSinceMs = &staleSince
	healthStateMu.Unlock()

	if !TryAdmitSiteModelRuntimeRequest(siteID, "") {
		t.Error("timed-out probe window must release and admit a fresh probe")
	}
	healthStateMu.RLock()
	freshSince := *state.HalfOpenSinceMs
	healthStateMu.RUnlock()
	if freshSince <= staleSince {
		t.Errorf("expected refreshed probe window, stale=%d fresh=%d", staleSince, freshSince)
	}
}

func TestHalfOpenProbe_ModelLevelGrant(t *testing.T) {
	ResetSiteRuntimeHealthState()
	t.Cleanup(ResetSiteRuntimeHealthState)

	siteID := int64(32006)
	model := "gpt-model-probe"
	healthStateMu.Lock()
	modelState := getOrCreateSiteModelRuntimeHealthState(siteID, model)
	expiredUntil := nowMs() - 1
	modelState.BreakerLevel = 1
	modelState.BreakerUntilMs = &expiredUntil
	healthStateMu.Unlock()

	if !TryAdmitSiteModelRuntimeRequest(siteID, model) {
		t.Fatal("expired model breaker must admit the probe")
	}
	if TryAdmitSiteModelRuntimeRequest(siteID, model) {
		t.Fatal("second request must be rejected while the model probe is in flight")
	}
	details := GetSiteRuntimeHealthDetails(siteID, model)
	if !details.ModelHalfOpen {
		t.Error("expected ModelHalfOpen while the model probe is in flight")
	}

	// An unrelated model on the same site is not gated by this probe.
	if !TryAdmitSiteModelRuntimeRequest(siteID, "unrelated-model") {
		t.Error("unrelated model must not be gated by another model's probe")
	}
}

func TestHalfOpenProbe_HardOpenDeniesModelProbe(t *testing.T) {
	ResetSiteRuntimeHealthState()
	t.Cleanup(ResetSiteRuntimeHealthState)

	siteID := int64(32007)
	model := "gpt-denied-probe"
	healthStateMu.Lock()
	globalState := getOrCreateSiteRuntimeHealthState(siteID)
	futureUntil := nowMs() + 60_000
	globalState.BreakerLevel = 1
	globalState.BreakerUntilMs = &futureUntil

	modelState := getOrCreateSiteModelRuntimeHealthState(siteID, model)
	expiredUntil := nowMs() - 1
	modelState.BreakerLevel = 1
	modelState.BreakerUntilMs = &expiredUntil
	healthStateMu.Unlock()

	if TryAdmitSiteModelRuntimeRequest(siteID, model) {
		t.Fatal("hard-open global breaker must deny admission")
	}
	if modelState.HalfOpenSinceMs != nil {
		t.Error("denied admission must not partially grant the model probe")
	}
}

func TestHalfOpenProbe_MultiplierFloorsDuringProbe(t *testing.T) {
	ResetSiteRuntimeHealthState()
	t.Cleanup(ResetSiteRuntimeHealthState)

	n := nowMs()
	expiredUntil := n - 1
	state := &SiteRuntimeHealthState{
		PenaltyScore:            0,
		BreakerLevel:            1,
		BreakerUntilMs:          &expiredUntil,
		HalfOpenSinceMs:         &n,
		LastUpdatedAtMs:         n,
		RecentWindowUpdatedAtMs: n,
	}

	got := GetRuntimeHealthMultiplier(state)
	if math.Abs(got-SiteRuntimeMinMultiplier) > 0.001 {
		t.Errorf("expected min multiplier %.4f during half-open probe, got %.4f", SiteRuntimeMinMultiplier, got)
	}
}

func TestHalfOpenProbe_FilterBlocksProbingCandidates(t *testing.T) {
	ResetSiteRuntimeHealthState()
	t.Cleanup(ResetSiteRuntimeHealthState)

	model := "gpt-filter"
	// Trip + expire + grant on site 30 so it is half-open.
	siteID := int64(30)
	tripAndExpireBreaker(t, siteID)
	if !TryAdmitSiteModelRuntimeRequest(siteID, model) {
		t.Fatal("expected probe grant")
	}

	mkCandidate := func(channelID, candidateSiteID, accountID int64) RouteChannelCandidate {
		token := "tok"
		return RouteChannelCandidate{
			Channel: store.RouteChannel{
				ID: channelID, RouteID: 1, AccountID: accountID,
				SourceModel: &model, Priority: 0, Weight: 10, Enabled: true,
			},
			Account: store.Account{ID: accountID, SiteID: candidateSiteID, Status: "active", APIToken: &token, Balance: ptrFloat(100)},
			Site:    store.Site{ID: candidateSiteID, Status: "active"},
		}
	}
	candidates := []RouteChannelCandidate{
		mkCandidate(301, 30, 3001),
		mkCandidate(302, 30, 3002),
		mkCandidate(311, 31, 3101), // healthy sibling
	}

	healthy, avoided := FilterSiteRuntimeBrokenCandidatesByModel(candidates, model)
	if len(healthy) != 1 || healthy[0].Channel.ID != 311 {
		t.Fatalf("expected healthy sibling 311 only, got %+v", healthy)
	}
	if len(avoided) != 2 {
		t.Fatalf("expected 2 avoided half-open candidates, got %d", len(avoided))
	}
	for _, a := range avoided {
		if a.Reason != "站点半开探测中，优先避让" {
			t.Errorf("unexpected avoid reason %q", a.Reason)
		}
	}
}

func TestHalfOpenProbe_StatePersistsRoundTrip(t *testing.T) {
	ResetSiteRuntimeHealthState()
	t.Cleanup(ResetSiteRuntimeHealthState)

	siteID := int64(32008)
	state := tripAndExpireBreaker(t, siteID)
	if !TryAdmitSiteModelRuntimeRequest(siteID, "") {
		t.Fatal("expected probe grant")
	}

	if !shouldPersistSiteRuntimeHealthState(state) {
		t.Error("half-open state must be persist-worthy")
	}
	clone := cloneSiteRuntimeHealthState(state)
	if clone.HalfOpenSinceMs == nil || *clone.HalfOpenSinceMs != *state.HalfOpenSinceMs {
		t.Error("clone must preserve halfOpenSinceMs")
	}

	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	var restored SiteRuntimeHealthState
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if restored.HalfOpenSinceMs == nil || *restored.HalfOpenSinceMs != *state.HalfOpenSinceMs {
		t.Error("JSON round-trip must preserve halfOpenSinceMs")
	}
}

// TestSelectChannel_HalfOpenProbeGate covers the dispatch-path integration: a
// single-candidate route with an expired breaker admits exactly one request as
// the probe; while the probe is in flight the selector declines.
func TestSelectChannel_HalfOpenProbeGate(t *testing.T) {
	ResetSiteRuntimeHealthState()
	siteRuntimeHealthLoaded = true
	t.Cleanup(ResetSiteRuntimeHealthState)

	model := "gpt-gate-probe"
	db := &preferredDB{
		routes: []store.TokenRoute{{
			ID:              1,
			ModelPattern:    model,
			RouteMode:       "pattern",
			RoutingStrategy: "weighted",
			Enabled:         true,
		}},
		joined: []struct {
			Channel store.RouteChannel
			Account store.Account
			Site    store.Site
			Token   *store.AccountToken
		}{
			preferredEligibleJoined(401, 40, 4001, model),
		},
	}

	// Open the breaker on site 40, then let the cooldown expire.
	status500 := 500
	for i := 0; i < 3; i++ {
		RecordSiteRuntimeFailure(40, SiteRuntimeFailureContext{Status: &status500})
	}
	healthStateMu.Lock()
	state := getOrCreateSiteRuntimeHealthState(40)
	expiredUntil := nowMs() - 1
	state.BreakerUntilMs = &expiredUntil
	healthStateMu.Unlock()

	selector := newPreferredSelector(db)

	first, err := selector.SelectChannel(context.Background(), model, EmptyDownstreamRoutingPolicy)
	if err != nil {
		t.Fatalf("SelectChannel error: %v", err)
	}
	if first == nil || first.Channel.ID != 401 {
		t.Fatalf("first request after expiry must be dispatched as the probe, got %+v", first)
	}
	if state.HalfOpenSinceMs == nil {
		t.Fatal("dispatched probe must mark the breaker half-open")
	}

	second, err := selector.SelectChannel(context.Background(), model, EmptyDownstreamRoutingPolicy)
	if err != nil {
		t.Fatalf("SelectChannel error: %v", err)
	}
	if second != nil {
		t.Fatalf("request while probe in flight must be declined, got channel %d", second.Channel.ID)
	}

	// Probe success reopens the gate: selection must work again.
	RecordSiteRuntimeSuccess(40, 250.0, &model)
	third, err := selector.SelectChannel(context.Background(), model, EmptyDownstreamRoutingPolicy)
	if err != nil {
		t.Fatalf("SelectChannel error: %v", err)
	}
	if third == nil {
		t.Fatal("selection must succeed after probe success resets the breaker")
	}
}
