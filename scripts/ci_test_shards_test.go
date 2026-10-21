package scripts

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const shardModule = "github.com/deliciousbuding/metapi-go/"

func TestCITestShardsCLI(t *testing.T) {
	root := filepath.Dir(mustWorkingDir(t))
	bin := filepath.Join(t.TempDir(), "ci-test-shards")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, "scripts/ci-test-shards.go")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real selector: %v\n%s", err, out)
	}
	weights := filepath.Join(t.TempDir(), "weights.tsv")
	mustWrite(t, weights, []byte("# fixture milliseconds\na 100000\nb 80000\nc 40000\nd 20000\n"))
	input := shardModule + "a\n" + shardModule + "b\n" + shardModule + "c\n" + shardModule + "d\n" + shardModule + "new-package\n"
	run := func(input string, args ...string) (string, string, error) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = root
		cmd.Stdin = strings.NewReader(input)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	}
	t.Run("deterministic LPT exact coverage and new packages", func(t *testing.T) {
		want := [][]string{{shardModule + "a", shardModule + "d", shardModule + "new-package"}, {shardModule + "b", shardModule + "c"}}
		lines := strings.Fields(input)
		sort.Sort(sort.Reverse(sort.StringSlice(lines)))
		reordered := strings.Join(lines, "\n") + "\n"
		var all []string
		for shard := 0; shard < 2; shard++ {
			args := []string{"-shard", fmt.Sprint(shard), "-total", "2", "-weights", weights}
			out, stderr, err := run(input, args...)
			if err != nil || !reflect.DeepEqual(strings.Fields(out), want[shard]) {
				t.Fatalf("shard %d: %v stdout=%q stderr=%q", shard, err, out, stderr)
			}
			again, _, err := run(reordered, args...)
			if err != nil || again != out {
				t.Fatalf("assignment depends on input order: %v %q != %q", err, again, out)
			}
			all = append(all, strings.Fields(out)...)
		}
		sort.Strings(all)
		sort.Strings(lines)
		if !reflect.DeepEqual(all, lines) {
			t.Fatalf("not exactly once: got %v want %v", all, lines)
		}
	})
	t.Run("bad input fails without partial selection", func(t *testing.T) {
		for _, tc := range []struct{ name, input, shard, total string }{
			{"empty packages", "", "0", "2"},
			{"too few packages", shardModule + "a\n", "0", "2"},
			{"duplicate", input + shardModule + "a\n", "0", "2"},
			{"blank line", input + "\n", "0", "2"},
			{"foreign module", input + "example.com/foreign\n", "0", "2"},
			{"shell argument", input + shardModule + "-run=X\n", "0", "2"},
			{"traversal", input + shardModule + "../evil\n", "0", "2"},
			{"scanner overflow", strings.Repeat("x", 70000), "0", "2"},
			{"negative shard", input, "-1", "2"},
			{"out of bounds", input, "2", "2"},
			{"zero total", input, "0", "0"},
			{"invalid number", input, "x", "2"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				out, stderr, err := run(tc.input, "-shard", tc.shard, "-total", tc.total, "-weights", weights)
				if err == nil || out != "" || stderr == "" {
					t.Fatalf("bad input passed/partially emitted: err=%v stdout=%q stderr=%q", err, out, stderr)
				}
			})
		}
	})
	t.Run("bad weights fail", func(t *testing.T) {
		for _, text := range []string{"", "# no rows\n", "a NaN\n", "a 0\n", "a -1\n", "a 9223372036854775808\n", "a 86400001\n", "a 1\na 2\n", "a\n", "../evil 1\n", strings.Repeat("x", 70000)} {
			bad := filepath.Join(t.TempDir(), "bad.tsv")
			mustWrite(t, bad, []byte(text))
			out, _, err := run(input, "-shard", "0", "-total", "2", "-weights", bad)
			if err == nil || out != "" {
				t.Fatalf("bad weights %q passed: %v %q", text[:min(len(text), 40)], err, out)
			}
		}
		if out, _, err := run(input, "-shard", "0", "-total", "2", "-weights", filepath.Join(t.TempDir(), "missing")); err == nil || out != "" {
			t.Fatalf("missing weights passed: %v %q", err, out)
		}
		stale := filepath.Join(t.TempDir(), "stale.tsv")
		mustWrite(t, stale, []byte("a 1\nmissing 2\n"))
		if out, _, err := run(shardModule+"a\n"+shardModule+"b\n", "-shard", "0", "-total", "2", "-weights", stale); err == nil || out != "" {
			t.Fatalf("stale weights passed: %v %q", err, out)
		}
	})
	t.Run("real workflow entry fails closed", func(t *testing.T) {
		if _, err := exec.LookPath("bash"); err != nil {
			t.Skip("workflow uses bash")
		}
		workflow, err := os.ReadFile(filepath.Join(root, ".github/workflows/main.yml"))
		if err != nil {
			t.Fatal(err)
		}
		runBody := func(marker string) string {
			t.Helper()
			_, body, found := strings.Cut(string(workflow), marker)
			if !found {
				t.Fatalf("missing workflow entrypoint %q", marker)
			}
			_, body, found = strings.Cut(body, "        run: |\n")
			if !found {
				t.Fatalf("missing workflow run body after %q", marker)
			}
			body, _, _ = strings.Cut(body, "\n\n")
			return strings.ReplaceAll(body, "          ", "")
		}
		entries := []struct {
			name     string
			body     string
			wantArgs string
		}{
			{name: "sqlite", body: runBody("      - name: Run tests (shard"), wantArgs: "-count=1\n-race\n"},
			{name: "postgres", body: runBody("      - name: Run PostgreSQL tests (shard"), wantArgs: "-count=1\n-tags=integration\n-p\n1\n"},
		}
		for _, entry := range entries {
			workflowBody := strings.NewReplacer(
				"scripts/ci-test-shards.weights.tsv", weights,
				"scripts/ci-test-pg.weights.tsv", weights,
			).Replace(entry.body)
			for _, mode := range []string{"normal", "list-fail", "selector-fail", "selector-empty", "list-empty", "invalid-shard"} {
				t.Run(entry.name+"/"+mode, func(t *testing.T) {
					tmp := t.TempDir()
					stub := `#!/usr/bin/env bash
set -euo pipefail
case "$1" in
  list)
    if [[ "$STUB_MODE" != list-empty ]]; then printf '%s\n' "${STUB_MODULE}a" "${STUB_MODULE}b" "${STUB_MODULE}c" "${STUB_MODULE}d"; fi
    if [[ "$STUB_MODE" == list-fail ]]; then exit 17; fi
    ;;
  run)
    if [[ "$STUB_MODE" == selector-empty ]]; then exit 0; fi
    if [[ "$STUB_MODE" == selector-fail ]]; then printf '%s\n' "${STUB_MODULE}a"; exit 17; fi
    shift 2
    "$SELECTOR_BIN" "$@"
    ;;
  test) printf '%s\n' "$@" > "$RUNNER_TEMP/invoked" ;;
  *) exit 99 ;;
esac
`
					goStub := filepath.Join(tmp, "go")
					mustWrite(t, goStub, []byte(stub))
					if err := os.Chmod(goStub, 0o755); err != nil {
						t.Fatal(err)
					}
					shard := "0"
					if mode == "invalid-shard" {
						shard = "bad"
					}
					cmd := exec.Command("bash", "-c", workflowBody)
					cmd.Dir = root
					cmd.Env = append(os.Environ(), "PATH="+tmp+string(os.PathListSeparator)+os.Getenv("PATH"), "SHARD="+shard, "SHARD_TOTAL=2", "RUNNER_TEMP="+tmp, "STUB_MODE="+mode, "SELECTOR_BIN="+bin, "STUB_MODULE="+shardModule, "PG_SCHEME=postgres", "PG_USER=postgres", "PG_PASSWORD=test", "PG_DATABASE=metapi_test")
					out, err := cmd.CombinedOutput()
					invoked, invocationErr := os.ReadFile(filepath.Join(tmp, "invoked"))
					if mode == "normal" {
						if err != nil || invocationErr != nil || !strings.Contains(string(invoked), entry.wantArgs) {
							t.Fatalf("valid workflow failed: %v %v\n%s\n%s", err, invocationErr, out, invoked)
						}
					} else if err == nil || !os.IsNotExist(invocationErr) {
						t.Fatalf("bad workflow ran tests/passed: %v invocation=%v\n%s", err, invocationErr, out)
					}
				})
			}
		}
	})
	t.Run("PostgreSQL weights cover the complete package list exactly once", func(t *testing.T) {
		// -e keeps the package inventory available in a source-only worktree
		// where web/dist has not been built yet. The real workflow intentionally
		// uses plain go list after downloading the embedded asset.
		cmd := exec.Command("go", "list", "-e", "./...")
		cmd.Dir = root
		packages, err := cmd.Output()
		if err != nil {
			t.Fatalf("go list: %v", err)
		}
		weights := filepath.Join(root, "scripts", "ci-test-pg.weights.tsv")
		expected := strings.Fields(string(packages))
		var assigned []string
		for shard := 0; shard < 4; shard++ {
			out, stderr, err := run(string(packages), "-shard", fmt.Sprint(shard), "-total", "4", "-weights", weights)
			if err != nil || len(strings.Fields(out)) == 0 {
				t.Fatalf("PG shard %d: %v stdout=%q stderr=%q", shard, err, out, stderr)
			}
			assigned = append(assigned, strings.Fields(out)...)
		}
		sort.Strings(expected)
		sort.Strings(assigned)
		if !reflect.DeepEqual(assigned, expected) {
			t.Fatalf("PostgreSQL shards do not cover all packages exactly once")
		}
	})
}

func TestCIBunDownloadCache(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "main.yml"))
	if err != nil {
		t.Fatal(err)
	}
	// Validate actual cache step blocks, not mentions in workflow comments.
	blocks := regexp.MustCompile(`(?m)^      - name: Cache Bun downloads\n(?:        .*\n)+`).FindAllString(string(data), -1)
	if len(blocks) != 4 { // frontend, web-dist, a11y, release
		t.Fatalf("got %d Bun cache steps, want all four frozen-install consumers", len(blocks))
	}
	for _, block := range blocks {
		for _, want := range []string{
			"uses: actions/cache@55cc8345863c7cc4c66a329aec7e433d2d1c52a9",
			"path: ~/.bun/install/cache\n",
			"key: bun-downloads-${{ runner.os }}-${{ runner.arch }}-${{ env.BUN_VERSION }}-${{ hashFiles('web/bun.lock') }}\n",
		} {
			if !strings.Contains(block, want) {
				t.Errorf("cache step missing %q:\n%s", want, block)
			}
		}
		if strings.Contains(block, "node_modules") || strings.Contains(block, "restore-keys:") {
			t.Errorf("only downloads for the exact toolchain/lockfile may be restored:\n%s", block)
		}
	}
}
