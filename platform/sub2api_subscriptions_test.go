package platform

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
	"time"
)

// expectedRFC3339 computes the same local-timezone RFC3339 string that the
// production parseDateTime produces for a Unix-millisecond timestamp. This
// keeps the tests timezone-independent (the production code uses
// time.UnixMilli(...).Format(time.RFC3339) which honours the local zone).
func expectedRFC333For(unixMillis int64) string {
	return time.UnixMilli(unixMillis).Format(time.RFC3339)
}

// mustParseJSONSub unmarshals a JSON literal into a generic interface{} so the
// parser tests can feed realistic shapes (maps, arrays, numbers as float64)
// exactly as the production fetchJSON path would.
func mustParseJSONSub(t *testing.T, raw string) interface{} {
	t.Helper()
	var v interface{}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("mustParseJSONSub(%q): %v", raw, err)
	}
	return v
}

func newSub2ApiAdapterForSubs() *Sub2ApiAdapter {
	return &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
}

// --- parseNonNegativeNumber ---

func TestSub2ApiSubscriptions_parseNonNegativeNumber(t *testing.T) {
	s := newSub2ApiAdapterForSubs()

	tests := []struct {
		name   string
		values []interface{}
		want   *float64
	}{
		{"positive float64", []interface{}{float64(5.0)}, floatPtr(5.0)},
		{"zero is non-negative", []interface{}{float64(0.0)}, floatPtr(0.0)},
		{"negative rejected", []interface{}{float64(-1.5)}, nil},
		{"string number", []interface{}{"3.14"}, floatPtr(3.14)},
		{"string zero", []interface{}{"0"}, floatPtr(0.0)},
		{"negative string rejected", []interface{}{"-2.5"}, nil},
		{"empty string skipped", []interface{}{""}, nil},
		{"whitespace string skipped", []interface{}{"  "}, nil},
		{"non-numeric string skipped", []interface{}{"abc"}, nil},
		{"nil skipped", []interface{}{nil}, nil},
		{"no arguments", []interface{}{}, nil},
		{"first valid wins", []interface{}{float64(-1), float64(7.0)}, floatPtr(7.0)},
		{"nil then valid", []interface{}{nil, float64(9.0)}, floatPtr(9.0)},
		{"string then float", []interface{}{"invalid", float64(4.0)}, floatPtr(4.0)},
		{"rounding to 6 decimals", []interface{}{float64(1.23456789)}, floatPtr(math.Round(1.23456789*1e6) / 1e6)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.parseNonNegativeNumber(tt.values...)
			if !floatPtrEqual(got, tt.want) {
				t.Fatalf("parseNonNegativeNumber(%v) = %v, want %v", tt.values, got, tt.want)
			}
		})
	}
}

// floatPtr is a test helper that returns a pointer to the given float64.
func floatPtr(v float64) *float64 {
	return &v
}

// floatPtrEqual compares two *float64 for equality (both nil or both equal).
func floatPtrEqual(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

// --- parseDateTime ---

func TestSub2ApiSubscriptions_parseDateTime(t *testing.T) {
	s := newSub2ApiAdapterForSubs()

	// 1705312200 is a Unix-seconds timestamp; the production code multiplies
	// by 1000 when the value is below 10e9 (seconds → millis). Both the seconds
	// and the millis form must produce the same RFC3339 string.
	expectedFromSeconds := expectedRFC333For(1705312200000)

	tests := []struct {
		name         string
		values       []string
		want         string
		wantNonEmpty bool
	}{
		{"unix seconds", []string{"1705312200"}, expectedFromSeconds, true},
		{"unix milliseconds", []string{"1705312200000"}, expectedFromSeconds, true},
		{"empty string", []string{""}, "", false},
		{"whitespace string", []string{"  "}, "", false},
		{"non-numeric non-date", []string{"not-a-date"}, "", false},
		{"first empty then valid", []string{"", "1705312200"}, expectedFromSeconds, true},
		{"no arguments", []string{}, "", false},
		{"negative numeric rejected (<=0)", []string{"-100"}, "", false},
		{"zero numeric rejected (<=0)", []string{"0"}, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.parseDateTime(tt.values...)
			if tt.wantNonEmpty {
				if got == "" {
					t.Fatalf("parseDateTime(%v) = empty, want non-empty", tt.values)
				}
				if tt.want != "" && got != tt.want {
					t.Fatalf("parseDateTime(%v) = %q, want %q", tt.values, got, tt.want)
				}
			} else {
				if got != "" {
					t.Fatalf("parseDateTime(%v) = %q, want empty", tt.values, got)
				}
			}
		})
	}
}

