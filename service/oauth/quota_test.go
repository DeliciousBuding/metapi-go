package oauth

import (
	"encoding/json"
	"testing"
)

// ---- OauthQuotaSnapshot Types ----

func TestOauthQuotaSnapshot_ZeroValue(t *testing.T) {
	snap := OauthQuotaSnapshot{}
	if snap.Status != "" {
		t.Errorf("zero Status should be empty, got %q", snap.Status)
	}
	if snap.Source != "" {
		t.Errorf("zero Source should be empty, got %q", snap.Source)
	}
	if snap.Windows != nil {
		t.Error("zero Windows should be nil")
	}
}

func TestOauthQuotaSnapshot_JSONRoundTrip(t *testing.T) {
	limit := 1000.0
	used := 500.0
	remaining := 500.0
	snap := OauthQuotaSnapshot{
		Status:        "reverse_engineered",
		Source:        "codex_probe",
		LastSyncAt:    "2026-07-04T00:00:00Z",
		ProviderMessage: "Rate limits active",
		Windows: &OauthQuotaWindows{
			FiveHour: &OauthQuotaWindowSnapshot{
				Supported: true,
				Limit:     &limit,
				Used:      &used,
				Remaining: &remaining,
				ResetAt:   "2026-07-04T05:00:00Z",
			},
			SevenDay: &OauthQuotaWindowSnapshot{
				Supported: true,
				Limit:     &limit,
				Used:      &used,
				Remaining: &remaining,
				ResetAt:   "2026-07-11T00:00:00Z",
			},
		},
	}

	bytes, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var restored OauthQuotaSnapshot
	if err := json.Unmarshal(bytes, &restored); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if restored.Status != "reverse_engineered" {
		t.Errorf("expected status 'reverse_engineered', got %q", restored.Status)
	}
	if restored.Windows.FiveHour.Supported != true {
		t.Error("fiveHour should be supported")
	}
	if restored.Windows.SevenDay.Supported != true {
		t.Error("sevenDay should be supported")
	}
	if restored.LastSyncAt != "2026-07-04T00:00:00Z" {
		t.Errorf("expected LastSyncAt, got %q", restored.LastSyncAt)
	}
}

// ---- OauthQuotaWindowSnapshot Tests ----

func TestOauthQuotaWindowSnapshot_NilFields(t *testing.T) {
	w := OauthQuotaWindowSnapshot{Supported: false}
	if w.Limit != nil {
		t.Error("Limit should be nil when not set")
	}
	if w.Used != nil {
		t.Error("Used should be nil when not set")
	}
	if w.Remaining != nil {
		t.Error("Remaining should be nil when not set")
	}
}

func TestOauthQuotaWindowSnapshot_ErrorSnapshot(t *testing.T) {
	w := OauthQuotaWindowSnapshot{
		Supported: false,
		Message:   "probe timeout after 10s",
	}
	bytes, _ := json.Marshal(w)
	if len(bytes) == 0 {
		t.Error("error snapshot should serialize")
	}
}

// ---- OauthQuotaWindows Tests ----

func TestOauthQuotaWindows_NilWindows(t *testing.T) {
	w := OauthQuotaWindows{}
	if w.FiveHour != nil {
		t.Error("FiveHour should be nil by default")
	}
	if w.SevenDay != nil {
		t.Error("SevenDay should be nil by default")
	}
}

// ---- OauthSubscription Tests ----

func TestOauthSubscription_ZeroValue(t *testing.T) {
	s := OauthSubscription{}
	if s.PlanType != "" {
		t.Errorf("zero PlanType should be empty, got %q", s.PlanType)
	}
}

// ---- Quota Snapshot Status Values ----

func TestQuotaSnapshotStatuses(t *testing.T) {
	validStatuses := []string{"reverse_engineered", "error", "unsupported"}
	for _, status := range validStatuses {
		snap := OauthQuotaSnapshot{
			Status: status,
			Source: "codex_probe",
		}
		if snap.Status != status {
			t.Errorf("expected status %q, got %q", status, snap.Status)
		}
	}
}

