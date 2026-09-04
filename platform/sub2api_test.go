package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSub2ApiAdapter_Detect_URLKeyword(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	ctx := context.Background()

	tests := []struct {
		url     string
		matches bool
	}{
		{"https://sub2api.example.com/v1/models", true},
		{"https://SUB2API.example.com", true},
		{"https://example.com/sub2api/v1", true},
	}
	for _, tt := range tests {
		ok, err := s.Detect(ctx, tt.url)
		if err != nil {
			t.Errorf("Detect(%q) error: %v", tt.url, err)
			continue
		}
		if ok != tt.matches {
			t.Errorf("Detect(%q) = %v, want %v", tt.url, ok, tt.matches)
		}
	}
}

func TestSub2ApiAdapter_Detect_ErrorEnvelope(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/me" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"UNAUTHORIZED","message":"authorization header is required"}`))
	}))
	defer server.Close()

	ok, err := s.Detect(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if !ok {
		t.Fatal("Detect should match a Sub2API authorization error envelope")
	}
}

func TestSub2ApiAdapter_Detect_RejectsOversizedErrorEnvelope(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"UNAUTHORIZED","message":"` + strings.Repeat("x", platformJSONResponseBodyLimit) + `"}`))
	}))
	defer server.Close()

	ok, err := s.Detect(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Detect returned error: %v", err)
	}
	if ok {
		t.Fatal("Detect should reject oversized Sub2API error envelopes")
	}
}

func TestSub2ApiAdapter_LoginUnspported(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	ctx := context.Background()

	lr, err := s.Login(ctx, "http://x", "u", "p", nil, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if lr.Success {
		t.Error("Sub2API login should always return Success=false")
	}
	if lr.Message != "Sub2API uses JWT authentication; login is not supported" {
		t.Errorf("Login message: %q", lr.Message)
	}
}

func TestSub2ApiAdapter_CheckinUnspported(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	ctx := context.Background()

	cr, err := s.Checkin(ctx, "http://x", "t", nil, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if cr.Success {
		t.Error("Sub2API checkin should always return Success=false")
	}
	if cr.Message != "Check-in is not supported by Sub2API" {
		t.Errorf("Checkin message: %q", cr.Message)
	}
}

func TestSub2ApiAdapter_GetBalance(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// On an unreachable URL, GetBalance must surface the fetch error instead
	// of swallowing it into an empty BalanceInfo. Otherwise balance-refresh
	// callers would overwrite stored balance/quota with zeros and revive
	// expired accounts on a transient upstream failure.
	bi, err := s.GetBalance(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err == nil {
		t.Errorf("GetBalance on unreachable should surface the fetch error")
	}
	if bi != nil {
		t.Errorf("GetBalance on unreachable should return nil BalanceInfo, got %+v", bi)
	}
}

func TestSub2ApiAdapter_GetModels(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	ctx := context.Background()

	models, err := s.GetModels(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err == nil {
		t.Error("GetModels on unreachable should surface a fetch error")
	}
	if len(models) != 0 {
		t.Error("GetModels on unreachable should return empty models")
	}
}

func TestSub2ApiAdapter_ParseSub2ApiEnvelope(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}

	// Valid envelope: code=0
	resp := map[string]interface{}{
		"code": float64(0),
		"data": map[string]interface{}{"key": "val"},
	}
	err := s.parseSub2ApiEnvelope(resp, "/test")
	if err != nil {
		t.Errorf("valid envelope should not error: %v", err)
	}

	// Error envelope: code != 0
	resp2 := map[string]interface{}{
		"code":    float64(401),
		"message": "Unauthorized",
	}
	err2 := s.parseSub2ApiEnvelope(resp2, "/test")
	if err2 == nil {
		t.Error("error envelope should return error")
	}

	// Missing code
	resp3 := map[string]interface{}{}
	err3 := s.parseSub2ApiEnvelope(resp3, "/test")
	if err3 == nil {
		t.Error("missing code should return error")
	}

	// Non-numeric code
	resp4 := map[string]interface{}{
		"code": "not-a-number",
	}
	err4 := s.parseSub2ApiEnvelope(resp4, "/test")
	if err4 == nil {
		t.Error("non-numeric code should return error")
	}

	// code=0 but missing data
	resp5 := map[string]interface{}{
		"code": float64(0),
	}
	err5 := s.parseSub2ApiEnvelope(resp5, "/test")
	if err5 == nil {
		t.Error("missing data should return error")
	}
}

func TestSub2ApiAdapter_ParseSub2ApiEnvelopeRaw(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}

	resp := map[string]interface{}{
		"code": float64(0),
		"data": map[string]interface{}{"username": "test"},
	}
	data, err := s.parseSub2ApiEnvelopeRaw(resp, "/test")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if data == nil {
		t.Error("data should not be nil")
	}
}

func TestSub2ApiAdapter_UsdToQuota(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}

	q := s.usdToQuota(1.0)
	if q != 500000 {
		t.Errorf("1 USD: %f, want 500000", q)
	}

	q2 := s.usdToQuota(0)
	if q2 != 0 {
		t.Errorf("0 USD: %f, want 0", q2)
	}

	q3 := s.usdToQuota(-5)
	if q3 != 0 {
		t.Errorf("negative USD: %f, want 0", q3)
	}

	q4 := s.usdToQuota(2.5)
	if q4 != 1250000 {
		t.Errorf("2.5 USD: %f, want 1250000", q4)
	}
}

func TestSub2ApiAdapter_ResolveExpiresInDays(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}

	// Zero / negative
	if d := s.resolveExpiresInDays(0); d != 0 {
		t.Errorf("resolveExpiresInDays(0) = %d, want 0", d)
	}
	if d := s.resolveExpiresInDays(-1); d != 0 {
		t.Errorf("resolveExpiresInDays(-1) = %d, want 0", d)
	}

	// The cap is applied when delta days > 3650. Timestamps in the past
	// produce delta < 0, which maxes out at 1.
	// Test with 0/negative is sufficient; cap on far-future requires
	// a timestamp that is > 3650 days from now.
	now := time.Now()
	farMs := now.Add(4000 * 24 * time.Hour).UnixMilli()
	// farMs is already > 10_000_000_000 so it's treated as ms directly
	if d := s.resolveExpiresInDays(farMs); d != 3650 {
		t.Errorf("far future (4000 days) capped: %d, want 3650", d)
	}
}

func TestSub2ApiAdapter_ResolveManagementBaseURL(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}

	tests := []struct {
		input    string
		expected string
	}{
		{"https://sub2api.example.com/v1/models", "https://sub2api.example.com"},
		{"https://sub2api.example.com/api/v1/models", "https://sub2api.example.com"},
		{"https://sub2api.example.com/antigravity/v1beta/models", "https://sub2api.example.com"},
		{"https://sub2api.example.com", "https://sub2api.example.com"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := s.resolveManagementBaseURL(tt.input); got != tt.expected {
			t.Errorf("resolveManagementBaseURL(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestSub2ApiAdapter_ResolveModelEndpoints(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}

	// normalizeBaseURL strips path to origin, so all URLs resolve to 4 candidates
	// (the /models suffix check operates on the origin, which never has /models)
	result := s.resolveModelEndpoints("https://example.com/v1/models")
	if len(result) != 4 {
		t.Fatalf("expected 4 endpoints, got %d: %v", len(result), result)
	}

	// Regular URL: 4 candidates
	result2 := s.resolveModelEndpoints("https://example.com")
	if len(result2) != 4 {
		t.Fatalf("expected 4 endpoints, got %d: %v", len(result2), result2)
	}
	expected := []string{
		"https://example.com/v1/models",
		"https://example.com/api/v1/models",
		"https://example.com/v1beta/models",
		"https://example.com/antigravity/v1beta/models",
	}
	for i, e := range expected {
		if result2[i] != e {
			t.Errorf("endpoint[%d]: %q, want %q", i, result2[i], e)
		}
	}

	// URL ending with /antigravity also gets normalized to origin
	result3 := s.resolveModelEndpoints("https://example.com/antigravity")
	if len(result3) != 4 {
		t.Fatalf("antigravity base: expected 4 endpoints, got %d: %v", len(result3), result3)
	}

	// Empty
	result5 := s.resolveModelEndpoints("")
	if result5 != nil {
		t.Errorf("empty URL should return nil: %v", result5)
	}
}

func TestSub2ApiAdapter_GetUserGroups(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	ctx := context.Background()

	groups, err := s.GetUserGroups(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(groups) != 1 || groups[0] != "default" {
		t.Errorf("GetUserGroups on unreachable should return ['default'], got %v", groups)
	}
}

// sub2apiKeyUpstream stands in for a sub2api version: `allow` decides which of the
// two version-dependent endpoint families exists, and every other path 404s the way
// a version without it would.
type sub2apiKeyUpstream struct {
	mu       sync.Mutex
	allow    func(method, path string) bool
	listBody string
	writeOK  bool
	calls    []string
}

func (f *sub2apiKeyUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if f.allow == nil || !f.allow(r.Method, r.URL.Path) {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		_, _ = w.Write([]byte(f.listBody))
	case http.MethodPost:
		if f.writeOK {
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":7,"key":"sk-new"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":1,"message":"upstream refused the create"}`))
	case http.MethodDelete:
		if f.writeOK {
			_, _ = w.Write([]byte(`{"code":0,"data":null}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":1,"message":"upstream refused the delete"}`))
	default:
		http.NotFound(w, r)
	}
}

