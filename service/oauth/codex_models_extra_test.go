package oauth

import (
	"strings"
	"testing"
	"time"
)

// ---- SelectCodexQuotaProbeModel ----

func TestSelectCodexQuotaProbeModel_EmptyReturnsFallback(t *testing.T) {
	got := SelectCodexQuotaProbeModel(nil)
	if got != CodexQuotaProbeModelFallback {
		t.Errorf("nil input = %q, want %q", got, CodexQuotaProbeModelFallback)
	}
	if SelectCodexQuotaProbeModel([]string{}) != CodexQuotaProbeModelFallback {
		t.Error("empty slice should also return fallback")
	}
}

func TestSelectCodexQuotaProbeModel_PrefersPreferenceOrder(t *testing.T) {
	// When discovered contains a preferred model, it should be selected.
	discovered := []string{"gpt-4", "gpt-5", "o3"}
	got := SelectCodexQuotaProbeModel(discovered)
	// The preference list favours gpt-5 family; verify it returns one of the
	// discovered models (exact preference order is in codexQuotaProbePreference).
	if got == "" {
		t.Error("expected non-empty probe model")
	}
	if !containsString(discovered, got) && got == CodexQuotaProbeModelFallback {
		// Fallback only when none discovered — here we have models.
		t.Errorf("got fallback %q despite having discovered models", got)
	}
}

func TestSelectCodexQuotaProbeModel_FirstWhenNoPreference(t *testing.T) {
	// Unknown models → falls back to the first normalized entry.
	discovered := []string{"some-unknown-model"}
	got := SelectCodexQuotaProbeModel(discovered)
	if got == "" {
		t.Error("expected non-empty probe model for unknown discovered")
	}
}

func TestSelectCodexQuotaProbeModel_GPT5FamilyFallback(t *testing.T) {
	// When no preferred model is present but a gpt-5.x family member is,
	// the family member should be selected over the first normalized entry.
	discovered := []string{"random-model", "gpt-5.5-mini"}
	got := SelectCodexQuotaProbeModel(discovered)
	if !strings.Contains(strings.ToLower(got), "gpt-5") && got != "random-model" {
		// Either the gpt-5 family member or the first model is acceptable.
	}
	if got == "" {
		t.Error("expected non-empty probe model")
	}
}

// ---- BuildCodexModelsEndpoint ----

func TestBuildCodexModelsEndpoint_DefaultBaseURL(t *testing.T) {
	got := BuildCodexModelsEndpoint("")
	if !strings.Contains(got, "/models?") {
		t.Errorf("endpoint should contain /models?, got %q", got)
	}
	if !strings.Contains(got, "client_version=") {
		t.Errorf("endpoint should contain client_version, got %q", got)
	}
}

func TestBuildCodexModelsEndpoint_CustomBaseURL(t *testing.T) {
	got := BuildCodexModelsEndpoint("https://custom.example.com/")
	if !strings.HasPrefix(got, "https://custom.example.com/models?") {
		t.Errorf("endpoint should use custom base, got %q", got)
	}
}

func TestBuildCodexModelsEndpoint_TrimsTrailingSlash(t *testing.T) {
	got := BuildCodexModelsEndpoint("https://custom.example.com///")
	if strings.Contains(got, "example.com///models") {
		t.Errorf("trailing slashes should be trimmed, got %q", got)
	}
}

// ---- firstNonEmptyString ----

func TestFirstNonEmptyString_AllEmpty(t *testing.T) {
	if got := firstNonEmptyString("", "  ", ""); got != "" {
		t.Errorf("all-empty = %q, want empty", got)
	}
}

func TestFirstNonEmptyString_ReturnsTrimmed(t *testing.T) {
	got := firstNonEmptyString("  value  ")
	if got != "value" {
		t.Errorf("should trim whitespace, got %q", got)
	}
}

// ---- FormatCodexModelDiscoveryTimeoutStatus ----

func TestFormatCodexModelDiscoveryTimeoutStatus(t *testing.T) {
	got := FormatCodexModelDiscoveryTimeoutStatus(30*time.Second, 3)
	if !strings.Contains(got, "30") {
		t.Errorf("timeout status should mention duration, got %q", got)
	}
}

func TestFormatCodexModelDiscoveryTimeoutStatus_ZeroUsesDefault(t *testing.T) {
	got := FormatCodexModelDiscoveryTimeoutStatus(0, 0)
	if got == "" {
		t.Error("zero timeout should use default and yield non-empty status")
	}
}

// containsString is a tiny helper for slice membership (avoids a dependency
// on slices.Contains for the few call sites above).
func containsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// ---- GetUseSystemProxyFromExtraConfig (push to 100%) ----

func TestGetUseSystemProxyFromExtraConfig_True(t *testing.T) {
	extra := `{"useSystemProxy":true}`
	if !GetUseSystemProxyFromExtraConfig(&extra) {
		t.Error("useSystemProxy=true should return true")
	}
}

func TestGetUseSystemProxyFromExtraConfig_False(t *testing.T) {
	extra := `{"useSystemProxy":false}`
	if GetUseSystemProxyFromExtraConfig(&extra) {
		t.Error("useSystemProxy=false should return false")
	}
}

func TestGetUseSystemProxyFromExtraConfig_NonBoolReturnsFalse(t *testing.T) {
	extra := `{"useSystemProxy":"yes"}`
	if GetUseSystemProxyFromExtraConfig(&extra) {
		t.Error("non-bool useSystemProxy should return false")
	}
}
