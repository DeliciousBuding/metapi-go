package checkin

import (
	"strings"
	"sync"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	notifypkg "github.com/deliciousbuding/metapi-go/service/notify"
)

// notifyCall records a single invocation of the fake notifySend.
type notifyCall struct {
	title   string
	message string
	level   string
}

// swapNotifySend replaces the package-level notifySend with a recorder for the
// duration of a test and returns the call slice (protected by its own mutex).
// The original is restored on cleanup.
func swapNotifySend(t *testing.T) *notifyRecorder {
	t.Helper()
	rec := &notifyRecorder{}
	original := notifySend
	notifySend = func(cfg *config.Config, title, message, level string, options *notifypkg.SendNotificationOptions) (*notifypkg.DispatchResult, error) {
		rec.record(title, message, level)
		return &notifypkg.DispatchResult{}, nil
	}
	t.Cleanup(func() { notifySend = original })
	return rec
}

type notifyRecorder struct {
	mu    sync.Mutex
	calls []notifyCall
}

func (r *notifyRecorder) record(title, message, level string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, notifyCall{title: title, message: message, level: level})
}

func (r *notifyRecorder) snapshot() []notifyCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]notifyCall, len(r.calls))
	copy(out, r.calls)
	return out
}

// resultsFixture builds a CheckinAll result slice with the requested number of
// successes, failures, and skips. Failures get distinct names/messages so the
// test can assert both appear in the aggregated body.
func resultsFixture(successes, failures, skips int) []CheckinAllResult {
	var out []CheckinAllResult
	for i := 0; i < successes; i++ {
		out = append(out, CheckinAllResult{
			AccountID: int64(i) + 1,
			Username:  "okuser" + itoa(i),
			Site:      "ok-site",
			Result:    CheckinResult{Success: true, Status: CheckinSuccess, Message: "checkin success", Reward: "10"},
		})
	}
	for i := 0; i < failures; i++ {
		// Reverse-insert failures so the pre-sort order is non-alphabetic and
		// the sort in buildCheckinRoundResult is actually exercised.
		idx := failures - i
		out = append(out, CheckinAllResult{
			AccountID: int64(1000 + idx),
			Username:  "failuser" + itoa(idx),
			Site:      "fail-site-" + itoa(idx),
			Result: CheckinResult{
				Success: false, Status: CheckinFailed,
				Message: "HTTP 500: upstream error " + itoa(idx),
			},
		})
	}
	for i := 0; i < skips; i++ {
		out = append(out, CheckinAllResult{
			AccountID: int64(2000 + i + 1),
			Username:  "skipuser" + itoa(i),
			Site:      "skip-site",
			Result:    CheckinResult{Success: true, Status: CheckinSkipped, Skipped: true, Reason: "checkin_not_supported"},
		})
	}
	return out
}

// itoa is a tiny local int->string to avoid pulling strconv into every fixture.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// ---- buildCheckinRoundResult ----

func TestBuildCheckinRoundResult_ThreeSuccessTwoFailures(t *testing.T) {
	results := resultsFixture(3, 2, 0)
	round := buildCheckinRoundResult(results)
	if round == nil {
		t.Fatal("expected non-nil round for 2 failures")
	}
	if round.successes != 3 {
		t.Errorf("successes = %d, want 3", round.successes)
	}
	if len(round.failures) != 2 {
		t.Fatalf("failures len = %d, want 2", len(round.failures))
	}

	// Failures must be sorted by accountName ascending.
	if round.failures[0].accountName != "failuser1" {
		t.Errorf("first failure name = %q, want %q (sorted)", round.failures[0].accountName, "failuser1")
	}
	if round.failures[1].accountName != "failuser2" {
		t.Errorf("second failure name = %q, want %q (sorted)", round.failures[1].accountName, "failuser2")
	}

	// Each failure carries the first-line error and site.
	for _, f := range round.failures {
		if f.site == "" {
			t.Errorf("failure %q has empty site", f.accountName)
		}
		if f.error == "" {
			t.Errorf("failure %q has empty error", f.accountName)
		}
	}

	// roundID must be a non-empty UTC timestamp-ish string.
	if round.roundID == "" || !strings.Contains(round.roundID, "T") {
		t.Errorf("roundID = %q, want non-empty ISO-ish timestamp", round.roundID)
	}
}

