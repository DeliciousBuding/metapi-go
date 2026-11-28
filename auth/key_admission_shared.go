package auth

import (
	"log/slog"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/sharedcount"
)

// sharedCounters bundles the optional multi-instance counters.
// Fail-open: Redis errors fall back to the process-local window.
// Both fields are typically the same RedisCounter instance (distinct key
// namespaces); either may be nil to keep that dimension memory-only.
type sharedCounters struct {
	rpm sharedcount.WindowCounter
	tpm sharedcount.WindowCounter
}

// SetSharedRPMCounter wires an optional multi-instance window counter.
// Pass nil to clear (memory-only). Safe to call at process startup.
func (l *KeyAdmissionLimiter) SetSharedRPMCounter(c sharedcount.WindowCounter) {
	if l == nil {
		return
	}
	l.cfgMu.Lock()
	defer l.cfgMu.Unlock()
	next := sharedCounters{}
	if cur := l.counters.Load(); cur != nil {
		next = *cur
	}
	next.rpm = c
	l.counters.Store(&next)
}

// SetSharedTPMCounter wires an optional multi-instance token counter.
// Pass nil to clear (memory-only). Safe to call at process startup.
// May reuse the same WindowCounter instance as SetSharedRPMCounter.
func (l *KeyAdmissionLimiter) SetSharedTPMCounter(c sharedcount.WindowCounter) {
	if l == nil {
		return
	}
	l.cfgMu.Lock()
	defer l.cfgMu.Unlock()
	next := sharedCounters{}
	if cur := l.counters.Load(); cur != nil {
		next = *cur
	}
	next.tpm = c
	l.counters.Store(&next)
}

// ConfigureSharedAdmissionFromRedisURL enables Redis-backed RPM+TPM counting when url is non-empty.
// Both counters share one RedisCounter instance (distinct key namespaces).
// On parse/dial setup failure, logs and keeps memory-only. Runtime Redis errors fail open.
func ConfigureSharedAdmissionFromRedisURL(redisURL string) {
	redisURL = strings.TrimSpace(redisURL)
	if redisURL == "" {
		GlobalKeyAdmission.SetSharedRPMCounter(nil)
		GlobalKeyAdmission.SetSharedTPMCounter(nil)
		return
	}
	rc, err := sharedcount.NewRedisCounter(redisURL)
	if err != nil {
		slog.Warn("redis admission: disabled (bad REDIS_URL)", "error", err)
		GlobalKeyAdmission.SetSharedRPMCounter(nil)
		GlobalKeyAdmission.SetSharedTPMCounter(nil)
		return
	}
	GlobalKeyAdmission.SetSharedRPMCounter(rc)
	GlobalKeyAdmission.SetSharedTPMCounter(rc)
	slog.Info("redis admission: shared RPM+TPM counters enabled")
}

func rpmSharedKey(keyID int64) string {
	return "metapi:rpm:" + formatInt64(keyID)
}

func tpmSharedKey(keyID int64) string {
	return "metapi:tpm:" + formatInt64(keyID)
}
