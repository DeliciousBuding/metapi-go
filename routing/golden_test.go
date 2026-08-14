package routing

import (
	"bytes"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const updateRoutingGoldenEnv = "UPDATE_ROUTING_GOLDEN"

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

func readOrUpdateGoldenFile(t *testing.T, path string, content []byte) []byte {
	t.Helper()

	if os.Getenv(updateRoutingGoldenEnv) == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create golden directory for %s: %v", path, err)
		}
		if err := os.WriteFile(path, content, 0644); err != nil {
			t.Fatalf("failed to update golden file %s: %v", path, err)
		}
		t.Logf("Golden file updated: %s", path)
		return content
	}

	readBack, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read golden file %s (set %s=1 to regenerate): %v", path, updateRoutingGoldenEnv, err)
	}
	if len(readBack) == 0 {
		t.Fatalf("golden file %s is empty", path)
	}
	if !bytes.Equal(normalizeGoldenNewlines(readBack), normalizeGoldenNewlines(content)) {
		t.Fatalf("golden file %s does not match generated content (set %s=1 to regenerate)", path, updateRoutingGoldenEnv)
	}
	return readBack
}

func normalizeGoldenNewlines(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

func TestReadOrUpdateGoldenFileAcceptsCRLFCheckout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "golden.txt")
	if err := os.WriteFile(path, []byte("same\r\ncontent\r\n"), 0644); err != nil {
		t.Fatalf("write golden: %v", err)
	}

	readOrUpdateGoldenFile(t, path, []byte("same\ncontent\n"))
}

func TestReadOrUpdateGoldenFileMismatchFails(t *testing.T) {
	if os.Getenv("ROUTING_GOLDEN_MISMATCH_HELPER") == "1" {
		path := os.Getenv("ROUTING_GOLDEN_MISMATCH_PATH")
		if path == "" {
			t.Fatal("missing ROUTING_GOLDEN_MISMATCH_PATH")
		}
		readOrUpdateGoldenFile(t, path, []byte("new\n"))
		return
	}

	path := filepath.Join(t.TempDir(), "golden.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0644); err != nil {
		t.Fatalf("write golden: %v", err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestReadOrUpdateGoldenFileMismatchFails")
	cmd.Env = append(os.Environ(),
		"ROUTING_GOLDEN_MISMATCH_HELPER=1",
		"ROUTING_GOLDEN_MISMATCH_PATH="+path,
	)
	if err := cmd.Run(); err == nil {
		t.Fatal("mismatched golden content did not fail")
	}
}
