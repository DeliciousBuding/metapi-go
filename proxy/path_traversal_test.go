package proxy

import "testing"

// TestContainsPathTraversal covers the Wave 4 security handoff T1 helper:
// detection must catch every ".." segment shape that can arrive via
// r.URL.Path (already percent-decoded by net/http) while leaving every
// legitimate path untouched — the forwarding contract forbids altering
// traversal-free paths.
func TestContainsPathTraversal(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"empty path is clean", "", false},
		{"root path is clean", "/", false},
		{"simple endpoint is clean", "/v1/chat/completions", false},
		{"gemini model action is clean", "/v1beta/models/gemini-2.5-pro:generateContent", false},
		{"dot in model name is clean", "/v1beta/models/gemini-2.5-pro:streamGenerateContent", false},
		{"segment containing dots is clean", "/v1/files/file..name/content", false},
		{"single dot segment is clean", "/v1/./models", false},
		{"trailing single dot is clean", "/v1/models/.", false},
		{"leading dot-dot is traversal", "../etc/passwd", true},
		{"mid-path dot-dot is traversal", "/v1beta/models/../../admin:probe", true},
		{"deep dot-dot chain is traversal", "/v1beta/models/../../../../etc/passwd", true},
		{"lone dot-dot is traversal", "..", true},
		{"slash-delimited dot-dot only is traversal", "/..", true},
		{"trailing dot-dot is traversal", "/v1/videos/..", true},
		{"dot-dot inside segment suffix is clean", "/v1/models/..hidden", false},
		{"dot-dot inside segment prefix is clean", "/v1/models/hidden..", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ContainsPathTraversal(tc.path); got != tc.want {
				t.Fatalf("ContainsPathTraversal(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}