// ---- asQuotaSnapshotGo Tests ----

func TestAsQuotaSnapshotGo_ValidSnapshot(t *testing.T) {
	limit := 50.0
	raw := map[string]interface{}{
		"status": "reverse_engineered",
		"source": "codex_probe",
		"lastSyncAt": "2026-07-04T12:00:00Z",
		"lastError": "none",
		"providerMessage": "ok",
		"windows": map[string]interface{}{
			"fiveHour": map[string]interface{}{
				"supported": true,
				"limit":     limit,
				"used":      25.0,
				"remaining": 25.0,
				"resetAt":   "2026-07-04T17:00:00Z",
			},
			"sevenDay": map[string]interface{}{
				"supported": false,
				"message":   "not available",
			},
		},
	}

	snap := asQuotaSnapshotGo(raw)
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if snap.Status != "reverse_engineered" {
		t.Errorf("expected status 'reverse_engineered', got %q", snap.Status)
	}
	if snap.Source != "codex_probe" {
		t.Errorf("expected source 'codex_probe', got %q", snap.Source)
	}
	if snap.LastSyncAt != "2026-07-04T12:00:00Z" {
		t.Errorf("expected LastSyncAt, got %q", snap.LastSyncAt)
	}
	if snap.LastError != "none" {
		t.Errorf("expected LastError 'none', got %q", snap.LastError)
	}
	if snap.ProviderMessage != "ok" {
		t.Errorf("expected ProviderMessage 'ok', got %q", snap.ProviderMessage)
	}
	if snap.Windows.FiveHour == nil {
		t.Fatal("FiveHour should be non-nil")
	}
	if !snap.Windows.FiveHour.Supported {
		t.Error("FiveHour should be supported")
	}
	if *snap.Windows.FiveHour.Limit != 50.0 {
		t.Errorf("expected FiveHour limit 50.0, got %f", *snap.Windows.FiveHour.Limit)
	}
	if snap.Windows.SevenDay == nil {
		t.Fatal("SevenDay should be non-nil")
	}
	if snap.Windows.SevenDay.Supported {
		t.Error("SevenDay should be unsupported")
	}
}

func TestAsQuotaSnapshotGo_Nil(t *testing.T) {
	snap := asQuotaSnapshotGo(nil)
	if snap != nil {
		t.Error("expected nil for nil input")
	}
}

func TestAsQuotaSnapshotGo_NonMap(t *testing.T) {
	snap := asQuotaSnapshotGo("not a map")
	if snap != nil {
		t.Error("expected nil for non-map input")
	}
}

func TestAsQuotaSnapshotGo_MissingStatus(t *testing.T) {
	raw := map[string]interface{}{
		"source": "codex_probe",
	}
	snap := asQuotaSnapshotGo(raw)
	if snap != nil {
		t.Error("expected nil when status is missing")
	}
}

func TestAsQuotaSnapshotGo_MissingSource(t *testing.T) {
	raw := map[string]interface{}{
		"status": "reverse_engineered",
	}
	snap := asQuotaSnapshotGo(raw)
	if snap != nil {
		t.Error("expected nil when source is missing")
	}
}

// ---- normalizeQuotaWindowGo Tests ----

func TestNormalizeQuotaWindowGo_Supported(t *testing.T) {
	limit := 100.0
	raw := map[string]interface{}{
		"supported": true,
		"limit":     limit,
		"used":      30.0,
		"remaining": 70.0,
		"resetAt":   "2026-07-04T23:00:00Z",
	}
	w := normalizeQuotaWindowGo(raw)
	if w == nil {
		t.Fatal("expected non-nil window")
	}
	if !w.Supported {
		t.Error("expected supported true")
	}
	if *w.Limit != 100.0 {
		t.Errorf("expected limit 100.0, got %f", *w.Limit)
	}
	if *w.Used != 30.0 {
		t.Errorf("expected used 30.0, got %f", *w.Used)
	}
	if *w.Remaining != 70.0 {
		t.Errorf("expected remaining 70.0, got %f", *w.Remaining)
	}
	if w.ResetAt != "2026-07-04T23:00:00Z" {
		t.Errorf("expected resetAt, got %q", w.ResetAt)
	}
	if w.Message != "" {
		t.Errorf("expected no message, got %q", w.Message)
	}
}

