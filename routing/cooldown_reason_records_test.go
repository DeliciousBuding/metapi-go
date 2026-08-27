package routing

import (
	"context"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// =============================================================================
// P0-3 — RecordFailure / RecordProbeFailure persist structured cooldown
// reasons; success and clear paths reset them. Uses the isolationDB harness
// (failure_isolation_test.go) and asserts on the captured update maps, which
// are the exact payloads ProxyRoutingStore writes to route_channels /
// oauth_route_unit_members.
// =============================================================================

func reasonFieldsFromUpdates(t *testing.T, updates map[string]interface{}) (code string, summary interface{}, at string) {
	t.Helper()
	rawCode, ok := updates["cooldownReasonCode"]
	if !ok {
		t.Fatal("update map missing cooldownReasonCode")
	}
	code, _ = rawCode.(string)
	summary, ok = updates["cooldownReason"]
	if !ok {
		t.Fatal("update map missing cooldownReason")
	}
	rawAt, ok := updates["cooldownReasonAt"]
	if !ok {
		t.Fatal("update map missing cooldownReasonAt")
	}
	at, _ = rawAt.(string)
	return code, summary, at
}

func TestRecordFailure_PersistsStructuredReason(t *testing.T) {
	ResetSiteRuntimeHealthState()
	siteRuntimeHealthLoaded = true
	t.Cleanup(ResetSiteRuntimeHealthState)

	db := newIsolationDB()
	route := isolationRoute(1, "weighted")
	ch := isolationChannel(601, 1, 6001)
	db.seedChannel(ch, isolationAccount(6001, 60), route)
	tr := newIsolationRouter(db)

	status := 500
	errText := "upstream exploded:\nstack trace follows"
	if err := tr.RecordFailure(context.Background(), 601, SiteRuntimeFailureContext{
		Status:    &status,
		ErrorText: &errText,
	}, nil); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	code, summary, at := reasonFieldsFromUpdates(t, db.lastCooldownUpdates)
	if code != CooldownReasonCodeUpstreamError {
		t.Errorf("reason code = %q, want %q", code, CooldownReasonCodeUpstreamError)
	}
	wantSummary := "upstream exploded: stack trace follows" // control chars flattened
	if summary != wantSummary {
		t.Errorf("reason summary = %v, want %q", summary, wantSummary)
	}
	if at == "" {
		t.Error("reason recorded-at is empty")
	}
	if db.lastCooldownUpdates["cooldownUntil"] == nil {
		t.Error("expected cooldownUntil to be set alongside the reason")
	}
}

func TestRecordFailure_UsageLimitReasonScopesCredentialSiblings(t *testing.T) {
	ResetSiteRuntimeHealthState()
	siteRuntimeHealthLoaded = true
	t.Cleanup(ResetSiteRuntimeHealthState)

	db := newIsolationDB()
	route := isolationRoute(1, "weighted")
	ch := isolationChannel(611, 1, 6101)
	sibling := isolationChannel(612, 1, 6102)
	db.seedChannel(ch, isolationAccount(6101, 61), route)
	db.seedChannel(sibling, isolationAccount(6102, 61), route)
	db.credentialScope[611] = []int64{611, 612}
	tr := newIsolationRouter(db)

	status := 429
	errText := "usage_limit_reached"
	if err := tr.RecordFailure(context.Background(), 611, SiteRuntimeFailureContext{
		Status:    &status,
		ErrorText: &errText,
	}, nil); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	code, _, _ := reasonFieldsFromUpdates(t, db.lastCooldownUpdates)
	if code != CooldownReasonCodeUsageLimit {
		t.Errorf("reason code = %q, want %q", code, CooldownReasonCodeUsageLimit)
	}
	if len(db.lastCooldownIDs) != 2 {
		t.Fatalf("usage-limit cooldown should cover credential siblings, got ids %v", db.lastCooldownIDs)
	}
}

func TestRecordFailure_EmptyErrorTextPersistsNullSummary(t *testing.T) {
	ResetSiteRuntimeHealthState()
	siteRuntimeHealthLoaded = true
	t.Cleanup(ResetSiteRuntimeHealthState)

	db := newIsolationDB()
	route := isolationRoute(1, "weighted")
	db.seedChannel(isolationChannel(621, 1, 6201), isolationAccount(6201, 62), route)
	tr := newIsolationRouter(db)

	status := 502
	if err := tr.RecordFailure(context.Background(), 621, SiteRuntimeFailureContext{
		Status: &status,
	}, nil); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	code, summary, _ := reasonFieldsFromUpdates(t, db.lastCooldownUpdates)
	if code != CooldownReasonCodeUpstreamError {
		t.Errorf("reason code = %q, want %q", code, CooldownReasonCodeUpstreamError)
	}
	if summary != nil {
		t.Errorf("summary = %v, want nil (no error text)", summary)
	}
}

func TestRecordFailure_TruncatesLongErrorSummary(t *testing.T) {
	ResetSiteRuntimeHealthState()
	siteRuntimeHealthLoaded = true
	t.Cleanup(ResetSiteRuntimeHealthState)

	db := newIsolationDB()
	route := isolationRoute(1, "weighted")
	db.seedChannel(isolationChannel(631, 1, 6301), isolationAccount(6301, 63), route)
	tr := newIsolationRouter(db)

	status := 500
	errText := strings.Repeat("x", CooldownReasonSummaryMaxRunes*3)
	if err := tr.RecordFailure(context.Background(), 631, SiteRuntimeFailureContext{
		Status:    &status,
		ErrorText: &errText,
	}, nil); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	_, summary, _ := reasonFieldsFromUpdates(t, db.lastCooldownUpdates)
	s, ok := summary.(string)
	if !ok {
		t.Fatalf("summary = %v (%T), want string", summary, summary)
	}
	if got := len([]rune(s)); got != CooldownReasonSummaryMaxRunes {
		t.Fatalf("persisted summary length = %d runes, want cap %d", got, CooldownReasonSummaryMaxRunes)
	}
}

func TestRecordSuccess_ClearsStructuredReason(t *testing.T) {
	ResetSiteRuntimeHealthState()
	siteRuntimeHealthLoaded = true
	t.Cleanup(ResetSiteRuntimeHealthState)

	db := newIsolationDB()
	route := isolationRoute(1, "weighted")
	db.seedChannel(isolationChannel(641, 1, 6401), isolationAccount(6401, 64), route)
	tr := newIsolationRouter(db)

	status := 500
	errText := "boom"
	if err := tr.RecordFailure(context.Background(), 641, SiteRuntimeFailureContext{
		Status:    &status,
		ErrorText: &errText,
	}, nil); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}
	if err := tr.RecordSuccess(context.Background(), 641, 10, 0, nil, nil); err != nil {
		t.Fatalf("RecordSuccess: %v", err)
	}

	for key, value := range map[string]interface{}{
		"cooldownReasonCode": nil,
		"cooldownReason":     nil,
		"cooldownReasonAt":   nil,
	} {
		got, ok := db.lastSuccessUpdates[key]
		if !ok {
			t.Errorf("success update map missing %s", key)
			continue
		}
		if got != value {
			t.Errorf("success update %s = %v, want %v", key, got, value)
		}
	}
}

