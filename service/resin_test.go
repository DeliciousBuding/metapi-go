package service

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

func resinCfg(rawURL, platform string) *config.Config {
	return &config.Config{
		ResinURL:          rawURL,
		ResinPlatformName: platform,
		ResinEnabled:      true,
	}
}

// ---- Enabled gating ----

func TestResinEnabledGating(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  *config.Config
		site *store.Site
		want bool
	}{
		{"nil config", nil, nil, false},
		{"disabled flag", &config.Config{ResinEnabled: false, ResinURL: "http://resin.local:2260/tok"}, nil, false},
		{"enabled but empty url", &config.Config{ResinEnabled: true, ResinURL: ""}, nil, false},
		{"enabled but blank url", &config.Config{ResinEnabled: true, ResinURL: "   "}, nil, false},
		{"enabled with url", resinCfg("http://resin.local:2260/tok", "Default"), nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResinEnabled(tc.cfg, tc.site); got != tc.want {
				t.Fatalf("ResinEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

// ---- Per-site override ----

func TestResinEnabledPerSiteOverride(t *testing.T) {
	t.Parallel()
	enabled := true
	disabled := false
	cases := []struct {
		name   string
		cfg    *config.Config
		site   *store.Site
		want   bool
	}{
		{
			name: "global on, site nil override → inherit global",
			cfg:  resinCfg("http://resin.local:2260/tok", "Default"),
			site: &store.Site{ID: 1, ResinEnabled: nil},
			want: true,
		},
		{
			name: "global on, site explicit true → still true",
			cfg:  resinCfg("http://resin.local:2260/tok", "Default"),
			site: &store.Site{ID: 1, ResinEnabled: &enabled},
			want: true,
		},
		{
			name: "global on, site explicit false → false (per-site opt-out)",
			cfg:  resinCfg("http://resin.local:2260/tok", "Default"),
			site: &store.Site{ID: 1, ResinEnabled: &disabled},
			want: false,
		},
		{
			name: "global off, site explicit true → true (per-site opt-in)",
			cfg:  &config.Config{ResinEnabled: false, ResinURL: "http://resin.local:2260/tok"},
			site: &store.Site{ID: 1, ResinEnabled: &enabled},
			want: true,
		},
		{
			name: "global off, site nil override → false (inherit global)",
			cfg:  &config.Config{ResinEnabled: false, ResinURL: "http://resin.local:2260/tok"},
			site: &store.Site{ID: 1, ResinEnabled: nil},
			want: false,
		},
		{
			name: "global off, site explicit false → false",
			cfg:  &config.Config{ResinEnabled: false, ResinURL: "http://resin.local:2260/tok"},
			site: &store.Site{ID: 1, ResinEnabled: &disabled},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ResinEnabled(tc.cfg, tc.site); got != tc.want {
				t.Fatalf("ResinEnabled = %v, want %v", got, tc.want)
			}
		})
	}
}

// ---- Lease tracker ----

func TestTouchResinLeaseAndActiveLeases(t *testing.T) {
	t.Parallel()
	// Use a unique identity to avoid collisions with other tests.
	const accountID = "acc-test-lease-42"
	TouchResinLease(accountID)
	t.Cleanup(func() { resinLeaseTracker.Delete(accountID) })

	leases := ActiveLeases()
	found := false
	for _, lease := range leases {
		if lease.AccountID == accountID {
			found = true
			if lease.LastUsed == "" {
				t.Fatalf("lease LastUsed is empty for %q", accountID)
			}
			break
		}
	}
	if !found {
		t.Fatalf("TouchResinLease did not surface in ActiveLeases: %v", leases)
	}
}

func TestTouchResinLeaseEmptyIsNoop(t *testing.T) {
	t.Parallel()
	TouchResinLease("")
	TouchResinLease("   ")
	leases := ActiveLeases()
	for _, lease := range leases {
		if lease.AccountID == "" {
			t.Fatalf("empty account id stored in lease tracker: %v", lease)
		}
	}
}

func TestActiveLeasesPrunesStaleEntries(t *testing.T) {
	t.Parallel()
	const staleID = "acc-stale-test-99"
	const freshID = "acc-fresh-test-99"
	// Plant a stale entry directly to bypass the time.Now() write.
	resinLeaseTracker.Store(staleID, time.Now().UTC().Add(-2*resinLeaseStaleTTL))
	resinLeaseTracker.Store(freshID, time.Now().UTC())
	t.Cleanup(func() {
		resinLeaseTracker.Delete(staleID)
		resinLeaseTracker.Delete(freshID)
	})

	leases := ActiveLeases()
	hasStale := false
	hasFresh := false
	for _, lease := range leases {
		switch lease.AccountID {
		case staleID:
			hasStale = true
		case freshID:
			hasFresh = true
		}
	}
	if hasStale {
		t.Fatalf("stale lease %q should have been pruned from ActiveLeases", staleID)
	}
	if !hasFresh {
		t.Fatalf("fresh lease %q should be present in ActiveLeases", freshID)
	}

	// Verify the stale entry was actually deleted from the underlying map.
	if _, ok := resinLeaseTracker.Load(staleID); ok {
		t.Fatalf("stale lease %q was not deleted from resinLeaseTracker", staleID)
	}
}

// ---- Forward proxy URL userinfo format ----

func TestForwardProxyURLUserinfoFormat(t *testing.T) {
	t.Parallel()
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")

	got, ok := ForwardProxyURL(cfg, "Default", "acc-42")
	if !ok {
		t.Fatal("ForwardProxyURL returned ok=false for valid config")
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("forward proxy URL unparseable: %v (url=%q)", err, got)
	}
	if parsed.Scheme != "http" {
		t.Fatalf("scheme = %q, want http", parsed.Scheme)
	}
	if parsed.Host != "resin.local:2260" {
		t.Fatalf("host = %q, want resin.local:2260", parsed.Host)
	}
	// Go's url.UserPassword stores raw values; Username()/Password() return unescaped.
	username := parsed.User.Username()
	password, passwordOk := parsed.User.Password()
	if !passwordOk {
		t.Fatalf("password not set in userinfo: %q", got)
	}
	if username != "Default.acc-42" {
		t.Fatalf("userinfo username = %q, want Default.acc-42", username)
	}
	if password != "my-token" {
		t.Fatalf("userinfo password (token) = %q, want my-token", password)
	}
}

func TestForwardProxyURLMisconfigured(t *testing.T) {
	t.Parallel()
	// ForwardProxyURL is a low-level builder; it does not check ResinEnabled
	// (gating is the caller's job via ResinEnabled). These cases test URL
	// parsing / identity completeness, not the enabled flag.
	cases := []struct {
		name     string
		cfg      *config.Config
		platform string
		account  string
	}{
		{"empty platform", resinCfg("http://resin.local:2260/tok", ""), "", "acc-1"},
		{"empty account", resinCfg("http://resin.local:2260/tok", "Default"), "Default", ""},
		{"no token in url", resinCfg("http://resin.local:2260", "Default"), "Default", "acc-1"},
		{"empty url", resinCfg("", "Default"), "Default", "acc-1"},
		{"nil cfg", nil, "Default", "acc-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ForwardProxyURL(tc.cfg, tc.platform, tc.account)
			if ok || got != "" {
				t.Fatalf("ForwardProxyURL = (%q, %v), want (\"\", false)", got, ok)
			}
		})
	}
}

// ---- Temp identity determinism ----

func TestTempAccountForDeterminism(t *testing.T) {
	t.Parallel()
	siteID := int64(7)
	token := "sk-abc123"

	// Same (siteID, token) → same identity.
	first := TempAccountFor(siteID, token)
	second := TempAccountFor(siteID, token)
	if first != second {
		t.Fatalf("temp identity not deterministic: %q vs %q", first, second)
	}
	if !strings.HasPrefix(first, "tmp-") {
		t.Fatalf("temp identity should start with tmp-: %q", first)
	}
	if len(first) != len("tmp-")+16 {
		t.Fatalf("temp identity should be tmp- + 16 hex chars: %q (len=%d)", first, len(first))
	}
}

func TestTempAccountForDifferentTokensDifferentIDs(t *testing.T) {
	t.Parallel()
	siteID := int64(1)
	a := TempAccountFor(siteID, "token-alpha")
	b := TempAccountFor(siteID, "token-beta")
	if a == b {
		t.Fatalf("different tokens produced same temp identity: %q", a)
	}
}

func TestTempAccountForDifferentSitesDifferentIDs(t *testing.T) {
	t.Parallel()
	token := "shared-token"
	a := TempAccountFor(1, token)
	b := TempAccountFor(2, token)
	if a == b {
		t.Fatalf("different sites produced same temp identity: %q", a)
	}
}

func TestTempAccountForNoDotOrColon(t *testing.T) {
	t.Parallel()
	for _, token := range []string{"a", "sk-1234", "token.with.dots", "token:with:colons", "mixed.:token"} {
		identity := TempAccountFor(99, token)
		if strings.Contains(identity, ".") {
			t.Fatalf("temp identity %q contains '.' (token=%q)", identity, token)
		}
		if strings.Contains(identity, ":") {
			t.Fatalf("temp identity %q contains ':' (token=%q)", identity, token)
		}
	}
}

// ---- AccountFor stable identity ----

func TestAccountForStable(t *testing.T) {
	t.Parallel()
	account := &store.Account{ID: 42}
	got := AccountFor(account, nil)
	if got != "acc-42" {
		t.Fatalf("AccountFor = %q, want acc-42", got)
	}

	// No account → empty.
	if got := AccountFor(nil, &store.Site{ID: 1}); got != "" {
		t.Fatalf("AccountFor(nil account) = %q, want empty", got)
	}
	// Account with zero ID → empty (pre-account).
	if got := AccountFor(&store.Account{ID: 0}, nil); got != "" {
		t.Fatalf("AccountFor(zero ID) = %q, want empty", got)
	}
}

// ---- WS URL rewrite form ----

func TestRewriteWSURLWssTarget(t *testing.T) {
	t.Parallel()
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")

	got := RewriteWSURL("wss://api.openai.com/v1/responses?stream=true", cfg, "Default", "acc-1")
	if got == "" {
		t.Fatal("RewriteWSURL returned empty for valid wss target")
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("rewritten URL unparseable: %v (url=%q)", err, got)
	}
	if parsed.Scheme != "ws" {
		t.Fatalf("scheme = %q, want ws (client→resin must be ws)", parsed.Scheme)
	}
	if parsed.Host != "resin.local:2260" {
		t.Fatalf("host = %q, want resin.local:2260", parsed.Host)
	}
	wantPathPrefix := "/my-token/Default/https/api.openai.com/v1/responses"
	if !strings.HasPrefix(parsed.Path, wantPathPrefix) {
		t.Fatalf("path = %q, want prefix %q", parsed.Path, wantPathPrefix)
	}
	if parsed.RawQuery != "stream=true" {
		t.Fatalf("query = %q, want stream=true", parsed.RawQuery)
	}
}

func TestRewriteWSURLWsTarget(t *testing.T) {
	t.Parallel()
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")

	got := RewriteWSURL("ws://api.example.com/chat", cfg, "Default", "acc-1")
	if got == "" {
		t.Fatal("RewriteWSURL returned empty for valid ws target")
	}

	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("rewritten URL unparseable: %v (url=%q)", err, got)
	}
	if parsed.Scheme != "ws" {
		t.Fatalf("scheme = %q, want ws", parsed.Scheme)
	}
	// ws target → protocol segment = http
	wantPathPrefix := "/my-token/Default/http/api.example.com/chat"
	if !strings.HasPrefix(parsed.Path, wantPathPrefix) {
		t.Fatalf("path = %q, want prefix %q", parsed.Path, wantPathPrefix)
	}
}

func TestRewriteWSURLWithPort(t *testing.T) {
	t.Parallel()
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")

	got := RewriteWSURL("wss://ws.example.com:8443/chat/room", cfg, "Default", "acc-1")
	if got == "" {
		t.Fatal("RewriteWSURL returned empty")
	}
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatalf("rewritten URL unparseable: %v (url=%q)", err, got)
	}
	wantPathPrefix := "/my-token/Default/https/ws.example.com:8443/chat/room"
	if !strings.HasPrefix(parsed.Path, wantPathPrefix) {
		t.Fatalf("path = %q, want prefix %q", parsed.Path, wantPathPrefix)
	}
}

