package scheduler

import (
	"fmt"
	"testing"
)

// ---- E1: random-window scheduling (all-api-hub borrow) ----

func TestRandomCronInWindow_Bounds(t *testing.T) {
	tests := []struct {
		start, end string
		wantMin    int
		wantMax    int
	}{
		{"00:00", "23:59", 0, 1439},    // full day
		{"02:00", "03:30", 120, 210},   // 90-min window
		{"12:00", "12:00", 720, 720},   // zero-width (single minute)
		{"08:15", "09:00", 495, 540},   // partial hours
		{"23:00", "23:59", 1380, 1439}, // late night
	}
	for _, tt := range tests {
		t.Run(tt.start+"-"+tt.end, func(t *testing.T) {
			for i := 0; i < 50; i++ {
				expr, err := RandomCronInWindow(tt.start, tt.end)
				if err != nil {
					t.Fatalf("RandomCronInWindow(%q,%q): %v", tt.start, tt.end, err)
				}
				if !ValidateCronExpr(expr) {
					t.Fatalf("rolled cron %q not valid", expr)
				}
				var h, m int
				if _, err := fmt.Sscanf(expr, "%d %d", &m, &h); err != nil {
					t.Fatalf("unexpected cron shape %q: %v", expr, err)
				}
				rolled := h*60 + m
				if rolled < tt.wantMin || rolled > tt.wantMax {
					t.Fatalf("rolled %d min (cron %q), want [%d,%d]", rolled, expr, tt.wantMin, tt.wantMax)
				}
			}
		})
	}
}

func TestRandomCronInWindow_ReRollsAcrossCalls(t *testing.T) {
	// Over many rolls in a wide window, more than one distinct value must
	// appear (statistically certain over 200 rolls of a 24h window).
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		expr, err := RandomCronInWindow("00:00", "23:59")
		if err != nil {
			t.Fatalf("RandomCronInWindow: %v", err)
		}
		seen[expr] = true
	}
	if len(seen) < 2 {
		t.Fatalf("expected re-rolls to vary, got single value %v", seen)
	}
}

func TestRandomCronInWindow_RejectsBadBounds(t *testing.T) {
	bad := [][2]string{
		{"13:00", "08:00"}, // start after end
		{"", "08:00"},      // empty start
		{"08:00", ""},      // empty end
		{"25:00", "26:00"}, // hour out of range
		{"08:60", "09:00"}, // minute out of range
		{"08:00", "9"},     // malformed end
		{"abc", "09:00"},   // non-numeric
	}
	for _, b := range bad {
		t.Run(b[0]+"-"+b[1], func(t *testing.T) {
			if _, err := RandomCronInWindow(b[0], b[1]); err == nil {
				t.Fatalf("expected error for bounds %q-%q", b[0], b[1])
			}
		})
	}
}

func TestParseHHMM(t *testing.T) {
	tests := []struct {
		raw  string
		want int
		ok   bool
	}{
		{"00:00", 0, true},
		{"08:15", 495, true},
		{"23:59", 1439, true},
		{"8:5", 485, true},  // tolerant of missing zero-padding
		{"24:00", 0, false}, // hour out of range
		{"08:60", 0, false},
		{"", 0, false},
		{"abc", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, err := parseHHMM(tt.raw)
			if tt.ok && err != nil {
				t.Fatalf("parseHHMM(%q) = %v, want ok", tt.raw, err)
			}
			if !tt.ok && err == nil {
				t.Fatalf("parseHHMM(%q) = %d, want error", tt.raw, got)
			}
			if tt.ok && got != tt.want {
				t.Fatalf("parseHHMM(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}
