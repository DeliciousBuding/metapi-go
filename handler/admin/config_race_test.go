package admin

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/routing"
)

// TestSettingsRuntime_ConcurrentApplyVsHotReaders reproduces the Wave 18
// config-race cluster end to end: the real PUT /api/settings/runtime writer
// (settings_apply.go) runs concurrently with the hot-path readers that
// dereference the same config fields lock-free:
//
//   - auth.AuthorizeDownstreamToken — reads cfg.ProxyToken on every proxy
//     request (auth/downstream.go:161)
//   - routing.ActiveRetryStatusRanges / ActiveDisableStatusRanges — reads
//     cfg.ProxyRetryStatusRanges / ProxyDisableStatusRanges on every
//     upstream verdict (routing/status_ranges.go)
//   - GET /api/settings/runtime — reads ~45 fields at once (settings.go)
//
// Before the fix this FAILS under -race (DATA RACE). After the atomic
// RuntimeSettings snapshot redesign it passes, and the trailing assertions
// prove hot-update semantics survive: a freshly applied proxyToken
// authenticates immediately (no restart) and the old token stops working.
func TestSettingsRuntime_ConcurrentApplyVsHotReaders(t *testing.T) {
	db, r, cfg := setupEdgeTest(t)

	const readers = 4
	const writerIters = 15

	stop := make(chan struct{})
	var readerWG sync.WaitGroup
	var writerWG sync.WaitGroup

	// Writer: hammer the real settings-apply handler (PUT /api/settings/runtime).
	writerWG.Add(1)
	go func() {
		defer writerWG.Done()
		for i := 0; i < writerIters; i++ {
			body := map[string]any{
				"proxyToken":             fmt.Sprintf("sk-race-%06d", i),
				"proxyRetryStatusRanges": "500-599",
				"systemProxyUrl":         fmt.Sprintf("http://10.0.0.%d:8080", i%5),
				"notifyCooldownSec":      i % 60,
				"webhookUrl":             fmt.Sprintf("https://hooks.example.com/race-%d", i),
				"checkinIntervalHours":   1 + (i % 24),
			}
			if i%2 == 0 {
				body["proxyDisableStatusRanges"] = "401"
			} else {
				body["proxyDisableStatusRanges"] = ""
			}
			if resp := doPutJSON(t, r, "/api/settings/runtime", body); resp.Code != http.StatusOK {
				t.Errorf("writer iter %d: status = %d body=%s", i, resp.Code, resp.Body.String())
				return
			}
		}
	}()

	// Reader 1..N: hot-path auth check (every proxy request) + status ranges.
	for i := 0; i < readers; i++ {
		readerWG.Add(1)
		go func() {
			defer readerWG.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = auth.AuthorizeDownstreamToken("sk-race-probe", cfg)
				_ = routing.ActiveRetryStatusRanges()
				_ = routing.ActiveDisableStatusRanges()
			}
		}()
	}

	// Reader N+1: the full runtime-settings read surface (admin UI poll).
	readerWG.Add(1)
	go func() {
		defer readerWG.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			req := httptest.NewRequest("GET", "/api/settings/runtime", nil)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("GET /api/settings/runtime: status = %d", rec.Code)
				return
			}
		}
	}()

	writerWG.Wait()
	close(stop)
	readerWG.Wait()

	// ---- Hot-update behavior proof: settings take effect without restart ----
	finalToken := "sk-hot-update-final-token"
	resp := doPutJSON(t, r, "/api/settings/runtime", map[string]any{
		"proxyToken":             finalToken,
		"proxyRetryStatusRanges": "429,500-599",
	})
	if resp.Code != http.StatusOK {
		t.Fatalf("final apply: status = %d body=%s", resp.Code, resp.Body.String())
	}

	got := auth.AuthorizeDownstreamToken(finalToken, cfg)
	if !got.OK || got.Source != "global" {
		t.Fatalf("hot-updated proxy token rejected: ok=%v source=%q reason=%q (hot update lost)", got.OK, got.Source, got.Reason)
	}
	if stale := auth.AuthorizeDownstreamToken("sk-race-000000", cfg); stale.OK && stale.Source == "global" {
		t.Fatalf("superseded proxy token still authenticates as global")
	}
	if ranges := routing.ActiveRetryStatusRanges(); len(ranges) != 2 {
		t.Fatalf("hot-updated retry ranges = %v, want 2 ranges (429 + 500-599)", ranges)
	}

	_ = db
}