func TestRewriteWSURLMisconfigured(t *testing.T) {
	t.Parallel()
	// RewriteWSURL is a low-level builder; it does not check ResinEnabled
	// (gating is the caller's job via ResinEnabled). These cases test URL
	// parsing / identity / scheme, not the enabled flag.
	cases := []struct {
		name      string
		cfg       *config.Config
		targetURL string
		platform  string
		account   string
	}{
		{"empty platform", resinCfg("http://resin.local:2260/tok", ""), "wss://a.com/x", "", "acc-1"},
		{"empty account", resinCfg("http://resin.local:2260/tok", "Default"), "wss://a.com/x", "Default", ""},
		{"non-ws scheme", resinCfg("http://resin.local:2260/tok", "Default"), "https://a.com/x", "Default", "acc-1"},
		{"no token in url", resinCfg("http://resin.local:2260", "Default"), "wss://a.com/x", "Default", "acc-1"},
		{"nil cfg", nil, "wss://a.com/x", "Default", "acc-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RewriteWSURL(tc.targetURL, tc.cfg, tc.platform, tc.account); got != "" {
				t.Fatalf("RewriteWSURL = %q, want empty", got)
			}
		})
	}
}

// ---- BuildPlatformProxyConfig integration with Resin ----

func TestBuildPlatformProxyConfigResinForwardProxy(t *testing.T) {
	t.Parallel()
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")
	account := &store.Account{ID: 5}
	site := &store.Site{ID: 1, Platform: "openai"}

	proxyCfg := BuildPlatformProxyConfig(cfg, account, site)
	if proxyCfg == nil {
		t.Fatal("proxy config is nil")
	}
	if proxyCfg.ProxyURL == "" {
		t.Fatal("ProxyURL is empty, expected resin forward proxy URL")
	}
	parsed, err := url.Parse(proxyCfg.ProxyURL)
	if err != nil {
		t.Fatalf("resin ProxyURL unparseable: %v", err)
	}
	if username := parsed.User.Username(); username != "Default.acc-5" {
		t.Fatalf("userinfo username = %q, want Default.acc-5", username)
	}
	if proxyCfg.ResinAccount != "acc-5" {
		t.Fatalf("ResinAccount = %q, want acc-5", proxyCfg.ResinAccount)
	}
}

