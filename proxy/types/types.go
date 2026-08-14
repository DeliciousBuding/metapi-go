// Package types holds shared type definitions for the proxy module.
// Both proxy/ and proxy/profiles/ import this package to avoid import cycles.
package types

// CliProfileID identifies a CLI client profile.
type CliProfileID string

const (
	ProfileGeneric    CliProfileID = "generic"
	ProfileCodex      CliProfileID = "codex"
	ProfileClaudeCode CliProfileID = "claude_code"
	ProfileGeminiCli  CliProfileID = "gemini_cli"
)

// CliProfileCapabilities describes what a CLI client supports.
// Capability flags were removed in chore/rm-dead-proxy-routing (read only in
// tests); the type is kept so DetectedProfile keeps a stable shape.
type CliProfileCapabilities struct {
}

// DetectInput is the input for profile detection.
type DetectInput struct {
	DownstreamPath string
	Headers        map[string]string
	Body           any
}

// DetectedProfile is the result of profile detection.
type DetectedProfile struct {
	ID               CliProfileID
	ClientAppID      string
	ClientAppName    string
	ClientConfidence string // "exact" or "heuristic"
	ClientKind       string
	SessionID        string
	TraceHint        string
	Capabilities     CliProfileCapabilities
}

// CliProfileDefinition is a CLI profile with detection logic.
type CliProfileDefinition struct {
	ID           CliProfileID
	Capabilities CliProfileCapabilities
	Detect       func(input DetectInput) *DetectedProfile
}
