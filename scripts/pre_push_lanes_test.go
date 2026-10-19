package scripts

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// gateStub replaces bun and go with traced fakes. The hook under test is the
// real entrypoint; only the expensive tool processes are substituted. The
// guard directory keeps a real tool from running if BASH_ENV is ever ignored.
const gateStub = `
trace="${LANE_TRACE:?}"
now() {
  if [ -n "${EPOCHREALTIME:-}" ]; then
    printf '%s\n' "$EPOCHREALTIME"
  else
    date +%s
  fi
}
record() { printf '%s %s\n' "$(now)" "$*" >>"$trace"; }

# Record the race entrypoint without replacing the real bash process.
bash() {
  if [ "${1:-}" = "./scripts/go-race.sh" ]; then
    record "bash go-race.sh"
  fi
  command bash "$@"
}

stub_fail() {
  case "${STUB_FAIL:-}" in
    "$1") echo "stub: $1 failed"; return 7 ;;
  esac
  return 0
}

bun() {
  local verb="${1:-}" sub="${2:-}"
  record "bun-start $verb $sub"
  local rc=0
  case "$verb $sub" in
    "run build:web")
      sleep "${STUB_BUILD_SLEEP:-0}"
      stub_fail bun-build || rc=$?
      if [ "$rc" -eq 0 ]; then : >"${LANE_DIST_MARKER:?}"; fi
      ;;
    "run check")
      if [ -n "${LANE_CHECK_PID_FILE:-}" ]; then printf '%s\n' "$BASHPID" >"$LANE_CHECK_PID_FILE"; fi
      if [ -n "${LANE_BODY_PID_FILE:-}" ]; then printf '%s\n' "$BASHPID" >"$LANE_BODY_PID_FILE"; fi
      if [ -n "${STUB_KILL_BODY:-}" ]; then kill -KILL "$BASHPID"; sleep 30; fi
      if [ -n "${STUB_SPAWN_GRANDCHILD:-}" ]; then
        sleep 300 &
        printf '%s\n' "$!" >"${LANE_GRANDCHILD_PID_FILE:?}"
      fi
      sleep "${STUB_CHECK_SLEEP:-0}"
      stub_fail bun-check || rc=$?
      ;;
    *)
      echo "stub: unexpected bun invocation: $*" >&2
      rc=99
      ;;
  esac
  record "bun-end $verb $sub exit=$rc"
  return "$rc"
}

go() {
  local verb="${1:-}"
  shift || true
  record "go-start $verb $*"
  local rc=0
  case "$verb" in
    build)
      if [ ! -f "${LANE_DIST_MARKER:?}" ]; then
        echo "stub: go build started before web/dist was generated" >&2
        rc=90
      else
        sleep "${STUB_GO_BUILD_SLEEP:-0}"
        stub_fail go-build || rc=$?
      fi
      ;;
    vet)
      sleep "${STUB_GO_VET_SLEEP:-0}"
      stub_fail go-vet || rc=$?
      ;;
    test)
      sleep "${STUB_GO_TEST_SLEEP:-0}"
      stub_fail go-test || rc=$?
      ;;
    *)
      echo "stub: unexpected go invocation: $*" >&2
      rc=99
      ;;
  esac
  record "go-end $verb $* exit=$rc"
  return "$rc"
}
`

type laneEvent struct {
	at   float64
	text string
}