func TestBuildPlatformProxyConfigResinFallsBackWhenAccountIDZero(t *testing.T) {
	t.Parallel()
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")
	account := &store.Account{ID: 0} // pre-account: no ID
	site := &store.Site{ID: 1, Platform: "openai"}

	proxyCfg := BuildPlatformProxyConfig(cfg, account, site)
	// No account ID → no stable identity → resin skipped → nil (direct).
	if proxyCfg != nil {
		t.Fatalf("expected nil (direct) when account has no ID, got %+v", proxyCfg)
	}
}

func TestBuildPlatformProxyConfigAccountProxyURLBeatsResin(t *testing.T) {
	t.Parallel()
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")
	accountProxy := `{"proxyUrl":"http://account-proxy:8080"}`
	account := &store.Account{ID: 5, ExtraConfig: &accountProxy}
	site := &store.Site{ID: 1, Platform: "openai"}

	proxyCfg := BuildPlatformProxyConfig(cfg, account, site)
	if proxyCfg == nil {
		t.Fatal("proxy config is nil")
	}
	if proxyCfg.ProxyURL != "http://account-proxy:8080" {
		t.Fatalf("ProxyURL = %q, want account proxy (resin should not override)", proxyCfg.ProxyURL)
	}
	if proxyCfg.ResinAccount != "" {
		t.Fatalf("ResinAccount = %q, want empty (account proxy wins, not resin)", proxyCfg.ResinAccount)
	}
}

