package alert

import (
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	notifypkg "github.com/deliciousbuding/metapi-go/service/notify"
	"github.com/deliciousbuding/metapi-go/store"
)

// resetProxyAllFailedThrottle swaps in a fresh throttle so tests never see
// state left behind by earlier tests in the package.
func resetProxyAllFailedThrottle(t *testing.T) {
	t.Helper()
	prev := proxyAllFailedThrottle
	proxyAllFailedThrottle = notifypkg.NewNotificationThrottle()
	t.Cleanup(func() { proxyAllFailedThrottle = prev })
}

func countProxyAllFailedEvents(t *testing.T, db *store.DB) int {
	t.Helper()
	var n int
	if err := db.Get(&n, `SELECT COUNT(*) FROM events WHERE type = 'proxy'`); err != nil {
		t.Fatalf("count proxy events: %v", err)
	}
	return n
}

func TestReportProxyAllFailed_FiresEventOnceThenThrottles(t *testing.T) {
	resetProxyAllFailedThrottle(t)
	db := setupAlertTestDB(t)
	// NotifyCooldownSec=0 → the floor cooldown still applies (storm guard).
	rt := &config.RuntimeSettings{AuthToken: "a", ProxyToken: "p"}

	fire := func(model string) {
		ReportProxyAllFailed(rt, db.DB, ProxyAllFailedParams{
			Model: model, Reason: "all channels exhausted",
		})
	}

	fire("gpt-allfailed-a")
	if n := countProxyAllFailedEvents(t, db); n != 1 {
		t.Fatalf("expected 1 proxy event after first fire, got %d", n)
	}

	// Repeats within the cooldown window are suppressed: a dead upstream must
	// not write an event + notification per request.
	fire("gpt-allfailed-a")
	fire("gpt-allfailed-a")
	if n := countProxyAllFailedEvents(t, db); n != 1 {
		t.Fatalf("throttle failed: expected 1 proxy event, got %d", n)
	}

	// A distinct model is tracked independently.
	fire("gpt-allfailed-b")
	if n := countProxyAllFailedEvents(t, db); n != 2 {
		t.Fatalf("expected 2 proxy events across 2 models, got %d", n)
	}

	// After the cooldown window elapses the alert fires again.
	proxyAllFailedThrottle.PruneNotificationThrottleState(
		time.Now().UnixMilli()+proxyAllFailedCooldownFloorMs*2, 1)
	fire("gpt-allfailed-a")
	if n := countProxyAllFailedEvents(t, db); n != 3 {
		t.Fatalf("expected alert to re-fire after cooldown, got %d events", n)
	}
}

func TestReportProxyAllFailed_EventCarriesModelAndReason(t *testing.T) {
	resetProxyAllFailedThrottle(t)
	db := setupAlertTestDB(t)
	rt := &config.RuntimeSettings{AuthToken: "a", ProxyToken: "p"}

	ReportProxyAllFailed(rt, db.DB, ProxyAllFailedParams{
		Model: "gpt-allfailed-msg", Reason: "no available channels",
	})

	var title, message string
	if err := db.Get(&title,
		`SELECT title FROM events WHERE type = 'proxy' ORDER BY id DESC LIMIT 1`); err != nil {
		t.Fatalf("load event title: %v", err)
	}
	if title != "All proxies failed" {
		t.Fatalf("title = %q, want All proxies failed", title)
	}
	if err := db.Get(&message,
		`SELECT message FROM events WHERE type = 'proxy' ORDER BY id DESC LIMIT 1`); err != nil {
		t.Fatalf("load event message: %v", err)
	}
	if !strings.Contains(message, "gpt-allfailed-msg") || !strings.Contains(message, "no available channels") {
		t.Fatalf("event message missing model/reason: %q", message)
	}
}

func TestReportProxyAllFailed_GuardsNilDBAndEmptyModel(t *testing.T) {
	resetProxyAllFailedThrottle(t)
	// Must not panic; guards run before the throttle so no state is consumed.
	ReportProxyAllFailed(&config.RuntimeSettings{}, nil, ProxyAllFailedParams{Model: "m", Reason: "r"})
	ReportProxyAllFailed(nil, nil, ProxyAllFailedParams{Model: "m", Reason: "r"})

	db := setupAlertTestDB(t)
	ReportProxyAllFailed(&config.RuntimeSettings{}, db.DB, ProxyAllFailedParams{Model: "", Reason: "r"})
	if n := countProxyAllFailedEvents(t, db); n != 0 {
		t.Fatalf("expected 0 events for empty model, got %d", n)
	}
}
