package pricingcatalog

import "testing"

func TestClassifySite(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		url      string
		want     SiteClass
	}{
		{"openai official", "openai", "https://api.openai.com/v1", SiteClassOfficial},
		{"openai relay", "openai", "https://relay.example.com/v1", SiteClassRelay},
		{"openai suffix-squatting host", "openai", "https://api.openai.com.evil.example/v1", SiteClassRelay},
		{"claude official", "claude", "https://api.anthropic.com/v1", SiteClassOfficial},
		{"gemini official", "gemini", "https://generativelanguage.googleapis.com/v1beta", SiteClassOfficial},
		{"codex official openai host", "codex", "https://api.openai.com/codex", SiteClassOfficial},
		{"codex official chatgpt host", "codex", "https://chatgpt.com/backend-api/codex", SiteClassOfficial},
		{"grok official", "grok", "https://api.x.ai/v1", SiteClassOfficial},
		{"newapi relay", "new-api", "https://api.openai.com/v1", SiteClassRelay},
		{"unknown platform", "mystery", "https://api.openai.com/v1", SiteClassRelay},
		{"empty platform", "", "https://api.openai.com/v1", SiteClassRelay},
		{"empty url", "openai", "", SiteClassRelay},
		{"empty both", "", "", SiteClassRelay},
		{"scheme-less host", "openai", "api.openai.com/v1", SiteClassOfficial},
		{"scheme-less relay", "openai", "relay.example.com/v1", SiteClassRelay},
		{"uppercase host", "openai", "HTTPS://API.OPENAI.COM/v1", SiteClassOfficial},
		{"port on official host", "openai", "https://api.openai.com:443/v1", SiteClassOfficial},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifySite(SiteSnapshot{Platform: tt.platform, URL: tt.url}); got != tt.want {
				t.Errorf("ClassifySite(%q, %q) = %q, want %q", tt.platform, tt.url, got, tt.want)
			}
		})
	}
}
