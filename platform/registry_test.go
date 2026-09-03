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
		"antigravity", "grok", "cliproxyapi", "sensetime", "anyrouter", "done-hub",
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

func TestGetAdapter_SenseTime(t *testing.T) {
	a := GetAdapter("sensetime")
	if a == nil {
		t.Fatal("expected adapter for 'sensetime', got nil")
	}
	if a.PlatformName() != "sensetime" {
		t.Errorf("expected PlatformName 'sensetime', got %q", a.PlatformName())
	}
	// Alias normalization: mixed-case input resolves via PlatformAliases.
	a2 := GetAdapter("SenseTime")
	if a2 == nil || a2.PlatformName() != "sensetime" {
		t.Errorf("GetAdapter('SenseTime') should resolve via alias: %v", a2)
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

// TestAdapterPlatformNames asserts every registered adapter reports its
// canonical platform name. Each expectation (wantName) is an explicit literal
// — never derived from the adapter or the registry — so a production wiring
// regression fails this test. After the table runs, an anti-shrink cross-check
// asserts the table's adapter set exactly equals the set the registry exposes
// (ListAdapters), so adding an adapter without a row (or deleting a row)
// makes the gate go red.
func TestAdapterPlatformNames(t *testing.T) {
	tests := []struct {
		adapter  PlatformAdapter
		wantName string
	}{
		{adapter: buildAdapter("openai"), wantName: "openai"},
		{adapter: buildAdapter("codex"), wantName: "codex"},
		{adapter: buildAdapter("claude"), wantName: "claude"},
		{adapter: buildAdapter("gemini"), wantName: "gemini"},
		{adapter: buildAdapter("gemini-cli"), wantName: "gemini-cli"},
		{adapter: buildAdapter("antigravity"), wantName: "antigravity"},
		{adapter: buildAdapter("grok"), wantName: "grok"},
		{adapter: buildAdapter("cliproxyapi"), wantName: "cliproxyapi"},
		{adapter: buildAdapter("sensetime"), wantName: "sensetime"},
		{adapter: buildAdapter("anyrouter"), wantName: "anyrouter"},
		{adapter: buildAdapter("done-hub"), wantName: "done-hub"},
		{adapter: buildAdapter("one-hub"), wantName: "one-hub"},
		{adapter: buildAdapter("veloera"), wantName: "veloera"},
		{adapter: buildAdapter("new-api"), wantName: "new-api"},
		{adapter: buildAdapter("sub2api"), wantName: "sub2api"},
		{adapter: buildAdapter("one-api"), wantName: "one-api"},
	}

	for _, tt := range tests {
		t.Run(tt.wantName, func(t *testing.T) {
			if got := tt.adapter.PlatformName(); got != tt.wantName {
				t.Errorf("PlatformName() = %q, want %q", got, tt.wantName)
			}
		})
	}

	// Anti-shrink gate: the table must cover exactly the registry's adapters.
	tableNames := make(map[string]bool, len(tests))
	for _, tt := range tests {
		tableNames[tt.wantName] = true
	}
	registryNames := make(map[string]bool)
	for _, a := range ListAdapters() {
		registryNames[a.PlatformName()] = true
	}
	for name := range tableNames {
		if !registryNames[name] {
			t.Errorf("table row %q is not exposed by the registry", name)
		}
	}
	for name := range registryNames {
		if !tableNames[name] {
			t.Errorf("registry adapter %q is missing from the table", name)
		}
	}
}
