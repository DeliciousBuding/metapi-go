package platform

import (
	"context"
	"net/http"
	"net/http/httptest"
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