func TestBuildPlatformProxyConfigSiteProxyURLBeatsResin(t *testing.T) {
	t.Parallel()
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")
	siteProxy := "http://site-proxy:8080"
	account := &store.Account{ID: 5}
	site := &store.Site{ID: 1, Platform: "openai", ProxyURL: &siteProxy}

	proxyCfg := BuildPlatformProxyConfig(cfg, account, site)
	if proxyCfg == nil {
		t.Fatal("proxy config is nil")
	}
	if proxyCfg.ProxyURL != "http://site-proxy:8080" {
		t.Fatalf("ProxyURL = %q, want site proxy (resin should not override)", proxyCfg.ProxyURL)
	}
}

func TestBuildPlatformProxyConfigResinBeatsSystemProxy(t *testing.T) {
	// Not t.Parallel(): the system proxy URL lives on the shared atomic
	// runtime snapshot (publish + cleanup below).
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")
	config.SetRuntime(&config.RuntimeSettings{SystemProxyUrl: "http://system-proxy:8080"})
	t.Cleanup(func() { config.SetRuntime(nil) })
	account := &store.Account{ID: 5}
	site := &store.Site{ID: 1, Platform: "openai", UseSystemProxy: true}

	proxyCfg := BuildPlatformProxyConfig(cfg, account, site)
	if proxyCfg == nil {
		t.Fatal("proxy config is nil")
	}
	parsed, err := url.Parse(proxyCfg.ProxyURL)
	if err != nil {
		t.Fatalf("ProxyURL unparseable: %v", err)
	}
	if parsed.Host != "resin.local:2260" {
		t.Fatalf("ProxyURL host = %q, want resin.local:2260 (resin should beat system)", parsed.Host)
	}
	if proxyCfg.UseSystemProxy {
		t.Fatal("UseSystemProxy = true, want false (resin won)")
	}
}

