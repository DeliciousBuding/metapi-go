package config

import "testing"

func TestAntigravityClientSecretRequiresExplicitConfiguration(t *testing.T) {
	for _, value := range []string{"", "  ", "fixture-client-material"} {
		cfg, _ := Load(map[string]string{"ANTIGRAVITY_CLIENT_SECRET": value})
		want := value
		if value == "  " {
			want = ""
		}
		if cfg.AntigravityClientSecret != want {
			t.Fatal("Antigravity client configuration acquired a default or changed")
		}
	}
}
