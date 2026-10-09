package oauth

// OverrideClaudeTokenURLForTest exposes only the endpoint seam to external
// package integration tests; the actual provider refresh implementation runs.
func OverrideClaudeTokenURLForTest(endpoint string) func() {
	previous := claudeTokenURL
	claudeTokenURL = endpoint
	return func() { claudeTokenURL = previous }
}
