package checkin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

// ---- issue #669: ClassAuth self-heal, per-account timeout, manual-trigger lease ----
//
// These tests cover the four edge cases from #669:
//   - adapterSupportsLogin + shouldAttemptSelfHealLogin (re-login decision)
//   - end-to-end re-login mock: 401 → re-login → retry succeeds
//   - per-account timeout enforcement (hung upstream bounded by checkinTimeout)
//   - per-account in-progress lease contention (scheduler vs manual trigger)

// TestAdapterSupportsLogin pins the capability detection against every
// registered adapter. NewApi / OneApi family / Veloera support a real login
// (BaseAdapter.Login POST /api/user/login or override); StandardAdapter-based
// adapters and Sub2Api return a hardcoded unsupported message without a
// network call, so re-login would always no-op.
func TestAdapterSupportsLogin(t *testing.T) {
	cases := []struct {
		platformName string
		want         bool
	}{
		{"new-api", true},
		{"one-api", true},
		{"one-hub", true},
		{"done-hub", true},
		{"veloera", true},
		{"sub2api", false},
		{"openai", false},
		{"claude", false},
		{"gemini", false},
		{"gemini-cli", false},
		{"antigravity", false},
		{"codex", false},
		{"cliproxyapi", false},
	}
	for _, tc := range cases {
		t.Run(tc.platformName, func(t *testing.T) {
			adp := platform.GetAdapter(tc.platformName)
			if adp == nil {
				t.Fatalf("no adapter registered for %q", tc.platformName)
			}
			if got := adapterSupportsLogin(adp); got != tc.want {
				t.Errorf("adapterSupportsLogin(%s) = %v, want %v", tc.platformName, got, tc.want)
			}
		})
	}
}

// TestShouldAttemptSelfHealLogin pins the re-login trigger predicate. A
// ClassAuth failure (401/403 with auth residual) triggers re-login ONLY when
// the adapter can actually re-login. Confirmed token-expiry signals trigger
// regardless of adapter (legacy shouldAttemptAutoRelogin path). Transient /
// billing / unknown failures never trigger re-login.
func TestShouldAttemptSelfHealLogin(t *testing.T) {
	newapi := platform.GetAdapter("new-api")   // supports login
	sub2api := platform.GetAdapter("sub2api") // JWT-only, no login
	if newapi == nil || sub2api == nil {
		t.Fatal("required adapters not registered")
	}

	cases := []struct {
		name          string
		message       string
		wantNewApi    bool
		wantSub2Api   bool
		wantClassNew  platform.UpstreamErrorClass
	}{
		// ClassAuth (401/403 with auth residual) → re-login only on supporting adapter.
		// NOTE: avoid "access token invalid" wording — that is a confirmed
		// credential-expiry signal and classifies as ClassExpired, not ClassAuth.
		{"http 401 unauthorized", "HTTP 401: unauthorized", true, false, platform.ClassAuth},
		{"http 403 forbidden", "HTTP 403: forbidden", true, false, platform.ClassAuth},
		// Confirmed token-expiry → legacy path fires on every adapter.
		{"jwt expired", "jwt expired", true, true, platform.ClassExpired},
		{"invalid access token", "invalid access token", true, true, platform.ClassExpired},
		// Transient / billing / unknown → never re-login.
		{"rate limit", "HTTP 429: rate limit exceeded", false, false, platform.ClassTransient},
		{"billing", "insufficient quota", false, false, platform.ClassBilling},
		{"unknown opaque", "some opaque upstream error", false, false, platform.ClassUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotClass := platform.ClassifyUpstreamError(0, tc.message)
			if gotClass != tc.wantClassNew {
				t.Errorf("ClassifyUpstreamError(%q) = %s, want %s", tc.message, gotClass, tc.wantClassNew)
			}
			if got := shouldAttemptSelfHealLogin(tc.message, newapi); got != tc.wantNewApi {
				t.Errorf("shouldAttemptSelfHealLogin(%q, new-api) = %v, want %v", tc.message, got, tc.wantNewApi)
			}
			if got := shouldAttemptSelfHealLogin(tc.message, sub2api); got != tc.wantSub2Api {
				t.Errorf("shouldAttemptSelfHealLogin(%q, sub2api) = %v, want %v", tc.message, got, tc.wantSub2Api)
			}
		})
	}
}

