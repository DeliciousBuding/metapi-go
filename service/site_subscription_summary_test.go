package service

import (
	"encoding/json"
	"testing"
	"time"
)

// strPtr is provided by service/account_mutation_test.go (package service test helpers).

// TestBuildSubscriptionSummary_NoAccounts asserts the stub-equivalent
// behavior: with no accounts at all, the summary is nil.
func TestBuildSubscriptionSummary_NoAccounts(t *testing.T) {
	t.Parallel()
	if got := buildSubscriptionSummary(nil, 1); got != nil {
		t.Fatalf("expected nil for no accounts, got %#v", got)
	}
	if got := buildSubscriptionSummary([]accountAgg{}, 1); got != nil {
		t.Fatalf("expected nil for empty accounts, got %#v", got)
	}
}

// TestBuildSubscriptionSummary_NoSub2ApiAuth returns nil when none of the
// site's accounts carry a sub2apiAuth block (non-sub2api sites stay null).
func TestBuildSubscriptionSummary_NoSub2ApiAuth(t *testing.T) {
	t.Parallel()
	accounts := []accountAgg{
		{SiteID: 1, ExtraConfig: strPtr(`{"proxyUrl":"http://proxy.local"}`)},
		{SiteID: 1, ExtraConfig: nil},
		{SiteID: 1, ExtraConfig: strPtr(``)},
		{SiteID: 2, ExtraConfig: strPtr(`{"sub2apiAuth":{"group":"other-site"}}`)},
	}
	if got := buildSubscriptionSummary(accounts, 1); got != nil {
		t.Fatalf("expected nil when no sub2apiAuth for site 1, got %#v", got)
	}
}

// TestBuildSubscriptionSummary_MalformedExtraConfig is ignored gracefully —
// a broken extra_config must not crash the aggregator.
func TestBuildSubscriptionSummary_MalformedExtraConfig(t *testing.T) {
	t.Parallel()
	accounts := []accountAgg{
		{SiteID: 1, ExtraConfig: strPtr(`{not-json`)},
	}
	if got := buildSubscriptionSummary(accounts, 1); got != nil {
		t.Fatalf("expected nil for malformed extra_config, got %#v", got)
	}
}

// TestBuildSubscriptionSummary_ActiveToken aggregates a single sub2api
// account whose token expires in the future — it should be counted active.
func TestBuildSubscriptionSummary_ActiveToken(t *testing.T) {
	t.Parallel()
	future := time.Now().Add(1 * time.Hour).UnixMilli()
	accounts := []accountAgg{
		{SiteID: 1, ExtraConfig: strPtr(`{"sub2apiAuth":{"group":"pro-tier","tokenExpiresAt":` + itoa64(future) + `}}`)},
	}
	got := buildSubscriptionSummary(accounts, 1)
	if got == nil {
		t.Fatal("expected non-nil summary")
	}
	summary, ok := got.(*subscriptionSummary)
	if !ok {
		t.Fatalf("expected *subscriptionSummary, got %T", got)
	}
	if summary.Group != "pro-tier" {
		t.Fatalf("Group = %q, want pro-tier", summary.Group)
	}
	if !summary.Active {
		t.Fatalf("Active = false, want true")
	}
	if summary.ActiveCount != 1 {
		t.Fatalf("ActiveCount = %d, want 1", summary.ActiveCount)
	}
	if summary.ExpiresAt == nil {
		t.Fatal("ExpiresAt = nil, want set")
	}
	wantExpiry := time.UnixMilli(future).UTC()
	if !summary.ExpiresAt.Equal(wantExpiry) {
		t.Fatalf("ExpiresAt = %v, want %v", summary.ExpiresAt, wantExpiry)
	}
	if _, err := json.Marshal(summary); err != nil {
		t.Fatalf("marshal millisecond expiry summary: %v", err)
	}
}

