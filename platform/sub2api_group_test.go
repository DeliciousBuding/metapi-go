package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPickKeyForGroup_GroupMatch(t *testing.T) {
	tokens := []ApiTokenInfo{
		{Name: "alpha", Key: "sk-alpha", Enabled: true, TokenGroup: "group-a"},
		{Name: "beta", Key: "sk-beta", Enabled: true, TokenGroup: "group-b"},
		{Name: "gamma", Key: "sk-gamma", Enabled: true, TokenGroup: "group-a"},
	}
	got := pickKeyForGroup(tokens, "group-b")
	if got == nil || *got != "sk-beta" {
		t.Fatalf("expected sk-beta for group-b, got %v", got)
	}
}

func TestPickKeyForGroup_FirstEnabledWhenGroupMatchDisabled(t *testing.T) {
	tokens := []ApiTokenInfo{
		{Name: "alpha", Key: "sk-alpha", Enabled: false, TokenGroup: "group-a"},
		{Name: "beta", Key: "sk-beta", Enabled: true, TokenGroup: "group-b"},
	}
	got := pickKeyForGroup(tokens, "group-a")
	if got == nil || *got != "sk-beta" {
		t.Fatalf("expected fallback to first-enabled sk-beta, got %v", got)
	}
}

func TestPickKeyForGroup_EmptyGroupFallsBackToFirstEnabled(t *testing.T) {
	tokens := []ApiTokenInfo{
		{Name: "alpha", Key: "sk-alpha", Enabled: false, TokenGroup: "group-a"},
		{Name: "beta", Key: "sk-beta", Enabled: true, TokenGroup: "group-b"},
	}
	got := pickKeyForGroup(tokens, "")
	if got == nil || *got != "sk-beta" {
		t.Fatalf("expected first-enabled sk-beta for empty group, got %v", got)
	}
}

func TestPickKeyForGroup_NoMatchFallsBackToFirstEnabled(t *testing.T) {
	tokens := []ApiTokenInfo{
		{Name: "alpha", Key: "sk-alpha", Enabled: true, TokenGroup: "group-a"},
	}
	got := pickKeyForGroup(tokens, "nonexistent-group")
	if got == nil || *got != "sk-alpha" {
		t.Fatalf("expected fallback to sk-alpha, got %v", got)
	}
}

func TestPickKeyForGroup_EmptyTokensReturnsNil(t *testing.T) {
	got := pickKeyForGroup(nil, "group-a")
	if got != nil {
		t.Fatalf("expected nil for empty tokens, got %v", *got)
	}
}

func TestPickKeyForGroup_AllDisabledReturnsFirst(t *testing.T) {
	tokens := []ApiTokenInfo{
		{Name: "alpha", Key: "sk-alpha", Enabled: false, TokenGroup: "group-a"},
		{Name: "beta", Key: "sk-beta", Enabled: false, TokenGroup: "group-b"},
	}
	got := pickKeyForGroup(tokens, "group-a")
	if got == nil || *got != "sk-alpha" {
		t.Fatalf("expected first token when all disabled, got %v", got)
	}
}

func TestPickKeyForGroup_WhitespaceTrimmed(t *testing.T) {
	tokens := []ApiTokenInfo{
		{Name: "alpha", Key: "sk-alpha", Enabled: true, TokenGroup: "  group-a  "},
	}
	got := pickKeyForGroup(tokens, " group-a ")
	if got == nil || *got != "sk-alpha" {
		t.Fatalf("expected whitespace-trimmed match, got %v", got)
	}
}

func TestParseSub2ApiUserGroup_GroupID(t *testing.T) {
	data := map[string]interface{}{"group_id": float64(42)}
	if g := parseSub2ApiUserGroup(data); g != "42" {
		t.Fatalf("expected 42, got %q", g)
	}
}

func TestParseSub2ApiUserGroup_GroupName(t *testing.T) {
	data := map[string]interface{}{"group_name": "premium"}
	if g := parseSub2ApiUserGroup(data); g != "premium" {
		t.Fatalf("expected premium, got %q", g)
	}
}

func TestParseSub2ApiUserGroup_Group(t *testing.T) {
	data := map[string]interface{}{"group": "standard"}
	if g := parseSub2ApiUserGroup(data); g != "standard" {
		t.Fatalf("expected standard, got %q", g)
	}
}

func TestParseSub2ApiUserGroup_NestedSubscription(t *testing.T) {
	data := map[string]interface{}{
		"subscription": map[string]interface{}{"group_id": float64(7)},
	}
	if g := parseSub2ApiUserGroup(data); g != "7" {
		t.Fatalf("expected 7 from nested subscription, got %q", g)
	}
}

func TestParseSub2ApiUserGroup_Empty(t *testing.T) {
	if g := parseSub2ApiUserGroup(map[string]interface{}{}); g != "" {
		t.Fatalf("expected empty, got %q", g)
	}
	if g := parseSub2ApiUserGroup(nil); g != "" {
		t.Fatalf("expected empty for nil, got %q", g)
	}
}

