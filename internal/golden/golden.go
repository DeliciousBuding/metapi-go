// Package golden is a test-only snapshot harness for the protocol-conversion
// golden suites (P0-1). It is imported exclusively from _test.go files, so it
// never becomes part of the production dependency graph.
//
// Each golden case is a pair of checked-in files under the owning package's
// testdata/golden/<category>/ directory:
//
//	<name>.input.json   fixture input (hand-written, never auto-rewritten)
//	<name>.golden.json  recorded output snapshot
//
// By default tests compare current behavior against the checked-in snapshot.
// Setting GOLDEN_UPDATE=1 rewrites the snapshot files from current behavior:
//
//	GOLDEN_UPDATE=1 go test ./transform/... ./handler/proxy/...
//
// Regenerate only after deliberately reviewing the resulting diff.
package golden

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// UpdateEnv is the environment variable that switches the harness into
// snapshot-rewrite mode.
const UpdateEnv = "GOLDEN_UPDATE"

// Updating reports whether GOLDEN_UPDATE=1 is set for this test run.
func Updating() bool {
	return os.Getenv(UpdateEnv) == "1"
}

func inputPath(category, name string) string {
	return filepath.Join("testdata", "golden", category, name+".input.json")
}

func goldenPath(category, name, ext string) string {
	return filepath.Join("testdata", "golden", category, name+".golden"+ext)
}

// ReadInput reads and unmarshals testdata/golden/<category>/<name>.input.json
// into dst.
func ReadInput(t *testing.T, category, name string, dst any) {
	t.Helper()
	raw, err := os.ReadFile(inputPath(category, name))
	if err != nil {
		t.Fatalf("read golden input: %v", err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("parse golden input %s: %v", inputPath(category, name), err)
	}
}

// ReadRawInput returns the raw bytes of
// testdata/golden/<category>/<name>.input<ext> (for example ext ".sse" for
// stream fixtures where byte-level boundaries matter).
func ReadRawInput(t *testing.T, category, name, ext string) []byte {
	t.Helper()
	path := filepath.Join("testdata", "golden", category, name+".input"+ext)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden input: %v", err)
	}
	return raw
}

// Check marshals got (indent 2, sorted map keys via encoding/json) and
// compares it with testdata/golden/<category>/<name>.golden.json. With
// GOLDEN_UPDATE=1 the snapshot is rewritten instead.
func Check(t *testing.T, category, name string, got any) {
	t.Helper()
	data, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden output: %v", err)
	}
	data = append(data, '\n')
	CheckBytes(t, category, name, ".json", data)
}

// CheckBytes compares raw bytes against
// testdata/golden/<category>/<name>.golden<ext>. With GOLDEN_UPDATE=1 the
// snapshot is rewritten instead. Use this for byte-level contracts such as
// pass-through transforms and raw SSE fixtures.
func CheckBytes(t *testing.T, category, name, ext string, got []byte) {
	t.Helper()
	path := goldenPath(category, name, ext)
	if Updating() {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden snapshot: %v", err)
		}
		t.Logf("golden snapshot updated: %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("golden snapshot missing (%v); regenerate with %s=1 go test ./%s", err, UpdateEnv, packageDir(t))
	}
	if string(want) != string(got) {
		t.Fatalf("behavior drifted from golden snapshot %s;\n--- want ---\n%s\n--- got ---\n%s\nif the change is intentional, regenerate with %s=1 go test",
			path, truncate(want), truncate(got), UpdateEnv)
	}
}

func packageDir(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		return "..."
	}
	return filepath.ToSlash(wd)
}

func truncate(b []byte) string {
	const max = 4000
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "\n... (truncated)"
}
