package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/shared"
)

func TestPrometheusHandler_FormatSmoke(t *testing.T) {
	shared.ResetMetricsForTest()
	shared.RecordProxyRequest()
	shared.RecordRouteRebuildCompleted()
	shared.SetDBConnections(3)
	shared.ObserveProxyOutcome(shared.ProxyObservation{
		Endpoint: shared.EndpointChat,
		Status:   shared.OutcomeSuccess,
		Stream:   false,
		Latency:  100 * time.Millisecond,
	})

	rec := httptest.NewRecorder()
	PrometheusHandler(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Fatalf("Content-Type = %q, want text/plain", ct)
	}
	body := rec.Body.String()
	required := []string{
		"# HELP metapi_proxy_requests_total",
		"# TYPE metapi_proxy_requests_total counter",
		"metapi_proxy_requests_total 1",
		"metapi_db_connections_open 3",
		`metapi_route_rebuild_total{result="completed"} 1`,
		"# HELP metapi_uptime_seconds",
		"# TYPE metapi_proxy_outcomes_total counter",
		`metapi_proxy_outcomes_total{endpoint="chat",status="success",stream="false"} 1`,
		"# TYPE metapi_proxy_request_duration_seconds histogram",
		`metapi_proxy_request_duration_seconds_count{endpoint="chat",status="success"} 1`,
	}
	for _, want := range required {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics body missing %q\n%s", want, body)
		}
	}
}