func TestBuildCheckinRoundResult_SkipsAreNeitherSuccessNorFailure(t *testing.T) {
	results := resultsFixture(2, 1, 4) // 2 ok, 1 failed, 4 skipped
	round := buildCheckinRoundResult(results)
	if round == nil {
		t.Fatal("expected non-nil round (1 failure present)")
	}
	if round.successes != 2 {
		t.Errorf("successes = %d, want 2 (skips excluded)", round.successes)
	}
	if len(round.failures) != 1 {
		t.Errorf("failures len = %d, want 1 (skips excluded)", len(round.failures))
	}
}

func TestBuildCheckinRoundResult_AllSuccessReturnsNil(t *testing.T) {
	if round := buildCheckinRoundResult(resultsFixture(5, 0, 0)); round != nil {
		t.Errorf("expected nil round for clean success round, got %+v", round)
	}
}

func TestBuildCheckinRoundResult_AllSkippedReturnsNil(t *testing.T) {
	// Skipped-only rounds (unsupported endpoints, disabled accounts) must NOT
	// alert: there is no failure to surface.
	if round := buildCheckinRoundResult(resultsFixture(0, 0, 3)); round != nil {
		t.Errorf("expected nil round for all-skipped round, got %+v", round)
	}
}

func TestBuildCheckinRoundResult_EmptyResultsReturnsNil(t *testing.T) {
	if round := buildCheckinRoundResult(nil); round != nil {
		t.Errorf("expected nil round for nil results, got %+v", round)
	}
}

func TestBuildCheckinRoundResult_EmptyUsernameFallsBackToIDLabel(t *testing.T) {
	results := []CheckinAllResult{{
		AccountID: 42,
		Username:  "", // no username → ID:42
		Site:      "site",
		Result:    CheckinResult{Success: false, Status: CheckinFailed, Message: "boom"},
	}}
	round := buildCheckinRoundResult(results)
	if round == nil || len(round.failures) != 1 {
		t.Fatal("expected 1 failure")
	}
	if got := round.failures[0].accountName; got != "ID:42" {
		t.Errorf("accountName = %q, want %q", got, "ID:42")
	}
}

func TestBuildCheckinRoundResult_MultiLineErrorTruncatedToFirstLine(t *testing.T) {
	results := []CheckinAllResult{{
		AccountID: 1,
		Username:  "u",
		Site:      "s",
		Result: CheckinResult{
			Success: false, Status: CheckinFailed,
			Message: "first line of error\nsecond line\nthird line",
		},
	}}
	round := buildCheckinRoundResult(results)
	if round == nil || len(round.failures) != 1 {
		t.Fatal("expected 1 failure")
	}
	if got := round.failures[0].error; got != "first line of error" {
		t.Errorf("error = %q, want first line only", got)
	}
}

// ---- checkinRoundResult rendering ----

func TestCheckinRoundResult_NotificationTitleFormat(t *testing.T) {
	round := &checkinRoundResult{
		roundID:   "2026-08-15T03:00:00Z",
		successes: 3,
		failures:  []checkinFailure{{}, {}}, // 2 failures
	}
	want := "Checkin round 2026-08-15T03:00:00Z: 3 ok, 2 failed"
	if got := round.notificationTitle(); got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
}

func TestCheckinRoundResult_NotificationBodyListsEveryFailure(t *testing.T) {
	round := &checkinRoundResult{
		successes: 0,
		failures: []checkinFailure{
			{accountName: "alpha", site: "siteA", error: "timeout"},
			{accountName: "beta", site: "siteB", error: "5xx"},
		},
	}
	body := round.notificationBody()
	if !strings.Contains(body, "alpha @ siteA: timeout") {
		t.Errorf("body missing alpha line: %q", body)
	}
	if !strings.Contains(body, "beta @ siteB: 5xx") {
		t.Errorf("body missing beta line: %q", body)
	}
}

