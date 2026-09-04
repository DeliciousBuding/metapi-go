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

func TestOneApiAdapter_Detect(t *testing.T) {
	o := &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-api")}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Detect requires HTTP probe; will fail and return false for non-existent URLs
	ok, err := o.Detect(ctx, unreachableBaseURL(t))
	if err != nil {
		t.Errorf("Detect should not return error on probe failure: %v", err)
	}
	if ok {
		t.Error("Detect should return false for unreachable URL")
	}
}

// Both directions, because a zero BalanceInfo with a nil error is not a neutral
// answer: service/balance stores it as the account's balance and every recovery
// path there hangs off err != nil. Asserting only the happy path is how one-api
// (and the one-hub adapter that embeds it) kept writing balance=0 for an upstream
// nobody reached.
func TestOneApiAdapter_GetBalance(t *testing.T) {
	o := &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-api")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Run("unreachable upstream is an error, not a zero balance", func(t *testing.T) {
		bi, err := o.GetBalance(ctx, unreachableBaseURL(t), "token", nil, nil)
		if err == nil {
			t.Fatal("GetBalance reported a balance for a call that never reached the upstream")
		}
		if bi != nil {
			t.Errorf("a failed fetch must not hand back a storable zero balance, got %+v", bi)
		}
	})

	t.Run("an answered balance keeps the one-api divisor and quota-is-total semantics", func(t *testing.T) {
		srv := answeringTokenServer(`{"success":true,"data":{"quota":1000000,"used_quota":250000}}`)
		defer srv.Close()
		bi, err := o.GetBalance(ctx, srv.URL, "token", nil, nil)
		if err != nil {
			t.Fatalf("GetBalance against an answering upstream: %v", err)
		}
		if bi == nil {
			t.Fatal("GetBalance returned no BalanceInfo and no error")
		}
		// quota is the total here: Quota = 1_000_000/500_000 = 2.0,
		// Used = 250_000/500_000 = 0.5, Balance = 2.0 - 0.5 = 1.5
		if bi.Balance != 1.5 || bi.Used != 0.5 || bi.Quota != 2.0 {
			t.Errorf("balance = (%g, %g, %g), want (1.5, 0.5, 2) for quota-is-total at 500k",
				bi.Balance, bi.Used, bi.Quota)
		}
	})

	t.Run("an answered zero balance is a real answer, not a failure", func(t *testing.T) {
		srv := answeringTokenServer(`{"success":true,"data":{"quota":0,"used_quota":0}}`)
		defer srv.Close()
		bi, err := o.GetBalance(ctx, srv.URL, "token", nil, nil)
		if err != nil {
			t.Fatalf("an account that genuinely has no funds must not be reported as a fetch failure: %v", err)
		}
		if bi == nil {
			t.Fatal("GetBalance returned no BalanceInfo and no error")
		}
		if bi.Balance != 0 || bi.Used != 0 || bi.Quota != 0 {
			t.Errorf("balance = (%g, %g, %g), want (0, 0, 0)", bi.Balance, bi.Used, bi.Quota)
		}
	})

	t.Run("an answered refusal carries the upstream reason", func(t *testing.T) {
		srv := answeringTokenServer(`{"success":false,"message":"session expired"}`)
		defer srv.Close()
		bi, err := o.GetBalance(ctx, srv.URL, "token", nil, nil)
		if err == nil {
			t.Fatal("an upstream that refused the read must not come back as a balance")
		}
		if bi != nil {
			t.Errorf("refused read returned %+v, want nil", bi)
		}
		if !strings.Contains(err.Error(), "session expired") {
			t.Errorf("error should carry the upstream reason, got %v", err)
		}
	})
}

// one-hub embeds the one-api adapter, so it inherits whichever contract that one
// has. Pinned separately because "inherits" is exactly the assumption that let the
// swallow survive a fix to its siblings.
func TestOneHubAdapter_GetBalancePropagatesUpstreamFailure(t *testing.T) {
	o := &OneHubAdapter{OneApiAdapter: &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-hub")}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bi, err := o.GetBalance(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err == nil {
		t.Fatal("one-hub reported a balance for an unreachable upstream")
	}
	if bi != nil {
		t.Errorf("a failed fetch must not hand back a storable zero balance, got %+v", bi)
	}
}

// oneApiTokenUpstream serves the /api/token/ listing plus the per-id DELETE route
// with and without the trailing slash, and records every call it received.
type oneApiTokenUpstream struct {
	mu         sync.Mutex
	listBody   string
	listStatus int
	deleteOK   func(id string, trailingSlash bool) bool
	calls      []string
}