func projectGateCommand(t *testing.T, overrides map[string]string, stdin string) (*exec.Cmd, string) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is required")
	}

	root := filepath.Dir(mustWorkingDir(t))
	tmp := t.TempDir()
	stub := filepath.Join(tmp, "gate-stub.sh")
	mustWrite(t, stub, []byte(gateStub))
	guard := filepath.Join(tmp, "guard")
	if err := os.Mkdir(guard, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"bun", "go"} {
		path := filepath.Join(guard, tool)
		mustWrite(t, path, []byte("#!/bin/sh\necho 'unstubbed "+tool+" invoked' >&2\nexit 99\n"))
		if err := os.Chmod(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	cmd := exec.Command("bash", ".githooks/pre-push-project")
	cmd.Dir = root
	cmd.Stdin = strings.NewReader(stdin)
	cmd.Env = make([]string, 0, len(os.Environ())+8)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		switch strings.ToUpper(name) {
		case "GOMAXPROCS", "GOFLAGS", "BASH_ENV", "PG_TEST_DSN",
			"METAPI_PREPUSH_LANES", "METAPI_RACE_PACKAGES", "METAPI_RACE_PARALLEL",
			"METAPI_RACE_TIMEOUT_SECONDS", "LANE_TRACE", "LANE_DIST_MARKER",
			"LANE_CHECK_PID_FILE", "LANE_BODY_PID_FILE", "LANE_GRANDCHILD_PID_FILE",
			"STUB_FAIL", "STUB_KILL_BODY", "STUB_SPAWN_GRANDCHILD", "STUB_BUILD_SLEEP",
			"STUB_CHECK_SLEEP", "STUB_GO_BUILD_SLEEP", "STUB_GO_VET_SLEEP",
			"STUB_GO_TEST_SLEEP":
			continue
		}
		cmd.Env = append(cmd.Env, entry)
	}
	extra := map[string]string{
		"BASH_ENV":         filepath.ToSlash(stub),
		"PATH":             guard + string(os.PathListSeparator) + os.Getenv("PATH"),
		"LANE_TRACE":       filepath.ToSlash(filepath.Join(tmp, "trace.log")),
		"LANE_DIST_MARKER": filepath.ToSlash(filepath.Join(tmp, "dist.marker")),
		"TMPDIR":           filepath.ToSlash(tmp),
	}
	for name, value := range overrides {
		extra[name] = value
	}
	names := make([]string, 0, len(extra))
	for name := range extra {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		cmd.Env = append(cmd.Env, name+"="+extra[name])
	}
	return cmd, tmp
}

func runProjectGate(t *testing.T, overrides map[string]string, stdin string) (string, error) {
	t.Helper()
	cmd, _ := projectGateCommand(t, overrides, stdin)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func readLaneTrace(t *testing.T, dir string) []laneEvent {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "trace.log"))
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	var events []laneEvent
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		atText, text, ok := strings.Cut(line, " ")
		if !ok {
			t.Fatalf("malformed trace line %q", line)
		}
		at, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(atText), ",", "."), 64)
		if err != nil {
			t.Fatalf("malformed trace timestamp %q: %v", atText, err)
		}
		events = append(events, laneEvent{at: at, text: text})
	}
	return events
}

func eventAt(t *testing.T, events []laneEvent, prefix string) laneEvent {
	t.Helper()
	for _, event := range events {
		if strings.HasPrefix(event.text, prefix) {
			return event
		}
	}
	t.Fatalf("missing trace event %q in %v", prefix, events)
	return laneEvent{}
}

func countEvents(events []laneEvent, prefix string) int {
	count := 0
	for _, event := range events {
		if strings.HasPrefix(event.text, prefix) {
			count++
		}
	}
	return count
}

func TestPrePushBuildsFrontendBeforeLanes(t *testing.T) {
	tmp := t.TempDir()
	output, err := runProjectGate(t, map[string]string{
		"METAPI_PREPUSH_LANES": "2",
		"LANE_TRACE":           filepath.ToSlash(filepath.Join(tmp, "trace.log")),
	}, "")
	if err != nil {
		t.Fatalf("gate: %v\n%s", err, output)
	}
	events := readLaneTrace(t, tmp)
	buildEnd := eventAt(t, events, "bun-end run build:web")
	goBuildStart := eventAt(t, events, "go-start build")
	if goBuildStart.at < buildEnd.at {
		t.Fatalf("go build started before the frontend build finished:\n%s", output)
	}
	if !strings.Contains(output, "frontend build passed") {
		t.Fatalf("frontend build result missing:\n%s", output)
	}
}

