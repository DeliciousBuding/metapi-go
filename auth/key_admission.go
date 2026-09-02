package auth

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/sharedcount"
)

// admissionShards is the number of lock stripes guarding the process-local
// windows: keyID % admissionShards picks the stripe, so unrelated keys never wait
// on each other. No stripe is ever held across a shared-counter round trip.
const admissionShards = 64

// shardPadBytes pads admissionShard out to a 64-byte cache line
// (sync.Mutex 8 + map header 8 + 48) so a hot stripe does not bounce the line of
// its neighbours.
const shardPadBytes = 48

// KeyAdmissionLimiter is an in-process sliding-window RPM/TPM gate for
// managed downstream keys. Default unlimited when limits are nil/<=0.
// Multi-instance deployments optionally share RPM/TPM via WindowCounter.
// Snapshot() remains process-local (known limitation).
//
// Locking model:
//   - shards[keyID%admissionShards].mu guards the local window data and is only
//     ever held for in-memory work (prune / check / append);
//   - keyWindow.sharedMu serializes the shared-counter round trips of one key, so
//     an Incr and its compensating Decr can never be reordered by a concurrent
//     request for the same key;
//   - nesting is always sharedMu -> shard mu, never the reverse (the shard lock is
//     released before sharedMu is taken), so slow Redis I/O blocks neither another
//     key nor a shard, and the two locks cannot deadlock.
type KeyAdmissionLimiter struct {
	shards [admissionShards]admissionShard
	// cfgMu serializes the read-modify-write of counters in the setters.
	cfgMu sync.Mutex
	// counters is swapped as a whole so Allow reads RPM+TPM with one lock-free
	// load. nil = memory-only.
	counters atomic.Pointer[sharedCounters]
	// nowFn is injectable for tests.
	nowFn func() time.Time
}

// sharedCounters bundles the optional multi-instance counters.
// Fail-open: Redis errors fall back to the process-local window.
// Both fields are typically the same RedisCounter instance (distinct key
// namespaces); either may be nil to keep that dimension memory-only.
type sharedCounters struct {
	rpm sharedcount.WindowCounter
	tpm sharedcount.WindowCounter
}

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

// sharedVerdict is the outcome of one dimension's shared-counter round trip.
type sharedVerdict struct {
	// counted reports that the shared counter answered, which makes its count
	// authoritative and skips the local check for that dimension.
	counted bool
	count   int64
}

// GlobalKeyAdmission is the process-wide limiter used by ProxyAuth.
var GlobalKeyAdmission = NewKeyAdmissionLimiter()

