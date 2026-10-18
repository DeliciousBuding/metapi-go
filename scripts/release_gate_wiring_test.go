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
	// The publication path is prepare → native per-architecture builds →
	// manifest merge → anonymous pull smoke → draft Release. Every edge is
	// required; a skipped matrix leg must not let a release through.
	needsInline := regexp.MustCompile(`(?m)^    needs: \[([^\]]*)\]`)
	hasNeed := func(job, dep string) bool {
		match := needsInline.FindStringSubmatch(job)
		if match == nil {
			return false
		}
		for _, item := range strings.Split(match[1], ",") {
			if strings.TrimSpace(item) == dep {
				return true
			}
		}
		return false
	}
	for _, edge := range []struct{ job, dep string }{
		{"docker-prepare", "docker-build"},
		{"docker-push", "docker-prepare"},
		{"docker-manifest", "docker-prepare"},
		{"docker-manifest", "docker-push"},
		{"image-verify", "docker-prepare"},
		{"image-verify", "docker-manifest"},
		{"release", "image-verify"},
	} {
		job, ok := jobs[edge.job]
		if !ok {
			t.Errorf("publication job %q is missing", edge.job)
			continue
		}
		if !hasNeed(job, edge.dep) {
			t.Errorf("%s does not require %s", edge.job, edge.dep)
		}
	}

	push := jobs["docker-push"]
	for _, want := range []string{
		"runner: ubuntu-latest",
		"runner: ubuntu-24.04-arm",
		"platform: linux/amd64",
		"platform: linux/arm64",
		"fail-fast: false",
	} {
		if !strings.Contains(push, want) {
			t.Errorf("docker-push is missing %q; native per-architecture builds are required", want)
		}
	}
	if !strings.Contains(jobs["docker-manifest"], "docker buildx imagetools create") {
		t.Error("docker-manifest must merge the per-architecture images with imagetools create")
	}
	if !strings.Contains(jobs["docker-prepare"], "published version tags are immutable") {
		t.Error("docker-prepare must refuse to overwrite an existing release tag")
	}
	for _, want := range []string{"docker logout ghcr.io", "docker pull", "/api/about"} {
		if !strings.Contains(jobs["image-verify"], want) {
			t.Errorf("image-verify is missing %q; the published image must be pulled anonymously and booted", want)
		}
	}
	if got := strings.Count(workflow, "sigstore/cosign-installer@"); got < 2 {
		t.Errorf("workflow pins cosign-installer %d time(s), want at least 2 (per-arch images and final manifest)", got)
	}
	if !strings.Contains(workflow, "cancel-in-progress: ${{ !startsWith(github.ref, 'refs/tags/') }}") {
		t.Error("tag publications must not be cancelled by a concurrent run")
	}
	// An index-only Accept returns 404 for a tag that exists as a single image
	// manifest, which would let the immutable check overwrite it.
	if !strings.Contains(jobs["docker-prepare"], "application/vnd.oci.image.manifest.v1+json") {
		t.Error("the immutable tag check must accept single OCI image manifests")
	}
	for _, want := range []string{"image-digest-", "${RUNNER_TEMP}/${ARCH}.txt", "tags: ${{ env.IMAGE_NAME }}:build-${{ matrix.arch }}"} {
		if !strings.Contains(jobs["docker-push"], want) {
			t.Errorf("docker-push is missing %q; architecture digests must be handed to the manifest job", want)
		}
	}
	for _, want := range []string{"pattern: image-digest-*", "${IMAGE_NAME}@${amd64}", "${IMAGE_NAME}@${arm64}", "unknown/unknown"} {
		if !strings.Contains(jobs["docker-manifest"], want) {
			t.Errorf("docker-manifest is missing %q; it must merge this run's digests and keep attestations", want)
		}
	}
	if !strings.Contains(jobs["image-verify"], "EXPECTED_DIGEST") || !strings.Contains(jobs["image-verify"], "resolved") {
		t.Error("image-verify must compare the pulled tag against the manifest digest")
	}
	dockerfile, err := os.ReadFile(filepath.Join("..", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"COPY LICENSE THIRD_PARTY_NOTICES.md /licenses/", "COPY licenses/ /licenses/licenses/"} {
		if !strings.Contains(string(dockerfile), path) {
			t.Errorf("Dockerfile must ship AGPL and third-party license files: missing %q", path)
		}
	}
}

// Third-party actions run with the workflow's token, including the publish
// job's packages:write and id-token:write. A floating major tag is mutable by
// its owner; pin the commit and let Dependabot move the pin.
func TestGitHubActionsArePinned(t *testing.T) {
	files := []string{
		".github/workflows/main.yml",
		".github/actions/go-toolchain/action.yml",
	}
	useRE := regexp.MustCompile(`(?m)^\s*-?\s*uses:\s+([^\s#]+)`)
	pinnedRE := regexp.MustCompile(`^[^@]+@[0-9a-f]{40}$`)
	checked := 0
	for _, rel := range files {
		data, err := os.ReadFile(filepath.Join("..", rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		for _, match := range useRE.FindAllStringSubmatch(string(data), -1) {
			ref := match[1]
			if strings.HasPrefix(ref, "./") {
				continue
			}
			checked++
			if !pinnedRE.MatchString(ref) {
				t.Errorf("%s uses %q; pin the commit SHA instead of a mutable tag", rel, ref)
			}
		}
	}
	if checked == 0 {
		t.Fatal("action pin scan found no external actions; a vacuous pass is not allowed")
	}
}
