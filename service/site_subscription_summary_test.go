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
	future := time.Now().Add(1 * time.Hour).Unix()
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
	if !summary.ExpiresAt.Equal(time.Unix(future, 0).UTC()) {
		t.Fatalf("ExpiresAt = %v, want %v", summary.ExpiresAt, time.Unix(future, 0).UTC())
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
