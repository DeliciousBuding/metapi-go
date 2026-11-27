package admin

import (
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

func TestSlowRequestObservations(t *testing.T) {
	bothStatsDatabases(t, func(t *testing.T, db *store.DB, r chi.Router) {
		get := func() []any { return catalogCall(t, r, "GET", "/api/stats/slow-requests", nil, 200)["items"].([]any) }
		if len(get()) != 0 {
			t.Fatal("expected empty slow requests")
		}
		now := time.Now().UTC().Format(time.RFC3339)
		mustExec(t, db, `INSERT INTO proxy_logs (latency_ms, created_at) VALUES (2000,?)`, now)
		mustExec(t, db, `INSERT INTO proxy_logs (latency_ms,created_at,first_byte_latency_ms,first_output_latency_ms,gateway_request_id,is_stream,downstream_path) VALUES (3000,?,0,450,'gateway-id',TRUE,'/v1/messages')`, now)
		items := get()
		observed, legacy := items[0].(map[string]any), items[1].(map[string]any)
		for k, want := range map[string]any{"firstByteLatencyMs": float64(0), "firstOutputLatencyMs": float64(450), "gatewayRequestId": "gateway-id", "isStream": true, "downstreamPath": "/v1/messages"} {
			if observed[k] != want {
				t.Errorf("observed %s = %v want %v", k, observed[k], want)
			}
			if v, ok := legacy[k]; !ok || v != nil {
				t.Errorf("legacy %s = %v present=%v", k, v, ok)
			}
		}
	})
}
