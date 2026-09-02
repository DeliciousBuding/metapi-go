package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

func TestTrustedRealIPIgnoresForwardedHeadersWithoutTrustedProxy(t *testing.T) {
	handler := TrustedRealIP(&config.Config{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "203.0.113.10:12345" {
			t.Fatalf("RemoteAddr = %q, want original direct peer", r.RemoteAddr)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.99")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestTrustedRealIPUsesForwardedHeaderFromTrustedProxy(t *testing.T) {
	handler := TrustedRealIP(&config.Config{
		TrustedProxyCidrs: []string{"127.0.0.1/32"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "198.51.100.99" {
			t.Fatalf("RemoteAddr = %q, want forwarded client IP", r.RemoteAddr)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Forwarded-For", "198.51.100.99, 127.0.0.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestTrustedRealIPIgnoresForwardedHeaderFromUntrustedPeer(t *testing.T) {
	handler := TrustedRealIP(&config.Config{
		TrustedProxyCidrs: []string{"127.0.0.1/32"},
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RemoteAddr != "203.0.113.10:12345" {
			t.Fatalf("RemoteAddr = %q, want original untrusted peer", r.RemoteAddr)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "203.0.113.10:12345"
	req.Header.Set("X-Forwarded-For", "198.51.100.99")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestRequestLoggerCapturesStatusAndBytes(t *testing.T) {
	var gotStatus, gotBytes int
	handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("hello"))
		sw, ok := w.(*statusRecorder)
		if !ok {
			t.Fatalf("writer = %T, want *statusRecorder", w)
		}
		gotStatus = sw.Status()
		gotBytes = sw.BytesWritten()
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", rec.Code)
	}
	if gotStatus != http.StatusCreated {
		t.Fatalf("recorded status = %d, want 201", gotStatus)
	}
	if gotBytes != 5 {
		t.Fatalf("recorded bytes = %d, want 5", gotBytes)
	}
	if rec.Body.String() != "hello" {
		t.Fatalf("body = %q, want hello", rec.Body.String())
	}
}

func TestRequestLoggerPreservesStreamingInterfaces(t *testing.T) {
	base := &fakeStreamWriter{}
	handler := RequestLogger(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Flusher); !ok {
			t.Fatalf("wrapped writer lost http.Flusher")
		}
		sd, ok := w.(interface{ SetWriteDeadline(time.Time) error })
		if !ok {
			t.Fatalf("wrapped writer lost SetWriteDeadline")
		}
		if err := sd.SetWriteDeadline(time.Time{}); err != nil {
			t.Fatalf("SetWriteDeadline: %v", err)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(base, req)

	if !base.deadlineSet {
		t.Fatalf("SetWriteDeadline was not forwarded to the underlying writer")
	}
}

func TestRecovererReturns500OnPanic(t *testing.T) {
	handler := Recoverer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

type fakeStreamWriter struct {
	header      http.Header
	status      int
	deadlineSet bool
}

func (f *fakeStreamWriter) Header() http.Header {
	if f.header == nil {
		f.header = http.Header{}
	}
	return f.header
}
func (f *fakeStreamWriter) WriteHeader(code int)        { f.status = code }
func (f *fakeStreamWriter) Write(b []byte) (int, error) { return len(b), nil }
func (f *fakeStreamWriter) Flush()                      {}
func (f *fakeStreamWriter) SetWriteDeadline(t time.Time) error {
	f.deadlineSet = true
	return nil
}

// forwardedProbe describes a request as it reaches the trusted-proxy gate.
// xff holds one entry per X-Forwarded-For header, in wire order, so a probe can
// reproduce the multi-header chains real proxy stacks produce.
type forwardedProbe struct {
	peer   string
	xff    []string
	realIP string
}

// runTrustedRealIPProbe drives TrustedRealIP and reports the client identity the
// middleware settled on, i.e. r.RemoteAddr as the wrapped handler sees it.
func runTrustedRealIPProbe(t *testing.T, cidrs []string, probe forwardedProbe) string {
	t.Helper()

	var resolved string
	handler := TrustedRealIP(&config.Config{
		TrustedProxyCidrs: cidrs,
	})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolved = r.RemoteAddr
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = probe.peer
	for _, value := range probe.xff {
		req.Header.Add("X-Forwarded-For", value)
	}
	if probe.realIP != "" {
		req.Header.Set("X-Real-IP", probe.realIP)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	return resolved
}

func assertForwardedProbes(t *testing.T, tests []struct {
	name  string
	cidrs []string
	probe forwardedProbe
	want  string
}) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runTrustedRealIPProbe(t, tt.cidrs, tt.probe)
			if got != tt.want {
				t.Fatalf("resolved client IP = %q, want %q (peer=%q xff=%q real-ip=%q)",
					got, tt.want, tt.probe.peer, tt.probe.xff, tt.probe.realIP)
			}
		})
	}
}

func TestTrustedRealIPDeniesSpoofedLeftmostAllowlistAndRateLimitBypass(t *testing.T) {
	// Attack shape: the client sends its own X-Forwarded-For and the reverse proxy
	// appends the real peer behind it (nginx: proxy_set_header X-Forwarded-For
	// $proxy_add_x_forwarded_for), so the chain is [forged value, real client,
	// hops...]. Reading the left-most entry would let any origin forge the admin
	// IP allowlist (handler/admin/settings.go) and rotate the per-IP token bucket
	// on every request (auth/admin.go), so the walk has to start at the right and
	// stop at the first hop outside TRUSTED_PROXY_CIDRS.
	assertForwardedProbes(t, []struct {
		name  string
		cidrs []string
		probe forwardedProbe
		want  string
	}{
		{
			name:  "forged_leftmost_value_never_becomes_the_client_identity",
			cidrs: []string{"127.0.0.1/32"},
			probe: forwardedProbe{
				peer: "127.0.0.1:54321",
				xff:  []string{"203.0.113.66, 198.51.100.99"},
			},
			want: "198.51.100.99",
		},
		{
			name:  "forged_value_matching_the_admin_allowlist_is_skipped",
			cidrs: []string{"127.0.0.1/32"},
			probe: forwardedProbe{
				peer: "127.0.0.1:54321",
				xff:  []string{"10.20.30.40, 10.20.30.41, 198.51.100.99"},
			},
			want: "198.51.100.99",
		},
	})
}

func TestTrustedRealIPSkipsEveryTrustedProxyHop(t *testing.T) {
	// A chain that crosses several trusted proxies must keep walking left until it
	// leaves TRUSTED_PROXY_CIDRS; an entry that is not trusted terminates the walk
	// even when trusted hops sit to its left.
	assertForwardedProbes(t, []struct {
		name  string
		cidrs []string
		probe forwardedProbe
		want  string
	}{
		{
			name:  "two_trusted_hops_between_proxy_and_client",
			cidrs: []string{"127.0.0.1/32", "10.0.0.0/8"},
			probe: forwardedProbe{
				peer: "127.0.0.1:1",
				xff:  []string{"198.51.100.99, 10.0.0.5, 10.0.0.6"},
			},
			want: "198.51.100.99",
		},
		{
			name:  "forged_leftmost_value_ahead_of_multi_hop_chain",
			cidrs: []string{"127.0.0.1/32", "10.0.0.0/8"},
			probe: forwardedProbe{
				peer: "127.0.0.1:1",
				xff:  []string{"203.0.113.66, 198.51.100.99, 10.0.0.5, 10.0.0.6"},
			},
			want: "198.51.100.99",
		},
		{
			name:  "walk_stops_at_first_untrusted_entry_seen_from_the_right",
			cidrs: []string{"127.0.0.1/32", "10.0.0.0/8"},
			probe: forwardedProbe{
				peer: "127.0.0.1:1",
				xff:  []string{"10.0.0.4, 198.51.100.99, 10.0.0.6"},
			},
			want: "198.51.100.99",
		},
	})
}

func TestTrustedRealIPJoinsEveryForwardedHeaderInOrder(t *testing.T) {
	// Each proxy in the stack may emit its own X-Forwarded-For header instead of
	// extending the previous one, so the chain is the concatenation of all headers
	// in wire order, not the first segment of the first header.
	assertForwardedProbes(t, []struct {
		name  string
		cidrs []string
		probe forwardedProbe
		want  string
	}{
		{
			name:  "client_identity_lives_in_the_second_header",
			cidrs: []string{"127.0.0.1/32"},
			probe: forwardedProbe{
				peer: "127.0.0.1:1",
				xff:  []string{"203.0.113.9", "198.51.100.7, 127.0.0.1"},
			},
			want: "198.51.100.7",
		},
		{
			name:  "three_headers_are_walked_right_to_left",
			cidrs: []string{"127.0.0.1/32"},
			probe: forwardedProbe{
				peer: "127.0.0.1:1",
				xff:  []string{"203.0.113.9", "198.51.100.7", "127.0.0.1"},
			},
			want: "198.51.100.7",
		},
	})
}

func TestTrustedRealIPFallsBackToLeftmostWhenWholeChainIsTrusted(t *testing.T) {
	// Internal proxy chains in front of an internal client produce a chain where
	// every hop is trusted. Returning nothing would collapse all of those callers
	// into a single rate-limit bucket, so the left-most entry is kept as a stable
	// per-client key. The peer is never the answer: it is a hop by definition.
	assertForwardedProbes(t, []struct {
		name  string
		cidrs []string
		probe forwardedProbe
		want  string
	}{
		{
			name:  "all_hops_trusted_returns_leftmost_entry",
			cidrs: []string{"127.0.0.1/32", "10.0.0.0/8"},
			probe: forwardedProbe{
				peer: "127.0.0.1:1",
				xff:  []string{"10.0.0.5, 10.0.0.6"},
			},
			want: "10.0.0.5",
		},
		{
			name:  "unparseable_entries_do_not_hide_the_leftmost_trusted_hop",
			cidrs: []string{"127.0.0.1/32", "10.0.0.0/8"},
			probe: forwardedProbe{
				peer: "127.0.0.1:1",
				xff:  []string{"not-an-ip, 10.0.0.5, 10.0.0.6"},
			},
			want: "10.0.0.5",
		},
	})
}

func TestTrustedRealIPResolvesIPv6ForwardedChain(t *testing.T) {
	// IPv6 chains reach us bracketed and IPv4-mapped; both forms must be
	// normalised before they are matched against TRUSTED_PROXY_CIDRS, and the
	// resolved client is reported as a bare address without a port.
	assertForwardedProbes(t, []struct {
		name  string
		cidrs []string
		probe forwardedProbe
		want  string
	}{
		{
			name:  "bracketed_and_mapped_entries_are_walked_right_to_left",
			cidrs: []string{"::1/128", "2001:db8::/32"},
			probe: forwardedProbe{
				peer: "[::1]:1",
				xff:  []string{"[2001:db8::9], ::ffff:198.51.100.99, 2001:db8::5"},
			},
			want: "198.51.100.99",
		},
		{
			name:  "mapped_peer_and_mapped_client_entry",
			cidrs: []string{"127.0.0.1/32"},
			probe: forwardedProbe{
				peer: "[::ffff:127.0.0.1]:54321",
				xff:  []string{"203.0.113.66, ::ffff:198.51.100.99"},
			},
			want: "198.51.100.99",
		},
		{
			name:  "forged_entry_inside_the_trusted_prefix_does_not_shadow_the_real_client",
			cidrs: []string{"2001:db8::/32"},
			probe: forwardedProbe{
				peer: "[2001:db8::1]:443",
				xff:  []string{"2001:db8::66, 203.0.113.66"},
			},
			want: "203.0.113.66",
		},
	})
}

func TestTrustedRealIPFallsBackToRealIPWhenForwardedChainIsUnusable(t *testing.T) {
	// X-Real-IP stays the single-value fallback, and it is only consulted when
	// X-Forwarded-For produced no address at all. An unusable pair of headers must
	// leave the peer untouched rather than rewrite it to garbage.
	assertForwardedProbes(t, []struct {
		name  string
		cidrs []string
		probe forwardedProbe
		want  string
	}{
		{
			name:  "garbage_forwarded_chain_falls_back_to_real_ip",
			cidrs: []string{"127.0.0.1/32"},
			probe: forwardedProbe{
				peer:   "127.0.0.1:1",
				xff:    []string{"not-an-ip, , 127.0.0.1:9999"},
				realIP: "198.51.100.44",
			},
			want: "198.51.100.44",
		},
		{
			name:  "real_ip_only",
			cidrs: []string{"127.0.0.1/32"},
			probe: forwardedProbe{
				peer:   "127.0.0.1:1",
				realIP: "198.51.100.44",
			},
			want: "198.51.100.44",
		},
		{
			name:  "unusable_headers_leave_the_peer_untouched",
			cidrs: []string{"127.0.0.1/32"},
			probe: forwardedProbe{
				peer:   "127.0.0.1:1",
				xff:    []string{"not-an-ip"},
				realIP: "also-not-an-ip",
			},
			want: "127.0.0.1:1",
		},
	})
}