// NewKeyAdmissionLimiter creates an empty limiter.
func NewKeyAdmissionLimiter() *KeyAdmissionLimiter {
	return &KeyAdmissionLimiter{
		nowFn: time.Now,
	}
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

// ResetKeyAdmissionForTest clears the global limiter state.
func ResetKeyAdmissionForTest() {
	GlobalKeyAdmission = NewKeyAdmissionLimiter()
}

// AdmissionDecision is the result of Allow.
type AdmissionDecision struct {
	Allowed    bool
	Reason     string // "" | "over_rpm" | "over_tpm"
	RetryAfter time.Duration
	UsedRPM    int64
	UsedTPM    int64
}

// Allow checks and records a request against optional RPM/TPM limits.
// estimatedTokens is reserved against TPM when maxTPM is set; pass 0 to skip TPM accounting.
// When allowed, the request is recorded immediately (admission reservation).
//
// Shared-counter round trips never run under a shard lock, so a slow or
// unreachable Redis costs latency only for its own key. Decisions keep the exact
// order they had under the previous single lock: shared RPM -> local RPM fallback
// -> shared TPM -> local TPM fallback -> reservation.
func (l *KeyAdmissionLimiter) Allow(keyID int64, maxRPM, maxTPM *int64, estimatedTokens int64) AdmissionDecision {
	if l == nil || keyID <= 0 {
		return AdmissionDecision{Allowed: true}
	}
	rpmLimit := int64(0)
	tpmLimit := int64(0)
	if maxRPM != nil && *maxRPM > 0 {
		rpmLimit = *maxRPM
	}
	if maxTPM != nil && *maxTPM > 0 {
		tpmLimit = *maxTPM
	}
	if rpmLimit == 0 && tpmLimit == 0 {
		return AdmissionDecision{Allowed: true}
	}

	now := l.nowFn().UTC()
	nowMs := now.UnixMilli()
	windowStart := nowMs - 60_000

	var rpmCounter, tpmCounter sharedcount.WindowCounter
	if sc := l.counters.Load(); sc != nil {
		rpmCounter, tpmCounter = sc.rpm, sc.tpm
	}
	useSharedRPM := rpmLimit > 0 && rpmCounter != nil
	useSharedTPM := tpmLimit > 0 && estimatedTokens > 0 && tpmCounter != nil

	sh := l.shard(keyID)
	if !useSharedRPM && !useSharedTPM {
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

	// Take the window and read local usage, then release the shard lock: every
	// Redis round trip below happens with no shard lock held.
	sh.mu.Lock()
	w := sh.windowLocked(keyID)
	loc := w.snapshotLocked(windowStart, nowMs)
	sh.mu.Unlock()

	// Serialize the shared round trips for this key only. Holding it across the
	// local fallback checks keeps check-and-reserve atomic for this key, which is
	// what the old global lock gave us.
	w.sharedMu.Lock()
	defer w.sharedMu.Unlock()

	ctx := context.Background()
	rpmKey := rpmSharedKey(keyID)
	tpmKey := tpmSharedKey(keyID)
	var rpm, tpm sharedVerdict

	// Optional multi-instance RPM. Fail-open on errors.
	// On deny, compensating Decr so denied requests do not occupy the window.
	if useSharedRPM {
		n, err := rpmCounter.Incr(ctx, rpmKey, time.Minute)
		if err != nil {
			slog.Debug("redis admission: fail-open on error", "key_id", keyID, "error", err)
		} else if n > rpmLimit {
			if _, rerr := rpmCounter.Decr(ctx, rpmKey, time.Minute); rerr != nil {
				slog.Debug("redis admission: rpm rollback failed", "key_id", keyID, "error", rerr)
			}
			return AdmissionDecision{
				Allowed:    false,
				Reason:     "over_rpm",
				RetryAfter: time.Second,
				UsedRPM:    n,
				UsedTPM:    loc.usedTPM,
			}
		} else {
			rpm = sharedVerdict{counted: true, count: n}
		}
	}

	// Local RPM verdict, only when the shared counter did not answer for RPM.
	// Re-read under the shard lock — the snapshot above is stale once we have
	// waited on Redis. Denying here happens before the TPM round trip, matching
	// the old order: an RPM deny never reserves TPM.
	if !rpm.counted && rpmLimit > 0 {
		loc = sh.snapshot(w, windowStart, nowMs)
		if loc.usedRPM >= rpmLimit {
			return AdmissionDecision{
				Allowed:    false,
				Reason:     "over_rpm",
				RetryAfter: loc.retryRPM,
				UsedRPM:    loc.usedRPM,
				UsedTPM:    loc.usedTPM,
			}
		}
	}

	// Optional multi-instance TPM. Fail-open on errors.
	// On deny, roll back TPM (and RPM if already reserved) so the window stays free.
	if useSharedTPM {
		n, err := tpmCounter.IncrBy(ctx, tpmKey, estimatedTokens, time.Minute)
		if err != nil {
			slog.Debug("redis admission tpm: fail-open on error", "key_id", keyID, "error", err)
		} else if n > tpmLimit {
			if _, rerr := tpmCounter.IncrBy(ctx, tpmKey, -estimatedTokens, time.Minute); rerr != nil {
				slog.Debug("redis admission: tpm rollback failed", "key_id", keyID, "error", rerr)
			}
			if rpm.counted {
				if _, rerr := rpmCounter.Decr(ctx, rpmKey, time.Minute); rerr != nil {
					slog.Debug("redis admission: rpm rollback after tpm deny failed", "key_id", keyID, "error", rerr)
				}
			}
			usedRPM := loc.usedRPM
			if rpm.counted {
				usedRPM = rpm.count
			}
			return AdmissionDecision{
				Allowed:    false,
				Reason:     "over_tpm",
				RetryAfter: time.Second,
				UsedRPM:    usedRPM,
				UsedTPM:    n,
			}
		} else {
			tpm = sharedVerdict{counted: true, count: n}
		}
	}

	// Local TPM verdict (when the shared counter did not answer for TPM) plus the
	// reservation, in one shard-lock hold so check-and-record stay atomic.
	sh.mu.Lock()
	defer sh.mu.Unlock()
	loc = w.snapshotLocked(windowStart, nowMs)
	usedRPM, usedTPM := loc.usedRPM, loc.usedTPM
	if rpm.counted {
		usedRPM = rpm.count
	}
	if tpm.counted {
		usedTPM = tpm.count
	} else if tpmLimit > 0 && estimatedTokens > 0 && usedTPM+estimatedTokens > tpmLimit {
		return AdmissionDecision{
			Allowed:    false,
			Reason:     "over_tpm",
			RetryAfter: loc.retryTPM,
			UsedRPM:    usedRPM,
			UsedTPM:    usedTPM,
		}
	}
	// reserve (process-local known limitation for Snapshot)
	w.reserveLocked(nowMs, tpmLimit, estimatedTokens)
	outRPM := usedRPM
	if !rpm.counted {
		outRPM = usedRPM + 1
	}
	outTPM := usedTPM
	if !tpm.counted {
		outTPM = usedTPM + max64z(estimatedTokens, 0)
	}
	return AdmissionDecision{
		Allowed: true,
		UsedRPM: outRPM,
		UsedTPM: outTPM,
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

func rpmSharedKey(keyID int64) string {
	return "metapi:rpm:" + formatInt64(keyID)
}

func tpmSharedKey(keyID int64) string {
	return "metapi:tpm:" + formatInt64(keyID)
}
