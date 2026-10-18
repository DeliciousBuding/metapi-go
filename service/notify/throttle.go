package notify

import (
	"strings"
	"sync"
)

// NotificationThrottleState tracks the last sent timestamp and suppressed count.
type NotificationThrottleState struct {
	LastSentAtMs    int64
	SuppressedCount int
}

// DefaultMaxThrottleEntries bounds the throttle map. Signatures can embed
// request-derived values (model names), so an attacker could otherwise grow
// this map without limit for the lifetime of the process.
const DefaultMaxThrottleEntries = 4096

// NotificationThrottle is a global throttle for notification deduplication.
type NotificationThrottle struct {
	mu         sync.Mutex
	state      map[string]*NotificationThrottleState
	maxEntries int
}

// NewNotificationThrottle creates a new throttle instance.
func NewNotificationThrottle() *NotificationThrottle {
	return &NotificationThrottle{
		state:      make(map[string]*NotificationThrottleState),
		maxEntries: DefaultMaxThrottleEntries,
	}
}

// SetMaxEntries overrides the entry cap (tests use a small value).
func (t *NotificationThrottle) SetMaxEntries(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.maxEntries = n
}

// enforceCapLocked evicts a slice of arbitrary entries once the map is full.
// Exact LRU would make every insert O(n); evicting a fraction keeps inserts
// amortized O(1) and only costs at most a few extra notifications under
// adversarial cardinality.
func (t *NotificationThrottle) enforceCapLocked() {
	if t.maxEntries <= 0 || len(t.state) < t.maxEntries {
		return
	}
	evict := len(t.state) / 8
	if evict < 1 {
		evict = 1
	}
	for key := range t.state {
		delete(t.state, key)
		evict--
		if evict <= 0 {
			break
		}
	}
}

// Default global throttle instance (same as TS module-level map).
var GlobalThrottle = NewNotificationThrottle()

// CreateNotificationSignature creates a signature from title, message, and level.
// Format: "level||title||message" (three fields joined by ||).
// Mirrors TS createNotificationSignature().
func CreateNotificationSignature(title, message, level string) string {
	level = strings.TrimSpace(strings.ToLower(level))
	title = strings.TrimSpace(title)
	message = strings.TrimSpace(message)
	return level + "||" + title + "||" + message
}

// EvaluateResult is the result of a throttle evaluation.
type EvaluateResult struct {
	ShouldSend  bool
	MergedCount int
}

// EvaluateNotificationThrottle checks whether a notification should be sent
// based on the cooldown period.
// Mirrors TS evaluateNotificationThrottle().
func (t *NotificationThrottle) EvaluateNotificationThrottle(signature string, nowMs int64, cooldownMs int64) EvaluateResult {
	if cooldownMs <= 0 {
		return EvaluateResult{ShouldSend: true, MergedCount: 0}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	current, exists := t.state[signature]
	if !exists {
		t.enforceCapLocked()
		t.state[signature] = &NotificationThrottleState{
			LastSentAtMs:    nowMs,
			SuppressedCount: 0,
		}
		return EvaluateResult{ShouldSend: true, MergedCount: 0}
	}

	if nowMs-current.LastSentAtMs < cooldownMs {
		current.SuppressedCount++
		return EvaluateResult{ShouldSend: false, MergedCount: 0}
	}

	mergedCount := current.SuppressedCount
	t.state[signature] = &NotificationThrottleState{
		LastSentAtMs:    nowMs,
		SuppressedCount: 0,
	}
	return EvaluateResult{ShouldSend: true, MergedCount: mergedCount}
}

// PruneNotificationThrottleState removes stale entries older than staleMs.
// Mirrors TS pruneNotificationThrottleState().
func (t *NotificationThrottle) PruneNotificationThrottleState(nowMs int64, staleMs int64) {
	if staleMs <= 0 {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for key, state := range t.state {
		if nowMs-state.LastSentAtMs > staleMs {
			delete(t.state, key)
		}
	}
}
