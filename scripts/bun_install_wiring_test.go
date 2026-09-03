package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every bun install in this repository must go through scripts/bun-install.sh.
//
// Two defects motivated this gate and neither was visible in a green build:
//
//   - A bare `bun install --frozen-lockfile` has no retry. bun downloads
//     tarballs through the npm CDN, and a corrupt or truncated one surfaces as
//     "Integrity check failed for tarball: <pkg>". That is not a property of
//     this commit -- a rerun installs cleanly -- but it fails a required check
//     and blocks a merge. On 2026-09-03 alone it hit four different packages
//     across three jobs: recharts (a11y), @base-ui/react and
//     @oxlint/binding-linux-x64-gnu (visual-regression), and
//     lightningcss-linux-arm64-musl (docker-push, inside the image build).
//   - The Dockerfile's cache mount pointed at /root/.bun/install-cache while bun
//     reads and writes /root/.bun/install/cache, so the mount cached nothing and
//     the comment claiming otherwise was false. Every image build re-downloaded
//     every tarball, which is also what made it the step most exposed to the
//     fault above.
//
// The retry policy deliberately does NOT blanket-retry: a frozen-lockfile
// mismatch or a lifecycle-script failure must still fail on the first attempt,
// so the script's non-transient branch is asserted here too.
func TestBunInstallWiring(t *testing.T) {
	scriptsDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(scriptsDir, "..")
	read := func(rel string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(b)
	}

	t.Run("policy script keeps both branches", func(t *testing.T) {
		script := read("scripts/bun-install.sh")
		for _, want := range []string{
			"bun install --frozen-lockfile",
			"Integrity check failed",
			"Fail extracting tarball",
			// The retry must be bounded and must give up loudly.
			"MAX_ATTEMPTS",
			"still failing after",
			// Without this the retry is decorative: a corrupt tarball bun
			// already cached would be re-read identically every attempt.
			"bun pm cache rm",
			// A real defect must not be retried into obscurity.
			"non-transient error; not retrying",
		} {
			if !strings.Contains(script, want) {
				t.Errorf("scripts/bun-install.sh no longer contains %q; the retry policy has been gutted", want)
			}
		}
	})

	t.Run("no workflow step calls bun install directly", func(t *testing.T) {
		workflow := read(".github/workflows/main.yml")
		// Comments are allowed to say "bun install" (this policy is documented
		// inline); an executable run: line is not.
		bare := regexp.MustCompile(`(?m)^\s*(?:-\s*)?run:\s*(?:\|.*)?bun install`)
		for i, line := range strings.Split(workflow, "\n") {
			if bare.MatchString(line) {
				t.Errorf(".github/workflows/main.yml:%d calls bun install without the retry policy: %s", i+1, strings.TrimSpace(line))
			}
		}
		got := strings.Count(workflow, "sh ../scripts/bun-install.sh")
		const want = 5 // frontend, a11y, release, ui-screenshots, visual-regression
		if got != want {
			t.Errorf(".github/workflows/main.yml routes %d web-deps installs through scripts/bun-install.sh, want %d; a new job that installs web deps must use it too", got, want)
		}
	})

	t.Run("Dockerfile uses the shared policy and the real cache path", func(t *testing.T) {
		dockerfile := read("Dockerfile")
		if !strings.Contains(dockerfile, "sh /tmp/bun-install.sh") {
			t.Error("Dockerfile no longer runs scripts/bun-install.sh; the image build has lost the retry policy and drifted from CI")
		}
		if !strings.Contains(dockerfile, "target=/root/.bun/install/cache") {
			t.Error("Dockerfile cache mount is not bun's real install cache (/root/.bun/install/cache, per `bun pm cache`); the mount would cache nothing")
		}
		for i, line := range strings.Split(dockerfile, "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				continue // the wrong path is named in a comment on purpose
			}
			if strings.Contains(trimmed, "install-cache") {
				t.Errorf("Dockerfile:%d uses the non-existent cache path /root/.bun/install-cache (bun uses install/cache): %s", i+1, trimmed)
			}
			if strings.HasPrefix(trimmed, "bun install") || strings.Contains(trimmed, "&& bun install") {
				t.Errorf("Dockerfile:%d calls bun install directly instead of the shared retry script: %s", i+1, trimmed)
			}
		}
	})
}