func TestBuildPlatformProxyConfigResinDisabledFallsBackToSystemProxy(t *testing.T) {
	// Not t.Parallel(): the system proxy URL lives on the shared atomic
	// runtime snapshot (publish + cleanup below).
	cfg := &config.Config{ResinEnabled: false}
	config.SetRuntime(&config.RuntimeSettings{SystemProxyUrl: "http://system-proxy:8080"})
	t.Cleanup(func() { config.SetRuntime(nil) })
	account := &store.Account{ID: 5}
	site := &store.Site{ID: 1, Platform: "openai", UseSystemProxy: true}

	proxyCfg := BuildPlatformProxyConfig(cfg, account, site)
	if proxyCfg == nil {
		t.Fatal("proxy config is nil")
	}
	if proxyCfg.ProxyURL != "http://system-proxy:8080" {
		t.Fatalf("ProxyURL = %q, want system proxy", proxyCfg.ProxyURL)
	}
	if !proxyCfg.UseSystemProxy {
		t.Fatal("UseSystemProxy = false, want true")
	}
}

func TestBuildPlatformProxyConfigResinPlatformFallback(t *testing.T) {
	t.Parallel()
	// No RESIN_PLATFORM_NAME → fall back to site.Platform.
	cfg := &config.Config{
		ResinURL:     "http://resin.local:2260/my-token",
		ResinEnabled: true,
		// ResinPlatformName intentionally empty
	}
	account := &store.Account{ID: 5}
	site := &store.Site{ID: 1, Platform: "codex"}

	proxyCfg := BuildPlatformProxyConfig(cfg, account, site)
	if proxyCfg == nil {
		t.Fatal("proxy config is nil")
	}
	parsed, err := url.Parse(proxyCfg.ProxyURL)
	if err != nil {
		t.Fatalf("ProxyURL unparseable: %v", err)
	}
	if username := parsed.User.Username(); !strings.HasPrefix(username, "codex.acc-5") {
		t.Fatalf("userinfo username = %q, want prefix codex.acc-5 (platform fallback)", username)
	}
}

// ---- BuildPlatformProxyConfigForToken (pre-account temp identity) ----

func TestBuildPlatformProxyConfigForTokenTempIdentity(t *testing.T) {
	t.Parallel()
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")
	site := &store.Site{ID: 7, Platform: "openai"}

	proxyCfg := BuildPlatformProxyConfigForToken(cfg, site, "sk-test-token")
	if proxyCfg == nil {
		t.Fatal("proxy config is nil")
	}
	if proxyCfg.ProxyURL == "" {
		t.Fatal("ProxyURL is empty, expected resin forward proxy URL")
	}
	expectedAccount := TempAccountFor(site.ID, "sk-test-token")
	if proxyCfg.ResinAccount != expectedAccount {
		t.Fatalf("ResinAccount = %q, want %q", proxyCfg.ResinAccount, expectedAccount)
	}
	// Verify the temp identity is embedded in the userinfo.
	parsed, err := url.Parse(proxyCfg.ProxyURL)
	if err != nil {
		t.Fatalf("ProxyURL unparseable: %v", err)
	}
	if username := parsed.User.Username(); username != "Default."+expectedAccount {
		t.Fatalf("userinfo username = %q, want Default.%s", username, expectedAccount)
	}
}

