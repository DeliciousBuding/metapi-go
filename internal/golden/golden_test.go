package golden

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// envWithout returns os.Environ with any existing entry for key removed, so a
// helper subprocess sees exactly one value for each key it sets.
func envWithout(key string) []string {
	env := os.Environ()
	out := env[:0:0]
	for _, kv := range env {
		if strings.HasPrefix(kv, key+"=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func runHarnessHelper(t *testing.T, testName string, extraEnv ...string) error {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+testName+"$")
	cmd.Env = append(envWithout(UpdateEnv), extraEnv...)
	return cmd.Run()
}

func TestCheckBytesAtAcceptsCRLFCheckout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "golden.txt")
	if err := os.WriteFile(path, []byte("same\r\ncontent\r\n"), 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
	CheckBytesAt(t, path, []byte("same\ncontent\n"))
}

func TestCheckBytesAtMismatchFails(t *testing.T) {
	if os.Getenv("GOLDEN_MISMATCH_HELPER") == "1" {
		CheckBytesAt(t, os.Getenv("GOLDEN_MISMATCH_PATH"), []byte("new\n"))
		return
	}
	path := filepath.Join(t.TempDir(), "golden.txt")
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
	if err := runHarnessHelper(t, "TestCheckBytesAtMismatchFails",
		"GOLDEN_MISMATCH_HELPER=1", "GOLDEN_MISMATCH_PATH="+path); err == nil {
		t.Fatal("mismatched golden content did not fail")
	}
}

func TestCheckBytesAtRefusesEmptySnapshot(t *testing.T) {
	if os.Getenv("GOLDEN_EMPTY_HELPER") == "1" {
		CheckBytesAt(t, os.Getenv("GOLDEN_EMPTY_PATH"), []byte("anything\n"))
		return
	}
	path := filepath.Join(t.TempDir(), "golden.txt")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write empty golden: %v", err)
	}
	if err := runHarnessHelper(t, "TestCheckBytesAtRefusesEmptySnapshot",
		"GOLDEN_EMPTY_HELPER=1", "GOLDEN_EMPTY_PATH="+path); err == nil {
		t.Fatal("empty golden snapshot did not fail")
	}
}

func TestCheckBytesAtUpdatesWhenRequested(t *testing.T) {
	if os.Getenv("GOLDEN_UPDATE_HELPER") == "1" {
		CheckBytesAt(t, os.Getenv("GOLDEN_UPDATE_PATH"), []byte("fresh\n"))
		return
	}
	path := filepath.Join(t.TempDir(), "nested", "golden.txt")
	if err := runHarnessHelper(t, "TestCheckBytesAtUpdatesWhenRequested",
		"GOLDEN_UPDATE_HELPER=1", "GOLDEN_UPDATE_PATH="+path, UpdateEnv+"=1"); err != nil {
		t.Fatalf("update helper failed: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("snapshot not written: %v", err)
	}
	if string(got) != "fresh\n" {
		t.Fatalf("snapshot content = %q", got)
	}
}
