package routing

import (
	"sort"
	"sync"
)

// ---- K1b: in-process model redirect registry ----

// Design: k1-model-redirect-design-2026-08-01.md §7. Per-account
// canonical→actual redirects (from model_name_redirects) are loaded into a
// lock-free-swap registry so the hot path (eligibility + forward rewrite) is
// an O(1) map lookup with no DB access.

// A redirect says: when a client requests `canonical` on this account, the
// upstream actually exposes it as `actual`. Registry consumers:
// - eligibility (matcher): a channel whose source_model is `actual` now
// matches a request for `canonical` (via the reverse index);
// - forward rewrite (selector): the outbound model becomes `actual`, while
// proxy_logs attribution keeps `canonical` (billing stays ratio-based on
// the requested/attribution name).

var (
	redirectMu        sync.RWMutex
	redirectByAccount map[int64]map[string]string // accountID → canonical → actual
	redirectActualRev map[int64]map[string]string // accountID → actual → canonical (derived)
)

// SetModelRedirects atomically replaces the whole registry. Pass nil to clear.
// Each account's map is canonical→actual; the reverse index is derived here
// so callers never pass inconsistent data.
func SetModelRedirects(byAccount map[int64]map[string]string) {
	normalized := make(map[int64]map[string]string, len(byAccount))
	reversed := make(map[int64]map[string]string, len(byAccount))
	for accountID, entries := range byAccount {
		fwd := make(map[string]string, len(entries))
		rev := make(map[string]string, len(entries))
		// Deterministic reverse-index selection: when several canonicals map
		// to the same actual (manual rows can collide), the lexicographically
		// smallest canonical wins — map iteration order is random in Go and
		// must never decide eligibility.
		canonicals := make([]string, 0, len(entries))
		for canonical := range entries {
			if canonical != "" && entries[canonical] != "" {
				canonicals = append(canonicals, canonical)
			}
		}
		sort.Strings(canonicals)
		for _, canonical := range canonicals {
			actual := entries[canonical]
			fwd[canonical] = actual
			if _, exists := rev[actual]; !exists {
				rev[actual] = canonical
			}
		}
		if len(fwd) > 0 {
			normalized[accountID] = fwd
			reversed[accountID] = rev
		}
	}

	redirectMu.Lock()
	redirectByAccount = normalized
	redirectActualRev = reversed
	redirectMu.Unlock()
}

// ModelRedirectActual returns the actual (upstream) name for a canonical model
// on a given account, or "" when no redirect applies. Hot path, no locks held
// after the read.
func ModelRedirectActual(accountID int64, canonical string) string {
	redirectMu.RLock()
	defer redirectMu.RUnlock()
	if redirectByAccount == nil {
		return ""
	}
	entries := redirectByAccount[accountID]
	if entries == nil {
		return ""
	}
	return entries[canonical]
}

// ModelRedirectCanonical returns the canonical name for an actual (upstream)
// model on a given account, or "" when no redirect applies. Used by
// eligibility: a channel exposing `actual` supports a request for `canonical`.
func ModelRedirectCanonical(accountID int64, actual string) string {
	redirectMu.RLock()
	defer redirectMu.RUnlock()
	if redirectActualRev == nil {
		return ""
	}
	entries := redirectActualRev[accountID]
	if entries == nil {
		return ""
	}
	return entries[actual]
}
