package app

import (
	"log/slog"
	"net/http"

	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/store"
)

func refreshRuntimeGauges() {
	if db := store.GetDB(); db != nil && db.DB != nil && db.DB.DB != nil {
		stats := db.DB.DB.Stats()
		shared.SetDBConnections(int64(stats.OpenConnections))
		shared.SetDBConnectionsInUse(int64(stats.InUse))
	}
}

// PrometheusHandler serves Prometheus text-format metrics at GET /metrics.
// Zero external dependencies — emits only the exposition format directly.
func PrometheusHandler(w http.ResponseWriter, r *http.Request) {
	refreshRuntimeGauges()
	if err := shared.WritePrometheusMetrics(w); err != nil {
		slog.Warn("metrics: failed to write prometheus exposition", "error", err)
	}
}
