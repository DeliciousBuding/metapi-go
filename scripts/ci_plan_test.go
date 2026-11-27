package scripts

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type verificationPlan struct {
	Mode         string          `json:"mode"`
	Full         bool            `json:"allPackages"`
	Backend      bool            `json:"backend"`
	Frontend     bool            `json:"frontend"`
	Modules      bool            `json:"modules"`
	Packages     []string        `json:"packages"`
	LintPackages []string        `json:"lintPackages"`
	Jobs         map[string]bool `json:"jobs"`
}

func TestCIPlanRealCLI(t *testing.T) {
	root := filepath.Dir(mustWorkingDir(t))
	bin := filepath.Join(t.TempDir(), "ci-plan")
	cmd := exec.Command("go", "build", "-o", bin, "scripts/ci-plan.go")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	for _, tc := range []struct {
		name, file, content          string
		full, backend, frontend, bad bool
		want                         []string
	}{
		{"docs", "docs/note.md", "updated\n", false, false, false, false, nil},
		{"reverse test import", "a/a.go", "package a\n// changed\n", false, true, false, false, []string{"a", "b", "c"}},
		{"new Go owner", "new/new.go", "package newpkg\n", false, true, false, false, []string{"new"}},
		{"frontend", "web/src/page.tsx", "export const value = 2\n", false, true, true, false, []string{"web"}},
		{"dependency", "go.sum", "changed\n", true, true, true, false, nil},
		{"frontend version", "web/package.json", `{"version":"0.29.0"}`, true, true, true, false, nil},
		{"frontend lock", "web/bun.lock", "changed\n", true, true, true, false, nil},
		{"selector itself", "scripts/ci-plan.go", "package main\n", true, true, true, false, nil},
		{"docs gate", "docs/contracts_test.go", "package docs_test\n", true, true, true, false, nil},
		{"unknown fixture", "a/testdata/input.json", "{}\n", true, true, true, false, nil},
		{"migration", "store/migrations/new.sql", "select 1;\n", true, true, true, false, nil},
		{"parse failure", "a/a.go", "package a\nimport broken\n", false, false, false, true, nil},
		{"body syntax failure", "a/a.go", "package a\nfunc broken( {\n", false, false, false, true, nil},
		{"unsafe package path", "-run=wrong/a.go", "package a\n", false, false, false, true, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			runGit(t, repo, "init", "-b", "master")
			configureTestRepo(t, repo)
			for name, text := range map[string]string{
				"go.mod": "module github.com/deliciousbuding/metapi-go\n",
				"a/a.go": "package a\n", "b/b_test.go": "package b_test\nimport _ \"github.com/deliciousbuding/metapi-go/a\"\n",
				"c/c_windows.go": "package c\nimport _ \"github.com/deliciousbuding/metapi-go/b\"\n",
				"web/embed.go":   "package web\n", "docs/note.md": "original\n",
			} {
				mustMkdir(t, filepath.Dir(filepath.Join(repo, name)))
				mustWrite(t, filepath.Join(repo, name), []byte(text))
			}
			runGit(t, repo, "add", ".")
			runGit(t, repo, "commit", "-m", "base")
			base := runGit(t, repo, "rev-parse", "HEAD")
			mustMkdir(t, filepath.Dir(filepath.Join(repo, tc.file)))
			mustWrite(t, filepath.Join(repo, tc.file), []byte(tc.content))
			// Exercise untracked/unstaged changes first, then the identical commit diff.
			for _, working := range []bool{true, false} {
				args := []string{"-base", base}
				if working {
					args = append(args, "-worktree")
				} else {
					runGit(t, repo, "add", ".")
					runGit(t, repo, "commit", "-m", "change")
				}
				cmd := exec.Command(bin, args...)
				cmd.Dir = repo
				cmd.Env = gitTestEnv(t)
				out, err := cmd.CombinedOutput()
				if tc.bad {
					if err == nil {
						t.Fatalf("bad syntax passed: %s", out)
					}
					continue
				}
				if err != nil {
					t.Fatalf("plan: %v\n%s", err, out)
				}
				var p verificationPlan
				if err := json.Unmarshal(out, &p); err != nil {
					t.Fatal(err)
				}
				if p.Full != tc.full || p.Backend != tc.backend || p.Frontend != tc.frontend {
					t.Fatalf("wrong scope: %s", out)
				}
				if len(p.Jobs) != 14 || !p.Jobs["docs-hygiene"] || !p.Jobs["secret-scan"] {
					t.Fatalf("lost rules checks: %s", out)
				}
				for _, job := range []string{"a11y", "test-sqlite", "test-pg", "runtime-smoke-matrix", "test-e2e"} {
					if p.Jobs[job] {
						t.Fatalf("heavy job %s on fast plan: %s", job, out)
					}
				}
				modules := tc.file == "go.mod" || tc.file == "go.sum"
				if p.Jobs["vulncheck"] != modules || p.Jobs["mod-verify"] != modules {
					t.Fatalf("dependency checks: %s", out)
				}
				if len(p.LintPackages) != len(p.Packages) {
					t.Fatalf("lint scope differs from test scope: %s", out)
				}
				for i, pkg := range p.Packages {
					if p.LintPackages[i] != "./"+strings.TrimPrefix(pkg, strings.TrimSuffix(shardModule, "/")+"/") {
						t.Fatalf("lint path not local: %s", out)
					}
				}
				if tc.want != nil {
					var want []string
					for _, suffix := range tc.want {
						want = append(want, strings.TrimSuffix(shardModule, "/")+"/"+suffix)
					}
					if !slices.Equal(p.Packages, want) {
						t.Fatalf("packages %v != %v", p.Packages, want)
					}
				}
			}
			cmd = exec.Command(bin, "-base", "not-a-valid-ref")
			cmd.Dir = repo
			cmd.Env = gitTestEnv(t)
			if out, err := cmd.CombinedOutput(); err == nil {
				t.Fatalf("invalid base passed: %s", out)
			}
			cmd = exec.Command(bin, "-mode", "full")
			cmd.Dir = repo
			cmd.Env = gitTestEnv(t)
			if !tc.bad {
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("full plan: %v %s", err, out)
				}
				var p verificationPlan
				if err := json.Unmarshal(out, &p); err != nil {
					t.Fatal(err)
				}
				if !p.Full || !p.Backend || !p.Frontend || !p.Modules {
					t.Fatalf("full scope lost coverage: %s", out)
				}
			}
		})
	}

	t.Run("actual aggregate entry rejects bad and skipped required checks", func(t *testing.T) {
		for _, mode := range []string{"change", "full", "release"} {
			cmd := exec.Command(bin, "-mode", mode, "-base", "HEAD")
			cmd.Dir = root
			data, err := cmd.Output()
			if err != nil {
				t.Fatal(err)
			}
			var p verificationPlan
			if err := json.Unmarshal(data, &p); err != nil {
				t.Fatal(err)
			}
			for _, fault := range []string{"", "failed", "skipped", "cancelled", "missing", "extra", "bad-json", "bad-plan", "scope-failed"} {
				needs := map[string]map[string]string{"scope": {"result": "success"}}
				for job, required := range p.Jobs {
					result := "skipped"
					if required {
						result = "success"
					}
					needs[job] = map[string]string{"result": result}
				}
				victim := "docs-hygiene"
				if mode == "release" {
					victim = "scope"
				}
				switch fault {
				case "failed", "skipped", "cancelled":
					needs[victim]["result"] = fault
				case "missing":
					delete(needs, "lint")
				case "extra":
					needs["surprise"] = map[string]string{"result": "success"}
				case "scope-failed":
					needs["scope"]["result"] = "failure"
				}
				payload, err := json.Marshal(needs)
				if err != nil {
					t.Fatal(err)
				}
				if fault == "bad-json" {
					payload = []byte("{")
				}
				planData := data
				if fault == "bad-plan" {
					planData = []byte("{}")
				}
				cmd := exec.Command(bin, "-verify-needs")
				cmd.Dir = root
				cmd.Env = append(os.Environ(), "PLAN_JSON="+string(planData), "NEEDS_JSON="+string(payload))
				out, err := cmd.CombinedOutput()
				if (err != nil) != (fault != "") {
					t.Fatalf("mode=%s fault=%s err=%v output=%s", mode, fault, err, out)
				}
			}
		}
	})
}