// TestCheckinTimeoutDefaultBudget documents the per-account checkin budget and
// guards against accidental changes. 30s mirrors the issue #669 spec.
func TestCheckinTimeoutDefaultBudget(t *testing.T) {
	if checkinTimeout != 30*time.Second {
		t.Fatalf("checkinTimeout = %v, want 30s", checkinTimeout)
	}
}

// TestCheckinContextEnforcesDeadline verifies the per-account context wrapper
// derives a deadline bounded by checkinTimeout (issue #669: per-account timeout).
func TestCheckinContextEnforcesDeadline(t *testing.T) {
	original := checkinTimeout
	checkinTimeout = 250 * time.Millisecond
	t.Cleanup(func() { checkinTimeout = original })

	ctx, cancel := checkinContext(context.Background())
	defer cancel()
	dl, ok := ctx.Deadline()
	if !ok {
		t.Fatal("checkinContext produced a context with no deadline")
	}
	got := time.Until(dl)
	if got <= 0 || got > 260*time.Millisecond {
		t.Fatalf("deadline lead = %v, want ~250ms (0 < lead <= 260ms)", got)
	}
}

// TestCheckinLeaseContention verifies the per-account in-progress lease is
// mutually exclusive for a single account and re-entrant after release
// (issue #669: manual-trigger lease).
func TestCheckinLeaseContention(t *testing.T) {
	const accountID int64 = 77799
	checkinInProgress.Delete(accountID)
	t.Cleanup(func() { checkinInProgress.Delete(accountID) })

	if !tryAcquireCheckinLease(accountID) {
		t.Fatal("first acquire failed on a free lease")
	}
	if tryAcquireCheckinLease(accountID) {
		t.Fatal("second acquire succeeded while lease is held")
	}
	releaseCheckinLease(accountID)
	if !tryAcquireCheckinLease(accountID) {
		t.Fatal("acquire after release failed")
	}
	releaseCheckinLease(accountID)
}