// TestFetchModelsByToken_CtxErrFastFail verifies that a cancelled context
// short-circuits the endpoint probe loop so a slow first endpoint cannot
// burn the whole budget (#675).
func TestFetchModelsByToken_CtxErrFastFail(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}

	var hitCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hitCount, 1)
		// Simulate a slow response.
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel before the first call so ctx.Err() is non-nil on the very first
	// iteration — no endpoint should be hit at all.
	cancel()

	models, err := s.fetchModelsByToken(ctx, server.URL, "sk-test", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 0 {
		t.Fatalf("expected empty models, got %v", models)
	}
	if atomic.LoadInt32(&hitCount) != 0 {
		t.Fatalf("expected 0 endpoint hits with pre-cancelled ctx, got %d", hitCount)
	}
}

// TestFetchModelsByToken_CtxCancelledMidLoop verifies that when the context is
// cancelled after the first endpoint returns, the remaining endpoints are
// skipped. The first endpoint triggers the cancel synchronously so the second
// iteration sees ctx.Err() != nil and breaks.
func TestFetchModelsByToken_CtxCancelledMidLoop(t *testing.T) {
	s := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var hitCount int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hitCount, 1)
		// Cancel the context from within the first endpoint hit so the next
		// loop iteration's ctx.Err() check breaks out.
		if atomic.LoadInt32(&hitCount) == 1 {
			cancel()
		}
		w.Header().Set("Content-Type", "application/json")
		// Return empty models so the loop would normally continue.
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	_, _ = s.fetchModelsByToken(ctx, server.URL, "sk-test", nil)
	// The first endpoint hit triggers cancel; the second iteration's
	// ctx.Err() check should break before hitting the server again.
	if got := atomic.LoadInt32(&hitCount); got > 1 {
		t.Fatalf("expected at most 1 endpoint hit after mid-loop cancel, got %d", got)
	}
}

// --- GetUserGroups: a failure to ask is not an answer ----------------------
//
// The discovery path had no coverage at all (this file pinned pickKeyForGroup
// and parseSub2ApiUserGroup only), which is how an unconditional ["default"]
// survived: the caller, GetTokenGroups, prefers any non-blank upstream answer
// over the local fallback it already has, and the service-level test for that
// fallback (TestGetTokenGroups_FallbackOnErrorEmptyNilAdapter) can only run when
// an adapter returns an error. This one never did.

// sub2GroupsStub serves every path the two ladders try. Paths absent from the
// map answer 404, which is what a version-skewed upstream does to the paths it
// does not have.
func sub2GroupsStub(t testing.TB, byPath map[string]string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := byPath[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":404,"message":"not found"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func sub2GroupPaths() []string {
	return []string{
		"/api/v1/groups/available",
		"/api/v1/groups",
		"/api/v1/group",
		"/api/v1/keys",
		"/api/v1/api-keys",
	}
}

func sub2AllPaths(body string) map[string]string {
	out := make(map[string]string)
	for _, p := range sub2GroupPaths() {
		out[p] = body
	}
	return out
}

func TestSub2ApiAdapter_GetUserGroups_FailureIsNotADefaultGroup(t *testing.T) {
	const sessionRewrite = "账号会话可能已过期，请重新登录后再拉取分组"
	cases := []struct {
		name       string
		baseURL    func(t testing.TB) string
		wantSubstr string
	}{
		{
			// The reported defect: nothing was reachable, and the operator got a
			// group the upstream never advertised.
			name:       "unreachable",
			baseURL:    unreachableBaseURL,
			wantSubstr: "fetch groups",
		},
		{
			name: "refused with a reason",
			baseURL: func(t testing.TB) string {
				return sub2GroupsStub(t, sub2AllPaths(`{"code":1,"message":"groups endpoint refused"}`)).URL
			},
			wantSubstr: "groups endpoint refused",
		},
		{
			// sub2api's own fixtures use a string code, which the numeric
			// code==0 check never treated as success — so this shape used to
			// parse to nothing and fall through to ["default"].
			name: "string code UNAUTHORIZED",
			baseURL: func(t testing.TB) string {
				return sub2GroupsStub(t, sub2AllPaths(`{"code":"UNAUTHORIZED","message":"authorization header is required"}`)).URL
			},
			wantSubstr: "authorization header is required",
		},
		{
			// The message the family already wrote for an expired session, and
			// that a sub2api account could never reach.
			name: "expired session reaches the family message",
			baseURL: func(t testing.TB) string {
				return sub2GroupsStub(t, sub2AllPaths(`{"code":1,"message":"未登录"}`)).URL
			},
			wantSubstr: sessionRewrite,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			groups, err := a.GetUserGroups(ctx, tc.baseURL(t), "token", nil, nil)
			if err == nil {
				t.Fatalf("want an error, got groups=%#v — a failure to ask was reported as an answer", groups)
			}
			if groups != nil {
				t.Fatalf("want nil groups alongside the error, got %#v", groups)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Fatalf("error %q does not carry %q", err.Error(), tc.wantSubstr)
			}
			for _, invented := range []string{"default"} {
				for _, g := range groups {
					if g == invented {
						t.Fatalf("fabricated %q group survived", invented)
					}
				}
			}
		})
	}
}