func (f *oneApiTokenUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/token"):
		w.WriteHeader(f.listStatus)
		_, _ = w.Write([]byte(f.listBody))
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/token/"):
		trimmed := strings.TrimPrefix(r.URL.Path, "/api/token/")
		trailingSlash := strings.HasSuffix(trimmed, "/")
		id := strings.TrimSuffix(trimmed, "/")
		if f.deleteOK != nil && f.deleteOK(id, trailingSlash) {
			_, _ = w.Write([]byte(`{"success":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":false,"message":"upstream refused the delete"}`))
	default:
		http.NotFound(w, r)
	}
}

func (f *oneApiTokenUpstream) callLog() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// Both directions on purpose, because the caller deletes the local row only when
// this returns nil. "The key is already gone upstream" is idempotence; "no request
// reached the upstream" is a failure that used to be reported as the same nil, so
// the row disappeared while the upstream key stayed live.
func TestOneApiAdapter_DeleteAPIToken(t *testing.T) {
	o := &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-api")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Run("unreachable upstream is a failure, not an idempotent success", func(t *testing.T) {
		if err := o.DeleteAPIToken(ctx, unreachableBaseURL(t), "token", "sk-test", nil, nil); err == nil {
			t.Fatal("DeleteAPIToken reported a completed delete for a call that never reached the upstream")
		}
	})

	t.Run("empty key needs no upstream call", func(t *testing.T) {
		f := &oneApiTokenUpstream{listBody: `{"success":true,"data":[]}`, listStatus: http.StatusOK}
		srv := httptest.NewServer(f)
		defer srv.Close()
		if err := o.DeleteAPIToken(ctx, srv.URL, "token", "", nil, nil); err != nil {
			t.Fatalf("empty key: %v", err)
		}
		if got := f.callLog(); len(got) != 0 {
			t.Fatalf("empty key made upstream calls: %v", got)
		}
	})

	t.Run("key already absent upstream is idempotent", func(t *testing.T) {
		f := &oneApiTokenUpstream{
			listBody:   `{"success":true,"data":[{"id":1,"key":"sk-someone-else"}]}`,
			listStatus: http.StatusOK,
		}
		srv := httptest.NewServer(f)
		defer srv.Close()
		if err := o.DeleteAPIToken(ctx, srv.URL, "token", "sk-test", nil, nil); err != nil {
			t.Fatalf("an answered listing without this key is not an error: %v", err)
		}
		if got := f.callLog(); len(got) != 1 {
			t.Fatalf("expected only the listing call, got %v", got)
		}
	})

	t.Run("a listing we could not read is a failure", func(t *testing.T) {
		f := &oneApiTokenUpstream{
			listBody:   `{"success":false,"message":"session expired"}`,
			listStatus: http.StatusInternalServerError,
		}
		srv := httptest.NewServer(f)
		defer srv.Close()
		err := o.DeleteAPIToken(ctx, srv.URL, "token", "sk-test", nil, nil)
		if err == nil {
			t.Fatal("without a readable listing we cannot know the key is gone")
		}
		if !strings.Contains(err.Error(), "list upstream tokens") {
			t.Fatalf("error should say which step failed: %v", err)
		}
	})

	t.Run("trailing-slash variant still completes the delete", func(t *testing.T) {
		f := &oneApiTokenUpstream{
			listBody:   `{"success":true,"data":[{"id":9,"key":"sk-test"}]}`,
			listStatus: http.StatusOK,
			deleteOK:   func(id string, trailingSlash bool) bool { return id == "9" && trailingSlash },
		}
		srv := httptest.NewServer(f)
		defer srv.Close()
		if err := o.DeleteAPIToken(ctx, srv.URL, "token", "sk-test", nil, nil); err != nil {
			t.Fatalf("double-delete strategy: %v", err)
		}
		got := f.callLog()
		want := []string{"GET /api/token/", "DELETE /api/token/9", "DELETE /api/token/9/"}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("calls = %v, want %v", got, want)
		}
	})

	t.Run("both delete variants refusing is a failure", func(t *testing.T) {
		f := &oneApiTokenUpstream{
			listBody:   `{"success":true,"data":[{"id":9,"key":"sk-test"}]}`,
			listStatus: http.StatusOK,
			deleteOK:   func(string, bool) bool { return false },
		}
		srv := httptest.NewServer(f)
		defer srv.Close()
		err := o.DeleteAPIToken(ctx, srv.URL, "token", "sk-test", nil, nil)
		if err == nil {
			t.Fatal("neither DELETE landed, so the upstream key may still be live")
		}
		if !strings.Contains(err.Error(), "delete upstream token 9") ||
			!strings.Contains(err.Error(), "upstream refused the delete") {
			t.Fatalf("error should name the token and carry the upstream reason: %v", err)
		}
	})
}

