package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docker-build is the required branch-protection check and the sole upstream
// dependency of image publishing. A sibling job that fails independently does
// not block a merge or release unless it is in this dependency chain.
func TestReleaseGateWiring(t *testing.T) {
	contents, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "main.yml"))
	if err != nil {
		t.Fatal(err)
	}
	workflow := strings.ReplaceAll(string(contents), "\r\n", "\n")
	jobHeader := regexp.MustCompile(`(?m)^  ([a-z][a-z0-9-]*):\s*$`)
	indices := jobHeader.FindAllStringSubmatchIndex(workflow, -1)
	jobs := make(map[string]string, len(indices))
	for i, match := range indices {
		end := len(workflow)
		if i+1 < len(indices) {
			end = indices[i+1][0]
		}
		jobs[workflow[match[2]:match[3]]] = workflow[match[1]:end]
	}
	if len(jobs) == 0 {
		t.Fatal("workflow scan found no jobs; a vacuous release gate must not pass")
	}

	// A comment or shell command mentioning a gate cannot make it gate.
	needsBlock := regexp.MustCompile(`(?m)^    needs:\n((?:      - [a-z][a-z0-9-]*\n)+)`)
	build, ok := jobs["docker-build"]
	if !ok {
		t.Fatal("required docker-build job is missing")
	}
	match := needsBlock.FindStringSubmatch(build)
	if match == nil {
		t.Fatal("docker-build has no nonempty multiline needs block")
	}
	for _, dep := range []string{"runtime-smoke-matrix", "visual-regression", "ui-screenshots"} {
		job, ok := jobs[dep]
		if !ok {
			t.Errorf("dependency %q has no actual job", dep)
			continue
		}
		if !strings.Contains(match[1], "      - "+dep+"\n") {
			t.Errorf("docker-build does not require %s; its failure would not block a merge or release", dep)
		}
		if regexp.MustCompile(`(?m)^    (?:if|continue-on-error):`).MatchString(job) {
			t.Errorf("%s has a job-level condition or allowed failure; it must gate every workflow trigger", dep)
		}
	}
	if !strings.Contains(jobs["docker-push"], "needs: [docker-build]") {
		t.Error("docker-push does not require docker-build")
	}
	if !strings.Contains(jobs["release"], "needs: [docker-push]") {
		t.Error("release does not require docker-push")
	}
}
