package scripts

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docker-build is the required branch-protection check and the sole upstream
// dependency of image publishing. An optional visual QA tool is not a release
// prerequisite; required runtime smoke still belongs in the dependency chain.
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
	if _, ok := jobs["web-dist"]; !ok {
		t.Fatal("web-dist producer is missing")
	}
	if !strings.Contains(jobs["web-dist"], "name: web-dist") || !strings.Contains(jobs["web-dist"], "bun run build:web") {
		t.Error("web-dist must build and publish the real SPA bundle")
	}
	for _, consumer := range []string{"a11y", "test-sqlite-shard", "test-pg"} {
		if !strings.Contains(jobs[consumer], "    needs: web-dist\n") || !strings.Contains(jobs[consumer], "name: web-dist") {
			t.Errorf("%s must consume the built bundle without waiting for frontend checks", consumer)
		}
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
	// Required checks reported as skipped can pass branch protection. The build
	// must run on failed/skipped needs and turn those results into a real failure
	// before checkout or image build. A dependency edge alone is not a gate.
	if !strings.Contains(build, "    if: ${{ always() }}\n") {
		t.Error("docker-build must run even when a prerequisite failed or skipped")
	}
	guard := "NEEDS_RESULTS: ${{ toJSON(needs.*.result) }}"
	assertion := `jq -e 'length > 0 and all(.[]; . == "success")'`
	guardAt := strings.Index(build, guard)
	assertionAt := strings.Index(build, assertion)
	checkoutAt := strings.Index(build, "uses: actions/checkout@")
	if guardAt < 0 || assertionAt < 0 || checkoutAt < 0 || guardAt >= checkoutAt || assertionAt >= checkoutAt {
		t.Error("docker-build must fail closed on every need result before checkout/build")
	}
	for _, dep := range []string{"runtime-smoke-matrix", "frontend", "web-dist"} {
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
