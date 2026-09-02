package proxy

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

// The tests in this file pin the single encoding owner both dispatch paths
// classify through, plus the judge's honest-abandonment short-circuit. The
// end-to-end behaviour on the production dispatch path is pinned in
// handler/proxy/upstream_encoding_honesty_test.go.

func gzipBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func zlibBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(raw); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}
	return buf.Bytes()
}

func rawDeflateBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	fw, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("flate writer: %v", err)
	}
	if _, err := fw.Write(raw); err != nil {
		t.Fatalf("flate write: %v", err)
	}
	if err := fw.Close(); err != nil {
		t.Fatalf("flate close: %v", err)
	}
	return buf.Bytes()
}

func headerWithEncoding(value string) http.Header {
	h := http.Header{}
	if value != "" {
		h.Set("Content-Encoding", value)
	}
	h.Set("Content-Type", "application/json")
	return h
}

func TestClassifyUpstreamEncoding(t *testing.T) {
	cases := []struct {
		value        string
		wantKind     UpstreamEncodingKind
		wantReadable bool
		wantRelay    bool
	}{
		{value: "", wantKind: UpstreamEncodingIdentity, wantReadable: true, wantRelay: false},
		{value: "identity", wantKind: UpstreamEncodingIdentity, wantReadable: true, wantRelay: false},
		{value: "gzip", wantKind: UpstreamEncodingDecodable, wantReadable: true, wantRelay: false},
		{value: "GZIP", wantKind: UpstreamEncodingDecodable, wantReadable: true, wantRelay: false},
		{value: "  gzip  ", wantKind: UpstreamEncodingDecodable, wantReadable: true, wantRelay: false},
		{value: "x-gzip", wantKind: UpstreamEncodingDecodable, wantReadable: true, wantRelay: false},
		{value: "deflate", wantKind: UpstreamEncodingDecodable, wantReadable: true, wantRelay: false},
		// Codecs the stdlib does not provide: never pretend to read them.
		{value: "br", wantKind: UpstreamEncodingUndecodable, wantReadable: false, wantRelay: true},
		{value: "zstd", wantKind: UpstreamEncodingUndecodable, wantReadable: false, wantRelay: true},
		{value: "snappy", wantKind: UpstreamEncodingUndecodable, wantReadable: false, wantRelay: true},
		// A stacked list is only partly understandable to us, so it is relayed
		// verbatim rather than half-peeled.
		{value: "gzip, br", wantKind: UpstreamEncodingUndecodable, wantReadable: false, wantRelay: true},
		{value: "deflate, gzip", wantKind: UpstreamEncodingUndecodable, wantReadable: false, wantRelay: true},
	}
	for _, tc := range cases {
		t.Run("value="+tc.value, func(t *testing.T) {
			got := ClassifyUpstreamEncoding(headerWithEncoding(tc.value))
			if got.Kind != tc.wantKind {
				t.Fatalf("Kind = %v, want %v", got.Kind, tc.wantKind)
			}
			if got.Readable() != tc.wantReadable {
				t.Fatalf("Readable() = %v, want %v", got.Readable(), tc.wantReadable)
			}
			if got.RelayHeader() != tc.wantRelay {
				t.Fatalf("RelayHeader() = %v, want %v", got.RelayHeader(), tc.wantRelay)
			}
		})
	}

	t.Run("nil header is identity", func(t *testing.T) {
		if got := ClassifyUpstreamEncoding(nil); got.Kind != UpstreamEncodingIdentity || !got.Readable() {
			t.Fatalf("ClassifyUpstreamEncoding(nil) = %+v, want a readable identity", got)
		}
	})
}

