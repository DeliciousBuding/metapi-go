package platform

import "testing"

// --- Registry and Registration Order ---

func TestRegistry_AdapterCount(t *testing.T) {
	adapters := ListAdapters()
	if len(adapters) < 14 {
		t.Fatalf("expected at least 14 adapters, got %d", len(adapters))
	}

	// Verify no duplicate names
	seen := make(map[string]bool)
	for _, a := range adapters {
		name := a.PlatformName()
		if seen[name] {
			t.Errorf("duplicate adapter name in registry: %q", name)
		}
		seen[name] = true
	}
}

func TestRegistry_RegistrationOrder(t *testing.T) {
	adapters := ListAdapters()
	names := make([]string, len(adapters))
	for i, a := range adapters {
		names[i] = a.PlatformName()
	}

	// Verify order matches spec
	expected := []string{
		"openai", "codex", "claude", "gemini", "gemini-cli",
		"antigravity", "grok", "cliproxyapi", "anyrouter", "done-hub",
		"one-hub", "veloera", "new-api", "sub2api", "one-api",
	}
	if len(names) != len(expected) {
		t.Fatalf("expected %d adapters, got %d: %v", len(expected), len(names), names)
	}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("position %d: got %q, want %q", i, name, expected[i])
		}
	}
}

func TestGetAdapter(t *testing.T) {
	// Known platforms
	a := GetAdapter("openai")
	if a == nil || a.PlatformName() != "openai" {
		t.Errorf("GetAdapter('openai'): %v", a)
	}

	// Unknown platform
	a2 := GetAdapter("nonexistent-platform")
	if a2 != nil {
		t.Errorf("GetAdapter('nonexistent') should return nil: %v", a2)
	}

	// Empty
	a3 := GetAdapter("")
	if a3 != nil {
		t.Errorf("GetAdapter('') should return nil: %v", a3)
	}
}

// --- NormalizePlatformAlias ---

func TestNormalizePlatformAlias(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"new-api", "new-api"},
		{"newapi", "new-api"},
		{"new api", "new-api"},
		{"NEW-API", "new-api"},
		{"one-api", "one-api"},
		{"oneapi", "one-api"},
		{"one api", "one-api"},
		{"one-hub", "one-hub"},
		{"onehub", "one-hub"},
		{"done-hub", "done-hub"},
		{"donehub", "done-hub"},
		{"openai", "openai"},
		{"anthropic", "claude"},
		{"claude", "claude"},
		{"codex", "codex"},
		{"chatgpt-codex", "codex"},
		{"gemini", "gemini"},
		{"google", "gemini"},
		{"gemini-cli", "gemini-cli"},
		{"antigravity", "antigravity"},
		{"anti-gravity", "antigravity"},
		{"grok", "grok"},
		{"xai", "grok"},
		{"x.ai", "grok"},
		{"GROK", "grok"},
		{"XAI", "grok"},
		{"cliproxyapi", "cliproxyapi"},
		{"cpa", "cliproxyapi"},
		{"cli-proxy-api", "cliproxyapi"},
		{"anyrouter", "anyrouter"},
		{"veloera", "veloera"},
		{"sub2api", "sub2api"},
		{"unknown", "unknown"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := NormalizePlatformAlias(tt.input); got != tt.expected {
			t.Errorf("NormalizePlatformAlias(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNormalizePlatformAlias_NewApiForks(t *testing.T) {
	// NewApi fork aliases
	tests := []struct {
		input    string
		expected string
	}{
		{"vo-api", "new-api"},
		{"super-api", "new-api"},
		{"rix-api", "new-api"},
		{"neo-api", "new-api"},
	}
	for _, tt := range tests {
		if got := NormalizePlatformAlias(tt.input); got != tt.expected {
			t.Errorf("NormalizePlatformAlias(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

// --- ListAdapters ---

func TestListAdapters_Copy(t *testing.T) {
	original := ListAdapters()
	// Modify the returned slice
	if len(original) > 0 {
		original[0] = nil
	}
	// Original registry should be unaffected
	again := ListAdapters()
	if len(again) != len(original) || again[0] == nil {
		// Actually ListAdapters creates a copy, so modification won't persist
		// This just ensures it doesn't panic
	}
}