func (f *sub2apiKeyUpstream) callLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// onlyAPIKeys accepts the newer endpoint family and 404s the older one, which is
// the version skew the two-endpoint ladder exists for.
func onlyAPIKeys(method, path string) bool { return strings.HasPrefix(path, "/api/v1/api-keys") }

func bothKeyFamilies(method, path string) bool {
	return strings.HasPrefix(path, "/api/v1/keys") || strings.HasPrefix(path, "/api/v1/api-keys")
}

// A failed create must not come back as the same `false, nil` the caller reports as
// "the upstream refused": an unreachable site never refused anything, and the
// operator is left with a 502 that has no reason attached.
func TestSub2ApiAdapter_CreateAPIToken(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	created, err := s.CreateAPIToken(ctx, unreachableBaseURL(t), "token", nil, nil, nil)
	if err == nil {
		t.Error("CreateAPIToken must report an unreachable upstream instead of a silent false")
	}
	if created {
		t.Error("nothing was created")
	}

	t.Run("the second endpoint family still creates", func(t *testing.T) {
		f := &sub2apiKeyUpstream{allow: onlyAPIKeys, writeOK: true}
		srv := httptest.NewServer(f)
		defer srv.Close()
		created, err := s.CreateAPIToken(ctx, srv.URL, "token", nil, nil, nil)
		if err != nil || !created {
			t.Fatalf("create = (%v, %v), want (true, nil)", created, err)
		}
		got := f.callLog()
		want := []string{"POST /api/v1/keys", "POST /api/v1/api-keys"}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("calls = %v, want %v", got, want)
		}
	})

	t.Run("an answered refusal carries the upstream reason", func(t *testing.T) {
		f := &sub2apiKeyUpstream{allow: bothKeyFamilies, writeOK: false}
		srv := httptest.NewServer(f)
		defer srv.Close()
		created, err := s.CreateAPIToken(ctx, srv.URL, "token", nil, nil, nil)
		if created {
			t.Fatal("created = true for a refused create")
		}
		if err == nil || !strings.Contains(err.Error(), "upstream refused the create") {
			t.Fatalf("error should carry the upstream reason, got %v", err)
		}
	})
}