// The other face of the same defect: an upstream that really has no groups must
// still be told the truth, so ["default"] stays reachable when — and only when —
// something answered.
func TestSub2ApiAdapter_GetUserGroups_AnsweredEmptyIsStillTrue(t *testing.T) {
	srv := sub2GroupsStub(t, sub2AllPaths(`{"code":0,"data":[]}`))
	a := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	groups, err := a.GetUserGroups(ctx, srv.URL, "token", nil, nil)
	if err != nil {
		t.Fatalf("an answered empty is a true statement, want nil error, got %v", err)
	}
	if len(groups) != 1 || groups[0] != "default" {
		t.Fatalf("want the family's last resort [\"default\"], got %#v", groups)
	}
}

// A 404 on an older path is version skew, not a failure: the predicate is
// "did anything answer", not "did anything error".
func TestSub2ApiAdapter_GetUserGroups_VersionSkewIsNotAFailure(t *testing.T) {
	srv := sub2GroupsStub(t, map[string]string{
		"/api/v1/groups": `{"code":0,"data":[{"id":7,"name":"vip"}]}`,
	})
	a := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	groups, err := a.GetUserGroups(ctx, srv.URL, "token", nil, nil)
	if err != nil {
		t.Fatalf("version skew must not become a failure: %v", err)
	}
	if len(groups) == 0 {
		t.Fatalf("want the groups the upstream advertised, got %#v", groups)
	}
}

// The predicate is "did anything answer", not "did anything error", and the case
// that separates them is a deployment where one alternative 404s and another
// answers with an empty list. Without this the two predicates are
// indistinguishable: when some rung does return groups the ladder returns early
// and never reaches the check at all (which is what VersionSkewIsNotAFailure
// covers), so a mutation of `!answered && lastReason != ""` to
// `lastReason != ""` went unnoticed until the probe tried it.
func TestSub2ApiAdapter_GetUserGroups_AnsweredEmptyAlongsideAFailedAlternativeIsNotAFailure(t *testing.T) {
	srv := sub2GroupsStub(t, map[string]string{
		// /api/v1/groups/available and /api/v1/group are absent -> 404, the way a
		// version without those paths answers.
		"/api/v1/groups":   `{"code":0,"data":[]}`,
		"/api/v1/api-keys": `{"code":0,"data":[]}`,
	})
	a := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	groups, err := a.GetUserGroups(ctx, srv.URL, "token", nil, nil)
	if err != nil {
		t.Fatalf("one alternative 404ing must not become a failure when another answered: %v", err)
	}
	if len(groups) != 1 || groups[0] != "default" {
		t.Fatalf("want the family's last resort [\"default\"], got %#v", groups)
	}
}

// Preference order is preserved: ask the group endpoint first, derive from
// existing keys second.
func TestSub2ApiAdapter_GetUserGroups_FallsBackToInferringFromKeys(t *testing.T) {
	srv := sub2GroupsStub(t, map[string]string{
		"/api/v1/groups":   `{"code":0,"data":[]}`,
		"/api/v1/group":    `{"code":0,"data":[]}`,
		"/api/v1/keys":     `{"code":0,"data":[{"id":1,"group_id":3}]}`,
		"/api/v1/api-keys": `{"code":0,"data":[]}`,
	})
	a := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	groups, err := a.GetUserGroups(ctx, srv.URL, "token", nil, nil)
	if err != nil {
		t.Fatalf("GetUserGroups: %v", err)
	}
	if len(groups) != 1 || groups[0] != "3" {
		t.Fatalf("want the group id inferred from keys, got %#v", groups)
	}
}

// The two rungs are a preference order, so the FIRST reason is the one reported
// — the group endpoint is the question the operator actually asked. The ladders
// inside each rung keep the last, because those endpoints are equivalent
// version-skew alternatives. Both are deliberate.
func TestSub2ApiAdapter_GetUserGroups_KeepsTheFirstReason(t *testing.T) {
	srv := sub2GroupsStub(t, map[string]string{
		"/api/v1/groups/available": `{"code":1,"message":"groups rung refused"}`,
		"/api/v1/groups":           `{"code":1,"message":"groups rung refused"}`,
		"/api/v1/group":            `{"code":1,"message":"groups rung refused"}`,
		"/api/v1/keys":             `{"code":1,"message":"keys rung refused"}`,
		"/api/v1/api-keys":         `{"code":1,"message":"keys rung refused"}`,
	})
	a := &Sub2ApiAdapter{BaseAdapter: NewBaseAdapter("sub2api")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := a.GetUserGroups(ctx, srv.URL, "token", nil, nil)
	if err == nil {
		t.Fatal("want an error")
	}
	if !strings.Contains(err.Error(), "groups rung refused") {
		t.Fatalf("want the first rung's reason, got %q", err.Error())
	}
	if strings.Contains(err.Error(), "keys rung refused") {
		t.Fatalf("the second rung's reason displaced the first: %q", err.Error())
	}
}
