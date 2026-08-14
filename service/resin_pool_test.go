package service

import (
	"net/url"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

// TestResinForwardProxyURL_StableCacheKey verifies the Tier 2 (#678) "client
// connection pooling" invariant for the Resin forward proxy.
//
// The pooled-transport cache in platform.getCachedTransport uses
// (proxyURL, insecureSkipTLS) as its key (see platform.transportCacheKey +
// the invariant pinned by platform.TestGetCachedTransport_SameKeyReturnsSamePointer).
// Resin's forward proxy URL embeds userinfo = "{Platform}.{Account}:{token}",
// so the cache key — and therefore the underlying TCP keep-alive pool — is
// stable only when ForwardProxyURL / BuildPlatformProxyConfig produce the
// same URL for the same (cfg, platform, account) tuple across calls.
//
// This test pins that input stability: a per-call nonce or fresh timestamp
// injected into the URL would silently defeat the pool (every call would
// miss the cache, re-handshake TLS, and present a new client IP surface to
// Resin's sticky-identity model). Two calls with identical inputs must
// yield byte-equal URLs; a different account identity must yield a different
// URL (so different identities do NOT alias to one transport).
func TestResinForwardProxyURL_StableCacheKey(t *testing.T) {
	t.Parallel()
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")

	urlFirst, ok := ForwardProxyURL(cfg, "Default", "acc-42")
	if !ok {
		t.Fatal("ForwardProxyURL returned ok=false for valid cfg")
	}
	urlSecond, ok := ForwardProxyURL(cfg, "Default", "acc-42")
	if !ok {
		t.Fatal("ForwardProxyURL returned ok=false on second call")
	}
	if urlFirst != urlSecond {
		t.Fatalf("Resin forward proxy URL is not stable across calls:\n  first  = %q\n  second = %q", urlFirst, urlSecond)
	}

	// Different account identity must yield a different URL (different cache
	// key → different transport → Resin sticky identity holds).
	urlOther, ok := ForwardProxyURL(cfg, "Default", "acc-99")
	if !ok {
		t.Fatal("ForwardProxyURL returned ok=false for second identity")
	}
	if urlFirst == urlOther {
		t.Fatal("different Resin account identities produced identical URLs — sticky identity broken")
	}

	// Sanity-check the userinfo carries the right account so the cache key
	// genuinely differs in the segment that matters.
	parsedFirst, err := url.Parse(urlFirst)
	if err != nil {
		t.Fatalf("first resin URL unparseable: %v", err)
	}
	parsedOther, err := url.Parse(urlOther)
	if err != nil {
		t.Fatalf("second resin URL unparseable: %v", err)
	}
	if parsedFirst.User.Username() == parsedOther.User.Username() {
		t.Fatalf("userinfo not differentiated by account identity: %q vs %q",
			parsedFirst.User.Username(), parsedOther.User.Username())
	}
}

// TestBuildPlatformProxyConfig_ResinURLStableForSameTuple verifies the
// end-to-end URL-stability invariant through BuildPlatformProxyConfig. This
// is the path the live proxy uses: the URL it sets on ProxyConfig becomes
// the cache key for platform.getCachedTransport via DoWithProxy.
//
// We assert: two calls with the same (cfg, account, site) tuple produce
// byte-equal ProxyURL strings; the account identity embedded in the userinfo
// matches the stable acc-{id} form; and per-site override off (nil) keeps
// Resin enabled so the URL is populated.
func TestBuildPlatformProxyConfig_ResinURLStableForSameTuple(t *testing.T) {
	t.Parallel()
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")
	account := &store.Account{ID: 7}
	site := &store.Site{ID: 1, Platform: "openai"}

	proxyCfgA := BuildPlatformProxyConfig(cfg, account, site)
	if proxyCfgA == nil || proxyCfgA.ProxyURL == "" {
		t.Fatal("BuildPlatformProxyConfig returned empty proxy for Resin-enabled site")
	}
	proxyCfgB := BuildPlatformProxyConfig(cfg, account, site)
	if proxyCfgB == nil || proxyCfgB.ProxyURL == "" {
		t.Fatal("BuildPlatformProxyConfig returned empty proxy on second call")
	}
	if proxyCfgA.ProxyURL != proxyCfgB.ProxyURL {
		t.Fatalf("Resin ProxyURL is not stable across BuildPlatformProxyConfig calls:\n  A=%q\n  B=%q",
			proxyCfgA.ProxyURL, proxyCfgB.ProxyURL)
	}
	if proxyCfgA.ResinAccount != "acc-7" {
		t.Fatalf("ResinAccount = %q, want acc-7", proxyCfgA.ResinAccount)
	}

	// Verify the embedded userinfo carries the stable identity so the cache
	// key is bound to the immutable account PK (not a per-call nonce).
	parsed, err := url.Parse(proxyCfgA.ProxyURL)
	if err != nil {
		t.Fatalf("resin ProxyURL unparseable: %v", err)
	}
	if got := parsed.User.Username(); got != "Default.acc-7" {
		t.Fatalf("userinfo username = %q, want Default.acc-7", got)
	}
}

// TestBuildPlatformProxyConfig_PerSiteOverrideDoesNotPolluteCacheKey pins
// the cross-cutting invariant: a site with ResinEnabled=nil (inherit global)
// and a site with ResinEnabled=true (force opt-in) resolve to the SAME Resin
// URL when global is on — because the override only flips the gate, not the
// URL contents. This means per-site overrides do NOT fragment the transport
// pool by accident: all sites that route through Resin with the same
// (cfg, platform, account) tuple still share the keep-alive pool.
func TestBuildPlatformProxyConfig_PerSiteOverrideDoesNotPolluteCacheKey(t *testing.T) {
	t.Parallel()
	cfg := resinCfg("http://resin.local:2260/my-token", "Default")
	account := &store.Account{ID: 11}

	inheritSite := &store.Site{ID: 1, Platform: "openai", ResinEnabled: nil}
	optInSite := &store.Site{ID: 2, Platform: "openai", ResinEnabled: boolPtr(true)}
	optOutSite := &store.Site{ID: 3, Platform: "openai", ResinEnabled: boolPtr(false)}

	inheritCfg := BuildPlatformProxyConfig(cfg, account, inheritSite)
	optInCfg := BuildPlatformProxyConfig(cfg, account, optInSite)
	optOutCfg := BuildPlatformProxyConfig(cfg, account, optOutSite)

	if inheritCfg == nil || inheritCfg.ProxyURL == "" {
		t.Fatal("inherit-site Resin proxy is nil/empty (global on should route through Resin)")
	}
	if optInCfg == nil || optInCfg.ProxyURL == "" {
		t.Fatal("opt-in-site Resin proxy is nil/empty")
	}
	if optOutCfg != nil && optOutCfg.ProxyURL != "" {
		t.Fatalf("opt-out site should not route through Resin, got ProxyURL=%q", optOutCfg.ProxyURL)
	}
	if inheritCfg.ProxyURL != optInCfg.ProxyURL {
		t.Fatalf("inherit and opt-in Resin URLs differ — per-site override is polluting the cache key:\n  inherit=%q\n  optIn=%q",
			inheritCfg.ProxyURL, optInCfg.ProxyURL)
	}
}

// boolPtr returns a pointer to b. Test-only helper for *bool override fields.
func boolPtr(b bool) *bool {
	return &b
}