func TestPrePushRunsLanesConcurrentlyAndCanBeSerialized(t *testing.T) {
	slow := map[string]string{
		"STUB_CHECK_SLEEP":    "2",
		"STUB_GO_BUILD_SLEEP": "2",
		"STUB_GO_VET_SLEEP":   "2",
		"STUB_GO_TEST_SLEEP":  "2",
	}
	parallelDir := t.TempDir()
	parallelEnv := map[string]string{"METAPI_PREPUSH_LANES": "2", "LANE_TRACE": filepath.ToSlash(filepath.Join(parallelDir, "trace.log"))}
	for name, value := range slow {
		parallelEnv[name] = value
	}
	output, err := runProjectGate(t, parallelEnv, "")
	if err != nil {
		t.Fatalf("parallel gate: %v\n%s", err, output)
	}
	events := readLaneTrace(t, parallelDir)
	checkStart := eventAt(t, events, "bun-start run check")
	checkEnd := eventAt(t, events, "bun-end run check")
	goStart := eventAt(t, events, "go-start build")
	if goStart.at >= checkEnd.at || checkStart.at >= eventAt(t, events, "go-end test").at {
		t.Fatalf("frontend check and Go lane did not overlap:\n%s", output)
	}

	serialDir := t.TempDir()
	serialEnv := map[string]string{"METAPI_PREPUSH_LANES": "1", "LANE_TRACE": filepath.ToSlash(filepath.Join(serialDir, "trace.log"))}
	for name, value := range slow {
		serialEnv[name] = value
	}
	output, err = runProjectGate(t, serialEnv, "")
	if err != nil {
		t.Fatalf("serial gate: %v\n%s", err, output)
	}
	events = readLaneTrace(t, serialDir)
	checkEnd = eventAt(t, events, "bun-end run check")
	goStart = eventAt(t, events, "go-start build")
	if goStart.at < checkEnd.at {
		t.Fatalf("serial mode overlapped lanes:\n%s", output)
	}
}

func TestPrePushPropagatesEachLaneFailure(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		lane string
		stub string
	}{
		{"frontend build", map[string]string{"STUB_FAIL": "bun-build"}, "frontend build", "stub: bun-build failed"},
		{"frontend check", map[string]string{"STUB_FAIL": "bun-check", "STUB_GO_TEST_SLEEP": "5"}, "frontend-check", "stub: bun-check failed"},
		{"go build", map[string]string{"STUB_FAIL": "go-build", "STUB_CHECK_SLEEP": "5"}, "go", "stub: go-build failed"},
		{"go vet", map[string]string{"STUB_FAIL": "go-vet", "STUB_CHECK_SLEEP": "5"}, "go", "stub: go-vet failed"},
		{"race", map[string]string{"STUB_FAIL": "go-test", "STUB_CHECK_SLEEP": "5"}, "go", "stub: go-test failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"METAPI_PREPUSH_LANES": "2"}
			for name, value := range tc.env {
				env[name] = value
			}
			output, err := runProjectGate(t, env, "")
			if err == nil {
				t.Fatalf("lane failure was swallowed:\n%s", output)
			}
			if !strings.Contains(output, tc.lane) || !strings.Contains(output, tc.stub) {
				t.Fatalf("failure did not identify lane %q and its log:\n%s", tc.lane, output)
			}
		})
	}
}

func TestPrePushRunsOneFullRaceInvocation(t *testing.T) {
	tmp := t.TempDir()
	output, err := runProjectGate(t, map[string]string{
		"METAPI_PREPUSH_LANES": "2",
		"LANE_TRACE":           filepath.ToSlash(filepath.Join(tmp, "trace.log")),
	}, "")
	if err != nil {
		t.Fatalf("gate: %v\n%s", err, output)
	}
	events := readLaneTrace(t, tmp)
	if got := countEvents(events, "go-start test ./... -count=1 -race"); got != 1 {
		t.Fatalf("full race invoked %d times, want exactly one:\n%v", got, events)
	}
	if got := countEvents(events, "go-start build ./cmd/server"); got != 1 {
		t.Fatalf("go build invoked %d times, want exactly one:\n%v", got, events)
	}
	if got := countEvents(events, "go-start vet ./..."); got != 1 {
		t.Fatalf("go vet invoked %d times, want exactly one:\n%v", got, events)
	}
	race := eventAt(t, events, "go-start test ./...")
	for _, banned := range []string{"-run ", "-skip ", "-short"} {
		if strings.Contains(race.text, banned) {
			t.Fatalf("race invocation narrowed by %q: %s", banned, race.text)
		}
	}
	if got := countEvents(events, "bash go-race.sh"); got != 1 {
		t.Fatalf("go-race.sh entrypoint invoked %d times, want exactly one:\n%v", got, events)
	}
	if got := countEvents(events, "bun-start run build:web"); got != 1 {
		t.Fatalf("frontend build invoked %d times, want exactly one:\n%v", got, events)
	}
	if got := countEvents(events, "bun-start run check"); got != 1 {
		t.Fatalf("frontend check invoked %d times, want exactly one:\n%v", got, events)
	}
	buildEnd := eventAt(t, events, "bun-end run build:web")
	checkStart := eventAt(t, events, "bun-start run check")
	if checkStart.at < buildEnd.at {
		t.Fatalf("frontend check started before the intentional web/dist build completed:\n%v", events)
	}
	buildEnd = eventAt(t, events, "go-end build")
	vetStart := eventAt(t, events, "go-start vet")
	vetEnd := eventAt(t, events, "go-end vet")
	raceStart := eventAt(t, events, "go-start test ./...")
	if vetStart.at < buildEnd.at || raceStart.at < vetEnd.at {
		t.Fatalf("Go lane order must be build -> vet -> race:\n%v", events)
	}
}

