package proxy

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// UpstreamEncodingSkippedMessage is the STABLE warn text the data plane emits
// whenever it holds upstream body bytes it cannot decode. Stability matters:
// operators alert on it, and it is the only surface that makes the resulting
// account/reality gap ("we recorded a success but no tokens") visible instead
// of silent. Never reword it without treating that as an operator-facing
// contract change.
const UpstreamEncodingSkippedMessage = "upstream response uses a content encoding metapi does not decode; usage accounting and content-failure judgement were skipped"

// UpstreamEncodingKind classifies an upstream Content-Encoding by what it means
// for our ability to READ the body. That ability — not the header itself — is
// what decides whether usage may be accounted and whether the content judge may
// run at all.
type UpstreamEncodingKind int

const (
	// UpstreamEncodingIdentity means the bytes we hold are the content: no
	// Content-Encoding, or the explicit "identity" token. Everything downstream
	// (usage extraction, keyword scan, empty-content rule) may run.
	UpstreamEncodingIdentity UpstreamEncodingKind = iota
	// UpstreamEncodingDecodable means a stdlib codec can turn the bytes back
	// into identity (gzip / deflate). The caller decodes, parses the decoded
	// bytes and re-frames the response WITHOUT Content-Encoding.
	UpstreamEncodingDecodable
	// UpstreamEncodingUndecodable means we cannot read the bytes at all (br,
	// zstd, a multi-layer stack, or a codec that failed mid-decode). The caller
	// MUST NOT parse, judge or account them, and MUST relay them verbatim
	// together with their Content-Encoding so the client — which may well be
	// able to decode them — still gets an intact response.
	UpstreamEncodingUndecodable
)

// UpstreamEncoding is the classification of one upstream response body.
type UpstreamEncoding struct {
	// Value is the raw Content-Encoding header value ("" when absent).
	Value string
	// Kind is the readability classification of Value.
	Kind UpstreamEncodingKind
}

// Readable reports whether the bytes the caller holds (after Decode for a
// decodable encoding) may be parsed as content. When false, usage extraction
// and content judgement must both be skipped: unreadable bytes carry no
// evidence, and inventing a failure from them is what poisons channel health
// with perfectly healthy upstream answers.
func (e UpstreamEncoding) Readable() bool { return e.Kind != UpstreamEncodingUndecodable }

// Decodable reports whether a stdlib codec can turn the body into identity.
func (e UpstreamEncoding) Decodable() bool { return e.Kind == UpstreamEncodingDecodable }

// RelayHeader reports whether the upstream Content-Encoding must stay on the
// response relayed to the downstream client. True exactly when the bytes are
// relayed verbatim and therefore still encoded; false once we decoded them (we
// re-frame the body ourselves, so keeping the header would be a lie) and false
// when there is no header to relay.
func (e UpstreamEncoding) RelayHeader() bool {
	return e.Kind == UpstreamEncodingUndecodable && e.Value != ""
}

// String keeps log lines readable.
func (e UpstreamEncoding) String() string {
	switch e.Kind {
	case UpstreamEncodingIdentity:
		return "identity"
	case UpstreamEncodingDecodable:
		return "decodable(" + e.Value + ")"
	case UpstreamEncodingUndecodable:
		return "undecodable(" + e.Value + ")"
	}
	return "unknown"
}

// decodableContentEncodings are the codecs the Go standard library provides.
// brotli and zstd are deliberately absent: decoding them would need a new
// dependency, and guessing at bytes we cannot decode is exactly the dishonesty
// this file exists to prevent.
var decodableContentEncodings = map[string]bool{
	"gzip":    true,
	"x-gzip":  true,
	"deflate": true,
}

// ClassifyUpstreamEncoding is the ONE owner of "can we read this upstream
// body?". Both the buffered and the streaming dispatch path classify through
// it, so the two can never disagree about what an encoding means.
//
// net/http already handles the dominant case before we get here: when the
// outbound request carried no explicit Accept-Encoding, the transport adds
// "gzip" itself, transparently decompresses a gzip answer and deletes
// Content-Encoding / Content-Length from resp.Header — which lands here as
// identity. This function therefore only ever sees an encoding the transport
// refused to touch: a codec it does not implement (deflate/br/zstd), a
// multi-layer stack, or a gzip answer that arrived on a transport which did
// not negotiate it.
func ClassifyUpstreamEncoding(h http.Header) UpstreamEncoding {
	value := ""
	if h != nil {
		value = strings.TrimSpace(h.Get("Content-Encoding"))
	}
	if value == "" || strings.EqualFold(value, "identity") {
		return UpstreamEncoding{Value: value, Kind: UpstreamEncodingIdentity}
	}
	// A single codec token only. RFC 9110 allows a stacked list ("gzip, br"),
	// and peeling a stack we only partly understand would leave the body in a
	// state neither we nor the client can describe — so a stack is treated as
	// undecodable and relayed verbatim.
	if !strings.Contains(value, ",") && decodableContentEncodings[strings.ToLower(value)] {
		return UpstreamEncoding{Value: value, Kind: UpstreamEncodingDecodable}
	}
	return UpstreamEncoding{Value: value, Kind: UpstreamEncodingUndecodable}
}

// UpstreamBufferedBody is the buffered-body view the data plane parses and
// relays. It is the single answer to "what do the bytes I just read mean?".
type UpstreamBufferedBody struct {
	// Bytes is what the caller must parse AND relay. Identity or freshly
	// decoded bytes when Readable; the untouched encoded bytes otherwise.
	Bytes []byte
	// Readable reports whether Bytes is actual content. When false the caller
	// MUST skip usage extraction and content judgement, record usage as
	// unknown, and relay Bytes together with the upstream Content-Encoding.
	Readable bool
	// Encoding is the classification, for logging.
	Encoding UpstreamEncoding
	// DecodeErr carries the stdlib failure when a codec we do implement could
	// not actually decode the body (corrupt, truncated, or the decoded size
	// exceeded the buffered limit). Logging only — the honesty contract is the
	// same as for an unsupported codec.
	DecodeErr error
}

