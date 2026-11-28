package proxyhandler

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
