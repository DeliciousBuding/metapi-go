package routing

import (
	"testing"
	"time"
)

func TestChannelRuntimeStatus(t *testing.T) {
	now := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339)

	t.Run("disabled maps to manually_disabled", func(t *testing.T) {
		if got := ChannelRuntimeStatus(1, "gpt-4o", false, nil); got != ChannelStatusManuallyDisabled {
			t.Fatalf("status = %s, want %s", got, ChannelStatusManuallyDisabled)
		}
	})

	t.Run("healthy enabled", func(t *testing.T) {
		if got := ChannelRuntimeStatus(9999, "gpt-4o", true, nil); got != ChannelStatusEnabled {
			t.Fatalf("status = %s, want %s", got, ChannelStatusEnabled)
		}
	})

	t.Run("active cooldown", func(t *testing.T) {
		if got := ChannelRuntimeStatus(9998, "gpt-4o", true, &now); got != ChannelStatusCooldown {
			t.Fatalf("status = %s, want %s", got, ChannelStatusCooldown)
		}
	})

	t.Run("expired cooldown is enabled", func(t *testing.T) {
		if got := ChannelRuntimeStatus(9997, "gpt-4o", true, &past); got != ChannelStatusEnabled {
			t.Fatalf("status = %s, want %s", got, ChannelStatusEnabled)
		}
	})

	t.Run("open site breaker", func(t *testing.T) {
		future := time.Now().UTC().Add(time.Hour).UnixMilli()
		healthStateMu.Lock()
		siteRuntimeHealthStates[9001] = &SiteRuntimeHealthState{BreakerLevel: 1, BreakerUntilMs: &future}
		healthStateMu.Unlock()
		t.Cleanup(func() {
			healthStateMu.Lock()
			delete(siteRuntimeHealthStates, 9001)
			healthStateMu.Unlock()
		})
		if got := ChannelRuntimeStatus(9001, "gpt-4o", true, nil); got != ChannelStatusBreakerOpen {
			t.Fatalf("status = %s, want %s", got, ChannelStatusBreakerOpen)
		}
	})

	t.Run("cooldown beats healthy but breaker wins over cooldown", func(t *testing.T) {
		future := time.Now().UTC().Add(time.Hour).UnixMilli()
		healthStateMu.Lock()
		siteRuntimeHealthStates[9002] = &SiteRuntimeHealthState{BreakerLevel: 1, BreakerUntilMs: &future}
		healthStateMu.Unlock()
		t.Cleanup(func() {
			healthStateMu.Lock()
			delete(siteRuntimeHealthStates, 9002)
			healthStateMu.Unlock()
		})
		if got := ChannelRuntimeStatus(9002, "gpt-4o", true, &now); got != ChannelStatusBreakerOpen {
			t.Fatalf("status = %s, want %s", got, ChannelStatusBreakerOpen)
		}
	})
}
