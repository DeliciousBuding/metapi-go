package routing

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/golden"
)

// formatGoldenFloat mirrors the removed production fmtFloat helper for
// golden-file output: up to 6 fractional digits (truncated, not rounded),
// trailing zeros trimmed, NaN/Inf rendered as "0". Keeping the exact digit
// extraction keeps the checked-in golden files byte-stable.
func formatGoldenFloat(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return "0"
	}
	neg := false
	if v < 0 {
		neg = true
		v = -v
	}
	intPart := int64(math.Trunc(v))
	fracPart := v - float64(intPart)
	s := strconv.FormatInt(intPart, 10)
	s += "."
	for i := 0; i < 6; i++ {
		fracPart *= 10
		d := int64(math.Trunc(fracPart))
		s += strconv.FormatInt(d, 10)
		fracPart -= float64(d)
	}
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" {
		return "0"
	}
	if neg {
		return "-" + s
	}
	return s
}

// readOrUpdateGoldenFile keeps routing's flat testdata/*.txt layout while
// reusing the shared snapshot harness: one comparison, one update switch
// (GOLDEN_UPDATE) and one empty-snapshot guard for the whole repository.
// The returned bytes are the generated content (equal to disk after a
// successful compare, modulo line-ending normalization).
func readOrUpdateGoldenFile(t *testing.T, path string, content []byte) []byte {
	t.Helper()
	golden.CheckBytesAt(t, path, content)
	return content
}
