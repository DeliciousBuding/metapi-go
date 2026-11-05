package proxyhandler

// Native Go fuzz targets for the response/stream half of protocol handling.
// In this codebase SSE bytes are generally pass-through; the parsed usage,
// finish flags and error state that drive billing and health are produced here.
// The invariants encoded below are the first principles of that surface:
//
//   - parsing an arbitrary body never panics and is deterministic;
//   - the incremental SSE analyzer's result does not depend on how the same
//     bytes are chopped into chunks, as long as the stream stays below the
//     pending-event cap (the cap is a separate, documented resource bound).
//
// The seed corpus is the checked-in golden fixtures, so the fuzzer starts from
// the same realistic protocol bodies the snapshot suite pins.
//
//	go test ./handler/proxy -run '^$' -fuzz FuzzSseChunkBoundaryIndependence -fuzztime=60s

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func seedGoldenFixtures(f *testing.F, dir, ext string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ext {
			continue
		}
		if raw, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
			f.Add(raw)
		}
	}
}

func FuzzParseUsageFromBody(f *testing.F) {
	// Seed from the checked-in golden response bodies; they already cover the
	// OpenAI, Anthropic and Gemini usage shapes.
	seedGoldenFixtures(f, filepath.Join("testdata", "golden", "response"), ".json")
	f.Add([]byte("not json"))
	f.Fuzz(func(t *testing.T, body []byte) {
		first := ParseUsageFromBody(body)
		second := ParseUsageFromBody(body)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("ParseUsageFromBody is non-deterministic for %q", body)
		}
	})
}

// FuzzSseChunkBoundaryIndependence pins chunk-boundary independence of the
// incremental analyzer: the state parsed from a stream must not change when the
// same bytes arrive in different-sized chunks.
func FuzzSseChunkBoundaryIndependence(f *testing.F) {
	seedGoldenFixtures(f, filepath.Join("testdata", "golden", "stream"), ".sse")
	f.Add([]byte("data: {\"a\":1}\n\ndata: [DONE]\n\n"))
	f.Add([]byte("event: message\r\ndata: {\"type\":\"x\"}\r\n\r\n"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		if len(raw) >= maxIncrementalSsePendingBytes {
			// The pending-event cap is a separate resource bound with its own
			// drop semantics; this invariant covers inputs below it.
			return
		}
		want := analyzeStream(t, raw, 0)
		for _, size := range []int{1, 2, 3, 5, 7, 13, 64, 255} {
			if got := analyzeStream(t, raw, size); got != want {
				t.Fatalf("chunk size %d changed parsed state:\nwhole %+v\nchunk %+v\nraw %q", size, want, got, raw)
			}
		}
	})
}
