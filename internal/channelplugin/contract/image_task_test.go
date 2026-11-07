package contract

import (
	"net/url"
	"strings"
	"testing"
)

func TestImageQueryTemplateKeepsTaskIDInOneEscapedSegment(t *testing.T) {
	const template = "https://vendor.example/base/tasks/{taskId}/status"
	const taskID = "image/task ?#%"
	address, err := ImageQueryURL(template, taskID)
	if err != nil || address != "https://vendor.example/base/tasks/image%2Ftask%20%3F%23%25/status" {
		t.Fatalf("task id escaped as URL control: %s %v", address, err)
	}
	u, _ := url.Parse(address)
	if u.Host != "vendor.example" || u.RawQuery != "" || u.Fragment != "" {
		t.Fatal("task id changed destination or URL controls")
	}
	for _, invalid := range []string{"", ".", "..", "task\nprivate", "task\x00", "task\t", strings.Repeat("x", 513)} {
		if _, err := ImageQueryURL(template, invalid); err == nil {
			t.Fatal("invalid task id was accepted")
		}
	}
}

func TestImageQueryTemplateRejectsAmbiguousOrUnboundedPaths(t *testing.T) {
	for _, path := range []string{"/tasks", "//host/{taskId}", "/{taskId}/{taskId}", "/{taskId}suffix", "/../{taskId}", "/%2e%2e/{taskId}", "/tasks/{taskId}?other=x", "/tasks/{taskId}#other", "/tasks/{other}/{taskId}", "/tasks/{taskId}\\other", "/tasks/{taskId}\t", "/tasks/{taskId}%09"} {
		if ValidImageQueryPath(path) {
			t.Fatalf("unsafe path accepted: %q", path)
		}
	}
	for _, template := range []string{"https://user:fixture@vendor.example/tasks/{taskId}", "https://vendor.example/tasks/{taskId}?key=fixture", "https://vendor.example/tasks/{taskId}#key", "file:///tasks/{taskId}"} {
		if _, err := ImageQueryURL(template, "fixture"); err == nil {
			t.Fatal("URL authority or query controls accepted")
		}
	}
}