// TestBuildSubscriptionSummary_ExpiredToken is not counted toward ActiveCount
// but still surfaces ExpiresAt so operators can see when the last sub lapsed.
func TestBuildSubscriptionSummary_ExpiredToken(t *testing.T) {
	t.Parallel()
	past := time.Now().Add(-30 * time.Minute).Unix()
	accounts := []accountAgg{
		{SiteID: 1, ExtraConfig: strPtr(`{"sub2apiAuth":{"group":"pro-tier","tokenExpiresAt":` + itoa64(past) + `}}`)},
	}
	got := buildSubscriptionSummary(accounts, 1)
	summary := got.(*subscriptionSummary)
	if summary.Group != "pro-tier" {
		t.Fatalf("Group = %q, want pro-tier", summary.Group)
	}
	if summary.Active {
		t.Fatalf("Active = true, want false for expired token")
	}
	if summary.ActiveCount != 0 {
		t.Fatalf("ActiveCount = %d, want 0", summary.ActiveCount)
	}
	if summary.ExpiresAt == nil || !summary.ExpiresAt.Equal(time.Unix(past, 0).UTC()) {
		t.Fatalf("ExpiresAt = %v, want %v", summary.ExpiresAt, time.Unix(past, 0).UTC())
	}
}

// TestBuildSubscriptionSummary_NoExpiryTreatedActive falls back to active when
// a sub2apiAuth block has no usable tokenExpiresAt (keeps accountCount fallback).
func TestBuildSubscriptionSummary_NoExpiryTreatedActive(t *testing.T) {
	t.Parallel()
	accounts := []accountAgg{
		{SiteID: 1, ExtraConfig: strPtr(`{"sub2apiAuth":{"group":"free","refreshToken":"rt_abc"}}`)},
	}
	got := buildSubscriptionSummary(accounts, 1)
	summary := got.(*subscriptionSummary)
	if !summary.Active {
		t.Fatal("Active = false, want true when expiry unknown")
	}
	if summary.ActiveCount != 1 {
		t.Fatalf("ActiveCount = %d, want 1", summary.ActiveCount)
	}
	if summary.ExpiresAt != nil {
		t.Fatalf("ExpiresAt = %v, want nil", summary.ExpiresAt)
	}
}

// TestBuildSubscriptionSummary_PlanNameFallback uses planName when group is
// absent — the TS stored-summary field name.
func TestBuildSubscriptionSummary_PlanNameFallback(t *testing.T) {
	t.Parallel()
	accounts := []accountAgg{
		{SiteID: 1, ExtraConfig: strPtr(`{"sub2apiAuth":{"planName":"enterprise"}}`)},
	}
	summary := buildSubscriptionSummary(accounts, 1).(*subscriptionSummary)
	if summary.Group != "enterprise" {
		t.Fatalf("Group = %q, want enterprise (planName fallback)", summary.Group)
	}
}

// TestBuildSubscriptionSummary_MultipleAccountsAggregate picks the latest
// expiry across accounts and counts only non-expired tokens as active.
func TestBuildSubscriptionSummary_MultipleAccountsAggregate(t *testing.T) {
	t.Parallel()
	now := time.Now().Unix()
	past := now - 60
	future1 := now + 60
	future2 := now + 3600
	accounts := []accountAgg{
		{SiteID: 1, ExtraConfig: strPtr(`{"sub2apiAuth":{"group":"a","tokenExpiresAt":` + itoa64(past) + `}}`)},
		{SiteID: 1, ExtraConfig: strPtr(`{"sub2apiAuth":{"group":"b","tokenExpiresAt":` + itoa64(future1) + `}}`)},
		{SiteID: 1, ExtraConfig: strPtr(`{"sub2apiAuth":{"tokenExpiresAt":` + itoa64(future2) + `}}`)},
		{SiteID: 2, ExtraConfig: strPtr(`{"sub2apiAuth":{"group":"other","tokenExpiresAt":` + itoa64(future2) + `}}`)},
	}
	summary := buildSubscriptionSummary(accounts, 1).(*subscriptionSummary)
	// group "a" wins on first-seen order (b and the third are same site).
	if summary.Group != "a" {
		t.Fatalf("Group = %q, want a", summary.Group)
	}
	if summary.ActiveCount != 2 {
		t.Fatalf("ActiveCount = %d, want 2", summary.ActiveCount)
	}
	if !summary.Active {
		t.Fatal("Active = false, want true")
	}
	if !summary.ExpiresAt.Equal(time.Unix(future2, 0).UTC()) {
		t.Fatalf("ExpiresAt = %v, want %v (latest)", summary.ExpiresAt, time.Unix(future2, 0).UTC())
	}
}