// --- getRaw ---

func TestSub2ApiSubscriptions_getRaw(t *testing.T) {
	s := newSub2ApiAdapterForSubs()

	item := map[string]interface{}{
		"present":   float64(42),
		"str":       "hello",
		"nilval":    nil,
		"nested":    map[string]interface{}{"inner": float64(1)},
	}

	tests := []struct {
		name string
		key  string
		want interface{}
	}{
		{"existing numeric", "present", float64(42)},
		{"existing string", "str", "hello"},
		{"existing nil value", "nilval", nil},
		{"existing map value", "nested", item["nested"]},
		{"missing key", "absent", nil},
		{"empty key", "", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.getRaw(item, tt.key)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("getRaw(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

// --- getRawString ---

func TestSub2ApiSubscriptions_getRawString(t *testing.T) {
	s := newSub2ApiAdapterForSubs()

	item := map[string]interface{}{
		"str":       "  spaced  ",
		"num":       float64(12),
		"nilval":    nil,
		"boolval":   true,
		"nested":    map[string]interface{}{"x": float64(1)},
	}

	tests := []struct {
		name string
		key  string
		want string
	}{
		{"string trimmed", "str", "spaced"},
		{"float64 formatted", "num", "12"},
		{"nil value empty", "nilval", ""},
		{"missing key empty", "absent", ""},
		{"bool value empty (unsupported type)", "boolval", ""},
		{"map value empty (unsupported type)", "nested", ""},
		{"empty key", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := s.getRawString(item, tt.key)
			if got != tt.want {
				t.Fatalf("getRawString(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

// --- parseSingleSubscription ---

func TestSub2ApiSubscriptions_parseSingleSubscription(t *testing.T) {
	s := newSub2ApiAdapterForSubs()

	t.Run("complete subscription", func(t *testing.T) {
		raw := mustParseJSONSub(t, `{
			"id": 101,
			"group_id": 5,
			"group_name": "premium",
			"status": "active",
			"expires_at": "1705312200",
			"daily_used_usd": 1.5,
			"daily_limit_usd": 10,
			"weekly_used_usd": 7.5,
			"weekly_limit_usd": 50,
			"monthly_used_usd": 30,
			"monthly_limit_usd": 100
		}`)
		item, _ := raw.(map[string]interface{})

		summary := s.parseSingleSubscription(item)
		if summary == nil {
			t.Fatal("parseSingleSubscription: got nil, want non-nil")
		}
		if summary.ID == nil || *summary.ID != 101 {
			t.Errorf("ID = %v, want 101", summary.ID)
		}
		if summary.GroupID == nil || *summary.GroupID != 5 {
			t.Errorf("GroupID = %v, want 5", summary.GroupID)
		}
		if summary.GroupName != "premium" {
			t.Errorf("GroupName = %q, want premium", summary.GroupName)
		}
		if summary.Status != "active" {
			t.Errorf("Status = %q, want active", summary.Status)
		}
		if summary.ExpiresAt != expectedRFC333For(1705312200000) {
			t.Errorf("ExpiresAt = %q, want %q", summary.ExpiresAt, expectedRFC333For(1705312200000))
		}
		if summary.DailyUsedUsd == nil || *summary.DailyUsedUsd != 1.5 {
			t.Errorf("DailyUsedUsd = %v, want 1.5", summary.DailyUsedUsd)
		}
		if summary.MonthlyLimitUsd == nil || *summary.MonthlyLimitUsd != 100 {
			t.Errorf("MonthlyLimitUsd = %v, want 100", summary.MonthlyLimitUsd)
		}
	})

	t.Run("nested group object", func(t *testing.T) {
		raw := mustParseJSONSub(t, `{"id": 7, "group": {"id": 3, "name": "nested-group"}}`)
		item, _ := raw.(map[string]interface{})

		summary := s.parseSingleSubscription(item)
		if summary == nil {
			t.Fatal("parseSingleSubscription: got nil")
		}
		if summary.GroupID == nil || *summary.GroupID != 3 {
			t.Errorf("GroupID = %v, want 3 (from nested group.id)", summary.GroupID)
		}
		if summary.GroupName != "nested-group" {
			t.Errorf("GroupName = %q, want nested-group", summary.GroupName)
		}
	})

	t.Run("camelCase keys", func(t *testing.T) {
		raw := mustParseJSONSub(t, `{
			"groupId": 88,
			"groupName": "camel",
			"monthlyUsedUsd": 5,
			"monthlyLimitUsd": 50
		}`)
		item, _ := raw.(map[string]interface{})

		summary := s.parseSingleSubscription(item)
		if summary == nil {
			t.Fatal("parseSingleSubscription: got nil")
		}
		if summary.GroupID == nil || *summary.GroupID != 88 {
			t.Errorf("GroupID = %v, want 88", summary.GroupID)
		}
		if summary.GroupName != "camel" {
			t.Errorf("GroupName = %q, want camel", summary.GroupName)
		}
		if summary.MonthlyUsedUsd == nil || *summary.MonthlyUsedUsd != 5 {
			t.Errorf("MonthlyUsedUsd = %v, want 5", summary.MonthlyUsedUsd)
		}
	})

	t.Run("partial - only status", func(t *testing.T) {
		item := map[string]interface{}{"status": "expired"}

		summary := s.parseSingleSubscription(item)
		if summary == nil {
			t.Fatal("parseSingleSubscription: got nil for partial")
		}
		if summary.Status != "expired" {
			t.Errorf("Status = %q, want expired", summary.Status)
		}
		if summary.GroupName != "" {
			t.Errorf("GroupName = %q, want empty", summary.GroupName)
		}
	})

	t.Run("empty map returns nil", func(t *testing.T) {
		summary := s.parseSingleSubscription(map[string]interface{}{})
		if summary != nil {
			t.Fatalf("parseSingleSubscription(empty) = %v, want nil", summary)
		}
	})

	t.Run("all-negative numbers returns nil", func(t *testing.T) {
		raw := mustParseJSONSub(t, `{"daily_used_usd": -1, "monthly_used_usd": -2}`)
		item, _ := raw.(map[string]interface{})

		summary := s.parseSingleSubscription(item)
		if summary != nil {
			t.Fatalf("parseSingleSubscription(all-negative) = %v, want nil", summary)
		}
	})

	t.Run("alias keys for monthly used", func(t *testing.T) {
		raw := mustParseJSONSub(t, `{"used_usd": 15.5, "limit_usd": 200}`)
		item, _ := raw.(map[string]interface{})

		summary := s.parseSingleSubscription(item)
		if summary == nil {
			t.Fatal("parseSingleSubscription: got nil")
		}
		if summary.MonthlyUsedUsd == nil || *summary.MonthlyUsedUsd != 15.5 {
			t.Errorf("MonthlyUsedUsd = %v, want 15.5 (from used_usd)", summary.MonthlyUsedUsd)
		}
		if summary.MonthlyLimitUsd == nil || *summary.MonthlyLimitUsd != 200 {
			t.Errorf("MonthlyLimitUsd = %v, want 200 (from limit_usd)", summary.MonthlyLimitUsd)
		}
	})
}

// --- parseSubscriptionItems ---

func TestSub2ApiSubscriptions_parseSubscriptionItems(t *testing.T) {
	s := newSub2ApiAdapterForSubs()

	t.Run("array of subscriptions", func(t *testing.T) {
		raw := mustParseJSONSub(t, `[
			{"id": 1, "status": "active", "monthly_used_usd": 5},
			{"id": 2, "status": "expired"}
		]`)
		items := s.parseSubscriptionItems(raw)
		if len(items) != 2 {
			t.Fatalf("got %d items, want 2", len(items))
		}
		if items[0].ID == nil || *items[0].ID != 1 {
			t.Errorf("items[0].ID = %v, want 1", items[0].ID)
		}
	})

	t.Run("map with subscriptions key", func(t *testing.T) {
		raw := mustParseJSONSub(t, `{"subscriptions": [{"id": 10}]}`)
		items := s.parseSubscriptionItems(raw)
		if len(items) != 1 {
			t.Fatalf("got %d items, want 1", len(items))
		}
	})

	t.Run("map with items key", func(t *testing.T) {
		raw := mustParseJSONSub(t, `{"items": [{"id": 20}]}`)
		items := s.parseSubscriptionItems(raw)
		if len(items) != 1 {
			t.Fatalf("got %d items, want 1", len(items))
		}
	})

	t.Run("map with data key", func(t *testing.T) {
		raw := mustParseJSONSub(t, `{"data": [{"id": 30}]}`)
		items := s.parseSubscriptionItems(raw)
		if len(items) != 1 {
			t.Fatalf("got %d items, want 1", len(items))
		}
	})

	t.Run("map with list key", func(t *testing.T) {
		raw := mustParseJSONSub(t, `{"list": [{"id": 40}]}`)
		items := s.parseSubscriptionItems(raw)
		if len(items) != 1 {
			t.Fatalf("got %d items, want 1", len(items))
		}
	})

	t.Run("empty array", func(t *testing.T) {
		items := s.parseSubscriptionItems([]interface{}{})
		if len(items) != 0 {
			t.Fatalf("got %d items, want 0", len(items))
		}
	})

	t.Run("map with no array keys", func(t *testing.T) {
		raw := mustParseJSONSub(t, `{"foo": "bar"}`)
		items := s.parseSubscriptionItems(raw)
		if len(items) != 0 {
			t.Fatalf("got %d items, want 0", len(items))
		}
	})

	t.Run("nil input", func(t *testing.T) {
		items := s.parseSubscriptionItems(nil)
		if len(items) != 0 {
			t.Fatalf("got %d items, want 0", len(items))
		}
	})

	t.Run("malformed items skipped", func(t *testing.T) {
		raw := mustParseJSONSub(t, `[
			{"id": 1},
			"not-a-map",
			42,
			null,
			{"id": 2}
		]`)
		items := s.parseSubscriptionItems(raw)
		if len(items) != 2 {
			t.Fatalf("got %d items, want 2 (non-map items skipped)", len(items))
		}
	})

	t.Run("items that parse to nil are skipped", func(t *testing.T) {
		raw := mustParseJSONSub(t, `[
			{"id": 1},
			{"daily_used_usd": -5},
			{"id": 2}
		]`)
		items := s.parseSubscriptionItems(raw)
		if len(items) != 2 {
			t.Fatalf("got %d items, want 2 (nil-summary item skipped)", len(items))
		}
	})
}

// --- buildSubscriptionSummary ---

func TestSub2ApiSubscriptions_buildSubscriptionSummary(t *testing.T) {
	s := newSub2ApiAdapterForSubs()

	t.Run("array with active_count and total_used_usd", func(t *testing.T) {
		// When raw is a bare array, body is nil so active_count/total_used_usd
		// come from the fallback logic (len + sum).
		raw := mustParseJSONSub(t, `[
			{"id": 1, "monthly_used_usd": 5.0},
			{"id": 2, "monthly_used_usd": 3.0}
		]`)
		summary := s.buildSubscriptionSummary(raw)
		if summary == nil {
			t.Fatal("buildSubscriptionSummary: got nil")
		}
		if summary.ActiveCount != 2 {
			t.Errorf("ActiveCount = %d, want 2", summary.ActiveCount)
		}
		if summary.TotalUsedUsd != 8.0 {
			t.Errorf("TotalUsedUsd = %f, want 8.0", summary.TotalUsedUsd)
		}
		if len(summary.Subscriptions) != 2 {
			t.Errorf("len(Subscriptions) = %d, want 2", len(summary.Subscriptions))
		}
	})

	t.Run("map with explicit counts", func(t *testing.T) {
		raw := mustParseJSONSub(t, `{
			"active_count": 5,
			"total_used_usd": 42.5,
			"subscriptions": [{"id": 1, "monthly_used_usd": 10}]
		}`)
		summary := s.buildSubscriptionSummary(raw)
		if summary == nil {
			t.Fatal("buildSubscriptionSummary: got nil")
		}
		if summary.ActiveCount != 5 {
			t.Errorf("ActiveCount = %d, want 5", summary.ActiveCount)
		}
		if summary.TotalUsedUsd != 42.5 {
			t.Errorf("TotalUsedUsd = %f, want 42.5", summary.TotalUsedUsd)
		}
	})

	t.Run("camelCase counts", func(t *testing.T) {
		raw := mustParseJSONSub(t, `{
			"activeCount": 3,
			"totalUsedUsd": 7.0
		}`)
		summary := s.buildSubscriptionSummary(raw)
		if summary == nil {
			t.Fatal("buildSubscriptionSummary: got nil")
		}
		if summary.ActiveCount != 3 {
			t.Errorf("ActiveCount = %d, want 3", summary.ActiveCount)
		}
		if summary.TotalUsedUsd != 7.0 {
			t.Errorf("TotalUsedUsd = %f, want 7.0", summary.TotalUsedUsd)
		}
	})

	t.Run("empty input", func(t *testing.T) {
		summary := s.buildSubscriptionSummary(map[string]interface{}{})
		if summary == nil {
			t.Fatal("buildSubscriptionSummary: got nil")
		}
		if summary.ActiveCount != 0 {
			t.Errorf("ActiveCount = %d, want 0", summary.ActiveCount)
		}
		if summary.TotalUsedUsd != 0 {
			t.Errorf("TotalUsedUsd = %f, want 0", summary.TotalUsedUsd)
		}
		if len(summary.Subscriptions) != 0 {
			t.Errorf("len(Subscriptions) = %d, want 0", len(summary.Subscriptions))
		}
	})

	t.Run("nil input", func(t *testing.T) {
		summary := s.buildSubscriptionSummary(nil)
		if summary == nil {
			t.Fatal("buildSubscriptionSummary: got nil")
		}
		if summary.ActiveCount != 0 {
			t.Errorf("ActiveCount = %d, want 0", summary.ActiveCount)
		}
	})

	t.Run("fallback total from subscriptions when total_used_usd is zero", func(t *testing.T) {
		raw := mustParseJSONSub(t, `{
			"subscriptions": [
				{"id": 1, "monthly_used_usd": 2.0},
				{"id": 2, "monthly_used_usd": 3.0}
			]
		}`)
		summary := s.buildSubscriptionSummary(raw)
		if summary == nil {
			t.Fatal("buildSubscriptionSummary: got nil")
		}
		if summary.TotalUsedUsd != 5.0 {
			t.Errorf("TotalUsedUsd = %f, want 5.0 (sum of monthly_used_usd)", summary.TotalUsedUsd)
		}
	})
}
