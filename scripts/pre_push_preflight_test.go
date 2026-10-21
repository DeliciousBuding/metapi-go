package scripts

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPreflightRealRepository(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("preflight uses bash")
	}
	goVersion, err := exec.Command("go", "env", "GOVERSION").Output()
	if err != nil {
		t.Fatal(err)
	}
	script, err := os.ReadFile("pre-push-preflight.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, want string
		prepare    func(*testing.T, string)
	}{
		{"clean including spaced filename", "preflight: frontend static reached", nil},
		{"format drift", "gofmt drift", func(t *testing.T, root string) {
			mustWrite(t, filepath.Join(root, "format fixture.go"), []byte("package fixture\nfunc spaced( ){ }\n"))
		}},
		{"invalid syntax", "expected", func(t *testing.T, root string) {
			mustWrite(t, filepath.Join(root, "format fixture.go"), []byte("package fixture\nfunc broken( {\n"))
		}},
		{"unstaged whitespace", "trailing whitespace", func(t *testing.T, root string) {
			mustWrite(t, filepath.Join(root, "note.md"), []byte("bad  \n"))
		}},
		{"staged whitespace", "trailing whitespace", func(t *testing.T, root string) {
			mustWrite(t, filepath.Join(root, "note.md"), []byte("bad  \n"))
			runGit(t, root, "add", "note.md")
		}},
		{"missing tracked file", "no such file", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "format fixture.go")); err != nil {
				t.Fatal(err)
			}
		}},
		{"empty scan", "no tracked Go files", func(t *testing.T, root string) {
			runGit(t, root, "rm", "format fixture.go")
		}},
		{"toolchain mismatch", "toolchain mismatch", func(t *testing.T, root string) {
			mustWrite(t, filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.0.0\n"))
		}},
		{"unreadable module", "go.mod", func(t *testing.T, root string) {
			if err := os.Remove(filepath.Join(root, "go.mod")); err != nil {
				t.Fatal(err)
			}
		}},
		{"invalid repository", "not a git repository", func(t *testing.T, root string) {
			if err := os.Rename(filepath.Join(root, ".git"), filepath.Join(root, "not-git")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			runGit(t, root, "init", "-b", "main")
			configureTestRepo(t, root)
			mustMkdir(t, filepath.Join(root, "web"))
			mustWrite(t, filepath.Join(root, "preflight.sh"), script)
			mustWrite(t, filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo "+strings.TrimPrefix(strings.TrimSpace(string(goVersion)), "go")+"\n"))
			mustWrite(t, filepath.Join(root, "format fixture.go"), []byte("package fixture\n"))
			mustWrite(t, filepath.Join(root, "note.md"), []byte("clean\n"))
			runGit(t, root, "add", ".")
			runGit(t, root, "commit", "-m", "fixture")
			if tc.prepare != nil {
				tc.prepare(t, root)
			}
			stub := filepath.Join(t.TempDir(), "frontend.sh")
			mustWrite(t, stub, []byte("bun() { echo 'preflight: frontend static reached'; }\n"))
			cmd := exec.Command("bash", "preflight.sh")
			cmd.Dir = root
			cmd.Env = append(gitTestEnv(t), "BASH_ENV="+filepath.ToSlash(stub), "GOTOOLCHAIN=local")
			output, err := cmd.CombinedOutput()
			if (err != nil) != (tc.prepare != nil) || !strings.Contains(strings.ToLower(string(output)), strings.ToLower(tc.want)) {
				t.Fatalf("unexpected preflight result: %v\n%s", err, output)
			}
			if tc.prepare != nil && strings.Contains(string(output), "frontend static reached") {
				t.Fatalf("frontend checks started after an earlier fault:\n%s", output)
			}
		})
	}
}

func TestFrontendStaticSplitKeepsFullCheck(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "web", "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Scripts["check"] != "bun run check:static && bun run test" ||
		pkg.Scripts["check:static"] != "bun run typecheck && bun run lint && bun run format:check && bun run knip" {
		t.Fatal("static/test split dropped or duplicated frontend checks")
	}
}
