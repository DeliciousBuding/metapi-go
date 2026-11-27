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
		name, workflow, event, branch, head, target, fault string
		valid                                              bool
	}{
		{"legacy full", "main", "push", "master", head, head, "", true},
		{"edge failed after full", "main", "push", "master", head, head, "edge-failed", true},
		{"lightweight aggregate", "main", "push", "master", head, head, "skip-tests", false},
		{"skipped image", "main", "push", "master", head, head, "skip-image", false},
		{"wrong legacy SHA", "main", "push", "master", "other", head, "", false},
		{"scheduled full", "nightly", "schedule", "master", head, head, "", true},
		{"wrong scheduled SHA", "nightly", "schedule", "master", "other", head, "", false},
		{"dispatched exact older commit", "nightly", "workflow_dispatch", "master", "other", head, "", true},
		{"dispatch tested another commit", "nightly", "workflow_dispatch", "master", head, "other", "", false},
		{"untrusted branch", "nightly", "workflow_dispatch", "feature", head, head, "", false},
		{"wrong event", "nightly", "pull_request", "master", head, head, "", false},
		{"failed gate", "nightly", "workflow_dispatch", "master", head, head, "failed", false},
		{"cancelled gate", "nightly", "workflow_dispatch", "master", head, head, "cancelled", false},
		{"missing proof", "nightly", "schedule", "master", head, head, "missing", false},
		{"API error", "nightly", "schedule", "master", head, head, "api", false},
		{"malformed jobs", "nightly", "schedule", "master", head, head, "bad-json", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := filepath.Join(t.TempDir(), "stub.sh")
			mustWrite(t, stub, []byte(`gh() {
  case "$4" in
    */"$WORKFLOW".yml/runs) jq -cn '[{workflow_runs:[{id:123,head_sha:env.HEAD_SHA,head_branch:env.BRANCH,event:env.EVENT,status:"completed",conclusion:(if env.FAULT=="edge-failed" then "failure" else "success" end),display_title:("Full integration "+env.TARGET)}]}]';;
    */runs) echo '[{"workflow_runs":[]}]';;
    */123/jobs)
      case "$FAULT" in api) return 9;; bad-json) echo '{'; return;; esac
      jq -cn '[{jobs: (if env.WORKFLOW=="nightly" then [{name:"nightly-gate",conclusion:(if env.FAULT=="failed" or env.FAULT=="cancelled" then env.FAULT else "success" end),steps:(if env.FAULT=="missing" then [] else [{name:("Validated "+env.TARGET),conclusion:"success"}] end)}]
        else [{name:"docker-build",conclusion:"success",steps:(["Build Docker image","Smoke Docker image"]|map({name:.,conclusion:(if env.FAULT=="skip-image" then "skipped" else "success" end)}))}] + (["a11y","test-pg","test-sqlite","test-e2e"]|map({name:.,conclusion:(if env.FAULT=="skip-tests" then "skipped" else "success" end)})) end)}]';;
    *) return 99;;
  esac
}`))
			cmd := exec.Command("bash", "scripts/require-main-validation.sh", "HEAD")
			cmd.Dir = root
			cmd.Env = append(gitTestEnv(t), "BASH_ENV="+stub, "GITHUB_REPOSITORY=fixture/repo", "WORKFLOW="+tc.workflow, "HEAD_SHA="+tc.head, "BRANCH="+tc.branch, "EVENT="+tc.event, "TARGET="+tc.target, "FAULT="+tc.fault)
			out, err := cmd.CombinedOutput()
			if (err == nil) != tc.valid || tc.valid && !strings.Contains(string(out), head) {
				t.Fatalf("%v %s", err, out)
			}
		})
	}
}
