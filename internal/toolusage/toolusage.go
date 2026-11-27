// Package toolusage extracts metering evidence from one upstream response.
package toolusage

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"strings"
)

// MaxCount bounds upstream counters before they can become price multipliers.
const MaxCount int64 = 1 << 50

// CacheCreationTokens includes all TTLs. The unclassified remainder uses the
// ordinary write price (30m for OpenAI). Gemini reads are a subset of input.
type CacheUsage struct {
	CacheReadTokens        int64
	CacheCreationTokens    int64
	CacheCreation5mTokens  int64
	CacheCreation1hTokens  int64
	CacheCreation12hTokens int64
}

type object = map[string]any

func child(o object, key string) object {
	v, _ := o[key].(object)
	return v
}

func count(v any) (int64, bool) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, false
	}
	i, err := n.Int64()
	if err != nil || i < 0 || i > MaxCount {
		return 0, false
	}
	return i, true
}

func largest(o object, keys ...string) int64 {
	var best int64
	for _, key := range keys {
		n, _ := count(o[key])
		best = max(best, n)
	}
	return best
}

// payload accepts a JSON object, SSE data payload, or one complete SSE event.
func payload(raw []byte) object {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}
	if raw[0] != '{' {
		var data [][]byte
		for line := range bytes.SplitSeq(raw, []byte{'\n'}) {
			if value, ok := bytes.CutPrefix(line, []byte("data:")); ok {
				data = append(data, value)
			}
		}
		raw = bytes.Join(data, []byte{'\n'})
	}
	var o object
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if d.Decode(&o) != nil || len(bytes.TrimSpace(raw[d.InputOffset():])) != 0 {
		return nil
	}
	return o
}

func response(o object) object {
	for _, key := range []string{"response", "message"} {
		if nested := child(o, key); nested != nil {
			return nested
		}
	}
	return o
}

// ExtractCacheUsage reads one JSON/SSE frame or a standalone usage object.
// Malformed counters are ignored individually, preserving valid sibling fields.
func ExtractCacheUsage(raw []byte) CacheUsage {
	r := response(payload(raw))
	u := child(r, "usage")
	if u == nil {
		u = r
	}
	total := largest(u, "cache_creation_input_tokens")
	read := max(largest(u, "cache_read_input_tokens"), largest(child(r, "usageMetadata"), "cachedContentTokenCount"), largest(u, "cachedContentTokenCount"))
	var long int64
	for _, key := range []string{"prompt_tokens_details", "input_tokens_details"} {
		d := child(u, key)
		total = max(total, largest(d, "cache_write_tokens", "cached_creation_tokens", "cache_creation_input_tokens"))
		read = max(read, largest(d, "cached_tokens"))
		long = max(long, largest(child(d, "cache_write_tokens_details"), "ttl_12h_tokens"))
	}
	c := child(u, "cache_creation")
	five := largest(c, "ephemeral_5m_input_tokens")
	hour := largest(c, "ephemeral_1h_input_tokens")
	if _, present := u["cache_creation_input_tokens"]; !present && total == 0 && five+hour <= MaxCount {
		total = five + hour
	}
	long = min(long, total)
	hour = min(hour, total-long)
	five = min(five, total-long-hour)
	return CacheUsage{CacheReadTokens: read, CacheCreationTokens: total, CacheCreation5mTokens: five, CacheCreation1hTokens: hour, CacheCreation12hTokens: long}
}

const (
	SourceVendorReported  = "vendor_reported"
	SourceResponseDerived = "response_derived"
)

// ToolUsageItem contains only normalized labels and a quantity, never vendor text.
type ToolUsageItem struct {
	Tool     string `json:"tool"`
	Unit     string `json:"unit"`
	Quantity int64  `json:"quantity"`
	Source   string `json:"source,omitempty"`
}

// Tracker's zero value is ready for a single response, including its stream.
type Tracker struct {
	web, file callCount
	queries   map[[sha256.Size]byte]struct{}
	vendorWeb *int64
}

