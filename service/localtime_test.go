package service

import (
	"testing"
	"time"
)

func TestGetLocalDayRangeUTCUsesExclusiveNextMidnight(t *testing.T) {
	location := time.FixedZone("HKT", 8*60*60)
	now := time.Date(2026, 8, 1, 12, 34, 56, 0, location)

	got := GetLocalDayRangeUTC(now)

	if got.LocalDay != "2026-08-01" {
		t.Fatalf("LocalDay = %q, want 2026-08-01", got.LocalDay)
	}
	if got.StartUTC != "2026-07-31T16:00:00Z" {
		t.Fatalf("StartUTC = %q, want 2026-07-31T16:00:00Z", got.StartUTC)
	}
	if got.EndUTC != "2026-08-01T16:00:00Z" {
		t.Fatalf("EndUTC = %q, want 2026-08-01T16:00:00Z", got.EndUTC)
	}
}