// TestBuildSubscriptionSummary_JSONShape verifies the serialized shape keeps
// the camelCase keys the frontend (sites-columns.tsx) reads via activeCount.
func TestBuildSubscriptionSummary_JSONShape(t *testing.T) {
	t.Parallel()
	future := time.Now().Add(time.Hour).Unix()
	accounts := []accountAgg{
		{SiteID: 7, ExtraConfig: strPtr(`{"sub2apiAuth":{"group":"x","tokenExpiresAt":` + itoa64(future) + `}}`)},
	}
	got := buildSubscriptionSummary(accounts, 7)
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["activeCount"].(float64) != 1 {
		t.Fatalf("activeCount = %v, want 1", decoded["activeCount"])
	}
	if decoded["active"].(bool) != true {
		t.Fatalf("active = %v, want true", decoded["active"])
	}
	if decoded["group"] != "x" {
		t.Fatalf("group = %v, want x", decoded["group"])
	}
	if _, ok := decoded["expiresAt"]; !ok {
		t.Fatal("expiresAt key missing")
	}
}

// TestBuildSubscriptionSummariesBySite_MultiSiteSinglePass verifies the new
// single-pass aggregator (Fix 2) produces correct per-site summaries in one
// scan of the accounts slice, instead of rescanning per site. This is the
// O(accounts) replacement for the old O(sites × accounts) per-site loop.
func TestBuildSubscriptionSummariesBySite_MultiSiteSinglePass(t *testing.T) {
	t.Parallel()
	now := time.Now().Unix()
	past := now - 60
	future1 := now + 60
	future2 := now + 3600
	accounts := []accountAgg{
		// Site 1: two sub2api accounts — one expired (past), one active (future1).
		{SiteID: 1, ExtraConfig: strPtr(`{"sub2apiAuth":{"group":"a","tokenExpiresAt":` + itoa64(past) + `}}`)},
		{SiteID: 1, ExtraConfig: strPtr(`{"sub2apiAuth":{"group":"b","tokenExpiresAt":` + itoa64(future1) + `}}`)},
		// Site 2: one sub2api account — active with the latest expiry.
		{SiteID: 2, ExtraConfig: strPtr(`{"sub2apiAuth":{"group":"c","tokenExpiresAt":` + itoa64(future2) + `}}`)},
		// Site 3: no sub2api account — must be ABSENT from the map (not a
		// typed-nil entry, which would break nil comparisons downstream).
		{SiteID: 3, ExtraConfig: strPtr(`{"proxyUrl":"http://proxy.local"}`)},
		{SiteID: 3, ExtraConfig: nil},
	}
	summaries := buildSubscriptionSummariesBySite(accounts)
	if len(summaries) != 2 {
		t.Fatalf("expected 2 sites with summaries, got %d (%#v)", len(summaries), summaries)
	}
	if _, ok := summaries[3]; ok {
		t.Fatal("site 3 (no sub2api) should be absent from map, not a typed-nil entry")
	}
	if _, ok := summaries[99]; ok {
		t.Fatal("site 99 (no accounts at all) should be absent from map")
	}

	s1 := summaries[1]
	if s1 == nil {
		t.Fatal("site 1 summary is nil")
	}
	if s1.Group != "a" {
		t.Fatalf("site 1 Group = %q, want a (first-seen wins)", s1.Group)
	}
	if s1.ActiveCount != 1 {
		t.Fatalf("site 1 ActiveCount = %d, want 1 (only future1 is active)", s1.ActiveCount)
	}
	if !s1.Active {
		t.Fatal("site 1 Active = false, want true (ActiveCount > 0)")
	}
	if s1.ExpiresAt == nil || !s1.ExpiresAt.Equal(time.Unix(future1, 0).UTC()) {
		t.Fatalf("site 1 ExpiresAt = %v, want %v (latest of past+future1)", s1.ExpiresAt, time.Unix(future1, 0).UTC())
	}

	s2 := summaries[2]
	if s2 == nil {
		t.Fatal("site 2 summary is nil")
	}
	if s2.Group != "c" {
		t.Fatalf("site 2 Group = %q, want c", s2.Group)
	}
	if s2.ActiveCount != 1 {
		t.Fatalf("site 2 ActiveCount = %d, want 1", s2.ActiveCount)
	}
	if !s2.ExpiresAt.Equal(time.Unix(future2, 0).UTC()) {
		t.Fatalf("site 2 ExpiresAt = %v, want %v", s2.ExpiresAt, time.Unix(future2, 0).UTC())
	}
}

