package proxyhandler

import (
	"context"
	"net/netip"
	"net/url"
	"testing"
)

type fixedMediaResolver struct {
	answers []netip.Addr
	calls   int
}

func (r *fixedMediaResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	r.calls++
	return r.answers, nil
}

func TestPublicMediaDownloadTargetPinsEveryValidatedAnswer(t *testing.T) {
	resolver := &fixedMediaResolver{answers: []netip.Addr{netip.MustParseAddr("93.184.216.34")}}
	target, host, sni, err := pinnedPublicMediaTarget(t.Context(), "https://cdn.fixture.invalid:443/movie?signature=fixture-private", resolver)
	if err != nil || target.Host != "93.184.216.34:443" || host != "cdn.fixture.invalid:443" || sni != "cdn.fixture.invalid" || resolver.calls != 1 {
		t.Fatalf("public target not pinned: %v %s %s %v", target, host, sni, err)
	}
	for _, addresses := range [][]netip.Addr{{netip.MustParseAddr("127.0.0.1")}, {netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("169.254.169.254")}, {netip.MustParseAddr("100.100.100.200")}} {
		if _, _, _, err = pinnedPublicMediaTarget(t.Context(), "https://cdn.fixture.invalid/movie", &fixedMediaResolver{answers: addresses}); err == nil {
			t.Fatal("mixed/non-public/metadata DNS answer accepted")
		}
	}
	userinfo := &url.URL{Scheme: "https", Host: "93.184.216.34", Path: "/file", User: url.UserPassword("fixture-user", "fixture-password")}
	for _, raw := range []string{"http://127.0.0.1/file", "http://metadata.google.internal/file", userinfo.String(), "https://93.184.216.34/file#secret", "file:///etc/passwd"} {
		if _, _, _, err = pinnedPublicMediaTarget(t.Context(), raw, resolver); err == nil {
			t.Fatalf("unsafe download URL accepted: %s", raw)
		}
	}
}
