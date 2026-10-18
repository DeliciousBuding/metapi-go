package alert

import (
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
)

// A downstream client controls the model name. It must not be able to grow
// the throttle map with unbounded keys or write megabytes into the event.
func TestReportProxyAllFailed_BoundsAttackerControlledModelName(t *testing.T) {
	resetProxyAllFailedThrottle(t)
	db := setupAlertTestDB(t)
	rt := &config.RuntimeSettings{AuthToken: "a", ProxyToken: "p"}

	model := strings.Repeat("m", 8<<20)
	ReportProxyAllFailed(rt, db.DB, ProxyAllFailedParams{Model: model, Reason: "no channels"})

	var message string
	if err := db.Get(&message, `SELECT message FROM events WHERE type = 'proxy' ORDER BY id DESC LIMIT 1`); err != nil {
		t.Fatalf("load event message: %v", err)
	}
	if len(message) > 16<<10 {
		t.Fatalf("event message = %d bytes, want a bounded message", len(message))
	}
	if !strings.Contains(message, "…[truncated]") {
		t.Fatalf("event message should mark truncation: %q", message[:min(len(message), 200)])
	}

	proxyAllFailedThrottle.PruneNotificationThrottleState(time.Now().UnixMilli(), 1)
	// The signature is a fixed-size hash, and the raw model never appears in it.
	signature := proxyAllFailedSignature(model)
	if strings.Contains(signature, "mmmm") {
		t.Fatalf("throttle signature leaks the raw model: %q", signature)
	}
	if len(signature) > 128 {
		t.Fatalf("throttle signature = %d bytes, want a fixed-size hash", len(signature))
	}
}
