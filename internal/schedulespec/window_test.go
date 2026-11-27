package schedulespec

import "testing"

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
