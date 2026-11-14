package proxy

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/service"
)

func TestIsSessionScopedChannel(t *testing.T) {
	t.Run("session credential mode", func(t *testing.T) {
		ec := `{"credentialMode":"session"}`
		if !IsSessionScopedChannel(&ec, nil) {
			t.Error("expected session-scoped with credentialMode=session")
		}
	})

	t.Run("apikey credential mode, no oauth", func(t *testing.T) {
		ec := `{"credentialMode":"apikey"}`
		if IsSessionScopedChannel(&ec, nil) {
			t.Error("expected NOT session-scoped with credentialMode=apikey and no oauth")
		}
	})

	t.Run("apikey with oauth", func(t *testing.T) {
		ec := `{"credentialMode":"apikey"}`
		op := "claude"
		if !IsSessionScopedChannel(&ec, &op) {
			t.Error("expected session-scoped when oauth provider is present")
		}
	})

	t.Run("empty oauth provider", func(t *testing.T) {
		ec := `{"credentialMode":"apikey"}`
		op := ""
		if IsSessionScopedChannel(&ec, &op) {
			t.Error("expected NOT session-scoped with empty oauth provider")
		}
	})

	t.Run("nil configs", func(t *testing.T) {
		if IsSessionScopedChannel(nil, nil) {
			t.Error("expected NOT session-scoped with nil configs")
		}
	})

	t.Run("auto credential mode no oauth", func(t *testing.T) {
		ec := `{"credentialMode":"auto"}`
		if IsSessionScopedChannel(&ec, nil) {
			t.Error("expected NOT session-scoped with credentialMode=auto")
		}
	})

	t.Run("nil extraConfig with oauth", func(t *testing.T) {
		op := "claude"
		if !IsSessionScopedChannel(nil, &op) {
			t.Error("expected session-scoped when oauth provider present even with nil extraConfig")
		}
	})
}

func TestCredentialModeIntegration(t *testing.T) {
	t.Run("correct mode string from service", func(t *testing.T) {
		ec := `{"credentialMode":"session"}`
		mode := service.GetCredentialModeFromExtraConfig(&ec)
		if mode != service.CredentialModeSession {
			t.Errorf("expected session mode, got %q", mode)
		}
	})

	t.Run("auto mode string", func(t *testing.T) {
		ec := `{"credentialMode":"auto"}`
		mode := service.GetCredentialModeFromExtraConfig(&ec)
		if mode != "auto" {
			t.Errorf("expected auto mode, got %q", mode)
		}
	})

	t.Run("nil extra config", func(t *testing.T) {
		mode := service.GetCredentialModeFromExtraConfig(nil)
		if mode != "" {
			t.Errorf("expected empty mode for nil config, got %q", mode)
		}
	})
}
