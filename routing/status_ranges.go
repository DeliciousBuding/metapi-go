// metapi-go/routing — operator-tunable status-code range policy
// (competitor-study-2026-08 P1-2).
//
// Two runtime settings (settings table, live-applied via PUT
// /api/settings/runtime and rehydrated at startup) replace the status
// verdicts that were hardcoded in proxy.ShouldRetryProxyRequest:
//
//   - proxy_retry_status_ranges: which upstream HTTP statuses count as
//     channel-fault retryable. Default reproduces the historical hardcoded
//     set exactly (5xx + 401/403 + 408/409/425/429), so an unconfigured
//     deployment behaves identically.
//   - proxy_disable_status_ranges: which statuses auto-disable the failing
//     channel outright (enabled=false + manual_override, in addition to the
//     normal cooldown escalation). Default empty = no auto-disable =
//     historical behavior. Operators can opt into new-api-style semantics
//     (e.g. "401" — an invalid credential should not keep rotating).
//
// Specs are comma-separated single codes ("429") and inclusive ranges
// ("500-599"), validated at write time; the hot path parses at most once
// per distinct spec (mutex + last-spec cache).

package routing

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/deliciousbuding/metapi-go/config"
)

// StatusRange is an inclusive HTTP status interval [Lo, Hi].
type StatusRange struct {
	Lo int
	Hi int
}

const (
	// DefaultRetryStatusRangesSpec reproduces exactly the retryable statuses
	// previously hardcoded in proxy.ShouldRetryProxyRequest: 5xx plus
	// 401/403 (auth faults that an OAuth token refresh may resolve) and
	// 408/409/425/429.
	DefaultRetryStatusRangesSpec = "401,403,408,409,425,429,500-599"

	// DefaultDisableStatusRangesSpec is empty: no status auto-disables a
	// channel (historical metapi behavior — failures only escalate
	// cooldowns). "401" is the documented new-api-style preset.
	DefaultDisableStatusRangesSpec = ""
)

// ParseStatusRanges parses a comma-separated spec of single codes ("429")
// and inclusive ranges ("500-599"). Codes must be 100-599 with Lo <= Hi.
// A blank spec yields no ranges and no error. Results are sorted with
// adjacent/overlapping ranges merged. Any malformed entry is an error —
// settings writes validate with this before persisting.
func ParseStatusRanges(spec string) ([]StatusRange, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	ranges := make([]StatusRange, 0, 4)
	for _, part := range strings.Split(spec, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, fmt.Errorf("empty status range entry")
		}
		loStr, hiStr := part, part
		if idx := strings.Index(part, "-"); idx >= 0 {
			loStr = strings.TrimSpace(part[:idx])
			hiStr = strings.TrimSpace(part[idx+1:])
			if loStr == "" || hiStr == "" {
				return nil, fmt.Errorf("invalid status range %q", part)
			}
		}
		lo, errLo := strconv.Atoi(loStr)
		hi, errHi := strconv.Atoi(hiStr)
		if errLo != nil || errHi != nil {
			return nil, fmt.Errorf("invalid status %q", part)
		}
		if lo < 100 || lo > 599 || hi < 100 || hi > 599 {
			return nil, fmt.Errorf("status out of range 100-599: %q", part)
		}
		if lo > hi {
			return nil, fmt.Errorf("range start exceeds end: %q", part)
		}
		ranges = append(ranges, StatusRange{Lo: lo, Hi: hi})
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].Lo < ranges[j].Lo })
	merged := ranges[:0]
	for _, r := range ranges {
		if n := len(merged); n > 0 && r.Lo <= merged[n-1].Hi+1 {
			if r.Hi > merged[n-1].Hi {
				merged[n-1].Hi = r.Hi
			}
		} else {
			merged = append(merged, r)
		}
	}
	return merged, nil
}

// StatusInRanges reports whether status falls within any of the ranges.
func StatusInRanges(status int, ranges []StatusRange) bool {
	for _, r := range ranges {
		if status >= r.Lo && status <= r.Hi {
			return true
		}
	}
	return false
}

var (
	activeRangesMu         sync.Mutex
	lastRetrySpec          string
	retryRangesInitialized bool
	cachedRetryRanges      []StatusRange
	lastDisableSpec        string
	disableRangesInit      bool
	cachedDisableRanges    []StatusRange
)

// ActiveRetryStatusRanges returns the operator-configured retryable status
// ranges. Blank or invalid specs fall back to DefaultRetryStatusRangesSpec,
// which reproduces the historical hardcoded verdicts exactly. Parsed once
// per distinct spec.
func ActiveRetryStatusRanges() []StatusRange {
	raw := ""
	if rt := config.RuntimeSafe(); rt != nil {
		raw = strings.TrimSpace(rt.ProxyRetryStatusRanges)
	}
	activeRangesMu.Lock()
	defer activeRangesMu.Unlock()
	if !retryRangesInitialized || raw != lastRetrySpec {
		spec := raw
		if spec == "" {
			spec = DefaultRetryStatusRangesSpec
		}
		parsed, err := ParseStatusRanges(spec)
		if err != nil || len(parsed) == 0 {
			parsed, _ = ParseStatusRanges(DefaultRetryStatusRangesSpec)
		}
		cachedRetryRanges = parsed
		lastRetrySpec = raw
		retryRangesInitialized = true
	}
	return cachedRetryRanges
}

// ActiveDisableStatusRanges returns the operator-configured auto-disable
// statuses. Blank (default) or invalid specs yield no ranges: failures keep
// the historical cooldown-only escalation. Parsed once per distinct spec.
func ActiveDisableStatusRanges() []StatusRange {
	raw := ""
	if rt := config.RuntimeSafe(); rt != nil {
		raw = strings.TrimSpace(rt.ProxyDisableStatusRanges)
	}
	activeRangesMu.Lock()
	defer activeRangesMu.Unlock()
	if !disableRangesInit || raw != lastDisableSpec {
		parsed, err := ParseStatusRanges(raw)
		if err != nil {
			parsed = nil
		}
		cachedDisableRanges = parsed
		lastDisableSpec = raw
		disableRangesInit = true
	}
	return cachedDisableRanges
}
