package proxyhandler

import (
	"bytes"
	"strings"
)

// ---------------------------------------------------------------------------
// Allocation-free top-level JSON object editing for the proxy hot path.
//
// Every proxied request used to pay two or three full decode/re-encode rounds
// over the request body (model swap, stream forcing, include_usage inject),
// each unmarshalling the whole payload into map[string]any or
// map[string]json.RawMessage and marshalling it back. The hot path only ever
// needs to inspect or patch a handful of TOP-LEVEL keys ("model", "stream",
// "stream_options"), so a depth- and escape-aware byte scan can answer those
// questions and splice the bytes directly, touching each payload byte once and
// allocating only the (single) rewritten output.
//
// All scan routines are fail-safe: on any irregularity (malformed structure,
// escaped top-level key, non-object body) they report failure and callers fall
// back to the legacy full-decode implementation, preserving exact semantics.
// Known limitation: duplicate top-level keys resolve to the FIRST occurrence
// here while encoding/json keeps the last; duplicate top-level keys are not
// emitted by any known client and RFC 8259 leaves their handling undefined.
// ---------------------------------------------------------------------------

// jsonSpan is a [start, end) byte range inside a request body.
type jsonSpan struct{ start, end int }

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

func isJSONSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// bytesEqualString compares b to s without allocating.
func bytesEqualString(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if b[i] != s[i] {
			return false
		}
	}
	return true
}

// skipJSONString advances *i past the JSON string starting at body[*i]=='"'.
func skipJSONString(body []byte, i *int) bool {
	j := *i + 1
	for j < len(body) {
		switch body[j] {
		case '\\':
			j += 2
		case '"':
			*i = j + 1
			return true
		default:
			j++
		}
	}
	return false
}

func skipJSONLiteral(body []byte, i *int, lit string) bool {
	end := *i + len(lit)
	if end > len(body) {
		return false
	}
	for k := 0; k < len(lit); k++ {
		if body[*i+k] != lit[k] {
			return false
		}
	}
	*i = end
	return true
}

func skipJSONNumber(body []byte, i *int) bool {
	j := *i
	if j < len(body) && body[j] == '-' {
		j++
	}
	digits := 0
	for j < len(body) && body[j] >= '0' && body[j] <= '9' {
		j++
		digits++
	}
	if digits == 0 {
		return false
	}
	if j < len(body) && body[j] == '.' {
		j++
		frac := 0
		for j < len(body) && body[j] >= '0' && body[j] <= '9' {
			j++
			frac++
		}
		if frac == 0 {
			return false
		}
	}
	if j < len(body) && (body[j] == 'e' || body[j] == 'E') {
		j++
		if j < len(body) && (body[j] == '+' || body[j] == '-') {
			j++
		}
		exp := 0
		for j < len(body) && body[j] >= '0' && body[j] <= '9' {
			j++
			exp++
		}
		if exp == 0 {
			return false
		}
	}
	*i = j
	return true
}

// skipJSONContainer advances *i past one balanced {...} or [...] container
// starting at body[*i], ignoring delimiters inside strings.
func skipJSONContainer(body []byte, i *int) bool {
	depth := 0
	j := *i
	for j < len(body) {
		switch body[j] {
		case '"':
			if !skipJSONString(body, &j) {
				return false
			}
			continue
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				*i = j + 1
				return true
			}
		}
		j++
	}
	return false
}

// skipJSONValue advances *i past one JSON value starting at body[*i].
func skipJSONValue(body []byte, i *int) bool {
	if *i >= len(body) {
		return false
	}
	switch body[*i] {
	case '"':
		return skipJSONString(body, i)
	case '{', '[':
		return skipJSONContainer(body, i)
	case 't':
		return skipJSONLiteral(body, i, "true")
	case 'f':
		return skipJSONLiteral(body, i, "false")
	case 'n':
		return skipJSONLiteral(body, i, "null")
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return skipJSONNumber(body, i)
	}
	return false
}