// answeringTokenServer serves a well-formed one-api token listing with no rows.
func answeringTokenServer(body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

// Both directions are pinned on purpose: "the upstream answered and there are no
// tokens" is a legitimate empty result, while "we could not ask the upstream" is a
// failure the caller has to hear about. Asserting only the second is how the first
// gets reported as a successful sync of nothing.
func TestOneApiAdapter_GetAPIToken(t *testing.T) {
	o := &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-api")}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	tok, err := o.GetAPIToken(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err == nil {
		t.Error("GetAPIToken must report an unreachable upstream, not a missing token")
	}
	if tok != nil {
		t.Error("GetAPIToken should return no token when the listing failed")
	}

	srv := answeringTokenServer(`{"success":true,"data":[]}`)
	defer srv.Close()
	tok, err = o.GetAPIToken(ctx, srv.URL, "token", nil, nil)
	if err != nil {
		t.Errorf("the upstream answered with no tokens; that is not an error: %v", err)
	}
	if tok != nil {
		t.Errorf("expected no token, got %q", *tok)
	}
}

// Both directions are pinned on purpose: "the upstream answered and there are no
// tokens" is a legitimate empty result, while "we could not ask the upstream" is a
// failure the caller has to hear about. Asserting only the second is how the first
// gets reported as a successful sync of nothing.
func TestOneApiAdapter_GetAPITokens(t *testing.T) {
	o := &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-api")}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	tokens, err := o.GetAPITokens(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err == nil {
		t.Error("GetAPITokens must report an unreachable upstream, not an empty token list")
	}
	if len(tokens) != 0 {
		t.Error("GetAPITokens should return no tokens when the listing failed")
	}

	srv := answeringTokenServer(`{"success":true,"data":[]}`)
	defer srv.Close()
	tokens, err = o.GetAPITokens(ctx, srv.URL, "token", nil, nil)
	if err != nil {
		t.Errorf("the upstream answered with an empty list; that is not an error: %v", err)
	}
	if len(tokens) != 0 {
		t.Errorf("expected zero tokens, got %d", len(tokens))
	}
}

// Renamed from TestOneApiAdapter_GetUserGroupsDefault, which accepted "error or
// [\"default\"]" and so could not fail: one-api has carried the terminalError
// guard since before this test was written, so an unreachable upstream has only
// ever produced an error. Accepting both is what let the same shape go
// unguarded in sub2api unnoticed — the family contract was written down here as
// "either is fine".
func TestOneApiAdapter_GetUserGroups_UnreachableIsAFailureNotADefaultGroup(t *testing.T) {
	o := &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-api")}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	groups, err := o.GetUserGroups(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err == nil {
		t.Fatalf("want an error, got groups=%#v — a failure to ask was reported as an answer", groups)
	}
	if groups != nil {
		t.Fatalf("want nil groups alongside the error, got %#v", groups)
	}
}

// `false, nil` means "the upstream answered and refused", which is why a failed
// fetch may not use it: the caller answers 502 either way, but only the error path
// carries the reason and writes the WARN log.
func TestOneApiAdapter_CreateAPIToken(t *testing.T) {
	o := &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-api")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	created, err := o.CreateAPIToken(ctx, unreachableBaseURL(t), "token", nil, nil, nil)
	if err == nil {
		t.Error("CreateAPIToken must report an unreachable upstream instead of a silent false")
	}
	if created {
		t.Error("nothing was created")
	}

	srv := answeringTokenServer(`{"success":true,"data":{"id":1,"key":"sk-new"}}`)
	defer srv.Close()
	created, err = o.CreateAPIToken(ctx, srv.URL, "token", nil, nil, nil)
	if err != nil || !created {
		t.Fatalf("answered success = (%v, %v), want (true, nil)", created, err)
	}

	refusal := answeringTokenServer(`{"success":false,"message":"quota exceeded"}`)
	defer refusal.Close()
	created, err = o.CreateAPIToken(ctx, refusal.URL, "token", nil, nil, nil)
	if err != nil {
		t.Fatalf("the upstream's own refusal is an answer, not a fetch failure: %v", err)
	}
	if created {
		t.Fatal("created = true for a refused create")
	}
}