func TestBuildPlatformProxyConfigForTokenSiteProxyURLWins(t *testing.T) {
	t.Parallel()
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")
	siteProxy := "http://site-proxy:8080"
	site := &store.Site{ID: 7, Platform: "openai", ProxyURL: &siteProxy}

	proxyCfg := BuildPlatformProxyConfigForToken(cfg, site, "sk-test-token")
	if proxyCfg == nil {
		t.Fatal("proxy config is nil")
	}
	if proxyCfg.ProxyURL != "http://site-proxy:8080" {
		t.Fatalf("ProxyURL = %q, want site proxy (beats resin)", proxyCfg.ProxyURL)
	}
	if proxyCfg.ResinAccount != "" {
		t.Fatalf("ResinAccount = %q, want empty (site proxy wins)", proxyCfg.ResinAccount)
	}
}

func TestBuildPlatformProxyConfigForTokenResinBeatsSystemProxy(t *testing.T) {
	// Not t.Parallel(): the system proxy URL lives on the shared atomic
	// runtime snapshot (publish + cleanup below).
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")
	config.SetRuntime(&config.RuntimeSettings{SystemProxyUrl: "http://system-proxy:8080"})
	t.Cleanup(func() { config.SetRuntime(nil) })
	site := &store.Site{ID: 7, Platform: "openai", UseSystemProxy: true}

	proxyCfg := BuildPlatformProxyConfigForToken(cfg, site, "sk-test-token")
	if proxyCfg == nil {
		t.Fatal("proxy config is nil")
	}
	parsed, err := url.Parse(proxyCfg.ProxyURL)
	if err != nil {
		t.Fatalf("ProxyURL unparseable: %v", err)
	}
	if parsed.Host != "resin.local:2260" {
		t.Fatalf("ProxyURL host = %q, want resin.local:2260 (resin beats system)", parsed.Host)
	}
}

func TestBuildPlatformProxyConfigForTokenResinDisabled(t *testing.T) {
	// Not t.Parallel(): the system proxy URL lives on the shared atomic
	// runtime snapshot (publish + cleanup below).
	cfg := &config.Config{ResinEnabled: false}
	config.SetRuntime(&config.RuntimeSettings{SystemProxyUrl: "http://system-proxy:8080"})
	t.Cleanup(func() { config.SetRuntime(nil) })
	site := &store.Site{ID: 7, Platform: "openai", UseSystemProxy: true}

	proxyCfg := BuildPlatformProxyConfigForToken(cfg, site, "sk-test-token")
	if proxyCfg == nil {
		t.Fatal("proxy config is nil")
	}
	if proxyCfg.ProxyURL != "http://system-proxy:8080" {
		t.Fatalf("ProxyURL = %q, want system proxy (resin disabled)", proxyCfg.ProxyURL)
	}
}

func TestBuildPlatformProxyConfigForTokenDeterministic(t *testing.T) {
	t.Parallel()
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")
	site := &store.Site{ID: 7, Platform: "openai"}

	// verify-token and create should derive the same temp identity for the same token.
	verifyCfg := BuildPlatformProxyConfigForToken(cfg, site, "shared-token")
	createCfg := BuildPlatformProxyConfigForToken(cfg, site, "shared-token")
	if verifyCfg == nil || createCfg == nil {
		t.Fatal("proxy config is nil")
	}
	if verifyCfg.ResinAccount != createCfg.ResinAccount {
		t.Fatalf("temp identity not deterministic: verify=%q create=%q",
			verifyCfg.ResinAccount, createCfg.ResinAccount)
	}
	if verifyCfg.ProxyURL != createCfg.ProxyURL {
		t.Fatalf("ProxyURL not deterministic: verify=%q create=%q",
			verifyCfg.ProxyURL, createCfg.ProxyURL)
	}
}
