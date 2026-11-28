package shared

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deliciousbuding/metapi-go/app/observability"
)

// MetricsCollector is a zero-dependency Prometheus metrics collector.
// It tracks proxy request counts and operational gauges using atomic counters.
// Kept in handler/shared so proxy and admin handlers can record without importing app
// (app already depends on handler packages).
type MetricsCollector struct {
	proxyRequestsTotal atomic.Int64
	proxyErrorsTotal   atomic.Int64
	proxyStreamsActive atomic.Int64
	activeChannels     atomic.Int64
	dbConnectionsOpen  atomic.Int64
	dbConnectionsInUse atomic.Int64
	routeRebuildOK     atomic.Int64
	// Streams that requested include_usage but ended without usable tokens (known limitation).
	streamMissingUsageTotal atomic.Int64
	startTime               time.Time

	// Labeled outcomes + latency histograms (low-cardinality only).
	outcomesMu sync.Mutex
	outcomes   map[outcomeKey]int64
	histMu     sync.Mutex
	histograms map[histKey]*latencyHistogram
}

// Default latency histogram buckets in seconds (TTFT-ish to long completions).
var defaultLatencyBuckets = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30, 60, 120,
}

type outcomeKey struct {
	endpoint string
	status   string
	stream   string
}

type histKey struct {
	endpoint string
	status   string
}

type latencyHistogram struct {
	buckets []float64
	counts  []uint64 // len = len(buckets)+1 (+Inf)
	sum     float64
	count   uint64
}

var globalMetrics = &MetricsCollector{
	startTime:  time.Now(),
	outcomes:   make(map[outcomeKey]int64),
	histograms: make(map[histKey]*latencyHistogram),
}

// ProxyObservation is a privacy-safe, low-cardinality proxy terminal event.
// It must never carry prompts, keys, cookies, model names, or upstream bodies.
type ProxyObservation struct {
	Endpoint string        // bounded label from EndpointLabelFromPath
	Status   string        // Outcome* constants
	Stream   bool          // stream vs non-stream
	Latency  time.Duration // total observed latency for the attempt/request
}

// RecordProxyRequest increments the proxy request counter.
func RecordProxyRequest() { globalMetrics.proxyRequestsTotal.Add(1) }

// SetDBConnections sets the DB open-connection gauge.
func SetDBConnections(n int64) { globalMetrics.dbConnectionsOpen.Store(n) }

// SetDBConnectionsInUse sets the DB in-use connection gauge.
func SetDBConnectionsInUse(n int64) { globalMetrics.dbConnectionsInUse.Store(n) }

// IncActiveStreams increments the metapi_proxy_streams_active gauge. Call at
// the start of every proxied SSE/stream response. Pair with DecActiveStreams.
func IncActiveStreams() { globalMetrics.proxyStreamsActive.Add(1) }

// DecActiveStreams decrements the metapi_proxy_streams_active gauge. Call when
// a proxied SSE/stream response ends (success, error, or client disconnect).
// Uses Add(-1) so a misplaced Dec can drive the gauge negative, which is the
// honest signal that Inc/Dec are unbalanced.
func DecActiveStreams() { globalMetrics.proxyStreamsActive.Add(-1) }

// SetActiveStreams sets the metapi_proxy_streams_active gauge to an absolute
// value (useful for reconciliation/edge resets from a coordinator snapshot).
func SetActiveStreams(n int64) { globalMetrics.proxyStreamsActive.Store(n) }

// SetActiveChannels sets the metapi_active_channels gauge. Call from the
// channel list handler (or a periodic refresh) so /metrics reflects the real
// count of proxy channels instead of a constant 0.
func SetActiveChannels(n int64) { globalMetrics.activeChannels.Store(n) }

// RecordRouteRebuildCompleted increments successful route rebuild/cache-invalidate counter.
func RecordRouteRebuildCompleted() { globalMetrics.routeRebuildOK.Add(1) }

// RecordStreamMissingUsage increments when a stream expected include_usage tokens
// but ended without usable counts (observability only; never invents tokens).
func RecordStreamMissingUsage() { globalMetrics.streamMissingUsageTotal.Add(1) }