// TestCheckinLeaseOnlyOneAcquiresUnderConcurrency hammers the lease from many
// goroutines and asserts exactly one acquires (issue #669: lease contention).
func TestCheckinLeaseOnlyOneAcquiresUnderConcurrency(t *testing.T) {
	const accountID int64 = 88899
	checkinInProgress.Delete(accountID)
	t.Cleanup(func() { checkinInProgress.Delete(accountID) })

	start := make(chan struct{})
	var acquired atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if tryAcquireCheckinLease(accountID) {
				acquired.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	if got := acquired.Load(); got != 1 {
		t.Fatalf("acquired = %d, want exactly 1", got)
	}
	releaseCheckinLease(accountID)
}

// ---- End-to-end re-login mock (issue #669: ClassAuth self-heal) ----
//
// A new-api account with an expired access token and a stored auto-relogin
// config. The upstream returns 401 on the first checkin (expired token),
// then a fresh token from /api/user/login, then 200 on the retried checkin.
// CheckinAccount must: detect ClassAuth → re-login → retry once → succeed,
// and persist the refreshed access token to the DB.

func newSelfHealReloginServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var seenTokens []string
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/checkin":
			token := r.Header.Get("Authorization")
			mu.Lock()
			seenTokens = append(seenTokens, token)
			mu.Unlock()
			if token == "Bearer expired-token" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"success":false,"message":"unauthorized"}`))
				return
			}
			if token == "Bearer fresh-token" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"success":true,"message":"checkin success","data":{"reward":"5"}}`))
				return
			}
			http.Error(w, `{"success":false,"message":"bad token"}`, http.StatusUnauthorized)
		case "/api/user/login":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"message":"登录成功","token":"fresh-token"}`))
		case "/api/user/self":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"username":"relogin-user","id":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, &seenTokens
}

func TestCheckinAccount_AuthSelfHeal_RetriesAfterRelogin(t *testing.T) {
	db := openCheckinEdgeTestDB(t)
	cfg := &config.Config{AccountCredentialSecret: "edge-test-secret"}

	upstream, seenTokens := newSelfHealReloginServer(t)
	siteID := insertEdgeSite(t, db, upstream.URL, "new-api")

	cipher, err := service.EncryptAccountPassword(cfg, "relogin-password")
	if err != nil {
		t.Fatalf("encrypt password: %v", err)
	}
	extraConfig := buildEdgeExtraConfig(t, 1, "relogin-user", cipher)
	accountID := insertEdgeAccount(t, db, siteID, "expired-token", extraConfig)

	result := CheckinAccount(cfg, db.DB, accountID, &CheckinOptions{SkipEvent: true, ScheduleMode: "manual"})

	if !result.Success || result.Status != CheckinSuccess {
		t.Fatalf("CheckinAccount result = %+v, want success", result)
	}

	// Re-login must have refreshed the stored access token.
	var storedToken string
	if err := db.Get(&storedToken, "SELECT access_token FROM accounts WHERE id = ?", accountID); err != nil {
		t.Fatalf("read access_token: %v", err)
	}
	if storedToken != "fresh-token" {
		t.Fatalf("access_token = %q, want %q (re-login did not persist)", storedToken, "fresh-token")
	}

	// The retried checkin must have used the refreshed token. The new-api
	// adapter also falls back to cookie-based checkin on a 401 (extra calls
	// with an empty Authorization header), so the exact call count is
	// adapter-dependent — what matters is that the fresh token was eventually
	// used for the successful retry.
	seen := *seenTokens
	last := ""
	if len(seen) > 0 {
		last = seen[len(seen)-1]
	}
	if last != "Bearer fresh-token" {
		t.Fatalf("last checkin token = %q, want %q (re-login retry did not use fresh token). seen=%v", last, "Bearer fresh-token", seen)
	}
	if !containsString(seen, "Bearer fresh-token") {
		t.Fatalf("fresh token never used for checkin; seen=%v", seen)
	}
}

// TestCheckinAccount_AuthSelfHeal_NoReloginConfigSurfacesOriginalError verifies
// that when a ClassAuth failure occurs but no auto-relogin config is stored,
// CheckinAccount surfaces the original auth error unchanged (no fabricated error).
func TestCheckinAccount_AuthSelfHeal_NoReloginConfigSurfacesOriginalError(t *testing.T) {
	db := openCheckinEdgeTestDB(t)
	cfg := &config.Config{AccountCredentialSecret: "edge-test-secret"}

	upstream, seenTokens := newSelfHealReloginServer(t)
	siteID := insertEdgeSite(t, db, upstream.URL, "new-api")
	// No autoRelogin in extra_config → tryAutoRelogin bails before calling Login.
	accountID := insertEdgeAccount(t, db, siteID, "expired-token", buildEdgeExtraConfig(t, 1, "", ""))

	result := CheckinAccount(cfg, db.DB, accountID, &CheckinOptions{SkipEvent: true, ScheduleMode: "manual"})

	if result.Success {
		t.Fatalf("CheckinAccount result = %+v, want failure", result)
	}
	if result.Status != CheckinFailed {
		t.Fatalf("status = %v, want failed", result.Status)
	}
	// Original auth error must propagate (not a fabricated "login failed" string).
	if result.Message == "" {
		t.Fatal("result message empty; want the original 401 auth error")
	}
	// Without a stored relogin config, tryAutoRelogin bails before calling
	// Login, so the refreshed token must NEVER be used for a checkin retry.
	// (The new-api adapter may internally fall back to cookie-based checkin on
	// a 401 — those calls carry an empty Authorization header, not the fresh
	// token — so the precise call count is adapter-dependent.)
	if containsString(*seenTokens, "Bearer fresh-token") {
		t.Fatalf("fresh token was used for a checkin retry even though no relogin config was stored. seen=%v", *seenTokens)
	}
}

// ---- Per-account timeout enforcement (issue #669) ----
//
// A hung upstream that never responds must not block the worker indefinitely.
// With checkinTimeout overridden to a short budget, CheckinAccount must return
// within that budget rather than hanging for the server's long sleep.

func newHangingCheckinServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/user/checkin" {
			select {
			case <-time.After(5 * time.Second):
			case <-r.Context().Done():
			}
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func TestCheckinAccount_PerAccountTimeoutEnforced(t *testing.T) {
	db := openCheckinEdgeTestDB(t)
	cfg := &config.Config{AccountCredentialSecret: "edge-test-secret"}

	original := checkinTimeout
	checkinTimeout = 400 * time.Millisecond
	t.Cleanup(func() { checkinTimeout = original })

	upstream := newHangingCheckinServer(t)
	siteID := insertEdgeSite(t, db, upstream.URL, "new-api")
	accountID := insertEdgeAccount(t, db, siteID, "some-token", buildEdgeExtraConfig(t, 1, "", ""))

	start := time.Now()
	result := CheckinAccount(cfg, db.DB, accountID, &CheckinOptions{SkipEvent: true, ScheduleMode: "manual"})
	elapsed := time.Since(start)

	if result.Success {
		t.Fatalf("CheckinAccount succeeded against a hanging upstream; result = %+v", result)
	}
	// Must return well within the 5s server sleep — bounded by the 400ms timeout.
	if elapsed > 2*time.Second {
		t.Fatalf("elapsed = %v, want < 2s (per-account timeout did not bound the hung call)", elapsed)
	}
}

// ---- Manual-trigger lease contention at the CheckinAccount level (issue #669) ----
//
// Two concurrent CheckinAccount calls for the same account must not double-run
// the upstream checkin: one proceeds, the other returns already_in_progress.

func newSlowCheckinServer(t *testing.T, delay time.Duration) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/user/checkin":
			calls.Add(1)
			time.Sleep(delay)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"message":"checkin success","data":{"reward":"5"}}`))
		case "/api/user/self":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"success":true,"data":{"username":"edge-user","id":1}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

