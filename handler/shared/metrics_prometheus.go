package shared

import (
	"fmt"
	"net/http"
	"runtime"
	"sort"
	"time"

	"github.com/deliciousbuding/metapi-go/app/observability"
)

// WritePrometheusMetrics writes Prometheus text-format metrics to w.
func WritePrometheusMetrics(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	m := globalMetrics
	uptime := time.Since(m.startTime).Seconds()

	var b []byte
	appendLine := func(format string, args ...any) {
		b = append(b, []byte(fmt.Sprintf(format, args...))...)
	}

	appendLine("# HELP metapi_uptime_seconds Process uptime in seconds\n")
	appendLine("# TYPE metapi_uptime_seconds gauge\n")
	appendLine("metapi_uptime_seconds %.0f\n", uptime)

	appendLine("# HELP metapi_proxy_requests_total Total proxy requests served\n")
	appendLine("# TYPE metapi_proxy_requests_total counter\n")
	appendLine("metapi_proxy_requests_total %d\n", m.proxyRequestsTotal.Load())

	appendLine("# HELP metapi_proxy_errors_total Total proxy upstream errors\n")
	appendLine("# TYPE metapi_proxy_errors_total counter\n")
	appendLine("metapi_proxy_errors_total %d\n", m.proxyErrorsTotal.Load())

	appendLine("# HELP metapi_proxy_streams_active Active SSE proxy streams\n")
	appendLine("# TYPE metapi_proxy_streams_active gauge\n")
	appendLine("metapi_proxy_streams_active %d\n", m.proxyStreamsActive.Load())

	appendLine("# HELP metapi_active_channels Active proxy channels\n")
	appendLine("# TYPE metapi_active_channels gauge\n")
	appendLine("metapi_active_channels %d\n", m.activeChannels.Load())

	appendLine("# HELP metapi_db_connections_open Open database connections\n")
	appendLine("# TYPE metapi_db_connections_open gauge\n")
	appendLine("metapi_db_connections_open %d\n", m.dbConnectionsOpen.Load())

	appendLine("# HELP metapi_db_connections_in_use In-use database connections\n")
	appendLine("# TYPE metapi_db_connections_in_use gauge\n")
	appendLine("metapi_db_connections_in_use %d\n", m.dbConnectionsInUse.Load())

	appendLine("# HELP metapi_db_conn_errors_total Database connection budget / open errors\n")
	appendLine("# TYPE metapi_db_conn_errors_total counter\n")
	appendLine("metapi_db_conn_errors_total %d\n", observability.DBConnErrorsTotal())

	appendLine("# HELP metapi_route_rebuild_total Total route cache rebuild/invalidate operations\n")
	appendLine("# TYPE metapi_route_rebuild_total counter\n")
	appendLine("metapi_route_rebuild_total{result=\"completed\"} %d\n", m.routeRebuildOK.Load())

	appendLine("# HELP metapi_stream_missing_usage_total Streams that requested include_usage but ended without usable tokens\n")
	appendLine("# TYPE metapi_stream_missing_usage_total counter\n")
	appendLine("metapi_stream_missing_usage_total %d\n", m.streamMissingUsageTotal.Load())

	// Labeled terminal outcomes (privacy-safe, bounded labels).
	appendLine("# HELP metapi_proxy_outcomes_total Terminal proxy outcomes by endpoint/status/stream\n")
	appendLine("# TYPE metapi_proxy_outcomes_total counter\n")
	m.outcomesMu.Lock()
	outcomeKeys := make([]outcomeKey, 0, len(m.outcomes))
	for k := range m.outcomes {
		outcomeKeys = append(outcomeKeys, k)
	}
	sort.Slice(outcomeKeys, func(i, j int) bool {
		a, b := outcomeKeys[i], outcomeKeys[j]
		if a.endpoint != b.endpoint {
			return a.endpoint < b.endpoint
		}
		if a.status != b.status {
			return a.status < b.status
		}
		return a.stream < b.stream
	})
	for _, k := range outcomeKeys {
		appendLine("metapi_proxy_outcomes_total{endpoint=%q,status=%q,stream=%q} %d\n",
			k.endpoint, k.status, k.stream, m.outcomes[k])
	}
	m.outcomesMu.Unlock()

	// Latency histogram (cumulative buckets).
	appendLine("# HELP metapi_proxy_request_duration_seconds Proxy request duration in seconds\n")
	appendLine("# TYPE metapi_proxy_request_duration_seconds histogram\n")
	m.histMu.Lock()
	histKeys := make([]histKey, 0, len(m.histograms))
	for k := range m.histograms {
		histKeys = append(histKeys, k)
	}
	sort.Slice(histKeys, func(i, j int) bool {
		a, b := histKeys[i], histKeys[j]
		if a.endpoint != b.endpoint {
			return a.endpoint < b.endpoint
		}
		return a.status < b.status
	})
	for _, k := range histKeys {
		h := m.histograms[k]
		if h == nil || h.count == 0 {
			continue
		}
		var cum uint64
		for i, bound := range h.buckets {
			cum += h.counts[i]
			appendLine("metapi_proxy_request_duration_seconds_bucket{endpoint=%q,status=%q,le=\"%g\"} %d\n",
				k.endpoint, k.status, bound, cum)
		}
		cum += h.counts[len(h.buckets)]
		appendLine("metapi_proxy_request_duration_seconds_bucket{endpoint=%q,status=%q,le=\"+Inf\"} %d\n",
			k.endpoint, k.status, cum)
		appendLine("metapi_proxy_request_duration_seconds_sum{endpoint=%q,status=%q} %g\n",
			k.endpoint, k.status, h.sum)
		appendLine("metapi_proxy_request_duration_seconds_count{endpoint=%q,status=%q} %d\n",
			k.endpoint, k.status, h.count)
	}
	m.histMu.Unlock()

	// Go runtime + memory metrics (stdlib only; keeps the zero-dependency design).
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	appendLine("# HELP go_goroutines Number of goroutines that currently exist\n")
	appendLine("# TYPE go_goroutines gauge\n")
	appendLine("go_goroutines %d\n", runtime.NumGoroutine())

	appendLine("# HELP go_memstats_alloc_bytes Number of bytes allocated and still in use\n")
	appendLine("# TYPE go_memstats_alloc_bytes gauge\n")
	appendLine("go_memstats_alloc_bytes %d\n", ms.Alloc)

	appendLine("# HELP go_memstats_heap_alloc_bytes Number of heap bytes allocated and still in use\n")
	appendLine("# TYPE go_memstats_heap_alloc_bytes gauge\n")
	appendLine("go_memstats_heap_alloc_bytes %d\n", ms.HeapAlloc)

	appendLine("# HELP go_memstats_heap_inuse_bytes Number of heap bytes that are in use\n")
	appendLine("# TYPE go_memstats_heap_inuse_bytes gauge\n")
	appendLine("go_memstats_heap_inuse_bytes %d\n", ms.HeapInuse)

	appendLine("# HELP go_memstats_heap_objects Number of allocated objects\n")
	appendLine("# TYPE go_memstats_heap_objects gauge\n")
	appendLine("go_memstats_heap_objects %d\n", ms.HeapObjects)

	appendLine("# HELP go_memstats_sys_bytes Number of bytes obtained from system\n")
	appendLine("# TYPE go_memstats_sys_bytes gauge\n")
	appendLine("go_memstats_sys_bytes %d\n", ms.Sys)

	appendLine("# HELP go_gc_duration_seconds A summary of the pause duration of garbage collection cycles\n")
	appendLine("# TYPE go_gc_duration_seconds summary\n")
	appendLine("go_gc_duration_seconds_sum %g\n", float64(ms.PauseTotalNs)/1e9)
	appendLine("go_gc_duration_seconds_count %d\n", ms.NumGC)

	_, err := w.Write(b)
	return err
}