// NormalizeUpstreamBufferedBody applies the encoding decision to a buffered
// upstream body. resp.Header is mutated exactly when the body was decoded: the
// Content-Encoding is deleted so the existing relay whitelist cannot forward a
// header that no longer describes the bytes we are about to write.
func NormalizeUpstreamBufferedBody(h http.Header, body []byte) UpstreamBufferedBody {
	enc := ClassifyUpstreamEncoding(h)
	if !enc.Decodable() {
		return UpstreamBufferedBody{Bytes: body, Readable: enc.Readable(), Encoding: enc}
	}
	decoded, err := decodeBufferedBody(enc.Value, body, MaxBufferedResponseBodyBytes())
	if err != nil {
		// We implement the codec but could not read this body. Same contract as
		// an unsupported codec: never judge bytes we cannot parse, and hand the
		// client exactly what the upstream sent.
		return UpstreamBufferedBody{Bytes: body, Readable: false, Encoding: enc, DecodeErr: err}
	}
	if h != nil {
		h.Del("Content-Encoding")
	}
	return UpstreamBufferedBody{Bytes: decoded, Readable: true, Encoding: enc}
}

// decodeBufferedBody decodes a whole buffered body, bounded by limit so a
// highly-compressible upstream cannot turn a size-capped read into an unbounded
// allocation.
func decodeBufferedBody(codec string, body []byte, limit int64) ([]byte, error) {
	switch strings.ToLower(codec) {
	case "gzip", "x-gzip":
		return readBounded(body, limit, func(r io.Reader) (io.ReadCloser, error) { return gzip.NewReader(r) })
	case "deflate":
		// HTTP "deflate" is zlib-wrapped (RFC 9110) in practice, but servers do
		// emit raw DEFLATE (RFC 1951). Try zlib first, then raw flate — both are
		// stdlib, and both verify their own trailing checksum on EOF.
		decoded, err := readBounded(body, limit, func(r io.Reader) (io.ReadCloser, error) { return zlib.NewReader(r) })
		if err == nil {
			return decoded, nil
		}
		return readBounded(body, limit, func(r io.Reader) (io.ReadCloser, error) {
			return flate.NewReader(r), nil
		})
	}
	return nil, fmt.Errorf("proxy: no decoder for content encoding %q", codec)
}

func readBounded(body []byte, limit int64, newReader func(io.Reader) (io.ReadCloser, error)) ([]byte, error) {
	reader, err := newReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	decoded, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(decoded)) > limit {
		return nil, fmt.Errorf("proxy: decoded upstream body exceeded buffered limit %d bytes", limit)
	}
	return decoded, nil
}

// UpstreamStreamBody is the streaming counterpart of UpstreamBufferedBody: the
// same classification, plus a replacement reader when the codec is one we
// implement.
type UpstreamStreamBody struct {
	// Reader replaces the upstream body when the stream must be decoded. Nil
	// means "keep the body you already have" (identity, or undecodable).
	Reader io.ReadCloser
	// Readable reports whether the relayed chunks are actual SSE content. When
	// false the SSE analyzer must not be fed and the content judge must not
	// run; the chunks are relayed verbatim with the upstream Content-Encoding.
	Readable bool
	// Encoding is the classification, for logging.
	Encoding UpstreamEncoding
}

// WrapUpstreamStreamBody applies the encoding decision to a streaming upstream
// body. Decoding is lazy: the codec header is read on the first Read so this
// call never blocks before the downstream SSE headers go out.
func WrapUpstreamStreamBody(h http.Header, body io.ReadCloser) UpstreamStreamBody {
	enc := ClassifyUpstreamEncoding(h)
	if !enc.Decodable() {
		return UpstreamStreamBody{Readable: enc.Readable(), Encoding: enc}
	}
	return UpstreamStreamBody{
		Reader:   &lazyDecodeReader{codec: strings.ToLower(enc.Value), src: body},
		Readable: true,
		Encoding: enc,
	}
}

// lazyDecodeReader decodes a stream on first Read. A single goroutine (the SSE
// relay loop) ever touches it, so no locking is needed. Close always closes the
// underlying body: the relay's idle guard closes through this reader to
// interrupt a stalled Read, and the upstream connection must not leak.
type lazyDecodeReader struct {
	codec string
	src   io.ReadCloser
	dec   io.Reader
	err   error
}

func (l *lazyDecodeReader) Read(p []byte) (int, error) {
	if l.dec == nil && l.err == nil {
		l.dec, l.err = newStreamDecoder(l.codec, l.src)
	}
	if l.err != nil {
		return 0, l.err
	}
	return l.dec.Read(p)
}

func (l *lazyDecodeReader) Close() error {
	if closer, ok := l.dec.(io.Closer); ok && closer != nil {
		_ = closer.Close()
	}
	if l.src != nil {
		return l.src.Close()
	}
	return nil
}

func newStreamDecoder(codec string, src io.Reader) (io.Reader, error) {
	switch codec {
	case "gzip", "x-gzip":
		return gzip.NewReader(src)
	case "deflate":
		reader, err := zlib.NewReader(src)
		if err == nil {
			return reader, nil
		}
		return flate.NewReader(src), nil
	}
	return nil, fmt.Errorf("proxy: no decoder for content encoding %q", codec)
}
