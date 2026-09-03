package platform

import (
	"context"
	"testing"
)

// newGrokAdapterForTest builds a GrokAdapter identical to the one registered in
// registry.go's buildAdapter("grok"), keeping these tests in sync with the
// production wiring (login/checkin unsupported message copy).
func newGrokAdapterForTest() *GrokAdapter {
	return &GrokAdapter{StandardAdapter: &StandardAdapter{
		BaseAdapter:               NewBaseAdapter("grok"),
		LoginUnsupportedMessage:   "grok oauth login is managed via Device OAuth flow",
		CheckinUnsupportedMessage: "grok oauth connections do not support checkin",
	}}
}

func TestGrokAdapter_Detect(t *testing.T) {
	a := newGrokAdapterForTest()
	ctx := context.Background()

	tests := []struct {
		url     string
		matches bool
	}{
		{"https://api.x.ai/v1/chat/completions", true},
		{"https://api.x.ai", true},
		{"https://x.ai", true},
		{"https://X.AI/v1/models", true},
		{"https://api.x.ai:443/v1", true},
		{"https://api.openai.com", false},
		{"https://chatgpt.com/backend-api/codex", false},
		{"https://api.groq.com", false},
		{"https://example.com/x.ai", false}, // path substring must not match
		{"", false},
	}
	for _, tt := range tests {
		ok, err := a.Detect(ctx, tt.url)
		if err != nil {
			t.Errorf("Detect(%q) returned error: %v", tt.url, err)
			continue
		}
		if ok != tt.matches {
			t.Errorf("Detect(%q) = %v, want %v", tt.url, ok, tt.matches)
		}
	}
}

func TestGrokAdapter_GetModels(t *testing.T) {
	a := newGrokAdapterForTest()
	ctx := context.Background()

	models, err := a.GetModels(ctx, "https://api.x.ai", "ignored-token", nil, nil)
	if err != nil {
		t.Fatalf("GetModels returned unexpected error: %v", err)
	}
	if len(models) == 0 {
		t.Fatal("GetModels returned empty model list; expected hardcoded seed")
	}

	// normalizeModelIDs dedups + sorts, so verify membership rather than order.
	seen := make(map[string]bool, len(models))
	for _, model := range models {
		seen[model] = true
	}
	expectedSeed := []string{
		"grok-3", "grok-3-mini", "grok-3-fast",
		"grok-2", "grok-2-mini", "grok-2-vision", "grok-2-latest",
	}
	for _, want := range expectedSeed {
		if !seen[want] {
			t.Errorf("expected seed model %q in GetModels output: %v", want, models)
		}
	}
}

func TestGrokAdapter_GetModels_DoesNotDependOnBaseURL(t *testing.T) {
	a := newGrokAdapterForTest()
	ctx := context.Background()

	// The seed list is hardcoded, so a bogus base URL must still return models.
	models, err := a.GetModels(ctx, "https://invalid.invalid", "ignored", nil, nil)
	if err != nil {
		t.Fatalf("GetModels with bogus base URL returned error: %v", err)
	}
	if len(models) != len(grokSeedModels) {
		t.Errorf("expected %d seed models, got %d", len(grokSeedModels), len(models))
	}
}

func TestGrokAdapter_LoginUnsupported(t *testing.T) {
	a := newGrokAdapterForTest()
	ctx := context.Background()

	result, err := a.Login(ctx, "https://api.x.ai", "u", "p", nil, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("Grok login should return Success=false (managed by Device OAuth)")
	}
	if result.Message != "grok oauth login is managed via Device OAuth flow" {
		t.Errorf("Login message: %q", result.Message)
	}
}

func TestGrokAdapter_CheckinUnsupported(t *testing.T) {
	a := newGrokAdapterForTest()
	ctx := context.Background()

	result, err := a.Checkin(ctx, "https://api.x.ai", "token", nil, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("Grok checkin should return Success=false")
	}
	if result.Message != "grok oauth connections do not support checkin" {
		t.Errorf("Checkin message: %q", result.Message)
	}
}

func TestGrokAdapter_GetBalanceZero(t *testing.T) {
	a := newGrokAdapterForTest()
	ctx := context.Background()

	balance, err := a.GetBalance(ctx, "https://api.x.ai", "token", nil, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if balance.Balance != 0 || balance.Used != 0 || balance.Quota != 0 {
		t.Error("Grok balance should be all zeros (no balance endpoint)")
	}
}

func TestGetAdapter_ResolvesGrok(t *testing.T) {
	// Canonical name.
	a := GetAdapter("grok")
	if a == nil || a.PlatformName() != "grok" {
		t.Fatalf("GetAdapter('grok') did not resolve to grok adapter: %v", a)
	}

	// Aliases.
	for _, alias := range []string{"xai", "x.ai", "Grok", "XAI"} {
		resolved := GetAdapter(alias)
		if resolved == nil || resolved.PlatformName() != "grok" {
			t.Errorf("GetAdapter(%q) should resolve to grok, got %v", alias, resolved)
		}
	}

	// Unknown alias must NOT resolve to grok.
	if GetAdapter("groq") != nil {
		t.Error("GetAdapter('groq') should return nil, not the grok adapter")
	}
}

func TestGrokAdapter_RegisteredOrder(t *testing.T) {
	// Grok must be registered after antigravity and before cliproxyapi so that
	// the OAuth-driven vendor platforms are grouped together before the
	// generic gateway-fork adapters.
	adapters := ListAdapters()
	names := make([]string, len(adapters))
	for i, a := range adapters {
		names[i] = a.PlatformName()
	}

	grokIdx := indexOfPlatform(names, "grok")
	antiIdx := indexOfPlatform(names, "antigravity")
	cliIdx := indexOfPlatform(names, "cliproxyapi")
	if grokIdx < 0 {
		t.Fatalf("grok not in registry: %v", names)
	}
	if antiIdx >= 0 && grokIdx < antiIdx {
		t.Errorf("grok (%d) should come after antigravity (%d)", grokIdx, antiIdx)
	}
	if cliIdx >= 0 && grokIdx > cliIdx {
		t.Errorf("grok (%d) should come before cliproxyapi (%d)", grokIdx, cliIdx)
	}
}

func indexOfPlatform(slice []string, target string) int {
	for i, value := range slice {
		if value == target {
			return i
		}
	}
	return -1
}
