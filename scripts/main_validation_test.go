package scripts

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequireMainValidationRealCLI(t *testing.T) {
	root := filepath.Dir(mustWorkingDir(t))
	head := runGit(t, root, "rev-parse", "HEAD")
	for _, tc := range []struct {
		name, json string
		bad        bool
	}{
		{"exact green", `{"workflow_runs":[{"id":123,"head_sha":"` + head + `","head_branch":"master","event":"push","conclusion":"success"}]}`, false},
		{"publish failed but suite green", `{"workflow_runs":[{"id":123,"head_sha":"` + head + `","head_branch":"master","event":"push","conclusion":"failure"}]}`, false},
		{"other SHA", `{"workflow_runs":[{"head_sha":"wrong","head_branch":"master","event":"push","conclusion":"success"}]}`, true},
		{"PR green", `{"workflow_runs":[{"head_sha":"` + head + `","head_branch":"master","event":"pull_request","conclusion":"success"}]}`, true},
		{"failed", `{"workflow_runs":[{"id":123,"head_sha":"` + head + `","head_branch":"master","event":"push","conclusion":"failure"}]}`, true},
		{"cancelled", `{"workflow_runs":[{"id":123,"head_sha":"` + head + `","head_branch":"master","event":"push","conclusion":"cancelled"}]}`, true},
		{"empty", `{"workflow_runs":[]}`, true}, {"invalid JSON", `{`, true},
		{"API failure", "API_FAILURE", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := filepath.Join(t.TempDir(), "stub.sh")
			mustWrite(t, stub, []byte(`gh() {
  if [[ "$FIXTURE" == API_FAILURE ]]; then return 9; fi
  if [[ "$4" == */jobs ]]; then
    if [[ "$CASE" == failed || "$CASE" == cancelled ]]; then printf '{"jobs":[{"name":"docker-build","conclusion":"failure"}]}'; else printf '{"jobs":[{"name":"docker-build","conclusion":"success"}]}'; fi
  else printf '%s' "$FIXTURE"; fi
}`))
			cmd := exec.Command("bash", "scripts/require-main-validation.sh", "HEAD")
			cmd.Dir = root
			cmd.Env = append(gitTestEnv(t), "BASH_ENV="+stub, "GITHUB_REPOSITORY=fixture/repo", "FIXTURE="+tc.json, "CASE="+tc.name)
			out, err := cmd.CombinedOutput()
			if (err != nil) != tc.bad {
				t.Fatalf("%v\n%s", err, out)
			}
			if !tc.bad && !strings.Contains(string(out), head) {
				t.Fatalf("missing SHA: %s", out)
			}
		})
	}
}
