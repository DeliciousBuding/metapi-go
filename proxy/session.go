package proxy

import (
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/service"
)

// ChannelLoadSnapshot is a snapshot of channel load.
type ChannelLoadSnapshot struct {
	ChannelID        int64
	SessionScoped    bool
	ConcurrencyLimit int
	ActiveLeaseCount int
	WaitingCount     int
	LoadRatio        float64
	Saturated        bool
}

// ProxyChannelCoordinator tracks per-channel session load for routing.
//
// Sticky-session leases (AcquireChannelLease / BindStickyChannel, the waiter
// queue, and lease keepalive/expiry goroutines) are DEFERRED: no caller in the
// proxy acquires leases today, so that machinery was removed in
// chore/rm-dead-proxy-routing and can be reintroduced from git history when
// sticky sessions are wired up. Until then every load snapshot reports zero
// active leases and zero waiters.
type ProxyChannelCoordinator struct{}

// NewProxyChannelCoordinator creates a new coordinator. The session-channel
// concurrency limit is read from the runtime-settings snapshot on every
// acquire so a settings change hot-applies without a restart.
func NewProxyChannelCoordinator() *ProxyChannelCoordinator {
	return &ProxyChannelCoordinator{}
}

// GetActiveChannelIDs returns channel IDs with active leases.
// Leases are deferred (see ProxyChannelCoordinator), so this is always empty.
func (c *ProxyChannelCoordinator) GetActiveChannelIDs() []int64 {
	return nil
}

// GetChannelLoadSnapshot returns a snapshot of a channel's load.
// Leases are deferred (see ProxyChannelCoordinator), so active and waiting
// counts are always zero.
func (c *ProxyChannelCoordinator) GetChannelLoadSnapshot(
	channelID int64,
	extraConfig *string,
	oauthProvider *string,
) ChannelLoadSnapshot {
	scoped := isSessionScopedChannel(extraConfig, oauthProvider)
	limit := c.channelConcurrencyLimit(extraConfig, oauthProvider)
	return ChannelLoadSnapshot{
		ChannelID:        channelID,
		SessionScoped:    scoped,
		ConcurrencyLimit: limit,
	}
}

func (c *ProxyChannelCoordinator) channelConcurrencyLimit(extraConfig *string, oauthProvider *string) int {
	if !isSessionScopedChannel(extraConfig, oauthProvider) {
		return 0
	}
	limit := int(config.Runtime().ProxySessionChannelConcurrencyLimit)
	if limit < 0 {
		return 0
	}
	return limit
}

// IsSessionScopedChannel checks if a channel uses session-scoped credentials.
func IsSessionScopedChannel(extraConfig *string, oauthProvider *string) bool {
	return isSessionScopedChannel(extraConfig, oauthProvider)
}

func isSessionScopedChannel(extraConfig *string, oauthProvider *string) bool {
	if service.GetCredentialModeFromExtraConfig(extraConfig) == service.CredentialModeSession {
		return true
	}
	if oauthProvider != nil && *oauthProvider != "" {
		return true
	}
	return false
}

// Reset resets all state (for testing).
func (c *ProxyChannelCoordinator) Reset() {}