func TestNormalizeUpstreamBufferedBody(t *testing.T) {
	payload := []byte(`{"choices":[{"message":{"content":"hi"}}],"usage":{"total_tokens":18}}`)

	t.Run("gzip is decoded and the header is dropped", func(t *testing.T) {
		h := headerWithEncoding("gzip")
		got := NormalizeUpstreamBufferedBody(h, gzipBytes(t, payload))
		if !got.Readable {
			t.Fatalf("Readable = false, want true (decode error: %v)", got.DecodeErr)
		}
		if !bytes.Equal(got.Bytes, payload) {
			t.Fatalf("Bytes = %q, want the decoded payload", got.Bytes)
		}
		// The relay whitelist copies from resp.Header, so the header must be gone
		// once the bytes it described are gone.
		if v := h.Get("Content-Encoding"); v != "" {
			t.Fatalf("Content-Encoding after decode = %q, want empty", v)
		}
	})

	t.Run("zlib-wrapped deflate is decoded", func(t *testing.T) {
		h := headerWithEncoding("deflate")
		got := NormalizeUpstreamBufferedBody(h, zlibBytes(t, payload))
		if !got.Readable || !bytes.Equal(got.Bytes, payload) {
			t.Fatalf("got %+v (err %v), want the decoded payload", got, got.DecodeErr)
		}
		if v := h.Get("Content-Encoding"); v != "" {
			t.Fatalf("Content-Encoding after decode = %q, want empty", v)
		}
	})

	t.Run("raw deflate is decoded", func(t *testing.T) {
		h := headerWithEncoding("deflate")
		got := NormalizeUpstreamBufferedBody(h, rawDeflateBytes(t, payload))
		if !got.Readable || !bytes.Equal(got.Bytes, payload) {
			t.Fatalf("got %+v (err %v), want the decoded payload", got, got.DecodeErr)
		}
	})

	t.Run("undecodable bytes are relayed verbatim under their header", func(t *testing.T) {
		opaque := []byte{0x8b, 0x21, 0x00, 0xff, 'n', 'o', 't', ' ', 'j', 's', 'o', 'n'}
		h := headerWithEncoding("br")
		got := NormalizeUpstreamBufferedBody(h, opaque)
		if got.Readable {
			t.Fatal("Readable = true for a br body — we must never pretend to read bytes we cannot decode")
		}
		if !bytes.Equal(got.Bytes, opaque) {
			t.Fatalf("Bytes = %v, want the original bytes untouched", got.Bytes)
		}
		if v := h.Get("Content-Encoding"); v != "br" {
			t.Fatalf("Content-Encoding = %q, want it preserved for the verbatim relay", v)
		}
		if !got.Encoding.RelayHeader() {
			t.Fatal("RelayHeader() = false, want true")
		}
	})

	t.Run("a corrupt gzip body is unreadable, not half-decoded", func(t *testing.T) {
		broken := gzipBytes(t, payload)
		broken[len(broken)-3] ^= 0xff
		h := headerWithEncoding("gzip")
		got := NormalizeUpstreamBufferedBody(h, broken)
		if got.Readable {
			t.Fatal("Readable = true for a corrupt gzip body")
		}
		if got.DecodeErr == nil {
			t.Fatal("DecodeErr = nil, want the stdlib failure for logging")
		}
		if !bytes.Equal(got.Bytes, broken) {
			t.Fatal("Bytes were mutated; the verbatim relay must keep exactly what the upstream sent")
		}
		if v := h.Get("Content-Encoding"); v != "gzip" {
			t.Fatalf("Content-Encoding = %q, want it preserved (the bytes are still encoded)", v)
		}
	})

	t.Run("a decode that exceeds the buffered limit stays unreadable", func(t *testing.T) {
		prev := config.GetSafe()
		config.Set(&config.Config{ProxyMaxBufferedResponseBytes: 16})
		t.Cleanup(func() { config.Set(prev) })

		h := headerWithEncoding("gzip")
		got := NormalizeUpstreamBufferedBody(h, gzipBytes(t, payload))
		if got.Readable {
			t.Fatalf("Readable = true although the decoded body (%d bytes) exceeds the 16-byte limit", len(got.Bytes))
		}
		if got.DecodeErr == nil || !strings.Contains(got.DecodeErr.Error(), "buffered limit") {
			t.Fatalf("DecodeErr = %v, want the limit error", got.DecodeErr)
		}
	})

	t.Run("identity bodies are untouched", func(t *testing.T) {
		h := headerWithEncoding("")
		got := NormalizeUpstreamBufferedBody(h, payload)
		if !got.Readable || !bytes.Equal(got.Bytes, payload) {
			t.Fatalf("got %+v, want the identity payload readable", got)
		}
	})
}

