package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

// ---------------------------------------------------------------------------
// normalizeIP tests
// ---------------------------------------------------------------------------

func TestNormalizeIP_PlainIPv4(t *testing.T) {
	if got := normalizeIP("192.168.1.1"); got != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %q", got)
	}
}

func TestNormalizeIP_PlainIPv6(t *testing.T) {
	if got := normalizeIP("2001:db8::1"); got != "2001:db8::1" {
		t.Errorf("expected 2001:db8::1, got %q", got)
	}
}

func TestNormalizeIP_IPv4MappedIPv6(t *testing.T) {
	if got := normalizeIP("::ffff:10.0.0.1"); got != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %q", got)
	}
}

func TestNormalizeIP_IPv4MappedIPv6WithSpace(t *testing.T) {
	if got := normalizeIP("::ffff: 127.0.0.1"); got != "127.0.0.1" {
		t.Errorf("expected 127.0.0.1, got %q", got)
	}
}

func TestNormalizeIP_IPv6Loopback(t *testing.T) {
	if got := normalizeIP("::1"); got != "127.0.0.1" {
		t.Errorf("expected 127.0.0.1, got %q", got)
	}
}

func TestNormalizeIP_Empty(t *testing.T) {
	if got := normalizeIP(""); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestNormalizeIP_WhitespaceOnly(t *testing.T) {
	if got := normalizeIP("   "); got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestNormalizeIP_LeadingTrailingWhitespace(t *testing.T) {
	if got := normalizeIP("  192.168.1.1  "); got != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// parseAllowlist tests
// ---------------------------------------------------------------------------

func TestParseAllowlist_Empty(t *testing.T) {
	result := parseAllowlist(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 entries, got %d", len(result))
	}
}

func TestParseAllowlist_ExactIP(t *testing.T) {
	result := parseAllowlist([]string{"192.168.1.1"})
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result[0].kind != "exact" {
		t.Errorf("expected kind=exact, got %q", result[0].kind)
	}
	if result[0].exactIP != "192.168.1.1" {
		t.Errorf("expected exactIP=192.168.1.1, got %q", result[0].exactIP)
	}
}

func TestParseAllowlist_ExactIPv6(t *testing.T) {
	result := parseAllowlist([]string{"2001:db8::1"})
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result[0].kind != "exact" {
		t.Errorf("expected kind=exact, got %q", result[0].kind)
	}
	if result[0].exactIP != "2001:db8::1" {
		t.Errorf("expected exactIP=2001:db8::1, got %q", result[0].exactIP)
	}
}

func TestParseAllowlist_IPv4MappedIPv6Exact(t *testing.T) {
	// ::ffff:x.x.x.x should be normalized to pure IPv4
	result := parseAllowlist([]string{"::ffff:10.0.0.1"})
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result[0].exactIP != "10.0.0.1" {
		t.Errorf("expected exactIP=10.0.0.1 (normalized), got %q", result[0].exactIP)
	}
}

func TestParseAllowlist_CIDR(t *testing.T) {
	result := parseAllowlist([]string{"10.0.0.0/8"})
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result[0].kind != "cidr" {
		t.Errorf("expected kind=cidr, got %q", result[0].kind)
	}
}

func TestParseAllowlist_CIDRPrefix0(t *testing.T) {
	// 0.0.0.0/0 should be valid — matches any IPv4
	result := parseAllowlist([]string{"0.0.0.0/0"})
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result[0].kind != "cidr" {
		t.Errorf("expected kind=cidr, got %q", result[0].kind)
	}
}

func TestParseAllowlist_InvalidCIDR(t *testing.T) {
	result := parseAllowlist([]string{"not-an-ip/24"})
	if len(result) != 0 {
		t.Errorf("expected 0 entries for invalid CIDR, got %d", len(result))
	}
}

func TestParseAllowlist_InvalidExactIP(t *testing.T) {
	result := parseAllowlist([]string{"not-an-ip"})
	if len(result) != 0 {
		t.Errorf("expected 0 entries for invalid IP, got %d", len(result))
	}
}

func TestParseAllowlist_EmptyEntry(t *testing.T) {
	result := parseAllowlist([]string{""})
	if len(result) != 0 {
		t.Errorf("expected 0 entries for empty string, got %d", len(result))
	}
}

func TestParseAllowlist_WhitespaceEntry(t *testing.T) {
	result := parseAllowlist([]string{"  "})
	if len(result) != 0 {
		t.Errorf("expected 0 entries for whitespace-only, got %d", len(result))
	}
}

func TestParseAllowlist_MultipleSlashInCIDR(t *testing.T) {
	// Multiple slashes should be skipped
	result := parseAllowlist([]string{"10.0.0.0/8/invalid"})
	if len(result) != 0 {
		t.Errorf("expected 0 entries for double-slash CIDR, got %d", len(result))
	}
}

func TestParseAllowlist_MixedEntries(t *testing.T) {
	result := parseAllowlist([]string{
		"192.168.1.1",
		"10.0.0.0/8",
		"invalid",
		"",
		"172.16.0.0/12",
	})
	if len(result) != 3 {
		t.Errorf("expected 3 valid entries, got %d: %+v", len(result), result)
	}
}

// ---------------------------------------------------------------------------
// isIPAllowed tests
// ---------------------------------------------------------------------------

func TestIsIPAllowed_EmptyAllowlist(t *testing.T) {
	// Empty allowlist means ALL IPs are allowed
	if !isIPAllowed("1.2.3.4", nil) {
		t.Error("expected IP allowed with nil allowlist")
	}
	if !isIPAllowed("1.2.3.4", []parsedAllowlistEntry{}) {
		t.Error("expected IP allowed with empty allowlist")
	}
}

func TestIsIPAllowed_ExactMatch(t *testing.T) {
	allowlist := parseAllowlist([]string{"192.168.1.100"})
	if !isIPAllowed("192.168.1.100", allowlist) {
		t.Error("expected exact IP match to be allowed")
	}
}

func TestIsIPAllowed_ExactMismatch(t *testing.T) {
	allowlist := parseAllowlist([]string{"192.168.1.100"})
	if isIPAllowed("192.168.1.200", allowlist) {
		t.Error("expected mismatched IP to be denied")
	}
}

func TestIsIPAllowed_CIDRMatch(t *testing.T) {
	allowlist := parseAllowlist([]string{"10.0.0.0/8"})
	if !isIPAllowed("10.1.2.3", allowlist) {
		t.Error("expected CIDR match to be allowed")
	}
}

func TestIsIPAllowed_CIDRMismatch(t *testing.T) {
	allowlist := parseAllowlist([]string{"10.0.0.0/8"})
	if isIPAllowed("192.168.1.1", allowlist) {
		t.Error("expected CIDR mismatch to be denied")
	}
}

func TestIsIPAllowed_CIDRPrefix0MatchesAll(t *testing.T) {
	// 0.0.0.0/0 should match any IPv4
	allowlist := parseAllowlist([]string{"0.0.0.0/0"})
	if !isIPAllowed("1.2.3.4", allowlist) {
		t.Error("expected 0.0.0.0/0 to match any IPv4")
	}
	if !isIPAllowed("255.255.255.255", allowlist) {
		t.Error("expected 0.0.0.0/0 to match any IPv4")
	}
}

func TestIsIPAllowed_IPv4MappedIPv6MatchedAsIPv4(t *testing.T) {
	// ::ffff:10.0.0.1 should be normalized to 10.0.0.1 and match CIDR 10.0.0.0/8
	allowlist := parseAllowlist([]string{"10.0.0.0/8"})
	if !isIPAllowed("::ffff:10.0.0.1", allowlist) {
		t.Error("expected IPv4-mapped IPv6 to be normalized and match CIDR")
	}
}

func TestIsIPAllowed_IPv6LoopbackMatchedAs127(t *testing.T) {
	// ::1 should be normalized to 127.0.0.1
	allowlist := parseAllowlist([]string{"127.0.0.1"})
	if !isIPAllowed("::1", allowlist) {
		t.Error("expected ::1 to be normalized to 127.0.0.1 and match exact entry")
	}
}

func TestIsIPAllowed_PureIPv6WithCIDROnly(t *testing.T) {
	// Pure IPv6 client with CIDR-only allowlist: CIDR entries only match IPv4
	allowlist := parseAllowlist([]string{"10.0.0.0/8"})
	if isIPAllowed("2001:db8::1", allowlist) {
		t.Error("expected pure IPv6 to be denied by IPv4 CIDR-only allowlist")
	}
}

func TestIsIPAllowed_PureIPv6WithExactMatch(t *testing.T) {
	// Pure IPv6 client with exact IPv6 entry in allowlist should match
	allowlist := parseAllowlist([]string{"2001:db8::1"})
	if !isIPAllowed("2001:db8::1", allowlist) {
		t.Error("expected pure IPv6 to match exact IPv6 entry")
	}
}

func TestIsIPAllowed_EmptyClientIP(t *testing.T) {
	allowlist := parseAllowlist([]string{"10.0.0.0/8"})
	if isIPAllowed("", allowlist) {
		t.Error("expected empty client IP to be denied")
	}
}

// ---------------------------------------------------------------------------
// isPublicAPIRoute tests
// ---------------------------------------------------------------------------

func TestIsPublicAPIRoute_OAuthCallback(t *testing.T) {
	if !isPublicAPIRoute("/api/oauth/callback/claude") {
		t.Error("expected /api/oauth/callback/claude to be public")
	}
}

func TestIsPublicAPIRoute_OAuthCallbackWithPath(t *testing.T) {
	if !isPublicAPIRoute("/api/oauth/callback/gemini/code") {
		t.Error("expected /api/oauth/callback/gemini/code to be public")
	}
}

func TestIsPublicAPIRoute_AdminRoute(t *testing.T) {
	if isPublicAPIRoute("/api/sites") {
		t.Error("expected /api/sites to NOT be public")
	}
}

func TestIsPublicAPIRoute_ProxyRoute(t *testing.T) {
	if isPublicAPIRoute("/v1/chat/completions") {
		t.Error("expected /v1/chat/completions to NOT be public")
	}
}

func TestIsPublicAPIRoute_SimilarButNotMatch(t *testing.T) {
	// /api/oauth/callbacks should not match /api/oauth/callback/
	if isPublicAPIRoute("/api/oauth/callbacks") {
		t.Error("expected /api/oauth/callbacks to NOT be public")
	}
}

// ---------------------------------------------------------------------------
// extractClientIP tests
// ---------------------------------------------------------------------------

func TestExtractClientIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "192.168.1.50:12345"
	if got := extractClientIP(req); got != "192.168.1.50" {
		t.Errorf("expected 192.168.1.50, got %q", got)
	}
}

func TestExtractClientIP_IgnoresXForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	req.RemoteAddr = "192.168.1.50:12345"
	if got := extractClientIP(req); got != "192.168.1.50" {
		t.Errorf("expected direct RemoteAddr 192.168.1.50, got %q", got)
	}
}

func TestExtractClientIP_IgnoresXForwardedForCommaSeparated(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1, 172.16.0.1, 192.168.1.1")
	req.RemoteAddr = "192.168.1.50:12345"
	if got := extractClientIP(req); got != "192.168.1.50" {
		t.Errorf("expected direct RemoteAddr 192.168.1.50, got %q", got)
	}
}

func TestExtractClientIP_IgnoresXForwardedForWithSpaces(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "  10.0.0.1 , 172.16.0.1")
	req.RemoteAddr = "192.168.1.50:12345"
	if got := extractClientIP(req); got != "192.168.1.50" {
		t.Errorf("expected direct RemoteAddr 192.168.1.50, got %q", got)
	}
}

