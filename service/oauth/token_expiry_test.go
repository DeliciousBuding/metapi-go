package oauth

import (
	"testing"
	"time"
)

func TestTokenExpiryContract(t *testing.T) {
	const absolute = "2030-01-02T03:04:05Z"
	fallback, err := time.Parse(time.RFC3339, absolute)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name      string
		expiresIn any
		expiry    string
		relative  bool
		seconds   int64
		want      int64
	}{
		{name: "JSON number", expiresIn: float64(60), relative: true, seconds: 60},
		{name: "trimmed decimal", expiresIn: " 60\t", relative: true, seconds: 60},
		{name: "fraction truncates before milliseconds", expiresIn: float64(1.9), relative: true, seconds: 1},
		{name: "positive subsecond retains current time", expiresIn: float64(0.5), relative: true},
		{name: "relative takes precedence", expiresIn: "60", expiry: absolute, relative: true, seconds: 60},
		{name: "absolute fallback", expiry: absolute, want: fallback.UnixMilli()},
		{name: "zero uses fallback", expiresIn: float64(0), expiry: absolute, want: fallback.UnixMilli()},
		{name: "negative uses fallback", expiresIn: float64(-1), expiry: absolute, want: fallback.UnixMilli()},
		{name: "integer type is not JSON number", expiresIn: int64(60), expiry: absolute, want: fallback.UnixMilli()},
		{name: "signed string uses fallback", expiresIn: "+60", expiry: absolute, want: fallback.UnixMilli()},
		{name: "invalid relative uses fallback", expiresIn: "invalid", expiry: absolute, want: fallback.UnixMilli()},
		{name: "invalid absolute", expiry: "invalid"},
		{name: "missing expiry"},
		{name: "invalid relative without fallback", expiresIn: "invalid"},
		{name: "unsupported type without fallback", expiresIn: int64(60)},
		{name: "negative without fallback", expiresIn: float64(-1)},
		{name: "empty relative", expiresIn: ""},
		{name: "negative string", expiresIn: "-60"},
		// 既有解析会回绕；溢出处理的修正不能混进保行为重构。
		{name: "legacy integer wraparound", expiresIn: "18446744073709551617", relative: true, seconds: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now().UnixMilli()
			got := parseTokenExpiresAt(tt.expiresIn, tt.expiry)
			after := time.Now().UnixMilli()
			if tt.relative {
				if lo, hi := before+tt.seconds*1000, after+tt.seconds*1000; got < lo || got > hi {
					t.Fatalf("expiry = %d, want [%d, %d]", got, lo, hi)
				}
			} else if got != tt.want {
				t.Fatalf("expiry = %d, want %d", got, tt.want)
			}
		})
	}
}