func TestWrapUpstreamStreamBody(t *testing.T) {
	events := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\ndata: [DONE]\n\n")

	t.Run("a gzip stream is decoded inline", func(t *testing.T) {
		wrapped := WrapUpstreamStreamBody(headerWithEncoding("gzip"), io.NopCloser(bytes.NewReader(gzipBytes(t, events))))
		if wrapped.Reader == nil {
			t.Fatal("Reader = nil, want a decoding reader")
		}
		if !wrapped.Readable {
			t.Fatal("Readable = false, want true")
		}
		got, err := io.ReadAll(wrapped.Reader)
		if err != nil {
			t.Fatalf("read decoded stream: %v", err)
		}
		if !bytes.Equal(got, events) {
			t.Fatalf("decoded stream = %q, want %q", got, events)
		}
	})

	t.Run("a deflate stream is decoded inline", func(t *testing.T) {
		wrapped := WrapUpstreamStreamBody(headerWithEncoding("deflate"), io.NopCloser(bytes.NewReader(zlibBytes(t, events))))
		if wrapped.Reader == nil || !wrapped.Readable {
			t.Fatalf("got %+v, want a readable decoding reader", wrapped)
		}
		got, err := io.ReadAll(wrapped.Reader)
		if err != nil {
			t.Fatalf("read decoded stream: %v", err)
		}
		if !bytes.Equal(got, events) {
			t.Fatalf("decoded stream = %q, want %q", got, events)
		}
	})

	t.Run("an undecodable stream keeps the original body and stays unreadable", func(t *testing.T) {
		wrapped := WrapUpstreamStreamBody(headerWithEncoding("br"), io.NopCloser(bytes.NewReader(events)))
		if wrapped.Reader != nil {
			t.Fatal("Reader != nil — the relay must keep the upstream body verbatim")
		}
		if wrapped.Readable {
			t.Fatal("Readable = true for a br stream")
		}
		if !wrapped.Encoding.RelayHeader() {
			t.Fatal("RelayHeader() = false, want the verbatim passthrough to keep Content-Encoding")
		}
	})

	t.Run("closing the decoder closes the underlying body", func(t *testing.T) {
		src := &closeCountingReader{Reader: bytes.NewReader(gzipBytes(t, events))}
		wrapped := WrapUpstreamStreamBody(headerWithEncoding("gzip"), src)
		if _, err := io.ReadAll(wrapped.Reader); err != nil {
			t.Fatalf("read: %v", err)
		}
		if err := wrapped.Reader.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		if src.closes != 1 {
			t.Fatalf("underlying Close calls = %d, want exactly 1 (the SSE idle guard closes through this reader)", src.closes)
		}
	})
}

type closeCountingReader struct {
	io.Reader
	closes int
}

func (c *closeCountingReader) Close() error {
	c.closes++
	return nil
}

// TestJudgeUpstreamContent_UnreadableBody pins the honest-abandonment rule on the
// single content judge: no readable content evidence means no verdict, however
// aggressive the operator's detection settings are.
func TestJudgeUpstreamContent_UnreadableBody(t *testing.T) {
	setupFailureCfg([]string{"overloaded", "blocked"}, true)

	t.Run("empty-content rule must not fire on unreadable bytes", func(t *testing.T) {
		got := JudgeUpstreamContent(UpstreamContentFacts{
			StatusCode: http.StatusOK,
			HasOutput:  false,
			Usage:      &UsageSummary{},
			Unreadable: true,
		})
		if got.Failed {
			t.Fatalf("verdict = %+v, want pass: an undecodable body is not evidence of an empty answer", got)
		}
		if got.Code != FailureCodeNone {
			t.Fatalf("code = %q, want %q", got.Code, FailureCodeNone)
		}
	})

	t.Run("keyword rule must not fire on unreadable bytes", func(t *testing.T) {
		got := JudgeUpstreamContent(UpstreamContentFacts{
			StatusCode: http.StatusOK,
			RawText:    "blocked",
			Unreadable: true,
		})
		if got.Failed {
			t.Fatalf("verdict = %+v, want pass: the caller must not hand noise to the keyword scan", got)
		}
	})

	t.Run("streaming facts with no data events are still judged when readable", func(t *testing.T) {
		// Guard against the short-circuit leaking into the readable stream path:
		// the very same facts minus Unreadable MUST fail under
		// PROXY_EMPTY_CONTENT_FAIL, otherwise the flag would be dead.
		got := JudgeUpstreamContent(UpstreamContentFacts{
			StatusCode: http.StatusOK,
			Streaming:  true,
			HasOutput:  false,
			Usage:      &UsageSummary{},
		})
		if !got.Failed || got.Code != FailureCodeEmptyContent {
			t.Fatalf("verdict = %+v, want an empty-content failure", got)
		}
	})
}
