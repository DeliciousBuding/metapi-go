package scripts

import (
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyChangeRealEntrypoint(t *testing.T) {
	root := filepath.Dir(mustWorkingDir(t))
	bin := filepath.Join(t.TempDir(), "ci-plan")
	build := exec.Command("go", "build", "-o", bin, "scripts/ci-plan.go")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	goRoot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Fatal(err)
	}
	goVersion, err := exec.Command("go", "env", "GOVERSION").Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, changed, content, fail string
		bad                          bool
	}{
		{"docs only", "docs/note.md", "changed\n", "", false},
		{"backend", "a/a.go", "package a\n// changed\n", "", false},
		{"failed affected test", "a/a.go", "package a\n// changed\n", "test", true},
		{"failed SPA build", "a/a.go", "package a\n// changed\n", "bun", true},
		{"bad syntax", "a/a.go", "package a\nfunc broken( {\n", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			runGit(t, repo, "init", "-b", "master")
			configureTestRepo(t, repo)
			for _, file := range []string{".githooks/pre-push-project", "scripts/verify-change.sh", "scripts/ci-plan.go"} {
				data, err := os.ReadFile(filepath.Join(root, file))
				if err != nil {
					t.Fatal(err)
				}
				mustMkdir(t, filepath.Dir(filepath.Join(repo, file)))
				mustWrite(t, filepath.Join(repo, file), data)
			}
			for file, text := range map[string]string{"a/a.go": "package a\n", "docs/note.md": "original\n", "web/embed.go": "package web\n", "go.mod": "module github.com/deliciousbuding/metapi-go\n\ngo " + strings.TrimPrefix(strings.TrimSpace(string(goVersion)), "go") + "\n"} {
				mustMkdir(t, filepath.Dir(filepath.Join(repo, file)))
				mustWrite(t, filepath.Join(repo, file), []byte(text))
			}
			runGit(t, repo, "add", ".")
			runGit(t, repo, "commit", "-m", "fixture")
			runGit(t, repo, "update-ref", "refs/remotes/origin/master", "HEAD")
			content := []byte(tc.content)
			if strings.HasSuffix(tc.changed, ".go") && tc.name != "bad syntax" {
				content, err = format.Source(content)
				if err != nil {
					t.Fatal(err)
				}
			}
			mustWrite(t, filepath.Join(repo, tc.changed), content)
			stub := filepath.Join(t.TempDir(), "tools.sh")
			mustWrite(t, stub, []byte(`
go() {
  if [[ "$1 $2" == 'env GOVERSION' ]]; then echo "$STUB_GOVERSION"; return; fi
  if [[ "$1 $2" == 'env GOROOT' ]]; then echo "$STUB_GOROOT"; return; fi
  if [[ "$1 $2" == 'run scripts/ci-plan.go' ]]; then shift 2; "$PLAN_BIN" "$@"; return; fi
  echo "go $*" >> "$TRACE"
  if [[ "$1" == test && "$2" != ./docs && "$FAIL" == test ]]; then return 7; fi
}
bun() { echo "bun $*" >> "$TRACE"; [[ "$FAIL" != bun ]]; }
`))
			trace := filepath.Join(t.TempDir(), "trace")
			cmd := exec.Command("bash", ".githooks/pre-push-project")
			cmd.Dir = repo
			cmd.Env = append(gitTestEnv(t), "BASH_ENV="+stub, "PLAN_BIN="+bin, "TRACE="+trace, "FAIL="+tc.fail, "PG_TEST_DSN=", "METAPI_VERIFY_FULL=0", "STUB_GOROOT="+strings.TrimSpace(string(goRoot)), "STUB_GOVERSION="+strings.TrimSpace(string(goVersion)))
			out, err := cmd.CombinedOutput()
			if (err != nil) != tc.bad {
				t.Fatalf("err=%v\n%s", err, out)
			}
			data, _ := os.ReadFile(trace)
			if tc.name == "docs only" && string(data) != "go test ./docs -count=1\n" {
				t.Fatalf("docs invoked unrelated tools: %s\n%s", data, out)
			}
			if tc.name == "backend" && (!strings.Contains(string(data), "-race") || !strings.Contains(string(data), "/a")) {
				t.Fatalf("lost affected race: %s", data)
			}
			if tc.name == "bad syntax" && len(data) != 0 {
				t.Fatalf("tools started after bad plan: %s", data)
			}
		})
	}
}