func TestNormalizeQuotaWindowGo_Unsupported(t *testing.T) {
	raw := map[string]interface{}{
		"supported": false,
		"message":   "quota window not available",
	}
	w := normalizeQuotaWindowGo(raw)
	if w == nil {
		t.Fatal("expected non-nil window")
	}
	if w.Supported {
		t.Error("expected supported false")
	}
	if w.Message != "quota window not available" {
		t.Errorf("expected message, got %q", w.Message)
	}
	if w.Limit != nil {
		t.Error("limit should be nil for unsupported")
	}
}

func TestNormalizeQuotaWindowGo_OptionalFields(t *testing.T) {
	raw := map[string]interface{}{
		"supported": true,
	}
	w := normalizeQuotaWindowGo(raw)
	if w == nil {
		t.Fatal("expected non-nil window")
	}
	if w.Limit != nil {
		t.Error("limit should be nil when not provided")
	}
	if w.ResetAt != "" {
		t.Errorf("resetAt should be empty, got %q", w.ResetAt)
	}
}

// ---- parseRateLimitWindow ----

func TestParseRateLimitWindow_FullHeaders(t *testing.T) {
	headers := map[string]string{
		"x-ratelimit-limit-request-limit":       "100",
		"x-ratelimit-limit-remaining-requests":  "30",
		"x-ratelimit-limit-requests-reset-in-seconds": "60",
	}
	w := parseRateLimitWindow(headers, "x-ratelimit-limit")
	if w == nil {
		t.Fatal("expected non-nil window")
	}
	if !w.Supported {
		t.Error("window should be supported when limit/remaining present")
	}
	if w.Limit == nil || *w.Limit != 100 {
		t.Errorf("Limit = %v, want 100", w.Limit)
	}
	if w.Remaining == nil || *w.Remaining != 30 {
		t.Errorf("Remaining = %v, want 30", w.Remaining)
	}
	if w.Used == nil || *w.Used != 70 {
		t.Errorf("Used = %v, want 70 (100-30)", w.Used)
	}
	if w.ResetAt == "" {
		t.Error("ResetAt should be set from reset-in-seconds")
	}
}

func TestParseRateLimitWindow_OnlyLimit(t *testing.T) {
	headers := map[string]string{
		"prefix-request-limit": "200",
	}
	w := parseRateLimitWindow(headers, "prefix")
	if w == nil {
		t.Fatal("expected non-nil window")
	}
	if w.Limit == nil || *w.Limit != 200 {
		t.Errorf("Limit = %v", w.Limit)
	}
	if w.Remaining != nil {
		t.Errorf("Remaining should be nil, got %v", *w.Remaining)
	}
	if w.Used != nil {
		t.Error("Used should be nil when Remaining is nil")
	}
}

func TestParseRateLimitWindow_NoLimitOrRemainingReturnsNil(t *testing.T) {
	headers := map[string]string{
		"prefix-requests-reset-in-seconds": "60",
	}
	if w := parseRateLimitWindow(headers, "prefix"); w != nil {
		t.Errorf("expected nil when neither limit nor remaining present, got %+v", w)
	}
}

func TestParseRateLimitWindow_UsedClampedToZero(t *testing.T) {
	// If remaining > limit (shouldn't happen, but defensive), used clamps to 0.
	headers := map[string]string{
		"w-request-limit":      "10",
		"w-remaining-requests":  "20",
	}
	w := parseRateLimitWindow(headers, "w")
	if w.Used == nil || *w.Used != 0 {
		t.Errorf("Used = %v, want 0 (clamped)", w.Used)
	}
}

