package auth

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/sharedcount"
)

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
// ctx bounds the shared-counter round trips: a client that goes away cancels its
// own Redis wait instead of holding the per-key serialization mutex for the full
// counter timeout. Pass the request context (nil is treated as Background). A
// cancelled round trip is an error, so it takes the existing fail-open path —
// the request is already gone, and the window self-heals on expiry.
//
// Shared-counter round trips never run under a shard lock, so a slow or
// unreachable Redis costs latency only for its own key. Decisions keep the exact
// order they had under the previous single lock: shared RPM -> local RPM fallback
// -> shared TPM -> local TPM fallback -> reservation.
func (l *KeyAdmissionLimiter) Allow(ctx context.Context, keyID int64, maxRPM, maxTPM *int64, estimatedTokens int64) AdmissionDecision {
	if l == nil || keyID <= 0 {
		return AdmissionDecision{Allowed: true}
	}
	if ctx == nil {
		ctx = context.Background()
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
		return sh.allowLocal(keyID, windowStart, nowMs, rpmLimit, tpmLimit, estimatedTokens)
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
