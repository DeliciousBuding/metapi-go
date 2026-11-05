package proxyhandler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/channellimit"
	"github.com/deliciousbuding/metapi-go/routing"
)

func TestChannelRateAdmissionSkipsSiblingsWithoutSpendingRetry(t *testing.T) {
	setDirectAccountMaxConcurrencyForTest(t, 1)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}],"usage":{"total_tokens":4}}`))
	}))
	defer upstream.Close()
	manager := channellimit.NewManager()
	first, sibling, other := directAccountSelected(21, -1, upstream.URL), directAccountSelected(22, -2, upstream.URL), directAccountSelected(23, -3, upstream.URL)
	first.Direct.ChannelID, sibling.Direct.ChannelID, other.Direct.ChannelID = 7, 7, 8
	first.Direct.RateLimit.RPM, sibling.Direct.RateLimit.RPM = 1, 1
	if err := manager.TryRequest(t.Context(), 7, first.Direct.RateLimit.Config); err != nil {
		t.Fatal(err)
	}
	router := &directSequenceRouter{candidates: []*routing.SelectedChannel{first, sibling, other}}
	response := dispatchDirectAccountTestRequest(t, &UpstreamConfig{Router: router, ChannelLimits: manager}, 0)
	if response.Code != 200 || calls.Load() != 1 || len(router.failures) != 0 || len(router.successes) != 1 || router.successes[0] != -3 {
		t.Fatalf("local limit exhausted retry budget or poisoned health: status=%d calls=%d failures=%v successes=%v", response.Code, calls.Load(), router.failures, router.successes)
	}
	if snapshot := manager.Snapshot(7, first.Direct.RateLimit.Config); snapshot.Active != 0 || snapshot.Requests != 1 || snapshot.UnknownUsage != 0 {
		t.Fatalf("rejected channel state=%+v", snapshot)
	}
}

func TestChannelRateUsageRecordedOncePerAttempt(t *testing.T) {
	manager := channellimit.NewManager()
	admission := &channelAdmission{manager: manager, id: 4, config: channellimit.Config{TPM: 20}}
	ctx, err := prepareChannelRateAttempt(context.WithValue(t.Context(), channelAdmissionKey{}, admission))
	if err != nil {
		t.Fatal(err)
	}
	recordChannelRateUsage(ctx, ParsedUsage{Found: true, TotalTokens: 7})
	recordChannelRateUsage(ctx, ParsedUsage{Found: true, TotalTokens: 7})
	recordChannelRateUsage(ctx, ParsedUsage{Source: usageSourceUnknown})
	if snapshot := manager.Snapshot(4, admission.config); snapshot.Tokens != 7 || snapshot.UnknownUsage != 0 {
		t.Fatalf("duplicate final usage counted: %+v", snapshot)
	}
}