func TestParseRateLimitWindow_ResetSecondsParsedAsDuration(t *testing.T) {
	headers := map[string]string{
		"w-request-limit":              "10",
		"w-remaining-requests":         "5",
		"w-requests-reset-in-seconds":  "3600",
	}
	w := parseRateLimitWindow(headers, "w")
	if w.ResetAt == "" {
		t.Error("ResetAt should be set")
	}
}

func TestParseRateLimitWindow_ResetSecondsNonNumericPassthrough(t *testing.T) {
	headers := map[string]string{
		"w-request-limit":              "10",
		"w-remaining-requests":         "5",
		"w-requests-reset-in-seconds":  "not-a-number",
	}
	w := parseRateLimitWindow(headers, "w")
	if w.ResetAt != "not-a-number" {
		t.Errorf("non-numeric reset should pass through verbatim, got %q", w.ResetAt)
	}
}

// ---- fiveHourOrDefault / sevenDayOrDefault ----

func TestFiveHourOrDefault_PassesThroughNonNil(t *testing.T) {
	w := &OauthQuotaWindowSnapshot{Supported: true, Limit: float64Ptr(100)}
	got := fiveHourOrDefault(w)
	if got != w {
		t.Error("non-nil window should pass through")
	}
}

func TestFiveHourOrDefault_DefaultsToUnsupported(t *testing.T) {
	got := fiveHourOrDefault(nil)
	if got == nil {
		t.Fatal("expected non-nil default")
	}
	if got.Supported {
		t.Error("default should be unsupported")
	}
}

func TestSevenDayOrDefault_PassesThroughNonNil(t *testing.T) {
	w := &OauthQuotaWindowSnapshot{Supported: true}
	got := sevenDayOrDefault(w)
	if got != w {
		t.Error("non-nil window should pass through")
	}
}

func TestSevenDayOrDefault_DefaultsToUnsupported(t *testing.T) {
	got := sevenDayOrDefault(nil)
	if got == nil {
		t.Fatal("expected non-nil default")
	}
	if got.Supported {
		t.Error("default should be unsupported")
	}
}

// float64Ptr is a helper for tests that need *float64 literals.
func float64Ptr(f float64) *float64 { return &f }

// ---- quotaFingerprint ----

func TestQuotaFingerprint_NilReturnsEmpty(t *testing.T) {
	if got := quotaFingerprint(nil); got != "" {
		t.Errorf("nil snapshot should yield empty fingerprint, got %q", got)
	}
}

func TestQuotaFingerprint_Deterministic(t *testing.T) {
	snap := &OauthQuotaSnapshot{Status: "ok", Source: "probe"}
	a := quotaFingerprint(snap)
	b := quotaFingerprint(snap)
	if a != b {
		t.Errorf("same snapshot should yield same fingerprint: %q vs %q", a, b)
	}
	if a == "" {
		t.Error("fingerprint should not be empty")
	}
}

func TestQuotaFingerprint_DifferentSnapshotsDiffer(t *testing.T) {
	snap1 := &OauthQuotaSnapshot{Status: "ok", Source: "probe"}
	snap2 := &OauthQuotaSnapshot{Status: "error", Source: "probe"}
	if quotaFingerprint(snap1) == quotaFingerprint(snap2) {
		t.Error("different snapshots should yield different fingerprints")
	}
}

// ---- parseUsageLimitResetHint ----

func TestParseUsageLimitResetHint_EmptyReturnsEmpty(t *testing.T) {
	if got := parseUsageLimitResetHint(""); got != "" {
		t.Errorf("empty input = %q", got)
	}
}

