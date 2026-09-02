package service

import (
	"testing"

	"github.com/deliciousbuding/metapi-go/platform"
)

// The assembly-side filter and the data-plane deny list are two guards over the
// same site-supplied map. They are allowed to differ in scope (the assembly side
// also reserves content-type and new-api-user, which the data plane owns), but a
// header that is security- or correctness-critical must be denied by BOTH: if
// only one of them denies it, whichever path builds the request last wins. That
// is exactly how Accept-Encoding used to slip through — it reached the outbound
// request via platform.SiteProxy.Do re-applying custom_headers after the data
// plane had stripped it, which switched off transparent decoding and turned a
// healthy upstream answer into unreadable bytes (zero-token billing, blind
// keyword scan, and a false "empty content" 502 on the SSE path).
func TestReservedPlatformCustomHeadersStayInStepWithPlatformDenyList(t *testing.T) {
	t.Parallel()
	shared := []string{
		"authorization",
		"cookie",
		"host",
		"content-length",
		"accept-encoding",
	}
	for _, name := range shared {
		for _, variant := range []string{name, upper(name), "  " + name + " "} {
			if !isReservedPlatformCustomHeader(variant) {
				t.Errorf("isReservedPlatformCustomHeader(%q) = false, want true (assembly-side filter must deny it)", variant)
			}
			if !platform.IsDeniedCustomHeader(variant) {
				t.Errorf("platform.IsDeniedCustomHeader(%q) = false, want true (data-plane deny list must deny it)", variant)
			}
		}
	}
}

func TestFilterPlatformCustomHeadersDropsReservedKeepsSiteOwn(t *testing.T) {
	t.Parallel()
	filtered := filterPlatformCustomHeaders(map[string]string{
		"Authorization":   "Bearer site-supplied",
		"Cookie":          "session=hijacked",
		"Accept-Encoding": "br",
		"Content-Type":    "text/plain",
		"Host":            "attacker.example",
		"Content-Length":  "17",
		"New-Api-User":    "4242",
		"X-Site-Header":   "kept",
	})
	if got := filtered["X-Site-Header"]; got != "kept" {
		t.Errorf("a site's own header must survive the filter, got %q", got)
	}
	for _, denied := range []string{"Authorization", "Cookie", "Accept-Encoding", "Content-Type", "Host", "Content-Length", "New-Api-User"} {
		if _, ok := filtered[denied]; ok {
			t.Errorf("%s survived filterPlatformCustomHeaders, want it dropped", denied)
		}
	}
	if len(filtered) != 1 {
		t.Errorf("filtered map has %d entries (%v), want exactly the 1 site-owned header", len(filtered), filtered)
	}
	if got := filterPlatformCustomHeaders(nil); got != nil {
		t.Errorf("filterPlatformCustomHeaders(nil) = %v, want nil", got)
	}
	if got := filterPlatformCustomHeaders(map[string]string{"Authorization": "x"}); got != nil {
		t.Errorf("a map whose every entry is reserved must collapse to nil, got %v", got)
	}
}

func upper(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		out = append(out, c)
	}
	return string(out)
}
