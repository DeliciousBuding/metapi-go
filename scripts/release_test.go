package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseScriptRejectsStaleMaster(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is required")
	}

	scriptsDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	releaseScript, err := os.ReadFile(filepath.Join(scriptsDir, "release.sh"))
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	seed := filepath.Join(root, "seed")
	stale := filepath.Join(root, "stale")
	writer := filepath.Join(root, "writer")

	runGit(t, root, "init", "--bare", origin)
	runGit(t, root, "init", "-b", "master", seed)
	configureTestRepo(t, seed)
	mustMkdir(t, filepath.Join(seed, "scripts"))
	mustMkdir(t, filepath.Join(seed, "web"))
	mustWrite(t, filepath.Join(seed, "scripts", "release.sh"), releaseScript)
	mustWrite(t, filepath.Join(seed, "CHANGELOG.md"), []byte("# Changelog\n\n## [v9.9.9]\n\n- Test release.\n"))
	mustWrite(t, filepath.Join(seed, "web", "package.json"), []byte("{\"version\":\"9.9.9\"}\n"))
	runGit(t, seed, "add", ".")
	runGit(t, seed, "commit", "-m", "initial")
	runGit(t, seed, "remote", "add", "origin", origin)
	runGit(t, seed, "push", "-u", "origin", "master")

	runGit(t, root, "clone", origin, stale)
	configureTestRepo(t, stale)
	runGit(t, root, "clone", origin, writer)
	configureTestRepo(t, writer)
	mustWrite(t, filepath.Join(writer, "newer.txt"), []byte("newer remote commit\n"))
	runGit(t, writer, "add", "newer.txt")
	runGit(t, writer, "commit", "-m", "advance remote")
	runGit(t, writer, "push", "origin", "master")

	cmd := exec.Command("bash", "scripts/release.sh", "9.9.9", "--dry-run")
	cmd.Dir = stale
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("release helper accepted stale master:\n%s", output)
	}
	if !strings.Contains(string(output), "not exactly synchronized with origin/master") {
		t.Fatalf("release helper failed for the wrong reason:\n%s", output)
	}
}

func TestProjectPrePushGateMatchesHookKitAndFreshCloneOrder(t *testing.T) {
	root := filepath.Dir(mustWorkingDir(t))
	wrapper, err := os.ReadFile(filepath.Join(root, ".githooks", "pre-push"))
	if err != nil {
		t.Fatal(err)
	}
	gate, err := os.ReadFile(filepath.Join(root, ".githooks", "pre-push-project"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wrapper), "pre-push-project") {
		t.Fatal("standalone pre-push wrapper does not delegate to pre-push-project")
	}
	gateText := string(gate)
	frontendBuild := strings.Index(gateText, "bun run build:check")
	goBuild := strings.Index(gateText, "go build ./cmd/server")
	if frontendBuild < 0 || goBuild < 0 || frontendBuild > goBuild {
		t.Fatal("project gate must build web/dist before compiling the embedded Go server")
	}
	if strings.Contains(gateText, "bun not found — skipping") {
		t.Fatal("project gate must fail closed when Bun is unavailable")
	}
}

func mustWorkingDir(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func configureTestRepo(t *testing.T, repo string) {
	t.Helper()
	runGit(t, repo, "config", "user.name", "Release Test")
	runGit(t, repo, "config", "user.email", "release-test@example.invalid")
	runGit(t, repo, "config", "core.hooksPath", ".disabled-hooks")
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