type callCount struct {
	ids       map[string]bool
	positions map[int64]string
	n         int64
}

// Keep both identities: a final output array can supply an ID that was absent
// from the stream, or omit one that was already observed.
func (c *callCount) observe(item object, index int64) {
	id, _ := item["id"].(string)
	if id == "" && index < 0 {
		return
	}
	if c.ids == nil {
		c.ids = make(map[string]bool)
		c.positions = make(map[int64]string)
	}
	previous, knownPosition := c.positions[index]
	if !c.ids[id] && (!knownPosition || id != "" && previous != "" && previous != id) {
		c.n++
	}
	if id != "" {
		c.ids[id] = true
	}
	if index >= 0 && (!knownPosition || id != "") {
		c.positions[index] = id
	}
}

func (t *Tracker) observeCall(item object, index int64) {
	switch item["type"] {
	case "web_search_call":
		t.web.observe(item, index)
	case "file_search_call":
		t.file.observe(item, index)
	case "server_tool_use":
		if item["name"] == "web_search" {
			t.web.observe(item, index)
		}
	}
}

// Observe consumes one response object or SSE event. Tool declarations and
// client functions supply no evidence of a hosted invocation.
func (t *Tracker) Observe(raw []byte) {
	event := payload(raw)
	r := response(event)
	switch event["type"] {
	case "response.output_item.done":
		t.observeCall(child(event, "item"), eventIndex(event, "output_index"))
	case "content_block_start":
		t.observeCall(child(event, "content_block"), eventIndex(event, "index"))
	}
	for _, key := range []string{"output", "content"} {
		items, _ := r[key].([]any)
		for i, v := range items {
			item, _ := v.(object)
			t.observeCall(item, int64(i))
		}
	}
	u := child(r, "usage")
	for _, v := range []any{u["web_search_requests"], child(u, "server_tool_use")["web_search_requests"], child(child(r, "tool_usage"), "web_search")["num_requests"]} {
		if n, ok := count(v); ok {
			t.vendorWeb = &n
		}
	}
	t.observeGrounding(r)
}

func eventIndex(event object, key string) int64 {
	if i, ok := count(event[key]); ok {
		return i
	}
	return -1
}

func (t *Tracker) observeGrounding(r object) {
	candidates, _ := r["candidates"].([]any)
	for _, v := range candidates {
		candidate, _ := v.(object)
		metadata := child(candidate, "groundingMetadata")
		for _, key := range []string{"webSearchQueries", "imageSearchQueries"} {
			queries, _ := metadata[key].([]any)
			for _, q := range queries {
				text, _ := q.(string)
				text = strings.TrimSpace(text)
				if text == "" {
					continue
				}
				if t.queries == nil {
					t.queries = make(map[[sha256.Size]byte]struct{})
				}
				t.queries[sha256.Sum256([]byte(text))] = struct{}{}
			}
		}
	}
}

// Snapshot returns detached quantities in a stable order. A vendor-reported
// zero stays visible and takes precedence over all observed response items.
func (t *Tracker) Snapshot() []ToolUsageItem {
	var out []ToolUsageItem
	if t.vendorWeb != nil {
		out = append(out, ToolUsageItem{"web_search", "calls", *t.vendorWeb, SourceVendorReported})
	} else if t.web.n > 0 {
		out = append(out, ToolUsageItem{"web_search", "calls", t.web.n, SourceResponseDerived})
	}
	if t.file.n > 0 {
		out = append(out, ToolUsageItem{"file_search", "calls", t.file.n, SourceResponseDerived})
	}
	if n := len(t.queries); n > 0 {
		out = append(out, ToolUsageItem{"grounding", "queries", int64(n), SourceResponseDerived}, ToolUsageItem{"grounding", "grounded_prompts", 1, SourceResponseDerived})
	}
	return out
}
