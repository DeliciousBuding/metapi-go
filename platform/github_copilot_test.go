package platform

import (
	"context"
	"strings"
	"testing"
)

func TestGitHubCopilotAdapter_Detect(t *testing.T) {
	adapter := GetAdapter("github-copilot")
	if adapter == nil {
		t.Fatal("github-copilot adapter not registered")
	}
	tests := []struct {
		url  string
		want bool
	}{
		{"https://api.githubcopilot.com/chat/completions", true},
		{"https://api.github.com/copilot_internal/v2/token", true},
		{"https://api.github.com/user", false},
		{"https://github.com/login/device/code", false},
	}
	for _, tc := range tests {
		got, err := adapter.Detect(context.Background(), tc.url)
		if err != nil {
			t.Fatalf("Detect(%q): %v", tc.url, err)
		}
		if got != tc.want {
			t.Errorf("Detect(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestGitHubCopilotAdapter_Aliases(t *testing.T) {
	for _, input := range []string{"github-copilot", "github copilot", "copilot"} {
		adapter := GetAdapter(input)
		if adapter == nil || adapter.PlatformName() != "github-copilot" {
			t.Errorf("GetAdapter(%q) = %v", input, adapter)
		}
	}
}

func TestGitHubCopilotAdapter_SeedModels(t *testing.T) {
	adapter := GetAdapter("github-copilot")
	if adapter == nil {
		t.Fatal("github-copilot adapter not registered")
	}
	models, err := adapter.GetModels(context.Background(), "https://api.githubcopilot.com", "", nil, nil)
	if err != nil {
		t.Fatalf("GetModels: %v", err)
	}
	if len(models) != len(copilotSeedModels) {
		t.Fatalf("GetModels returned %d models, want %d", len(models), len(copilotSeedModels))
	}
	for _, model := range models {
		if strings.TrimSpace(model) == "" || strings.Contains(model, "*") {
			t.Errorf("invalid seed model %q", model)
		}
	}
}

func TestGitHubCopilotAdapter_LoginUnsupported(t *testing.T) {
	adapter := GetAdapter("github-copilot")
	if adapter == nil {
		t.Fatal("github-copilot adapter not registered")
	}
	result, err := adapter.Login(context.Background(), "https://api.githubcopilot.com", "u", "p", nil, nil)
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if result.Success {
		t.Error("GitHub Copilot login must be managed by Device OAuth")
	}
	if result.Message != "github copilot oauth login is managed via Device OAuth flow" {
		t.Errorf("Login message = %q", result.Message)
	}
}
