package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/deliciousbuding/metapi-go/handler/admin/payloads"
	"github.com/deliciousbuding/metapi-go/scheduler"
)

// ---- Probe Now ----

// probeNowTimeout caps the total wall-clock a synchronous probe-now pass
// may take. The server's WriteTimeout is 60s (app.newHTTPServer); 30s keeps
// the handler well under that ceiling while leaving room for the typical
// 32-target / 8-worker batch (probes are sub-second in practice; the 15s
// per-probe timeout is a ceiling, not the median). Operators watching a
// slow site should use the streaming endpoint instead.
const probeNowTimeout = 30 * time.Second

func (h *sitesHandler) probeNow(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	var body payloads.ProbeNowBody
	// Body is optional for probe-now. When present, modelName filters which
	// results are surfaced (the previously-discarded field is now wired).
	_ = decodeJSONRequest(r, &body)
	modelFilter := ""
	if body.ModelName != nil {
		modelFilter = strings.TrimSpace(*body.ModelName)
	}

	sched := scheduler.GetGlobalModelProbeScheduler()
	if sched == nil {
		// Fall back: create ephemeral scheduler for one-shot probe.
		sched = scheduler.NewModelProbeScheduler(h.db, nil)
	}

	// Bound the total work via the request context so a slow site cannot block
	// the handler past WriteTimeout. Cancellation (client disconnect) also
	// stops scheduling new probes immediately.
	ctx, cancel := context.WithTimeout(r.Context(), probeNowTimeout)
	defer cancel()

	// The onResult callback is invoked from multiple probe goroutines
	// concurrently, so the result slice and counters must be guarded.
	var mu sync.Mutex
	var results []scheduler.ProbeSiteResult
	var available, unavailable int
	probeErr := sched.ProbeSiteIncremental(ctx, id, func(res scheduler.ProbeSiteResult) {
		if modelFilter != "" && res.Model != modelFilter {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		results = append(results, res)
		if res.Status == "success" {
			available++
		} else if res.Status == "failure" {
			unavailable++
		}
	})
	if results == nil {
		results = []scheduler.ProbeSiteResult{}
	}

	response := map[string]any{
		"success":     true,
		"totalModels": len(results),
		"available":   available,
		"unavailable": unavailable,
		"results":     results,
		"complete":    probeErr == nil,
	}
	if probeErr != nil {
		response["truncated"] = true
		response["reason"] = probeErr.Error()
	}
	writeJSON(w, http.StatusOK, response)
}

// ---- Probe Stream (SSE) ----

func (h *sitesHandler) probeStream(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	// Clear the server-level WriteTimeout for this SSE stream. Without this,
	// any probe pass exceeding app.Server.WriteTimeout (60s) is forcibly
	// closed by net/http — the same fix the proxy SSE path applies
	// (handler/proxy/upstream_stream.go:40-41). http.NewResponseController
	// unwraps the middleware statusRecorder to reach the real connection.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	// modelName, when provided, filters which probe results are streamed.
	// scope and latencyThresholdMs were previously read and immediately
	// discarded; they are now dropped to remove the dead-code smell.
	modelFilter := strings.TrimSpace(r.URL.Query().Get("modelName"))

	sched := scheduler.GetGlobalModelProbeScheduler()
	if sched == nil {
		sched = scheduler.NewModelProbeScheduler(h.db, nil)
	}

	sseWrite(w, flusher, "probe-start", map[string]any{
		"startedAt":   time.Now().UTC().Format(time.RFC3339),
		"streaming":   true,
		"modelFilter": modelFilter,
	})

	var available, unavailable, sent int
	// The onResult callback fires from multiple probe goroutines at once.
	// http.ResponseWriter is not safe for concurrent use, and the counters
	// are shared, so the whole write + bookkeeping must be serialized.
	var writeMu sync.Mutex
	probeErr := sched.ProbeSiteIncremental(r.Context(), id, func(res scheduler.ProbeSiteResult) {
		if modelFilter != "" && res.Model != modelFilter {
			return
		}
		writeMu.Lock()
		defer writeMu.Unlock()
		sseWrite(w, flusher, "probe-result", res)
		sent++
		if res.Status == "success" {
			available++
		} else if res.Status == "failure" {
			unavailable++
		}
	})

	// If the client disconnected (r.Context err'd), don't bother writing the
	// closing events — the connection is gone and the writes would error.
	if r.Context().Err() != nil {
		return
	}

	completePayload := map[string]any{
		"totalModels": sent,
		"available":   available,
		"unavailable": unavailable,
	}
	if probeErr != nil {
		completePayload["truncated"] = true
		completePayload["reason"] = probeErr.Error()
	}
	sseWrite(w, flusher, "complete", completePayload)

	// OpenAI-style [DONE] sentinel so clients know the stream is finished.
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	if flusher != nil {
		flusher.Flush()
	}
}

func sseWrite(w http.ResponseWriter, flusher http.Flusher, event string, data any) {
	jsonData, _ := json.Marshal(data)
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, string(jsonData))
	if flusher != nil {
		flusher.Flush()
	}
}
