package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Run the real entrypoint with only the expensive Go subprocess replaced. This
// checks argument/environment propagation and failure handling without nesting
// another full race gate inside go test ./....
func runRaceScript(t *testing.T, overrides map[string]string, args ...string) (string, error) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is required")
	}
	stub := filepath.Join(t.TempDir(), "go-stub.sh")
	mustWrite(t, stub, []byte(`go() {
  printf 'STUB_GO GOMAXPROCS=%s' "$GOMAXPROCS"
  printf ' <%s>' "$@"
  printf '\n'
  if [[ "${STUB_TIMEOUT:-}" == 1 ]]; then echo 'panic: test timed out'; fi
  return "${STUB_EXIT:-0}"
}
`))
	cmd := exec.Command("bash", append([]string{"go-race.sh"}, args...)...)
	cmd.Env = make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(name) {
		case "GOMAXPROCS", "METAPI_RACE_PACKAGES", "METAPI_RACE_PARALLEL", "METAPI_RACE_TIMEOUT_SECONDS", "PG_TEST_DSN", "BASH_ENV", "STUB_EXIT", "STUB_TIMEOUT":
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	cmd.Env = append(cmd.Env, "BASH_ENV="+filepath.ToSlash(stub))
	for name, value := range overrides {
		cmd.Env = append(cmd.Env, name+"="+value)
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func TestRaceScriptRunsFullSuiteWithBoundedDefaults(t *testing.T) {
	output, err := runRaceScript(t, nil)
	if err != nil {
		t.Fatalf("race script: %v\n%s", err, output)
	}
	const want = "STUB_GO GOMAXPROCS=2 <test> <./...> <-count=1> <-race> <-p> <2> <-parallel> <2> <-timeout> <900s>"
	if !strings.Contains(output, want) {
		t.Fatalf("full gate arguments changed:\n%s", output)
	}
}

func TestRaceScriptHonorsExplicitBudget(t *testing.T) {
	output, err := runRaceScript(t, map[string]string{
		"GOMAXPROCS": "4", "METAPI_RACE_PACKAGES": "3",
		"METAPI_RACE_PARALLEL": "5", "METAPI_RACE_TIMEOUT_SECONDS": "1200",
	})
	if err != nil {
		t.Fatalf("race script: %v\n%s", err, output)
	}
	if !strings.Contains(output, "GOMAXPROCS=4 <test> <./...> <-count=1> <-race> <-p> <3> <-parallel> <5> <-timeout> <1200s>") {
		t.Fatalf("explicit budget not propagated:\n%s", output)
	}
}

func TestRaceScriptSerializesSharedPostgresFixtures(t *testing.T) {
	output, err := runRaceScript(t, map[string]string{"PG_TEST_DSN": "synthetic-dsn"})
	if err != nil || !strings.Contains(output, "<-p> <1>") {
		t.Fatalf("shared PostgreSQL fixtures not serialized: %v\n%s", err, output)
	}
	output, err = runRaceScript(t, map[string]string{"PG_TEST_DSN": "synthetic-dsn", "METAPI_RACE_PACKAGES": "2"})
	if err == nil || !strings.Contains(output, "shared PG_TEST_DSN fixtures require") || strings.Contains(output, "STUB_GO") {
		t.Fatalf("unsafe shared PostgreSQL parallelism accepted: %v\n%s", err, output)
	}
}

func TestRaceScriptRejectsInvalidBudgetsBeforeRunningGo(t *testing.T) {
	for _, name := range []string{"GOMAXPROCS", "METAPI_RACE_PACKAGES", "METAPI_RACE_PARALLEL", "METAPI_RACE_TIMEOUT_SECONDS"} {
		for _, value := range []string{"0", "-1", "abc", "1.5", "08"} {
			t.Run(name+"="+value, func(t *testing.T) {
				output, err := runRaceScript(t, map[string]string{name: value})
				if err == nil || !strings.Contains(output, "positive integer") || strings.Contains(output, "STUB_GO") {
					t.Fatalf("invalid budget was not rejected: %v\n%s", err, output)
				}
			})
		}
	}
}

func TestRaceScriptPropagatesFailuresAndTimeoutDiagnostic(t *testing.T) {
	for _, timeout := range []string{"0", "1"} {
		t.Run("timeout="+timeout, func(t *testing.T) {
			output, err := runRaceScript(t, map[string]string{"STUB_EXIT": "1", "STUB_TIMEOUT": timeout})
			if err == nil || !strings.Contains(output, "STUB_GO") {
				t.Fatalf("Go failure was swallowed: %v\n%s", err, output)
			}
			if strings.Contains(output, "raise it with METAPI_RACE_TIMEOUT_SECONDS") != (timeout == "1") {
				t.Fatalf("incorrect timeout diagnostic:\n%s", output)
			}
		})
	}
}

func TestRaceScriptDoesNotAcceptPartialGateArguments(t *testing.T) {
	output, err := runRaceScript(t, nil, "-run", "TestOne")
	if err == nil || !strings.Contains(output, "unexpected arguments") || strings.Contains(output, "STUB_GO") {
		t.Fatalf("partial gate arguments accepted: %v\n%s", err, output)
	}
	output, err = runRaceScript(t, nil, "--help")
	if err != nil || !strings.Contains(output, "Usage:") || strings.Contains(output, "STUB_GO") {
		t.Fatalf("help should not run Go: %v\n%s", err, output)
	}
}