func TestPrePushKeepsSharedPostgresSerialAndOneLane(t *testing.T) {
	tmp := t.TempDir()
	output, err := runProjectGate(t, map[string]string{
		"PG_TEST_DSN": "synthetic-dsn",
		"LANE_TRACE":  filepath.ToSlash(filepath.Join(tmp, "trace.log")),
	}, "")
	if err != nil {
		t.Fatalf("gate: %v\n%s", err, output)
	}
	if !strings.Contains(output, "lanes=1") {
		t.Fatalf("PG_TEST_DSN should select one lane:\n%s", output)
	}
	events := readLaneTrace(t, tmp)
	race := eventAt(t, events, "go-start test ./...")
	if !strings.Contains(race.text, "-p 1 ") {
		t.Fatalf("shared PostgreSQL fixtures must stay serial: %s", race.text)
	}
	checkEnd := eventAt(t, events, "bun-end run check")
	goStart := eventAt(t, events, "go-start build")
	if goStart.at < checkEnd.at {
		t.Fatalf("PG_TEST_DSN should serialize the lanes:\n%s", output)
	}
}

func TestPrePushRejectsInvalidLaneCount(t *testing.T) {
	for _, value := range []string{"0", "3", "abc"} {
		t.Run(value, func(t *testing.T) {
			tmp := t.TempDir()
			output, err := runProjectGate(t, map[string]string{
				"METAPI_PREPUSH_LANES": value,
				"LANE_TRACE":           filepath.ToSlash(filepath.Join(tmp, "trace.log")),
			}, "")
			if err == nil || !strings.Contains(output, "METAPI_PREPUSH_LANES") {
				t.Fatalf("invalid lane count accepted: %v\n%s", err, output)
			}
			if _, statErr := os.Stat(filepath.Join(tmp, "trace.log")); statErr == nil {
				t.Fatalf("tools ran before lane validation failed:\n%s", output)
			}
		})
	}
}

func TestPrePushCancelsSiblingLaneOnFailure(t *testing.T) {
	tmp := t.TempDir()
	pidFile := filepath.Join(tmp, "check.pid")
	grandchildFile := filepath.Join(tmp, "grandchild.pid")
	start := time.Now()
	output, err := runProjectGate(t, map[string]string{
		"METAPI_PREPUSH_LANES":     "2",
		"STUB_FAIL":                "go-build",
		"STUB_CHECK_SLEEP":         "30",
		"STUB_SPAWN_GRANDCHILD":    "1",
		"LANE_CHECK_PID_FILE":      filepath.ToSlash(pidFile),
		"LANE_GRANDCHILD_PID_FILE": filepath.ToSlash(grandchildFile),
	}, "")
	if err == nil {
		t.Fatalf("failure was swallowed:\n%s", output)
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Fatalf("sibling lane was not cancelled promptly (took %s):\n%s", elapsed, output)
	}
	if !strings.Contains(output, "cancelled") {
		t.Fatalf("cancellation not reported:\n%s", output)
	}
	assertProcessGone(t, pidFile, "check lane", output)
	assertProcessGone(t, grandchildFile, "check lane grandchild", output)
}

