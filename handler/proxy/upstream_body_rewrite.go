package proxyhandler

import (
	"bytes"
	"strings"
)

// Pre-allocated key markers keep the hot-path scans allocation-free.
var (
	streamKeyMarker  = []byte(`"stream"`)
	streamOptsMarker = []byte(`"stream_options"`)
)

// containsLongMarker is a drop-in bytes.Contains replacement that stays on
// the SIMD-accelerated bytes.Index fast path for needles longer than 8 bytes
// (bytes.Index drops long needles to the slow Rabin-Karp fallback). The 8-byte
// prefix locates candidates; the full needle is verified at each rare hit.
func containsLongMarker(body, needle []byte) bool {
	if len(needle) <= 8 {
		return bytes.Contains(body, needle)
	}
	prefix := needle[:8]
	rest := body
	for {
		i := bytes.Index(rest, prefix)
		if i < 0 {
			return false
		}
		if i+len(needle) <= len(rest) && bytes.Equal(rest[i:i+len(needle)], needle) {
			return true
		}
		rest = rest[i+1:]
	}
}

// bodyStreamHints carries cheaply pre-scanned facts about a JSON request body
// so multiple candidate paths can share one look over the payload.
type bodyStreamHints struct {
	valid         bool // body is framed as a single top-level JSON object
	emptyObject   bool // body is exactly {} (modulo whitespace)
	openIdx       int  // index of the opening brace when valid
	hasStreamKey  bool // byte pattern "stream" present anywhere
	hasStreamOpts bool // byte pattern "stream_options" present anywhere
}

func scanBodyStreamHints(bodyBytes []byte) bodyStreamHints {
	var h bodyStreamHints
	h.hasStreamKey = bytes.Contains(bodyBytes, streamKeyMarker)
	h.hasStreamOpts = containsLongMarker(bodyBytes, streamOptsMarker)
	h.openIdx, h.emptyObject, h.valid = objectBraces(bodyBytes)
	return h
}

// streamValueAlreadyTrue mirrors the legacy check: JSON true or the exact
// strings "true" / "1" mean the client already streams (no rewrite, no force).
func streamValueAlreadyTrue(body []byte, s jsonSpan) bool {
	v := body[s.start:s.end]
	if len(v) == 4 && bytesEqualString(v, "true") {
		return true
	}
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		inner := v[1 : len(v)-1]
		return bytesEqualString(inner, "true") || bytesEqualString(inner, "1")
	}
	return false
}

// includeUsageTruthy mirrors jsonTruthyBool for a raw value span.
func includeUsageTruthy(body []byte, s jsonSpan) bool {
	v := body[s.start:s.end]
	if len(v) == 4 && bytesEqualString(v, "true") {
		return true
	}
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		t := strings.TrimSpace(strings.ToLower(string(v[1 : len(v)-1])))
		return t == "true" || t == "1" || t == "yes"
	}
	return false
}

// rewriteUpstreamStreamFlags applies site stream forcing (stream=true) and the
// OpenAI stream_options.include_usage inject to a JSON request body in one
// scan + splice. forceStream and injectUsage are the caller-computed gates;
// clientStream is ctx.IsStream. Returns the (possibly original) body, whether
// stream was forced, whether the outbound body is expected to yield a final
// usage chunk, and ok=false when the body is irregular and the caller must use
// the legacy decode path instead.
func rewriteUpstreamStreamFlags(bodyBytes []byte, hints bodyStreamHints, forceStream, clientStream, injectUsage bool) (out []byte, forced, expectUsage, ok bool) {
	body := bodyBytes

	// Stage 1: force stream=true for responses-only / stream-preferring sites.
	if forceStream {
		if len(body) == 0 {
			body = []byte(`{"stream":true}`)
			forced = true
		} else if !hints.valid {
			return nil, false, false, false
		} else if !hints.hasStreamKey {
			body = insertTopLevelEntry(body, hints.openIdx, `"stream":true`, hints.emptyObject)
			forced = true
		} else {
			s, found, scanOK := findTopLevelValue(body, "stream")
			if !scanOK {
				return nil, false, false, false
			}
			if found && streamValueAlreadyTrue(body, s) {
				forced = false // client already streams; leave body untouched
			} else if found {
				body = replaceSpan(body, s, "true")
				forced = true
			} else {
				// "stream" byte hit was nested only; no top-level key.
				body = insertTopLevelEntry(body, hints.openIdx, `"stream":true`, hints.emptyObject)
				forced = true
			}
		}
	}

	// Stage 2: ensure stream_options.include_usage=true on accepting paths.
	if injectUsage && (clientStream || forced) {
		if len(body) == 0 {
			return body, forced, false, true
		}
		emptyNow := hints.emptyObject && !forced
		if !hints.hasStreamOpts {
			if !hints.valid {
				return nil, false, false, false
			}
			body = insertTopLevelEntry(body, hints.openIdx, `"stream_options":{"include_usage":true}`, emptyNow)
			expectUsage = true
		} else {
			s, found, scanOK := findTopLevelValue(body, "stream_options")
			if !scanOK {
				return nil, false, false, false
			}
			if !found {
				// Byte hit was nested only; no top-level stream_options.
				body = insertTopLevelEntry(body, hints.openIdx, `"stream_options":{"include_usage":true}`, emptyNow)
				expectUsage = true
			} else if !spanIsObject(body, s) {
				body = replaceSpan(body, s, `{"include_usage":true}`)
				expectUsage = true
			} else {
				sub := body[s.start:s.end]
				subOpen, subEmpty, subOK := objectBraces(sub)
				if !subOK {
					return nil, false, false, false
				}
				is, iFound, iOK := findTopLevelValue(sub, "include_usage")
				if !iOK {
					return nil, false, false, false
				}
				switch {
				case !iFound:
					body = insertIntoObjectSpan(body, s, subOpen, `"include_usage":true`, subEmpty)
					expectUsage = true
				case includeUsageTruthy(sub, is):
					expectUsage = true // already requested; body untouched
				default:
					abs := jsonSpan{s.start + is.start, s.start + is.end}
					body = replaceSpan(body, abs, "true")
					expectUsage = true
				}
			}
		}
	}
	return body, forced, expectUsage, true
}
