package admin

import (
	"strings"
	"testing"
)

func TestBuildCredentialExportProfiles_Content(t *testing.T) {
	ps := buildCredentialExportProfiles("n", "sk-abc", "https://gw.test")
	if len(ps) != 6 {
		t.Fatalf("len=%d", len(ps))
	}
	if ps[0]["id"] != "openai" {
		t.Fatalf("%v", ps[0])
	}
	content, _ := ps[0]["content"].(string)
	if !strings.Contains(content, "OPENAI_BASE_URL") || !strings.Contains(content, "sk-abc") {
		t.Fatalf("content=%q", content)
	}
	cherry, _ := ps[1]["content"].(map[string]any)
	if cherry["apiKey"] != "sk-abc" {
		t.Fatalf("cherry=%v", cherry)
	}
	claude, _ := ps[3]["content"].(string)
	if ps[3]["id"] != "claude-code" ||
		!strings.Contains(claude, "ANTHROPIC_BASE_URL") ||
		!strings.Contains(claude, "ANTHROPIC_AUTH_TOKEN") ||
		!strings.Contains(claude, "sk-abc") ||
		!strings.Contains(claude, "https://gw.test/v1") {
		t.Fatalf("claude=%q", claude)
	}
	codex, _ := ps[4]["content"].(string)
	if ps[4]["id"] != "codex" ||
		!strings.Contains(codex, `model_provider = "metapi"`) ||
		!strings.Contains(codex, "[model_providers.metapi]") ||
		!strings.Contains(codex, "base_url") ||
		!strings.Contains(codex, "sk-abc") {
		t.Fatalf("codex=%q", codex)
	}
	webui, _ := ps[5]["content"].(map[string]any)
	if ps[5]["id"] != "openwebui" ||
		webui["baseUrl"] != "https://gw.test/v1" ||
		webui["apiKey"] != "sk-abc" {
		t.Fatalf("openwebui=%v", webui)
	}
}