// ObserveProxyOutcome records a terminal proxy outcome: labeled counter, latency
// histogram, and legacy error counter (when not success).
// Labels are sanitized to a fixed allowlist; unknown values collapse to safe defaults.
func ObserveProxyOutcome(obs ProxyObservation) {
	endpoint := sanitizeEndpoint(obs.Endpoint)
	status := sanitizeStatus(obs.Status)
	stream := "false"
	if obs.Stream {
		stream = "true"
	}
	if latency := obs.Latency; latency < 0 {
		obs.Latency = 0
	}

	key := outcomeKey{endpoint: endpoint, status: status, stream: stream}
	globalMetrics.outcomesMu.Lock()
	globalMetrics.outcomes[key]++
	globalMetrics.outcomesMu.Unlock()

	// B2: feed the live ops ring buffer (per-second total vs success).
	RecordRealtimeOutcome(status == OutcomeSuccess)

	observeLatency(endpoint, status, obs.Latency)

	if status != OutcomeSuccess {
		globalMetrics.proxyErrorsTotal.Add(1)
	}
}

func observeLatency(endpoint, status string, d time.Duration) {
	seconds := d.Seconds()
	if seconds < 0 {
		seconds = 0
	}
	hk := histKey{endpoint: endpoint, status: status}
	globalMetrics.histMu.Lock()
	h := globalMetrics.histograms[hk]
	if h == nil {
		h = newLatencyHistogram(defaultLatencyBuckets)
		globalMetrics.histograms[hk] = h
	}
	h.observe(seconds)
	globalMetrics.histMu.Unlock()
}

func newLatencyHistogram(buckets []float64) *latencyHistogram {
	cp := append([]float64(nil), buckets...)
	return &latencyHistogram{
		buckets: cp,
		counts:  make([]uint64, len(cp)+1),
	}
}

func (h *latencyHistogram) observe(seconds float64) {
	h.count++
	h.sum += seconds
	idx := len(h.buckets) // +Inf
	for i, b := range h.buckets {
		if seconds <= b {
			idx = i
			break
		}
	}
	h.counts[idx]++
}

// ResetMetricsForTest clears counters/gauges for deterministic tests.
func ResetMetricsForTest() {
	globalMetrics.proxyRequestsTotal.Store(0)
	globalMetrics.proxyErrorsTotal.Store(0)
	globalMetrics.proxyStreamsActive.Store(0)
	globalMetrics.activeChannels.Store(0)
	globalMetrics.dbConnectionsOpen.Store(0)
	globalMetrics.dbConnectionsInUse.Store(0)
	observability.ResetDBConnErrorsForTest()
	globalMetrics.routeRebuildOK.Store(0)
	globalMetrics.streamMissingUsageTotal.Store(0)
	globalMetrics.startTime = time.Now()
	globalMetrics.outcomesMu.Lock()
	globalMetrics.outcomes = make(map[outcomeKey]int64)
	globalMetrics.outcomesMu.Unlock()
	globalMetrics.histMu.Lock()
	globalMetrics.histograms = make(map[histKey]*latencyHistogram)
	globalMetrics.histMu.Unlock()
}

// SnapshotForTest returns current counter values for assertions.
func SnapshotForTest() (requests, errors, streams, rebuilds int64) {
	return globalMetrics.proxyRequestsTotal.Load(),
		globalMetrics.proxyErrorsTotal.Load(),
		globalMetrics.proxyStreamsActive.Load(),
		globalMetrics.routeRebuildOK.Load()
}

// ActiveChannelsForTest returns the current metapi_active_channels gauge value.
func ActiveChannelsForTest() int64 { return globalMetrics.activeChannels.Load() }

// ActiveStreamsForTest returns the current metapi_proxy_streams_active gauge value.
func ActiveStreamsForTest() int64 { return globalMetrics.proxyStreamsActive.Load() }

// OutcomeSnapshotForTest returns labeled outcome counts for assertions.
func OutcomeSnapshotForTest() map[string]int64 {
	globalMetrics.outcomesMu.Lock()
	defer globalMetrics.outcomesMu.Unlock()
	out := make(map[string]int64, len(globalMetrics.outcomes))
	for k, v := range globalMetrics.outcomes {
		out[fmt.Sprintf("%s|%s|%s", k.endpoint, k.status, k.stream)] = v
	}
	return out
}

// HistogramCountForTest returns sample count for one labeled histogram series.
func HistogramCountForTest(endpoint, status string) uint64 {
	endpoint = sanitizeEndpoint(endpoint)
	status = sanitizeStatus(status)
	globalMetrics.histMu.Lock()
	defer globalMetrics.histMu.Unlock()
	h := globalMetrics.histograms[histKey{endpoint: endpoint, status: status}]
	if h == nil {
		return 0
	}
	return h.count
}