func TestBuildDefaultTokenPayload(t *testing.T) {
	// Default options (nil)
	p := buildDefaultTokenPayload(nil)
	if p["name"] != "metapi" {
		t.Errorf("default name: %q", p["name"])
	}
	if p["unlimited_quota"] != true {
		t.Error("default unlimited_quota should be true")
	}
	if p["expired_time"] != int64(-1) {
		t.Errorf("default expired_time: %v", p["expired_time"])
	}

	// Custom options
	opts := &CreateAPITokenOptions{
		Name:               "custom-key",
		UnlimitedQuota:     false,
		RemainQuota:        100.5,
		ExpiredTime:        1735689600,
		AllowIPs:           "1.2.3.4",
		ModelLimits:        "gpt-4:100",
		Group:              "vip",
		ModelLimitsEnabled: true,
	}
	p2 := buildDefaultTokenPayload(opts)
	if p2["name"] != "custom-key" {
		t.Errorf("custom name: %q", p2["name"])
	}
	if p2["unlimited_quota"] != false {
		t.Error("custom unlimited_quota should be false")
	}
	if p2["remain_quota"] != 100.5 {
		t.Errorf("custom remain_quota: %v", p2["remain_quota"])
	}
	if p2["group"] != "vip" {
		t.Errorf("custom group: %q", p2["group"])
	}
}

func TestResolveGroupFetchErrorMessage(t *testing.T) {
	// Expired token
	msg := resolveGroupFetchErrorMessage(map[string]interface{}{
		"message": "Token expired",
	})
	if msg != "账号会话可能已过期，请重新登录后再拉取分组" {
		t.Errorf("expired token: %q", msg)
	}

	// Invalid token
	msg2 := resolveGroupFetchErrorMessage(map[string]interface{}{
		"message": "Invalid token",
	})
	if msg2 != "账号会话可能已过期，请重新登录后再拉取分组" {
		t.Errorf("invalid token: %q", msg2)
	}

	// Normal message
	msg3 := resolveGroupFetchErrorMessage(map[string]interface{}{
		"message": "Something else happened",
	})
	if msg3 != "Something else happened" {
		t.Errorf("normal message: %q", msg3)
	}

	// Empty
	msg4 := resolveGroupFetchErrorMessage(map[string]interface{}{})
	if msg4 != "failed to fetch groups" {
		t.Errorf("empty: %q", msg4)
	}

	// Non-auth messages must not be rewritten as session-expired UX (R0).
	msg5 := resolveGroupFetchErrorMessage(map[string]interface{}{
		"message": "No payment method. Add a payment method here: https://example.com/billing",
	})
	if msg5 != "No payment method. Add a payment method here: https://example.com/billing" {
		t.Errorf("billing should not rewrite to session-expired: %q", msg5)
	}
	msg6 := resolveGroupFetchErrorMessage(map[string]interface{}{
		"message": "Model foo is not supported for format openai",
	})
	if msg6 != "Model foo is not supported for format openai" {
		t.Errorf("model unsupported should not rewrite to session-expired: %q", msg6)
	}
}

func TestExtractGroupKeys(t *testing.T) {
	// With data wrapper
	resp := map[string]interface{}{
		"success": true,
		"message": "ok",
		"data": map[string]interface{}{
			"vip":     map[string]interface{}{},
			"default": map[string]interface{}{},
			"premium": map[string]interface{}{},
		},
	}
	keys := extractGroupKeys(resp)
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d: %v", len(keys), keys)
	}

	// Without data wrapper - excludes special keys
	resp2 := map[string]interface{}{
		"success": true,
		"message": "ok",
		"vip":     map[string]interface{}{},
	}
	keys2 := extractGroupKeys(resp2)
	if len(keys2) != 1 || keys2[0] != "vip" {
		t.Errorf("direct keys: %v", keys2)
	}
}

func TestDedupeStrings(t *testing.T) {
	items := []string{"a", "b", "a", "c", "  b  "}
	result := dedupeStrings(items)
	if len(result) != 3 {
		t.Fatalf("expected 3 unique, got %d: %v", len(result), result)
	}
	if result[0] != "a" || result[1] != "b" || result[2] != "c" {
		t.Errorf("deduped: %v", result)
	}

	// Empty
	result2 := dedupeStrings([]string{})
	if len(result2) != 0 {
		t.Errorf("empty input: %v", result2)
	}
}

func TestOneApiAdapter_Checkin(t *testing.T) {
	o := &OneApiAdapter{BaseAdapter: NewBaseAdapter("one-api")}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cr, err := o.Checkin(ctx, unreachableBaseURL(t), "token", nil, nil)
	if err != nil {
		t.Errorf("Checkin should not error: %v", err)
	}
	if cr.Success {
		t.Error("Checkin on unreachable URL should fail")
	}
}