func TestCheckinRoundResult_NotificationBodyTruncatesAtCap(t *testing.T) {
	// Build maxFailuresPerNotification+5 failures and confirm the cap kicks in
	// with a trailing "…and 5 more" and that entries beyond the cap are absent.
	failures := make([]checkinFailure, maxFailuresPerNotification+5)
	for i := range failures {
		failures[i] = checkinFailure{
			accountName: "acct" + itoa(i),
			site:        "site",
			error:       "err",
		}
	}
	round := &checkinRoundResult{failures: failures}
	body := round.notificationBody()

	if !strings.Contains(body, "…and 5 more") {
		t.Errorf("body missing tail count: %q", body)
	}
	// The entry just past the cap must NOT be fully rendered.
	capped := "acct" + itoa(maxFailuresPerNotification) + " @ site: err"
	if strings.Contains(body, capped) {
		t.Errorf("body rendered an entry beyond the cap: %q", body)
	}
}

// ---- sendCheckinRoundNotification (the issue #667 contract) ----
//
// The core assertion of the issue: a round with N failures produces exactly
// ONE notification that lists every failure. Mirrored for the clean-round
// case (zero notifications) and the single-failure case (still one).

func TestSendCheckinRoundNotification_TwoFailuresSendsExactlyOneNotification(t *testing.T) {
	rec := swapNotifySend(t)

	results := resultsFixture(3, 2, 0) // 3 successes + 2 failures
	sendCheckinRoundNotification(&config.Config{}, results)

	calls := rec.snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected exactly 1 notification, got %d", len(calls))
	}

	call := calls[0]
	if call.level != "error" {
		t.Errorf("level = %q, want error", call.level)
	}
	if !strings.Contains(call.title, "3 ok, 2 failed") {
		t.Errorf("title %q missing counts", call.title)
	}
	// Both failure account names must appear in the single notification body.
	for _, name := range []string{"failuser1", "failuser2"} {
		if !strings.Contains(call.message, name) {
			t.Errorf("notification body missing failure %q: %s", name, call.message)
		}
	}
}

func TestSendCheckinRoundNotification_AllSuccessSendsNothing(t *testing.T) {
	rec := swapNotifySend(t)
	sendCheckinRoundNotification(&config.Config{}, resultsFixture(5, 0, 0))
	if calls := rec.snapshot(); len(calls) != 0 {
		t.Errorf("expected 0 notifications on clean round, got %d", len(calls))
	}
}

func TestSendCheckinRoundNotification_SingleFailureStillSendsOne(t *testing.T) {
	rec := swapNotifySend(t)
	sendCheckinRoundNotification(&config.Config{}, resultsFixture(0, 1, 0))
	calls := rec.snapshot()
	if len(calls) != 1 {
		t.Fatalf("expected 1 notification for single failure, got %d", len(calls))
	}
	if !strings.Contains(calls[0].title, "0 ok, 1 failed") {
		t.Errorf("title %q missing counts", calls[0].title)
	}
}

func TestSendCheckinRoundNotification_EmptyResultsSendsNothing(t *testing.T) {
	rec := swapNotifySend(t)
	sendCheckinRoundNotification(&config.Config{}, nil)
	if calls := rec.snapshot(); len(calls) != 0 {
		t.Errorf("expected 0 notifications for empty round, got %d", len(calls))
	}
}

// ---- helper unit tests ----

func TestAccountLabel(t *testing.T) {
	if got := accountLabel("alice", 1); got != "alice" {
		t.Errorf("accountLabel(alice,1) = %q, want alice", got)
	}
	if got := accountLabel("  ", 7); got != "ID:7" {
		t.Errorf("accountLabel(blank,7) = %q, want ID:7", got)
	}
	if got := accountLabel("", 99); got != "ID:99" {
		t.Errorf("accountLabel(empty,99) = %q, want ID:99", got)
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("only line"); got != "only line" {
		t.Errorf("firstLine(single) = %q", got)
	}
	if got := firstLine("line one\nline two"); got != "line one" {
		t.Errorf("firstLine(multi) = %q, want %q", got, "line one")
	}
	if got := firstLine("  spaced  \nsecond"); got != "spaced" {
		t.Errorf("firstLine(trimmed) = %q, want %q", got, "spaced")
	}
	if got := firstLine(""); got != "" {
		t.Errorf("firstLine(empty) = %q, want empty", got)
	}
}
