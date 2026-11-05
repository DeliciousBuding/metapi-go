package relaykitbridge

// Native Go fuzz targets for the byte-oriented conversion boundary. They encode
// the contract the hand-written matrix tests sample instead of enumerate:
//
//   - conversion never panics on arbitrary bytes;
//   - a native (same-format) request/response keeps its exact bytes;
//   - a cross-protocol JSON body is valid JSON and never carries RelayKit's
//     internal billing metadata;
//   - one conversion is deterministic for one input;
//   - a failed conversion returns no bytes and a streaming session fails closed
//     (a failure emits no bytes and a second Finish is byte-empty).
//
// Run the seed corpus with the ordinary suite:
//
//	go test ./transform/relaykitbridge -count=1
//
// Explore new inputs for a fixed budget:
//
//	go test ./transform/relaykitbridge -run '^$' -fuzz FuzzConvertRequest -fuzztime=60s

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

// internalUsageMarkers are RelayKit metadata keys that must never reach a
// downstream client. The host bills from the original upstream bytes.
var internalUsageMarkers = [][]byte{
	[]byte("billing_usage"),
	[]byte("usage_semantic"),
	[]byte("usage_source"),
}

func formatIndex(format Format) uint8 {
	for i, f := range formats {
		if f == format {
			return uint8(i)
		}
	}
	return 0
}

func fuzzFormat(v uint8) Format {
	return formats[int(v)%len(formats)]
}

func containsInternalMarker(raw []byte) bool {
	for _, m := range internalUsageMarkers {
		if bytes.Contains(raw, m) {
			return true
		}
	}
	return false
}

func assertNoInternalMarker(t *testing.T, in, out []byte) {
	t.Helper()
	if containsInternalMarker(in) {
		return
	}
	for _, m := range internalUsageMarkers {
		if bytes.Contains(out, m) {
			t.Fatalf("conversion leaked internal metadata %q: %s", m, out)
		}
	}
}

func FuzzConvertRequest(f *testing.F) {
	for _, from := range formats {
		for _, to := range formats {
			f.Add(formatIndex(from), formatIndex(to), []byte(requests[from]))
		}
	}
	f.Fuzz(func(t *testing.T, fromIdx, toIdx uint8, raw []byte) {
		from, to := fuzzFormat(fromIdx), fuzzFormat(toIdx)
		_, out, err := ConvertRequest(context.Background(), from, to, "fixture-model", raw)
		if err != nil {
			if len(out) != 0 {
				t.Fatalf("failed conversion returned bytes: %s", out)
			}
			return
		}
		if from == to {
			if !bytes.Equal(out, raw) {
				t.Fatalf("native request reencoded: %s", out)
			}
			return
		}
		if !json.Valid(out) {
			t.Fatalf("converted request is not valid JSON: %s", out)
		}
		assertNoInternalMarker(t, raw, out)

		_, again, errAgain := ConvertRequest(context.Background(), from, to, "fixture-model", raw)
		if errAgain != nil {
			t.Fatalf("determinism: second conversion failed: %v", errAgain)
		}
		if !bytes.Equal(out, again) {
			t.Fatalf("non-deterministic conversion:\nfirst  %s\nsecond %s", out, again)
		}
	})
}

func FuzzConvertResponse(f *testing.F) {
	for _, from := range formats {
		for _, to := range formats {
			f.Add(formatIndex(from), formatIndex(to), []byte(replies[to]))
		}
	}
	f.Fuzz(func(t *testing.T, fromIdx, toIdx uint8, raw []byte) {
		from, to := fuzzFormat(fromIdx), fuzzFormat(toIdx)
		session, _, err := ConvertRequest(context.Background(), from, to, "fixture-model", []byte(requests[from]))
		if err != nil {
			return
		}
		out, err := session.Response(context.Background(), raw)
		if err != nil {
			if len(out) != 0 {
				t.Fatalf("failed response conversion returned bytes: %s", out)
			}
			return
		}
		if from == to {
			if !bytes.Equal(out, raw) {
				t.Fatalf("native response reencoded: %s", out)
			}
			return
		}
		if !json.Valid(out) {
			t.Fatalf("converted response is not valid JSON: %s", out)
		}
		assertNoInternalMarker(t, raw, out)
	})
}

func FuzzResponseStream(f *testing.F) {
	for _, from := range formats {
		for _, to := range formats {
			frame := []byte("data: " + streams[to][0] + "\n\n")
			f.Add(formatIndex(from), formatIndex(to), frame)
		}
	}
	f.Fuzz(func(t *testing.T, fromIdx, toIdx uint8, frame []byte) {
		from, to := fuzzFormat(fromIdx), fuzzFormat(toIdx)
		session, _, err := ConvertRequest(context.Background(), from, to, "fixture-model", []byte(requests[from]))
		if err != nil {
			return
		}
		stream, err := session.NewResponseStream()
		if err != nil {
			return
		}
		// Fail-closed: a failing transform never emits bytes.
		if out, err := stream.TransformEvent(frame); err != nil && len(out) != 0 {
			t.Fatalf("stream failure emitted bytes: %s", out)
		}
		// A second Finish is always byte-empty, whether the first succeeded or failed.
		stream.Finish()
		if trailer, _ := stream.Finish(); len(trailer) != 0 {
			t.Fatalf("Finish not idempotent: %s", trailer)
		}
	})
}
