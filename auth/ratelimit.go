package auth

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ---------------------------------------------------------------------------
// Per-IP token-bucket rate limiter with periodic cleanup.
//
// Each IP gets its own *rate.Limiter. Idle entries older than 5 minutes
// are removed by a background goroutine.
// ---------------------------------------------------------------------------

type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rateLimiterEntry
	rps      rate.Limit
	burst    int
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newIPRateLimiter(rps, burst int) *ipRateLimiter {
	rl := &ipRateLimiter{
		limiters: make(map[string]*rateLimiterEntry),
		rps:      rate.Limit(rps),
		burst:    burst,
	}
	go rl.cleanupLoop()
	return rl
}

// cleanupLoop runs every minute and evicts IP entries that have not been
// seen for more than 5 minutes.
func (rl *ipRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for ip, entry := range rl.limiters {
			if time.Since(entry.lastSeen) > 5*time.Minute {
				delete(rl.limiters, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// allow checks whether the given IP is within its rate limit. If the IP
// has no limiter yet, one is created with the configured rate and burst.
// Returns true if the request is allowed.
func (rl *ipRateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	entry, exists := rl.limiters[ip]
	if !exists {
		entry = &rateLimiterEntry{
			limiter: rate.NewLimiter(rl.rps, rl.burst),
		}
		rl.limiters[ip] = entry
	}
	entry.lastSeen = time.Now()
	rl.mu.Unlock()
	return entry.limiter.Allow()
}

// ---------------------------------------------------------------------------
// Rate-limit middleware factories.
// ---------------------------------------------------------------------------

// AdminRateLimit returns middleware that rate-limits every request by IP
// using a token bucket with the given sustained rate (req/s) and burst.
//
// Intended for the /api/* route group (all admin endpoints).
func AdminRateLimit(rps, burst int) func(http.Handler) http.Handler {
	rl := newIPRateLimiter(rps, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractClientIP(r)
			if !rl.allow(ip) {
				w.Header().Set("Retry-After", "1")
				writeJSON(w, http.StatusTooManyRequests, jsonError("Too many requests"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AuthRateLimit returns middleware that rate-limits ONLY /api/auth/*
// requests by IP (#1034). Login is the single surface that accepts the
// master token, so it gets a strict bucket regardless of the general admin
// bucket size. All other paths pass through without consuming tokens.
//
// Intended to be stacked alongside AdminRateLimit so auth endpoints are
// subject to both the general /api/ cap and this stricter cap.
func AuthRateLimit(rps, burst int) func(http.Handler) http.Handler {
	rl := newIPRateLimiter(rps, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/auth/") {
				next.ServeHTTP(w, r)
				return
			}
			ip := extractClientIP(r)
			if !rl.allow(ip) {
				w.Header().Set("Retry-After", "1")
				writeJSON(w, http.StatusTooManyRequests, jsonError("Too many requests"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// OAuthRateLimit returns middleware that rate-limits ONLY /api/oauth/*
// requests by IP. All other paths pass through without consuming tokens.
//
// Intended to be stacked after AdminRateLimit so OAuth endpoints are subject
// to both the general /api/ cap and this stricter cap.
func OAuthRateLimit(rps, burst int) func(http.Handler) http.Handler {
	rl := newIPRateLimiter(rps, burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/api/oauth/") {
				next.ServeHTTP(w, r)
				return
			}
			ip := extractClientIP(r)
			if !rl.allow(ip) {
				w.Header().Set("Retry-After", "1")
				writeJSON(w, http.StatusTooManyRequests, jsonError("Too many requests"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---------------------------------------------------------------------------
// Fixed-window per-IP RPM rate limiter for /v1 proxy routes.
//
// Unlike the token-bucket ipRateLimiter above (which is RPS-based and does
// not expose remaining/reset for standard X-RateLimit-* headers), this uses
// a simple fixed-window counter per IP per minute. The fixed-window model
// naturally provides Limit/Remaining/Reset values that map 1:1 to the
// conventional rate-limit response headers.
//
// In-memory only (sync.Mutex + map). Idle entries are evicted every minute.
// ---------------------------------------------------------------------------

// fixedWindowRateLimitResult holds the outcome of a fixed-window rate check.
type fixedWindowRateLimitResult struct {
	Allowed   bool
	Limit     int
	Remaining int
	ResetAt   time.Time
}

// fixedWindowIPRateLimiter is a per-IP fixed-window counter with a one-minute
// window. Each IP gets its own {count, resetAt} entry. When the current time
// passes resetAt, the window rolls over (count resets to 0, resetAt advances).
type fixedWindowIPRateLimiter struct {
	mu      sync.Mutex
	windows map[string]*fixedWindowEntry
	limit   int
	window  time.Duration
}

type fixedWindowEntry struct {
	count   int
	resetAt time.Time
}

// newFixedWindowIPRateLimiter creates a per-IP fixed-window limiter that allows
// at most limit requests per window per IP. A limit <= 0 means the limiter is
// disabled and allow() always returns Allowed=true.
func newFixedWindowIPRateLimiter(limit int) *fixedWindowIPRateLimiter {
	rl := &fixedWindowIPRateLimiter{
		windows: make(map[string]*fixedWindowEntry),
		limit:   limit,
		window:  time.Minute,
	}
	go rl.cleanupLoop()
	return rl
}

// cleanupLoop evicts idle IP entries older than 5 minutes, mirroring the
// token-bucket limiter's eviction policy.
func (rl *fixedWindowIPRateLimiter) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for ip, entry := range rl.windows {
			if now.After(entry.resetAt.Add(5 * time.Minute)) {
				delete(rl.windows, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// allow checks whether ip is within its per-minute request budget. On the
// first request (or after the window rolled over), a fresh window is created.
func (rl *fixedWindowIPRateLimiter) allow(ip string) fixedWindowRateLimitResult {
	if rl.limit <= 0 {
		return fixedWindowRateLimitResult{Allowed: true, Limit: 0, Remaining: 0, ResetAt: time.Time{}}
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	entry, exists := rl.windows[ip]
	if !exists || now.After(entry.resetAt) {
		entry = &fixedWindowEntry{count: 0, resetAt: now.Add(rl.window)}
		rl.windows[ip] = entry
	}
	entry.count++
	remaining := rl.limit - entry.count
	if remaining < 0 {
		remaining = 0
	}
	if entry.count > rl.limit {
		return fixedWindowRateLimitResult{Allowed: false, Limit: rl.limit, Remaining: 0, ResetAt: entry.resetAt}
	}
	return fixedWindowRateLimitResult{Allowed: true, Limit: rl.limit, Remaining: remaining, ResetAt: entry.resetAt}
}

// writeProxyRateLimitHeaders writes the conventional X-RateLimit-* response
// headers so compliant clients can back off intelligently.
func writeProxyRateLimitHeaders(w http.ResponseWriter, result fixedWindowRateLimitResult) {
	if result.Limit <= 0 {
		return
	}
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))
	w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(result.ResetAt.Unix(), 10))
}

// ProxyRateLimit returns middleware that enforces a per-IP RPM cap on /v1 proxy
// routes using a fixed-window counter. rpm<=0 disables the limiter (pass-through).
//
// On limit exceedance it responds with 429 Too Many Requests, a JSON error body,
// Retry-After, and X-RateLimit-Limit/Remaining/Reset headers.
func ProxyRateLimit(rpm int) func(http.Handler) http.Handler {
	if rpm <= 0 {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			})
		}
	}
	rl := newFixedWindowIPRateLimiter(rpm)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractClientIP(r)
			result := rl.allow(ip)
			writeProxyRateLimitHeaders(w, result)
			if !result.Allowed {
				sec := int(time.Until(result.ResetAt).Seconds())
				if sec < 1 {
					sec = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(sec))
				writeJSON(w, http.StatusTooManyRequests, jsonError("Too many requests"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---------------------------------------------------------------------------
// Global PROXY_TOKEN RPM cap — safety net against token leakage.
//
// A single fixed-window counter shared across all IPs. Only requests that
// authenticated with the global PROXY_TOKEN (source == "global") are counted.
// Managed keys already have their own RPM/TPM admission in ProxyAuth, so
// they are exempt.
// ---------------------------------------------------------------------------

// globalTokenRateLimiter is a single-bucket fixed-window counter for the
// global PROXY_TOKEN. Not per-IP — the cap is global across all callers.
type globalTokenRateLimiter struct {
	mu      sync.Mutex
	count   int
	resetAt time.Time
	limit   int
	window  time.Duration
}

func newGlobalTokenRateLimiter(limit int) *globalTokenRateLimiter {
	return &globalTokenRateLimiter{
		limit:  limit,
		window: time.Minute,
	}
}

// allow checks the shared global-token budget. On a rolled-over window the
// counter resets.
func (rl *globalTokenRateLimiter) allow() fixedWindowRateLimitResult {
	if rl.limit <= 0 {
		return fixedWindowRateLimitResult{Allowed: true, Limit: 0, Remaining: 0, ResetAt: time.Time{}}
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	if now.After(rl.resetAt) {
		rl.count = 0
		rl.resetAt = now.Add(rl.window)
	}
	rl.count++
	remaining := rl.limit - rl.count
	if remaining < 0 {
		remaining = 0
	}
	if rl.count > rl.limit {
		return fixedWindowRateLimitResult{Allowed: false, Limit: rl.limit, Remaining: 0, ResetAt: rl.resetAt}
	}
	return fixedWindowRateLimitResult{Allowed: true, Limit: rl.limit, Remaining: remaining, ResetAt: rl.resetAt}
}

// ProxyGlobalTokenRateLimit returns middleware that caps the global
// PROXY_TOKEN at rpm requests per minute across all IPs. Must be installed
// AFTER ProxyAuth so it can read the resolved auth source from the request
// context. rpm<=0 disables the cap (pass-through).
//
// Only requests whose ProxyAuthContext.Source == "global" are counted;
// managed keys pass through unaffected (they have their own RPM admission).
func ProxyGlobalTokenRateLimit(rpm int) func(http.Handler) http.Handler {
	if rpm <= 0 {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			})
		}
	}
	rl := newGlobalTokenRateLimiter(rpm)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			pac := GetProxyAuth(r.Context())
			if pac == nil || pac.Source != "global" {
				next.ServeHTTP(w, r)
				return
			}
			result := rl.allow()
			writeProxyRateLimitHeaders(w, result)
			if !result.Allowed {
				sec := int(time.Until(result.ResetAt).Seconds())
				if sec < 1 {
					sec = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(sec))
				writeJSON(w, http.StatusTooManyRequests, jsonError("Global proxy token rate limit exceeded"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