// Both directions on purpose, because the caller deletes the local row only when
// this returns nil. "Already absent upstream" is idempotence; "no request reached
// the upstream" is a failure that used to be reported as the same nil.
func TestSub2ApiAdapter_DeleteAPIToken(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.DeleteAPIToken(ctx, unreachableBaseURL(t), "token", "sk-test", nil, nil); err == nil {
		t.Fatal("DeleteAPIToken reported a completed delete for a call that never reached the upstream")
	}

	t.Run("empty key needs no upstream call", func(t *testing.T) {
		f := &sub2apiKeyUpstream{allow: bothKeyFamilies, listBody: `{"code":0,"data":[]}`}
		srv := httptest.NewServer(f)
		defer srv.Close()
		if err := s.DeleteAPIToken(ctx, srv.URL, "token", "", nil, nil); err != nil {
			t.Fatalf("empty key: %v", err)
		}
		if got := f.callLog(); len(got) != 0 {
			t.Fatalf("empty key made upstream calls: %v", got)
		}
	})

	t.Run("key already absent upstream is idempotent", func(t *testing.T) {
		f := &sub2apiKeyUpstream{
			allow:    onlyAPIKeys,
			listBody: `{"code":0,"data":[{"id":1,"key":"sk-someone-else"}]}`,
		}
		srv := httptest.NewServer(f)
		defer srv.Close()
		if err := s.DeleteAPIToken(ctx, srv.URL, "token", "sk-test", nil, nil); err != nil {
			t.Fatalf("an answered listing without this key is not an error: %v", err)
		}
	})

	t.Run("the second endpoint family still deletes", func(t *testing.T) {
		f := &sub2apiKeyUpstream{
			allow:    onlyAPIKeys,
			listBody: `{"code":0,"data":[{"id":7,"key":"sk-test"}]}`,
			writeOK:  true,
		}
		srv := httptest.NewServer(f)
		defer srv.Close()
		if err := s.DeleteAPIToken(ctx, srv.URL, "token", "sk-test", nil, nil); err != nil {
			t.Fatalf("delete across the version skew: %v", err)
		}
		got := f.callLog()
		want := []string{
			"GET /api/v1/keys", "GET /api/v1/api-keys",
			"DELETE /api/v1/keys/7", "DELETE /api/v1/api-keys/7",
		}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("calls = %v, want %v", got, want)
		}
	})

	t.Run("both delete variants refusing is a failure", func(t *testing.T) {
		f := &sub2apiKeyUpstream{
			allow:    bothKeyFamilies,
			listBody: `{"code":0,"data":[{"id":7,"key":"sk-test"}]}`,
			writeOK:  false,
		}
		srv := httptest.NewServer(f)
		defer srv.Close()
		err := s.DeleteAPIToken(ctx, srv.URL, "token", "sk-test", nil, nil)
		if err == nil {
			t.Fatal("neither DELETE landed, so the upstream key may still be live")
		}
		if !strings.Contains(err.Error(), "delete upstream key 7") ||
			!strings.Contains(err.Error(), "upstream refused the delete") {
			t.Fatalf("error should name the key and carry the upstream reason: %v", err)
		}
	})
}

