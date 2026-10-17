package scripts

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakeVerifyUsesFullGateBeforeReleaseChecks(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skip("make is required")
	}
	for _, target := range []string{"verify", "verify-race"} {
		t.Run(target, func(t *testing.T) {
			cmd := exec.Command("make", "--no-print-directory", "-n", "-j4", target)
			cmd.Dir = filepath.Dir(mustWorkingDir(t))
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("make %s: %v\n%s", target, err, output)
			}
			text := string(output)
			const gate = "bash .githooks/pre-push-project </dev/null"
			if strings.Count(text, gate) != 1 {
				t.Fatalf("expected one full local gate:\n%s", text)
			}
			for _, check := range []string{"go mod verify", "golangci-lint run", "golang.org/x/vuln", "go build -trimpath"} {
				if strings.Index(text, check) <= strings.Index(text, gate) {
					t.Fatalf("%s must follow the full gate:\n%s", check, text)
				}
			}
			if strings.Contains(text, "go test") || strings.Contains(text, "bash ./scripts/go-race.sh") {
				t.Fatalf("make repeats tests already owned by the full gate:\n%s", text)
			}
		})
	}
}