func TestExtractClientIP_XForwardedForEmpty(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "")
	req.RemoteAddr = "192.168.1.50:12345"
	if got := extractClientIP(req); got != "192.168.1.50" {
		t.Errorf("expected RemoteAddr fallback 192.168.1.50, got %q", got)
	}
}

func TestExtractClientIP_IgnoresXForwardedForIPv4MappedIPv6(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("X-Forwarded-For", "::ffff:10.0.0.1")
	req.RemoteAddr = "192.168.1.50:12345"
	if got := extractClientIP(req); got != "192.168.1.50" {
		t.Errorf("expected direct RemoteAddr 192.168.1.50, got %q", got)
	}
}

func TestExtractClientIP_IgnoresXForwardedForMultiHeader(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Add("X-Forwarded-For", "")
	req.Header.Add("X-Forwarded-For", "10.0.0.99")
	req.RemoteAddr = "192.168.1.50:12345"
	if got := extractClientIP(req); got != "192.168.1.50" {
		t.Errorf("expected direct RemoteAddr 192.168.1.50, got %q", got)
	}
}

func TestExtractClientIP_IPv6RemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "[::1]:12345"
	if got := extractClientIP(req); got != "127.0.0.1" {
		t.Errorf("expected normalized 127.0.0.1 from ::1 RemoteAddr, got %q", got)
	}
}