func TestSub2ApiAdapter_GetSiteAnnouncements(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	ctx := context.Background()

	anns, err := s.GetSiteAnnouncements(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err == nil {
		t.Fatal("GetSiteAnnouncements on unreachable URL should return an error")
	}
	if len(anns) != 0 {
		t.Error("GetSiteAnnouncements on unreachable should return empty")
	}
}

// Both directions, because sub2api lists keys from two version-dependent endpoints:
// one of them failing is expected, both failing is a fetch failure, and both
// answering with zero keys is a legitimate empty result.
func TestSub2ApiAdapter_GetAPIToken(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	ctx := context.Background()

	tok, err := s.GetAPIToken(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err == nil {
		t.Error("GetAPIToken must report an unreachable upstream, not a missing token")
	}
	if tok != nil {
		t.Error("GetAPIToken should return no token when the listing failed")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"ok","data":[]}`))
	}))
	defer srv.Close()
	tok, err = s.GetAPIToken(ctx, srv.URL, "token", nil, nil)
	if err != nil {
		t.Errorf("both endpoints answered with no keys; that is not an error: %v", err)
	}
	if tok != nil {
		t.Errorf("expected no token, got %q", *tok)
	}
}

func TestParseGroupID(t *testing.T) {
	id, err := parseGroupID("123")
	if err != nil || id != 123 {
		t.Errorf("parseGroupID('123'): %d, %v", id, err)
	}

	id2, err2 := parseGroupID("")
	if err2 == nil || id2 != 0 {
		t.Errorf("parseGroupID(''): %d, %v", id2, err2)
	}

	id3, err3 := parseGroupID("not-a-number")
	if err3 == nil {
		t.Errorf("parseGroupID('not-a-number'): %d, %v", id3, err3)
	}
}

func TestExtractModelIDs(t *testing.T) {
	// data as array of objects with id
	resp := map[string]interface{}{
		"data": []interface{}{
			map[string]interface{}{"id": "gpt-4"},
			map[string]interface{}{"id": "claude-3"},
		},
	}
	ids := extractModelIDs(resp)
	if len(ids) != 2 {
		t.Fatalf("expected 2 models, got %d: %v", len(ids), ids)
	}

	// data as array of strings
	resp2 := map[string]interface{}{
		"data": []interface{}{
			"models/gpt-4",
			"claude-3",
		},
	}
	ids2 := extractModelIDs(resp2)
	if len(ids2) != 2 {
		t.Fatalf("expected 2 models from strings, got %d: %v", len(ids2), ids2)
	}

	// data with items wrapper
	resp3 := map[string]interface{}{
		"data": map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"id": "gpt-4"},
			},
		},
	}
	ids3 := extractModelIDs(resp3)
	if len(ids3) != 1 || ids3[0] != "gpt-4" {
		t.Errorf("items wrapper: %v", ids3)
	}

	// Empty
	ids4 := extractModelIDs(map[string]interface{}{})
	if len(ids4) != 0 {
		t.Errorf("empty: %v", ids4)
	}
}
