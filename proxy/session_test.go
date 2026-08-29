package proxy

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
)

func newTestCoordinator() *ProxyChannelCoordinator {
	cfg := config.Load(map[string]string{
		"PORT":                                   "8080",
		"PROXY_SESSION_CHANNEL_CONCURRENCY_LIMIT": "2",
	})
	config.Set(cfg)
	return NewProxyChannelCoordinator()
}

func sessionScopedConfig() (*string, *string) {
	ec := `{"credentialMode":"session"}`
	op := "claude"
	return &ec, &op
}

func nonSessionScopedConfig() (*string, *string) {
	ec := `{"credentialMode":"apikey"}`
	return &ec, nil
}

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

func TestChannelLoadSnapshot(t *testing.T) {
	coord := newTestCoordinator()
	ec, op := sessionScopedConfig()

	snap := coord.GetChannelLoadSnapshot(800, ec, op)
	if !snap.SessionScoped {
		t.Error("expected session-scoped in snapshot")
	}
	if snap.ConcurrencyLimit != 2 {
		t.Errorf("expected limit=2, got %d", snap.ConcurrencyLimit)
	}
	if snap.ActiveLeaseCount != 0 {
		t.Errorf("expected 0 active (leases deferred), got %d", snap.ActiveLeaseCount)
	}
	if snap.WaitingCount != 0 {
		t.Errorf("expected 0 waiting (leases deferred), got %d", snap.WaitingCount)
	}
	if snap.Saturated {
		t.Error("expected not saturated")
	}

	nonEC, nonOP := nonSessionScopedConfig()
	nonSnap := coord.GetChannelLoadSnapshot(800, nonEC, nonOP)
	if nonSnap.SessionScoped {
		t.Error("expected NOT session-scoped in snapshot")
	}
	if nonSnap.ConcurrencyLimit != 0 {
		t.Errorf("expected limit=0 for non-session-scoped channel, got %d", nonSnap.ConcurrencyLimit)
	}
}

func TestGetActiveChannelIDs(t *testing.T) {
	coord := newTestCoordinator()
	ids := coord.GetActiveChannelIDs()
	if len(ids) != 0 {
		t.Errorf("expected 0 active channels (leases deferred), got %d", len(ids))
	}
}

func TestCoordinatorReset(t *testing.T) {
	coord := newTestCoordinator()
	coord.Reset()
	ids := coord.GetActiveChannelIDs()
	if len(ids) != 0 {
		t.Errorf("expected 0 active after reset")
	}
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