func TestExtractClientIP_RemoteAddrNoPort(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "192.168.1.50"
	if got := extractClientIP(req); got != "192.168.1.50" {
		t.Errorf("expected 192.168.1.50, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// stripPort tests
// ---------------------------------------------------------------------------

func TestStripPort_IPv4WithPort(t *testing.T) {
	if got := stripPort("192.168.1.1:12345"); got != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %q", got)
	}
}

func TestStripPort_IPv4NoPort(t *testing.T) {
	if got := stripPort("192.168.1.1"); got != "192.168.1.1" {
		t.Errorf("expected 192.168.1.1, got %q", got)
	}
}

func TestStripPort_IPv6BracketNotation(t *testing.T) {
	if got := stripPort("[::1]:12345"); got != "::1" {
		t.Errorf("expected ::1, got %q", got)
	}
}

func TestStripPort_IPv6BracketNoPort(t *testing.T) {
	if got := stripPort("[::1]"); got != "::1" {
		t.Errorf("expected ::1, got %q", got)
	}
}

// ---------------------------------------------------------------------------
// AdminAuth middleware integration tests
// ---------------------------------------------------------------------------

// newTestConfig publishes the admin auth runtime settings (token + IP
// allowlist) through the global atomic RuntimeSettings snapshot that
// AdminAuth reads at wiring/request time, and returns the snapshot.
func newTestConfig(t *testing.T, authToken string, allowlist []string) *config.RuntimeSettings {
	t.Helper()
	rt := &config.RuntimeSettings{
		AuthToken:        authToken,
		AdminIpAllowlist: allowlist,
	}
	config.SetRuntime(rt)
	t.Cleanup(func() { config.SetRuntime(nil) })
	return rt
}

// adminTestHelper runs the AdminAuth middleware against a request and returns the response.
func adminTestHelper(t *testing.T, rt *config.RuntimeSettings, method, path, authHeader, remoteAddr, xff string) *httptest.ResponseRecorder {
	t.Helper()
	middleware := AdminAuth(nil)

	nextCalled := false
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(method, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	if remoteAddr != "" {
		req.RemoteAddr = remoteAddr
	} else {
		req.RemoteAddr = "127.0.0.1:12345"
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	_ = nextCalled // used for debugging
	return w
}

func TestAdminAuth_ValidToken(t *testing.T) {
	rt := newTestConfig(t, "my-secret-token", nil)
	w := adminTestHelper(t, rt, "GET", "/api/sites", "Bearer my-secret-token", "", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminAuth_ContextSetOnSuccess(t *testing.T) {
	// Verify that IsAdmin is set in the request context after successful auth.
	newTestConfig(t, "my-secret-token", nil) // publishes the admin auth runtime
	middleware := AdminAuth(nil)

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsAdmin(r.Context()) {
			t.Error("expected IsAdmin=true in handler context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/sites", nil)
	req.Header.Set("Authorization", "Bearer my-secret-token")
	req.RemoteAddr = "127.0.0.1:12345"

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAdminAuth_WrongToken(t *testing.T) {
	rt := newTestConfig(t, "my-secret-token", nil)
	w := adminTestHelper(t, rt, "GET", "/api/sites", "Bearer wrong-token", "", "")
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Invalid token") {
		t.Errorf("expected 'Invalid token' in body, got %q", w.Body.String())
	}
}

// errorCode contract: auth rejections carry the additive machine-readable
// code the frontend maps to localized toast copy (the "error" text stays the
// display fallback, so both fields are asserted here). The session-expired
// case lives in admin_session_test.go (it needs the session harness).
func TestAdminAuth_ErrorCodes(t *testing.T) {
	cases := []struct {
		name       string
		bearer     string
		remoteAddr string
		allow      []string
		wantCode   string
	}{
		{name: "wrong bearer token", bearer: "Bearer nope", wantCode: ErrorCodeAuthInvalidToken},
		{name: "missing credentials", wantCode: ErrorCodeAuthMissingCredential},
		{name: "ip not allowlisted", bearer: "Bearer my-secret-token", remoteAddr: "10.0.0.9:12345", allow: []string{"192.168.1.100"}, wantCode: ErrorCodeAuthIPBlocked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := newTestConfig(t, "my-secret-token", tc.allow)
			w := adminTestHelper(t, rt, "GET", "/api/sites", tc.bearer, tc.remoteAddr, "")
			var body struct {
				Error     string `json:"error"`
				ErrorCode string `json:"errorCode"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not JSON: %v", err)
			}
			if body.Error == "" {
				t.Errorf("expected human-readable error text, got %q", w.Body.String())
			}
			if body.ErrorCode != tc.wantCode {
				t.Errorf("expected errorCode %q, got %q", tc.wantCode, body.ErrorCode)
			}
		})
	}
}

func TestAdminAuth_MissingAuthorizationHeader(t *testing.T) {
	rt := newTestConfig(t, "my-secret-token", nil)
	w := adminTestHelper(t, rt, "GET", "/api/sites", "", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Missing Authorization header") {
		t.Errorf("expected 'Missing Authorization header' in body, got %q", w.Body.String())
	}
}

func TestAdminAuth_EmptyAuthorizationHeader(t *testing.T) {
	// r.Header.Get returns "" for non-existent headers; we pass "" which means "don't set"
	// This is equivalent to missing header.
	rt := newTestConfig(t, "my-secret-token", nil)
	w := adminTestHelper(t, rt, "GET", "/api/sites", "", "", "")
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAdminAuth_IPAllowlistExactMatch(t *testing.T) {
	rt := newTestConfig(t, "my-secret-token", []string{"192.168.1.100"})
	w := adminTestHelper(t, rt, "GET", "/api/sites", "Bearer my-secret-token",
		"192.168.1.100:12345", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (exact IP match), got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminAuth_IPAllowlistExactMismatch(t *testing.T) {
	rt := newTestConfig(t, "my-secret-token", []string{"192.168.1.100"})
	w := adminTestHelper(t, rt, "GET", "/api/sites", "Bearer my-secret-token",
		"192.168.1.200:12345", "")
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 (exact IP mismatch), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "IP not allowed") {
		t.Errorf("expected 'IP not allowed' in body, got %q", w.Body.String())
	}
}

func TestAdminAuth_IPAllowlistCIDRMatch(t *testing.T) {
	rt := newTestConfig(t, "my-secret-token", []string{"10.0.0.0/8"})
	w := adminTestHelper(t, rt, "GET", "/api/sites", "Bearer my-secret-token",
		"10.1.2.3:12345", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (CIDR match), got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminAuth_IPAllowlistCIDRMismatch(t *testing.T) {
	rt := newTestConfig(t, "my-secret-token", []string{"10.0.0.0/8"})
	w := adminTestHelper(t, rt, "GET", "/api/sites", "Bearer my-secret-token",
		"192.168.1.1:12345", "")
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 (CIDR mismatch), got %d", w.Code)
	}
}

func TestAdminAuth_IPAllowlistEmptyAllowsAll(t *testing.T) {
	// Empty allowlist should skip IP check entirely
	rt := newTestConfig(t, "my-secret-token", nil)
	w := adminTestHelper(t, rt, "GET", "/api/sites", "Bearer my-secret-token",
		"any-ip:12345", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (empty allowlist), got %d", w.Code)
	}
}

func TestAdminAuth_IPAllowlistEmptySliceAllowsAll(t *testing.T) {
	rt := newTestConfig(t, "my-secret-token", []string{})
	w := adminTestHelper(t, rt, "GET", "/api/sites", "Bearer my-secret-token",
		"any-ip:12345", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (empty slice allowlist), got %d", w.Code)
	}
}

func TestAdminAuth_IgnoresSpoofedXForwardedForForAllowlist(t *testing.T) {
	// AdminAuth only trusts RemoteAddr. router.TrustedRealIP owns forwarded-header trust.
	rt := newTestConfig(t, "my-secret-token", []string{"10.0.0.1"})
	w := adminTestHelper(t, rt, "GET", "/api/sites", "Bearer my-secret-token",
		"192.168.1.1:12345", "10.0.0.1")
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 (spoofed XFF ignored), got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminAuth_PublicRouteBypass_OAuthCallback(t *testing.T) {
	rt := newTestConfig(t, "my-secret-token", []string{"10.0.0.0/8"})
	w := adminTestHelper(t, rt, "GET", "/api/oauth/callback/claude", "",
		"1.2.3.4:12345", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (public route bypass), got %d", w.Code)
	}
}

func TestAdminAuth_StartupPhase(t *testing.T) {
	// When no AUTH_TOKEN is set (using default), the token check must still work
	// but the default token has a fixed value. We test with an explicit token.
	rt := newTestConfig(t, "test-token", nil)
	w := adminTestHelper(t, rt, "GET", "/api/sites", "Bearer test-token", "", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAdminAuth_IPv4MappedIPv6Normalization(t *testing.T) {
	// ::ffff:10.0.0.1 should normalize to 10.0.0.1 and match CIDR 10.0.0.0/8
	rt := newTestConfig(t, "my-secret-token", []string{"10.0.0.0/8"})
	w := adminTestHelper(t, rt, "GET", "/api/sites", "Bearer my-secret-token",
		"[::ffff:10.0.0.1]:12345", "")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (IPv4-mapped IPv6 normalization), got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminAuth_IPv6LoopbackNormalization(t *testing.T) {
	// ::1 should normalize to 127.0.0.1
	rt := newTestConfig(t, "my-secret-token", []string{"127.0.0.1"})
	w := adminTestHelper(t, rt, "GET", "/api/sites", "Bearer my-secret-token",
		"", "::1")
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (IPv6 loopback normalization), got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminAuth_CaseSensitiveBearer(t *testing.T) {
	// AdminAuth uses case-SENSITIVE simple replace of "Bearer " (not regex)
	rt := newTestConfig(t, "my-secret-token", nil)
	w := adminTestHelper(t, rt, "GET", "/api/sites", "bearer my-secret-token", "", "")
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for case-sensitive Bearer mismatch, got %d", w.Code)
	}
}

func TestAdminAuth_BearerPrefixOnly(t *testing.T) {
	// "Bearer " without a token should not match the auth token
	rt := newTestConfig(t, "my-secret-token", nil)
	w := adminTestHelper(t, rt, "GET", "/api/sites", "Bearer ", "", "")
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for empty token after Bearer, got %d", w.Code)
	}
}

func TestAdminAuth_NoBearerPrefix(t *testing.T) {
	// Token without "Bearer " prefix should be compared literally
	rt := newTestConfig(t, "Bearer my-secret-token", nil)
	w := adminTestHelper(t, rt, "GET", "/api/sites", "Bearer my-secret-token", "", "")
	// replace("Bearer ", "") reduces it to "my-secret-token", which doesn't match "Bearer my-secret-token"
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 for mismatched after replace, got %d", w.Code)
	}
}

func TestAdminAuth_ResponseContentType(t *testing.T) {
	rt := newTestConfig(t, "my-secret-token", nil)
	w := adminTestHelper(t, rt, "GET", "/api/sites", "Bearer wrong", "", "")
	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected application/json Content-Type, got %q", ct)
	}
}

func TestAdminAuth_IPCheckBeforeAuthCheck(t *testing.T) {
	// IP check happens BEFORE Authorization check (order from spec).
	// An IP-denied request should get 403 even with missing auth header.
	rt := newTestConfig(t, "my-secret-token", []string{"10.0.0.0/8"})
	w := adminTestHelper(t, rt, "GET", "/api/sites", "", // no auth header
		"192.168.1.1:12345", "")
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 (IP denied before auth check), got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "IP not allowed") {
		t.Errorf("expected 'IP not allowed', got %q", w.Body.String())
	}
}

// ---------------------------------------------------------------------------
// parseAllowlistWithDiagnostics tests
// ---------------------------------------------------------------------------

func TestParseAllowlistWithDiagnostics_ReportsInvalidEntries(t *testing.T) {
	parsed, invalid := parseAllowlistWithDiagnostics([]string{
		"192.168.1.1",
		"10.0.0.0/8",
		"not-an-ip",
		"",
		"172.16.0.0/12",
		"10.0.0.0/8/invalid",
	})

	if len(parsed) != 3 {
		t.Errorf("expected 3 valid entries, got %d: %+v", len(parsed), parsed)
	}
	wantInvalid := []string{"not-an-ip", "10.0.0.0/8/invalid"}
	if len(invalid) != len(wantInvalid) {
		t.Fatalf("expected %d invalid entries, got %d: %v", len(wantInvalid), len(invalid), invalid)
	}
	for i, want := range wantInvalid {
		if invalid[i] != want {
			t.Errorf("invalid[%d] = %q, want %q", i, invalid[i], want)
		}
	}
}

func TestParseAllowlistWithDiagnostics_WhitespaceNotReported(t *testing.T) {
	// Whitespace-only entries are no-ops, not misconfigurations — they must
	// not appear in the invalid list.
	parsed, invalid := parseAllowlistWithDiagnostics([]string{"  ", "", "\t"})
	if len(parsed) != 0 {
		t.Errorf("expected 0 valid entries, got %d", len(parsed))
	}
	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid entries for whitespace-only input, got %v", invalid)
	}
}

func TestParseAllowlistWithDiagnostics_AllValid(t *testing.T) {
	parsed, invalid := parseAllowlistWithDiagnostics([]string{
		"192.168.1.1",
		"10.0.0.0/8",
		"::ffff:10.0.0.1",
	})
	if len(parsed) != 3 {
		t.Errorf("expected 3 valid entries, got %d", len(parsed))
	}
	if len(invalid) != 0 {
		t.Errorf("expected 0 invalid entries, got %v", invalid)
	}
}

func TestParseAllowlist_DelegatesAndDiscardsInvalid(t *testing.T) {
	// parseAllowlist must remain backward-compatible: it returns only valid
	// entries and silently drops invalid ones (same as before the diagnostics
	// split, so downstream.go callers keep working unchanged).
	result := parseAllowlist([]string{
		"192.168.1.1",
		"not-an-ip",
		"10.0.0.0/8",
	})
	if len(result) != 2 {
		t.Errorf("expected 2 valid entries (invalid dropped), got %d", len(result))
	}
}