func TestParseUsageLimitResetHint_InvalidJSONReturnsEmpty(t *testing.T) {
	if got := parseUsageLimitResetHint("not json"); got != "" {
		t.Errorf("invalid JSON = %q", got)
	}
}

func TestParseUsageLimitResetHint_TopLevelResetsAt(t *testing.T) {
	input := `{"resets_at":"2026-12-31T23:59:59Z"}`
	if got := parseUsageLimitResetHint(input); got != "2026-12-31T23:59:59Z" {
		t.Errorf("top-level resets_at = %q", got)
	}
}

func TestParseUsageLimitResetHint_TopLevelResetsInSeconds(t *testing.T) {
	input := `{"resets_in_seconds":3600}`
	got := parseUsageLimitResetHint(input)
	if got == "" {
		t.Error("resets_in_seconds should yield non-empty RFC3339")
	}
}

func TestParseUsageLimitResetHint_NestedUsageLimitReached(t *testing.T) {
	input := `{"usage_limit_reached":{"resets_at":"2026-06-01T00:00:00Z"}}`
	if got := parseUsageLimitResetHint(input); got != "2026-06-01T00:00:00Z" {
		t.Errorf("nested resets_at = %q", got)
	}
}

func TestParseUsageLimitResetHint_NestedResetsInSeconds(t *testing.T) {
	input := `{"usage_limit_reached":{"resets_in_seconds":1800}}`
	got := parseUsageLimitResetHint(input)
	if got == "" {
		t.Error("nested resets_in_seconds should yield non-empty RFC3339")
	}
}

func TestParseUsageLimitResetHint_NoRelevantFieldsReturnsEmpty(t *testing.T) {
	input := `{"other":"field"}`
	if got := parseUsageLimitResetHint(input); got != "" {
		t.Errorf("no relevant fields = %q, want empty", got)
	}
}

// ---- parseFloat64 ----

func TestParseFloat64_ValidNumber(t *testing.T) {
	v := parseFloat64("3.14")
	if v == nil || *v != 3.14 {
		t.Errorf("parseFloat64(3.14) = %v, want 3.14", v)
	}
}

func TestParseFloat64_Integer(t *testing.T) {
	v := parseFloat64("42")
	if v == nil || *v != 42 {
		t.Errorf("parseFloat64(42) = %v, want 42", v)
	}
}

func TestParseFloat64_EmptyReturnsNil(t *testing.T) {
	if v := parseFloat64(""); v != nil {
		t.Errorf("parseFloat64('') = %v, want nil", v)
	}
}

func TestParseFloat64_WhitespaceTrimmed(t *testing.T) {
	v := parseFloat64("  1.5  ")
	if v == nil || *v != 1.5 {
		t.Errorf("parseFloat64('  1.5  ') = %v, want 1.5", v)
	}
}

func TestParseFloat64_NonNumericReturnsNil(t *testing.T) {
	if v := parseFloat64("abc"); v != nil {
		t.Errorf("parseFloat64('abc') = %v, want nil", v)
	}
}

// ---- resolveAccountProxyURLForQuota ----

func TestResolveAccountProxyURLForQuota_FromExtraConfig(t *testing.T) {
	extra := `{"proxyUrl":"http://proxy:3128"}`
	v := resolveAccountProxyURLForQuota(1, &extra)
	if v == nil || *v != "http://proxy:3128" {
		t.Errorf("got %v, want http://proxy:3128", v)
	}
}

func TestResolveAccountProxyURLForQuota_NoProxyReturnsNil(t *testing.T) {
	extra := `{"other":"value"}`
	if v := resolveAccountProxyURLForQuota(1, &extra); v != nil {
		t.Errorf("no proxy in extraConfig = %v, want nil", v)
	}
}

func TestResolveAccountProxyURLForQuota_NilExtraConfigReturnsNil(t *testing.T) {
	if v := resolveAccountProxyURLForQuota(1, nil); v != nil {
		t.Errorf("nil extraConfig = %v, want nil", v)
	}
}
