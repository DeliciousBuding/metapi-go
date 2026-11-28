package auth

import (
	"sync"
	"time"
)

// admissionShards is the number of lock stripes guarding the process-local
// windows: keyID % admissionShards picks the stripe, so unrelated keys never wait
// on each other. No stripe is ever held across a shared-counter round trip.
const admissionShards = 64

// shardPadBytes pads admissionShard out to a 64-byte cache line
// (sync.Mutex 8 + map header 8 + 48) so a hot stripe does not bounce the line of
// its neighbours.
const shardPadBytes = 48

type admissionShard struct {
	mu   sync.Mutex
	keys map[int64]*keyWindow
	_    [shardPadBytes]byte
}

type keyWindow struct {
	// sharedMu serializes this key's shared-counter round trips. Only the shared
	// path takes it; the memory-only path and Snapshot never do.
	sharedMu sync.Mutex
	// request timestamps (unix ms) within the last 60s
	reqTimes []int64
	// token events: {atMs, tokens}
	tokenEvents []tokenEvent
}

type tokenEvent struct {
	atMs   int64
	tokens int64
}

// localWindow is a consistent snapshot of the process-local window taken under
// the shard lock. retryRPM/retryTPM are the Retry-After values the local deny
// paths report, computed from that same snapshot.
type localWindow struct {
	usedRPM  int64
	usedTPM  int64
	retryRPM time.Duration
	retryTPM time.Duration
}

// shard returns the lock stripe owning keyID.
func (l *KeyAdmissionLimiter) shard(keyID int64) *admissionShard {
	return &l.shards[uint64(keyID)%admissionShards]
}

// windowLocked returns (creating if needed) the window for keyID.
// Caller must hold sh.mu.
func (sh *admissionShard) windowLocked(keyID int64) *keyWindow {
	w := sh.keys[keyID]
	if w == nil {
		if sh.keys == nil {
			sh.keys = make(map[int64]*keyWindow)
		}
		w = &keyWindow{}
		sh.keys[keyID] = w
	}
	return w
}

// snapshotLocked prunes the window and reads local usage plus the Retry-After
// values the local deny paths would report. Caller must hold the shard lock.
func (w *keyWindow) snapshotLocked(windowStart, nowMs int64) localWindow {
	w.reqTimes = pruneTimes(w.reqTimes, windowStart)
	w.tokenEvents = pruneTokenEvents(w.tokenEvents, windowStart)
	return localWindow{
		usedRPM:  int64(len(w.reqTimes)),
		usedTPM:  sumTokens(w.tokenEvents),
		retryRPM: time.Duration(retryAfterMs(w.reqTimes, nowMs)) * time.Millisecond,
		retryTPM: time.Duration(retryAfterTokenMs(w.tokenEvents, nowMs)) * time.Millisecond,
	}
}

// reserveLocked records an admitted request in the process-local window.
// Caller must hold the shard lock.
func (w *keyWindow) reserveLocked(nowMs, tpmLimit, estimatedTokens int64) {
	w.reqTimes = append(w.reqTimes, nowMs)
	if estimatedTokens > 0 && tpmLimit > 0 {
		w.tokenEvents = append(w.tokenEvents, tokenEvent{atMs: nowMs, tokens: estimatedTokens})
	}
}

func (sh *admissionShard) allowLocal(keyID, windowStart, nowMs, rpmLimit, tpmLimit, estimatedTokens int64) AdmissionDecision {
	// Memory-only: nothing to move out of the lock, so decide and reserve in a
	// single shard-lock hold (exact under concurrency, as before).
	sh.mu.Lock()
	defer sh.mu.Unlock()
	w := sh.windowLocked(keyID)
	loc := w.snapshotLocked(windowStart, nowMs)
	if rpmLimit > 0 && loc.usedRPM >= rpmLimit {
		return AdmissionDecision{
			Allowed:    false,
			Reason:     "over_rpm",
			RetryAfter: loc.retryRPM,
			UsedRPM:    loc.usedRPM,
			UsedTPM:    loc.usedTPM,
		}
	}
	if tpmLimit > 0 && estimatedTokens > 0 && loc.usedTPM+estimatedTokens > tpmLimit {
		return AdmissionDecision{
			Allowed:    false,
			Reason:     "over_tpm",
			RetryAfter: loc.retryTPM,
			UsedRPM:    loc.usedRPM,
			UsedTPM:    loc.usedTPM,
		}
	}
	// reserve (process-local known limitation for Snapshot)
	w.reserveLocked(nowMs, tpmLimit, estimatedTokens)
	return AdmissionDecision{
		Allowed: true,
		UsedRPM: loc.usedRPM + 1,
		UsedTPM: loc.usedTPM + max64z(estimatedTokens, 0),
	}
}

// snapshot reads the local window for w under the shard lock.
func (sh *admissionShard) snapshot(w *keyWindow, windowStart, nowMs int64) localWindow {
	sh.mu.Lock()
	defer sh.mu.Unlock()
	return w.snapshotLocked(windowStart, nowMs)
}

// Snapshot returns current window usage for admin display.
func (l *KeyAdmissionLimiter) Snapshot(keyID int64) (usedRPM, usedTPM int64) {
	if l == nil || keyID <= 0 {
		return 0, 0
	}
	nowMs := l.nowFn().UTC().UnixMilli()
	windowStart := nowMs - 60_000
	sh := l.shard(keyID)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	w := sh.keys[keyID]
	if w == nil {
		return 0, 0
	}
	loc := w.snapshotLocked(windowStart, nowMs)
	return loc.usedRPM, loc.usedTPM
}

func pruneTimes(in []int64, windowStart int64) []int64 {
	if len(in) == 0 {
		return in
	}
	i := 0
	for i < len(in) && in[i] < windowStart {
		i++
	}
	if i == 0 {
		return in
	}
	out := make([]int64, len(in)-i)
	copy(out, in[i:])
	return out
}

func pruneTokenEvents(in []tokenEvent, windowStart int64) []tokenEvent {
	if len(in) == 0 {
		return in
	}
	i := 0
	for i < len(in) && in[i].atMs < windowStart {
		i++
	}
	if i == 0 {
		return in
	}
	out := make([]tokenEvent, len(in)-i)
	copy(out, in[i:])
	return out
}

func sumTokens(in []tokenEvent) int64 {
	var s int64
	for _, e := range in {
		s += e.tokens
	}
	return s
}

func retryAfterMs(times []int64, nowMs int64) int64 {
	if len(times) == 0 {
		return 1000
	}
	// oldest event leaves the window after (oldest+60s - now)
	oldest := times[0]
	remain := (oldest + 60_000) - nowMs
	if remain < 1000 {
		return 1000
	}
	return remain
}

func retryAfterTokenMs(events []tokenEvent, nowMs int64) int64 {
	if len(events) == 0 {
		return 1000
	}
	oldest := events[0].atMs
	remain := (oldest + 60_000) - nowMs
	if remain < 1000 {
		return 1000
	}
	return remain
}

func max64z(v, floor int64) int64 {
	if v < floor {
		return floor
	}
	return v
}