// findTopLevelValue locates the first top-level occurrence of key in the JSON
// object body and returns its value span. found=false when the object is clean
// but lacks the key. ok=false when the body is not a cleanly scannable
// top-level JSON object (malformed, non-object, trailing garbage, or an
// escaped top-level key that byte comparison cannot resolve); callers must
// then use the legacy full-decode behavior for exact semantics.
func findTopLevelValue(body []byte, key string) (s jsonSpan, found, ok bool) {
	i := 0
	for i < len(body) && isJSONSpace(body[i]) {
		i++
	}
	if i >= len(body) || body[i] != '{' {
		return jsonSpan{}, false, false
	}
	i++
	for {
		for i < len(body) && isJSONSpace(body[i]) {
			i++
		}
		if i >= len(body) {
			return jsonSpan{}, false, false
		}
		if body[i] == '}' {
			return jsonSpan{}, found, trailingWhitespaceOnly(body, i+1)
		}
		if body[i] != '"' {
			return jsonSpan{}, false, false
		}
		keyStart := i + 1
		if !skipJSONString(body, &i) {
			return jsonSpan{}, false, false
		}
		keyEnd := i - 1
		escaped := false
		for k := keyStart; k < keyEnd; k++ {
			if body[k] == '\\' {
				escaped = true
				break
			}
		}
		for i < len(body) && isJSONSpace(body[i]) {
			i++
		}
		if i >= len(body) || body[i] != ':' {
			return jsonSpan{}, false, false
		}
		i++
		for i < len(body) && isJSONSpace(body[i]) {
			i++
		}
		vStart := i
		if !skipJSONValue(body, &i) {
			return jsonSpan{}, false, false
		}
		if escaped {
			return jsonSpan{}, false, false
		}
		if bytesEqualString(body[keyStart:keyEnd], key) {
			return jsonSpan{vStart, i}, true, true
		}
		for i < len(body) && isJSONSpace(body[i]) {
			i++
		}
		if i >= len(body) {
			return jsonSpan{}, false, false
		}
		if body[i] == ',' {
			i++
			continue
		}
		if body[i] == '}' {
			return jsonSpan{}, found, trailingWhitespaceOnly(body, i+1)
		}
		return jsonSpan{}, false, false
	}
}

func trailingWhitespaceOnly(body []byte, from int) bool {
	for k := from; k < len(body); k++ {
		if !isJSONSpace(body[k]) {
			return false
		}
	}
	return true
}

// objectBraces validates that body is framed as a single top-level JSON
// object ({...} with optional surrounding whitespace) and returns the opening
// brace index and whether the object is empty.
func objectBraces(body []byte) (openIdx int, empty, ok bool) {
	i := 0
	for i < len(body) && isJSONSpace(body[i]) {
		i++
	}
	if i >= len(body) || body[i] != '{' {
		return 0, false, false
	}
	openIdx = i
	j := len(body) - 1
	for j > i && isJSONSpace(body[j]) {
		j--
	}
	if j <= i || body[j] != '}' {
		return 0, false, false
	}
	empty = true
	for k := i + 1; k < j; k++ {
		if !isJSONSpace(body[k]) {
			empty = false
			break
		}
	}
	return openIdx, empty, true
}

// spanStringEquals reports whether the span holds the JSON string "want"
// encoded without escape sequences.
func spanStringEquals(body []byte, s jsonSpan, want string) bool {
	v := body[s.start:s.end]
	if len(v) != len(want)+2 || v[0] != '"' || v[len(v)-1] != '"' {
		return false
	}
	return bytesEqualString(v[1:len(v)-1], want)
}

// spanIsObject reports whether the value span starts a JSON object.
func spanIsObject(body []byte, s jsonSpan) bool {
	for k := s.start; k < s.end; k++ {
		if isJSONSpace(body[k]) {
			continue
		}
		return body[k] == '{'
	}
	return false
}

// insertTopLevelEntry splices entry (e.g. `"stream":true`) in as the first
// member of the top-level object whose opening brace is at openIdx.
func insertTopLevelEntry(body []byte, openIdx int, entry string, emptyObject bool) []byte {
	out := make([]byte, 0, len(body)+len(entry)+1)
	out = append(out, body[:openIdx+1]...)
	out = append(out, entry...)
	if !emptyObject {
		out = append(out, ',')
	}
	out = append(out, body[openIdx+1:]...)
	return out
}

// insertIntoObjectSpan splices entry in as the first member of the object
// value at s; subOpen/subEmpty come from objectBraces(body[s.start:s.end]).
func insertIntoObjectSpan(body []byte, s jsonSpan, subOpen int, entry string, subEmpty bool) []byte {
	pos := s.start + subOpen + 1
	out := make([]byte, 0, len(body)+len(entry)+1)
	out = append(out, body[:pos]...)
	out = append(out, entry...)
	if !subEmpty {
		out = append(out, ',')
	}
	out = append(out, body[pos:]...)
	return out
}

// replaceSpan returns body with the span replaced by repl (single allocation).
func replaceSpan(body []byte, s jsonSpan, repl string) []byte {
	out := make([]byte, 0, len(body)-(s.end-s.start)+len(repl))
	out = append(out, body[:s.start]...)
	out = append(out, repl...)
	out = append(out, body[s.end:]...)
	return out
}

// sameByteSlice reports whether two slices cover the identical backing region.
func sameByteSlice(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	return &a[0] == &b[0]
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