func TestVerificationWorkflowScopeWiring(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "main.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{
		"mode=change", "mode=release", "go run scripts/ci-plan.go -mode",
		"bash scripts/require-main-validation.sh", "run: go run scripts/ci-plan.go -verify-needs",
		"PLAN_JSON: ${{ needs.scope.outputs.plan }}", "NEEDS_JSON: ${{ toJSON(needs) }}",
		"needs.scope.outputs.modules == 'true'", "PR end-to-end elapsed:", "elapsed > 120",
		"bunx vitest run src/i18n/__tests__/i18n-keys.test.ts src/styles/__tests__/",
		"args: --timeout=3m ${{ join(fromJSON(needs.scope.outputs.lintPackages), ' ') }}",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("workflow missing %q", want)
		}
	}
	if strings.Contains(text, "paths-ignore:") || strings.Contains(text, "continue-on-error: true") {
		t.Fatal("workflow must not silently bypass required scope")
	}
}

func TestPlannedLintPackagesRealTool(t *testing.T) {
	tool, err := exec.LookPath("golangci-lint")
	if err != nil {
		t.Skip("optional local golangci-lint; CI installs the pinned tool")
	}
	root := filepath.Dir(mustWorkingDir(t))
	bin := filepath.Join(t.TempDir(), "ci-plan")
	build := exec.Command("go", "build", "-o", bin, "scripts/ci-plan.go")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "master")
	configureTestRepo(t, repo)
	version, err := exec.Command("go", "env", "GOVERSION").Output()
	if err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(repo, "go.mod"), []byte("module "+strings.TrimSuffix(shardModule, "/")+"\n\ngo "+strings.TrimPrefix(strings.TrimSpace(string(version)), "go")+"\n"))
	mustWrite(t, filepath.Join(repo, ".golangci.yml"), []byte("version: \"2\"\nlinters:\n  default: none\n  enable: [govet]\n"))
	mustMkdir(t, filepath.Join(repo, "fixture"))
	mustWrite(t, filepath.Join(repo, "fixture", "fixture.go"), []byte("package fixture\nfunc Value() int { return 1 }\n"))
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "fixture")
	cmd := exec.Command(bin, "-mode", "full")
	cmd.Dir = repo
	cmd.Env = gitTestEnv(t)
	data, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var p verificationPlan
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	run := func(packages []string) ([]byte, error) {
		cmd := exec.Command(tool, append([]string{"run", "--timeout=1m"}, packages...)...)
		cmd.Dir = repo
		cmd.Env = gitTestEnv(t)
		return cmd.CombinedOutput()
	}
	if out, err := run(p.Packages); err == nil {
		t.Fatalf("old import-path args unexpectedly accepted: %s", out)
	}
	if out, err := run(p.LintPackages); err != nil {
		t.Fatalf("planned local lint args rejected: %v %s", err, out)
	}
	mustWrite(t, filepath.Join(repo, "fixture", "fixture.go"), []byte("package fixture\nfunc Value() int { return undefined }\n"))
	if out, err := run(p.LintPackages); err == nil {
		t.Fatalf("broken source passed planned lint: %s", out)
	}
}