// TestBuildSubscriptionSummariesBySite_EmptyInput verifies the single-pass
// aggregator returns a non-nil empty map (not nil) for nil/empty input, so
// callers can safely do `summaries[siteID]` without nil-map panics.
func TestBuildSubscriptionSummariesBySite_EmptyInput(t *testing.T) {
	t.Parallel()
	if got := buildSubscriptionSummariesBySite(nil); got == nil || len(got) != 0 {
		t.Fatalf("nil input: expected non-nil empty map, got %#v", got)
	}
	if got := buildSubscriptionSummariesBySite([]accountAgg{}); got == nil || len(got) != 0 {
		t.Fatalf("empty input: expected non-nil empty map, got %#v", got)
	}
}

// TestBuildSubscriptionSummary_DelegatesToSinglePass verifies the backward-
// compat wrapper returns the same value as the single-pass map lookup, and
// returns an untyped nil (not a typed-nil pointer) when the site has no
// sub2api accounts — guarding against the Go typed-nil comparison gotcha.
func TestBuildSubscriptionSummary_DelegatesToSinglePass(t *testing.T) {
	t.Parallel()
	future := time.Now().Add(time.Hour).Unix()
	accounts := []accountAgg{
		{SiteID: 1, ExtraConfig: strPtr(`{"sub2apiAuth":{"group":"x","tokenExpiresAt":` + itoa64(future) + `}}`)},
		{SiteID: 2, ExtraConfig: strPtr(`{"proxyUrl":"http://no-sub2api.local"}`)},
	}
	// Site 1 has a sub2api summary.
	got := buildSubscriptionSummary(accounts, 1)
	if got == nil {
		t.Fatal("site 1: expected non-nil summary")
	}
	summary, ok := got.(*subscriptionSummary)
	if !ok {
		t.Fatalf("site 1: expected *subscriptionSummary, got %T", got)
	}
	if summary.Group != "x" {
		t.Fatalf("site 1: Group = %q, want x", summary.Group)
	}
	// Site 2 has no sub2api — must be untyped nil so `got != nil` is false.
	// A typed-nil *subscriptionSummary would wrongly report non-nil.
	got = buildSubscriptionSummary(accounts, 2)
	if got != nil {
		t.Fatalf("site 2: expected untyped nil (no sub2api), got %#v (typed-nil trap?)", got)
	}
}

// itoa64 formats an int64 as a string for JSON splicing in test fixtures.
func itoa64(n int64) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