func TestRecordProbeFailure_PersistsProbeReason(t *testing.T) {
	ResetSiteRuntimeHealthState()
	siteRuntimeHealthLoaded = true
	t.Cleanup(ResetSiteRuntimeHealthState)

	db := newIsolationDB()
	route := isolationRoute(1, "weighted")
	db.seedChannel(isolationChannel(651, 1, 6501), isolationAccount(6501, 65), route)
	tr := newIsolationRouter(db)

	// Probe failure without HTTP status or known text: honest probe_failure code.
	if err := tr.RecordProbeFailure(context.Background(), 651, SiteRuntimeFailureContext{}, nil); err != nil {
		t.Fatalf("RecordProbeFailure: %v", err)
	}

	code, summary, at := reasonFieldsFromUpdates(t, db.lastCooldownUpdates)
	if code != CooldownReasonCodeProbeFailure {
		t.Errorf("reason code = %q, want %q", code, CooldownReasonCodeProbeFailure)
	}
	if summary != nil {
		t.Errorf("summary = %v, want nil", summary)
	}
	if at == "" {
		t.Error("reason recorded-at is empty")
	}
}

func TestRecordFailure_OAuthMemberPersistsReason(t *testing.T) {
	ResetSiteRuntimeHealthState()
	siteRuntimeHealthLoaded = true
	t.Cleanup(ResetSiteRuntimeHealthState)

	db := newIsolationDB()
	route := isolationRoute(1, "weighted")
	unitID := int64(9)
	ch := isolationChannel(661, 1, 6601)
	ch.OAuthRouteUnitID = &unitID
	acc := isolationAccount(6601, 66)
	db.seedChannel(ch, acc, route)
	unit := store.OAuthRouteUnit{
		ID: unitID, SiteID: 66, Provider: "codex", Name: "unit", Strategy: "round_robin", Enabled: true,
	}
	db.seedMember(store.OAuthRouteUnitMember{ID: 1, UnitID: unitID, AccountID: 6601}, acc, unit)
	tr := newIsolationRouter(db)

	status := 403
	errText := "forbidden by upstream"
	if err := tr.RecordFailure(context.Background(), 661, SiteRuntimeFailureContext{
		Status:    &status,
		ErrorText: &errText,
	}, nil); err != nil {
		t.Fatalf("RecordFailure: %v", err)
	}

	code, summary, at := reasonFieldsFromUpdates(t, db.lastMemberUpdates)
	if code != CooldownReasonCodeAuthError {
		t.Errorf("member reason code = %q, want %q", code, CooldownReasonCodeAuthError)
	}
	if summary != "forbidden by upstream" {
		t.Errorf("member reason summary = %v, want the error text", summary)
	}
	if at == "" {
		t.Error("member reason recorded-at is empty")
	}
	if db.cooldownCalls != 0 {
		t.Errorf("oauth member failure must not write channel cooldown fields, calls=%d", db.cooldownCalls)
	}
}