func TestCheckinAccount_LeaseSkipsConcurrentManualTrigger(t *testing.T) {
	db := openCheckinEdgeTestDB(t)
	cfg := &config.Config{AccountCredentialSecret: "edge-test-secret"}

	upstream, upstreamCalls := newSlowCheckinServer(t, 400*time.Millisecond)
	siteID := insertEdgeSite(t, db, upstream.URL, "new-api")
	accountID := insertEdgeAccount(t, db, siteID, "session-token", buildEdgeExtraConfig(t, 1, "", ""))

	start := make(chan struct{})
	var wg sync.WaitGroup
	var results []CheckinResult
	var mu sync.Mutex
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r := CheckinAccount(cfg, db.DB, accountID, &CheckinOptions{SkipEvent: true, ScheduleMode: "manual"})
			mu.Lock()
			results = append(results, r)
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()

	var successes, alreadyInProgress int
	for _, r := range results {
		switch {
		case r.Success && r.Status == CheckinSuccess:
			successes++
		case r.Skipped && r.Reason == "already_in_progress":
			alreadyInProgress++
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d, want 1 (results: %+v)", successes, results)
	}
	if alreadyInProgress != 1 {
		t.Fatalf("already_in_progress = %d, want 1 (results: %+v)", alreadyInProgress, results)
	}
	// The upstream checkin endpoint must have been hit exactly once — the
	// leased-out call never reached the adapter a second time.
	if got := upstreamCalls.Load(); got != 1 {
		t.Fatalf("upstream checkin calls = %d, want 1 (lease failed to prevent double-run)", got)
	}
}

// ---- helpers ----

func openCheckinEdgeTestDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("auto-migrate: %v", err)
	}
	return db
}

func insertEdgeSite(t *testing.T, db *store.DB, siteURL, platformName string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := db.Exec(
		"INSERT INTO sites (name, url, platform, status, created_at, updated_at) VALUES (?, ?, ?, 'active', ?, ?)",
		"Edge Checkin "+platformName, siteURL, platformName, now, now,
	)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("site id: %v", err)
	}
	return id
}

func insertEdgeAccount(t *testing.T, db *store.DB, siteID int64, accessToken, extraConfig string) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	result, err := db.Exec(
		"INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, extra_config, created_at, updated_at) VALUES (?, ?, ?, 'active', 1, ?, ?, ?)",
		siteID, "edge-user", accessToken, extraConfig, now, now,
	)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("account id: %v", err)
	}
	return id
}

// buildEdgeExtraConfig builds an extra_config JSON with platformUserId and,
// when username+passwordCipher are non-empty, an autoRelogin block.
func buildEdgeExtraConfig(t *testing.T, platformUserID int, username, passwordCipher string) string {
	t.Helper()
	cfg := map[string]any{"platformUserId": float64(platformUserID)}
	if username != "" && passwordCipher != "" {
		cfg["autoRelogin"] = map[string]any{
			"username":       username,
			"passwordCipher": passwordCipher,
		}
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal extra_config: %v", err)
	}
	return string(raw)
}

// containsString reports whether the slice contains the target string.
func containsString(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}