func TestPrePushFailsWhenLaneDiesBeforeStatus(t *testing.T) {
	tmp := t.TempDir()
	bodyFile := filepath.Join(tmp, "body.pid")
	var output bytes.Buffer
	cmd, gateTmp := projectGateCommand(t, map[string]string{
		"METAPI_PREPUSH_LANES": "2",
		"STUB_CHECK_SLEEP":     "30",
		"LANE_BODY_PID_FILE":   filepath.ToSlash(bodyFile),
	}, "")
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	supervisorPid := waitForLanePid(t, gateTmp, "frontend-check", &output)
	waitForFile(t, bodyFile)
	if out, err := exec.Command("bash", "-c", "kill -KILL "+supervisorPid).CombinedOutput(); err != nil {
		t.Fatalf("kill lane supervisor: %v\n%s", err, out)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatalf("lane that died without a status file was treated as success:\n%s", output.String())
		}
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("gate hung after a lane was SIGKILLed:\n%s", output.String())
	}
	text := output.String()
	if !strings.Contains(text, "without a status file") {
		t.Fatalf("missing-status failure not reported:\n%s", text)
	}
	assertProcessGone(t, bodyFile, "orphaned lane body", text)
}

func waitForLanePid(t *testing.T, tmp, lane string, gateOutput *bytes.Buffer) string {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		matches, err := filepath.Glob(filepath.Join(tmp, "metapi-pre-push.*", lane+".log"))
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) > 0 {
			data, readErr := os.ReadFile(matches[0])
			if readErr == nil {
				if pid := lanePidFromLog(string(data)); pid != "" {
					return pid
				}
			}
		}
		if time.Now().After(deadline) {
			entries, _ := os.ReadDir(tmp)
			names := make([]string, 0, len(entries))
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			t.Fatalf("lane %s did not publish its supervisor pid; tmp entries=%v\ngate output:\n%s", lane, names, gateOutput.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func lanePidFromLog(text string) string {
	const marker = "(pid "
	start := strings.Index(text, marker)
	if start < 0 {
		return ""
	}
	rest := text[start+len(marker):]
	end := strings.Index(rest, ")")
	if end <= 0 {
		return ""
	}
	return rest[:end]
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("file %s was not created", path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestPrePushSupervisorReportsBodyKilledWithoutStatus(t *testing.T) {
	tmp := t.TempDir()
	start := time.Now()
	output, err := runProjectGate(t, map[string]string{
		"METAPI_PREPUSH_LANES": "2",
		"STUB_KILL_BODY":       "1",
		"STUB_CHECK_SLEEP":     "30",
		"LANE_TRACE":           filepath.ToSlash(filepath.Join(tmp, "trace.log")),
	}, "")
	if err == nil {
		t.Fatalf("SIGKILLed lane body was treated as success:\n%s", output)
	}
	if elapsed := time.Since(start); elapsed > 15*time.Second {
		t.Fatalf("gate hung after the lane body was SIGKILLed (took %s):\n%s", elapsed, output)
	}
	if !strings.Contains(output, "exit 137") {
		t.Fatalf("supervisor did not report the killed body's status:\n%s", output)
	}
}

func assertProcessGone(t *testing.T, pidFile, label, output string) {
	t.Helper()
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("%s never started: %v\n%s", label, err, output)
	}
	pid := strings.TrimSpace(string(data))
	if pid == "" {
		t.Fatalf("%s wrote an empty pid\n%s", label, output)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if killErr := exec.Command("bash", "-c", "kill -0 "+pid).Run(); killErr != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s process %s survived cleanup:\n%s", label, pid, output)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestPrePushKeepsDeleteOnlyFastPath(t *testing.T) {
	tmp := t.TempDir()
	stdin := "refs/heads/x 0000000000000000000000000000000000000000 refs/heads/x 1111111111111111111111111111111111111111\n"
	output, err := runProjectGate(t, map[string]string{
		"LANE_TRACE": filepath.ToSlash(filepath.Join(tmp, "trace.log")),
	}, stdin)
	if err != nil {
		t.Fatalf("delete-only gate: %v\n%s", err, output)
	}
	if !strings.Contains(output, "only ref deletions") {
		t.Fatalf("delete-only path missing:\n%s", output)
	}
	if _, statErr := os.Stat(filepath.Join(tmp, "trace.log")); statErr == nil {
		t.Fatalf("delete-only push ran a lane:\n%s", output)
	}
}

func TestPrePushFailureKeepsExitStatus(t *testing.T) {
	output, err := runProjectGate(t, map[string]string{"STUB_FAIL": "bun-check"}, "")
	if err == nil {
		t.Fatalf("failure was swallowed:\n%s", output)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("unexpected error type %T: %v", err, err)
	}
	if got := exitErr.ExitCode(); got != 7 {
		t.Fatalf("gate exit status = %d, want lane status 7:\n%s", got, output)
	}
}
