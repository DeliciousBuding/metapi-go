package streamtiming

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

type clock struct{ ms int64 }

func (c *clock) now() time.Time { return time.UnixMilli(c.ms) }

func wantMs(t *testing.T, field string, got *int64, want int64) {
	t.Helper()
	if want < 0 && got == nil {
		return
	}
	if got == nil {
		t.Fatalf("%s = nil, want %d", field, want)
	}
	if *got != want || want < 0 {
		t.Fatalf("%s = %d, want %d (negative means absent)", field, *got, want)
	}
}

func wantSpeed(t *testing.T, s Snapshot, status string, tps float64) {
	t.Helper()
	if s.DecodeStatus != status {
		t.Fatalf("status = %s, want %s (%+v)", s.DecodeStatus, status, s)
	}
	if status == StatusMeasured {
		if s.DecodeTPS == nil || *s.DecodeTPS != tps {
			t.Fatalf("TPS = %v, want %v", s.DecodeTPS, tps)
		}
	} else if s.DecodeTPS != nil {
		t.Fatalf("%s returned TPS %v", status, *s.DecodeTPS)
	}
}

func TestProtocolStreams(t *testing.T) {
	// Identical arrival schedules exercise the wire formats through Finalize.
	for _, tc := range []struct {
		proto                                           Protocol
		open, reason1, reason2, text, tool, ping, usage string
		firstReason, gap, window, tokens                int64
		evidence                                        string
	}{
		{ProtocolChat, `{"choices":[{"delta":{"role":"assistant","content":""}}]}`,
			`{"choices":[{"delta":{"reasoning_content":"think"}}]}`, `{"choices":[{"delta":{"reasoning":"think"}}]}`,
			`{"choices":[{"delta":{"content":"words"}}]}`, `{"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"input"}}]}}]}`,
			`{"choices":[{"delta":{}}]}`, `{"choices":[],"usage":{"completion_tokens":70}}`, 100, 200, 1000, 70, "visible"},
		{ProtocolResponses, `{"type":"response.created"}`, `{"type":"response.output_item.added","item":{"type":"reasoning"}}`,
			`{"type":"response.reasoning_summary_text.delta","delta":"think"}`, `{"type":"response.output_text.delta","delta":"words"}`,
			`{"type":"response.function_call_arguments.delta","delta":"input"}`, `{"type":"ping"}`,
			`{"type":"response.completed","response":{"output":[{"type":"message","content":[{"text":"replay"}]}],"usage":{"output_tokens":70}}}`, 100, 200, 1000, 70, "visible"},
		{ProtocolMessages, `{"type":"message_start"}`, `{"type":"content_block_start","content_block":{"type":"thinking","thinking":""}}`,
			`{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"think"}}`,
			`{"type":"content_block_delta","delta":{"type":"text_delta","text":"words"}}`,
			`{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"input"}}`, `{"type":"ping"}`,
			`{"type":"message_delta","usage":{"output_tokens":70}}`, 100, 200, 1000, 70, "visible"},
		{ProtocolGemini, `{"candidates":[{"content":{"role":"model","parts":[]}}]}`, `{"candidates":[{"content":{"parts":[{"thought":true,"text":"think"}]}}]}`,
			`{"candidates":[{"content":{"parts":[{"thought":true,"text":"think"}]}}]}`, `{"candidates":[{"content":{"parts":[{"text":"words"}]}}]}`,
			`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"input"}}]}}]}`, `{"candidates":[]}`,
			`{"usageMetadata":{"candidatesTokenCount":50,"thoughtsTokenCount":20}}`, -1, 200, 700, 50, "summaryOnly"},
	} {
		t.Run(string(tc.proto), func(t *testing.T) {
			c := &clock{}
			o := NewObserver(c.now)
			c.ms = 10
			o.MarkHeader()
			for _, e := range []struct {
				ms   int64
				body string
			}{{20, tc.open}, {100, tc.reason1}, {200, tc.reason2}, {300, tc.ping}, {400, tc.text}, {600, tc.tool}, {800, tc.text}, {1000, tc.text}, {1100, tc.usage}} {
				c.ms = e.ms
				o.ObserveFrame(tc.proto, "", []byte(e.body))
			}
			end := c.now()
			c.ms = 30000
			o.MarkHeader()
			s := o.Finalize(FinalizeInput{OutputTokens: 70, ReasoningTokens: 20, HasUsage: true, EndedAt: end})
			wantMs(t, "header", s.HeaderMs, 10)
			wantMs(t, "reasoning", s.FirstReasoningMs, tc.firstReason)
			wantMs(t, "text", s.FirstTextMs, 400)
			wantMs(t, "last", s.LastContentMs, 1000)
			wantMs(t, "gap", s.MaxContentGapMs, tc.gap)
			wantMs(t, "window", s.DecodeWindowMs, tc.window)
			if s.ReasoningEvidence != tc.evidence || s.DecodeTokens != tc.tokens {
				t.Fatalf("evidence/tokens = %s/%d", s.ReasoningEvidence, s.DecodeTokens)
			}
			wantSpeed(t, s, StatusMeasured, float64(tc.tokens)/(float64(tc.window)/1000))
		})
	}
}

func TestHiddenAndSummary(t *testing.T) {
	for _, tc := range []struct {
		proto                 Protocol
		event, body, evidence string
	}{
		{ProtocolMessages, "content_block_start", `{"content_block":{"type":"redacted_thinking","data":"sealed"}}`, "hidden"},
		{ProtocolResponses, "response.output_item.added", `{"item":{"type":"reasoning","encrypted_content":"sealed"}}`, "hidden"},
		{ProtocolResponses, "response.completed", `{"response":{"output":[{"type":"reasoning","encrypted_content":"sealed"}]}}`, "hidden"},
		{ProtocolMessages, "content_block_start", `{"content_block":{"type":"thinking","summarized":true}}`, "hidden"},
		{ProtocolResponses, "response.reasoning_summary_text.delta", `{"delta":"summary"}`, "summaryOnly"},
		{ProtocolGemini, "", `{"candidates":[{"content":{"parts":[{"thought":true,"text":"summary"}]}}]}`, "summaryOnly"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			c := &clock{}
			o := NewObserver(c.now)
			o.ObserveFrame(tc.proto, tc.event, []byte(tc.body))
			in := FinalizeInput{HasUsage: true, OutputTokens: 100, ReasoningTokens: 90}
			s := o.Finalize(in)
			wantSpeed(t, s, StatusHiddenReasoning, 0)
			wantMs(t, "firstReasoning", s.FirstReasoningMs, -1)
			if s.ReasoningEvidence != tc.evidence {
				t.Fatalf("evidence = %s", s.ReasoningEvidence)
			}
			for _, ms := range []int64{200, 700, 1200} {
				c.ms = ms
				o.ObserveSignal(SignalTextDelta)
			}
			s = o.Finalize(in)
			wantSpeed(t, s, StatusMeasured, 10)
			wantMs(t, "window", s.DecodeWindowMs, 1000)
			if s.DecodeTokens != 10 {
				t.Fatal(s.DecodeTokens)
			}
			in.ReasoningTokens = 0
			wantSpeed(t, o.Finalize(in), StatusHiddenReasoning, 0)
		})
	}
}

func TestEvidenceAndSpeedLimits(t *testing.T) {
	usage := FinalizeInput{HasUsage: true, OutputTokens: 50}
	for _, tc := range []struct {
		name            string
		times           []int64
		tokenCount, end int64
		status          string
	}{
		{"short", []int64{0, 99}, 50, 99, StatusBufferedBurst},
		{"on boundary", []int64{0, 100}, 1000, 100, StatusMeasured},
		{"too fast", []int64{0, 100}, 1001, 100, StatusBufferedBurst},
		{"held", []int64{1000, 1200}, 50, 1200, StatusBufferedBurst},
		{"idle tail", []int64{0, 1}, 50, 2000, StatusBufferedBurst},
		{"coincident", []int64{0, 0}, 50, 0, StatusBufferedBurst},
		{"bad end", []int64{0, 200}, 50, 100, StatusInsufficientEvidence},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &clock{}
			o := NewObserver(c.now)
			for _, ms := range tc.times {
				c.ms = ms
				o.ObserveSignal(SignalToolDelta)
			}
			in := usage
			in.OutputTokens = tc.tokenCount
			in.EndedAt = time.UnixMilli(tc.end)
			s := o.Finalize(in)
			wantMs(t, "text", s.FirstTextMs, -1)
			wantSpeed(t, s, tc.status, 10000)
		})
	}
	c := &clock{}
	o := NewObserver(c.now)
	o.ObserveSignal(SignalReasoningDelta)
	c.ms = 99
	o.ObserveSignal(SignalReasoningDelta)
	s := o.Finalize(usage)
	wantMs(t, "reasoning", s.FirstReasoningMs, -1)
	wantSpeed(t, s, StatusBufferedBurst, 0)
	c.ms = 100
	o.ObserveSignal(SignalReasoningDelta)
	s = o.Finalize(usage)
	wantMs(t, "reasoning", s.FirstReasoningMs, 0)
	wantSpeed(t, s, StatusMeasured, 500)
	for _, in := range []FinalizeInput{{OutputTokens: 5}, {HasUsage: true}, {HasUsage: true, OutputTokens: 2, ReasoningTokens: 3}, {HasUsage: true, OutputTokens: -1}} {
		wantSpeed(t, o.Finalize(in), StatusInsufficientEvidence, 0)
	}
	usage.Interrupted = true
	wantSpeed(t, o.Finalize(usage), StatusInterrupted, 0)
}

func TestContentConcentratedByPayload(t *testing.T) {
	c := &clock{}
	o := NewObserver(c.now)
	for i := range 11 {
		c.ms = int64(i) * 100
		text := "x"
		if i == 10 {
			text = strings.Repeat("x", 1000)
		}
		o.ObserveFrame(ProtocolResponses, "response.output_text.delta", []byte(`{"delta":"`+text+`"}`))
	}
	s := o.Finalize(FinalizeInput{HasUsage: true, OutputTokens: 80})
	wantSpeed(t, s, StatusBufferedBurst, 0)
	wantMs(t, "window", s.DecodeWindowMs, 1000)
}

func TestHeartbeatDoesNotSplitContentGap(t *testing.T) {
	c := &clock{}
	o := NewObserver(c.now)
	o.ObserveFrame(ProtocolMessages, "content_block_start", []byte(`{"content_block":{"type":"tool_use"}}`))
	c.ms = 700
	o.ObserveFrame(ProtocolMessages, "ping", []byte(`{"type":"ping"}`))
	s := o.Finalize(FinalizeInput{})
	wantMs(t, "last", s.LastContentMs, 0)
	wantMs(t, "gap", s.MaxContentGapMs, -1)
	c.ms = 1000
	o.ObserveFrame(ProtocolMessages, "content_block_delta", []byte(`{"delta":{"type":"input_json_delta","partial_json":"}"}}`))
	s = o.Finalize(FinalizeInput{})
	wantMs(t, "gap", s.MaxContentGapMs, 1000)
	wantMs(t, "text", s.FirstTextMs, -1)
}

func TestFlowBoundAndExtremeUsage(t *testing.T) {
	c := &clock{}
	o := NewObserver(c.now)
	in := FinalizeInput{HasUsage: true, OutputTokens: 1<<63 - 1}
	o.ObserveSignal(SignalTextDelta)
	c.ms = 1000
	o.ObserveSignal(SignalTextDelta)
	wantSpeed(t, o.Finalize(in), StatusBufferedBurst, 0)
	for range 1 << 16 {
		c.ms++
		o.ObserveSignal(SignalTextDelta)
	}
	in.OutputTokens = 1000
	s := o.Finalize(in)
	wantSpeed(t, s, StatusInsufficientEvidence, 0)
	wantMs(t, "last", s.LastContentMs, c.ms)
	wantMs(t, "gap", s.MaxContentGapMs, 1000)
}

func TestEmptyFramesAndDetachedJSON(t *testing.T) {
	c := &clock{}
	o := NewObserver(c.now)
	for _, f := range []struct {
		p           Protocol
		event, body string
	}{
		{ProtocolChat, "", `{"choices":[{"delta":{"role":"assistant","tool_calls":[{"index":0}],"content":null}}]}`},
		{ProtocolResponses, "response.output_text.delta", `{"delta":""}`},
		{ProtocolResponses, "response.output_text.delta", `{"delta":true}`},
		{ProtocolResponses, "response.output_text.done", `{"text":"replayed"}`},
		{ProtocolMessages, "content_block_delta", `{"delta":{"type":"signature_delta","signature":"sealed"}}`},
		{ProtocolGemini, "", `{"candidates":[{"content":{"parts":[{"thoughtSignature":"signed"}]}}]}`},
		{ProtocolChat, "", `[DONE]`}, {ProtocolChat, "", `{"choices":[{"delta":{"content":"half"}}]`},
		{ProtocolChat, "", `{"candidates":[{"content":{"parts":[{"text":"wrong protocol"}]}}]}`},
	} {
		o.ObserveFrame(f.p, f.event, []byte(f.body))
	}
	s := o.Finalize(FinalizeInput{})
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err = json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"headerMs": nil, "firstReasoningMs": nil, "firstTextMs": nil, "lastContentMs": nil, "maxContentGapMs": nil, "reasoningEvidence": "none", "decodeTokens": float64(0), "decodeWindowMs": nil, "decodeTps": nil, "decodeStatus": "insufficientEvidence"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("empty JSON = %s", b)
	}
	o.MarkHeader()
	o.ObserveFrame(ProtocolChat, "", []byte(`{"choices":[{"delta":{"content":"secret body","tool_calls":[{"function":{"arguments":"private arguments"}}]}}]}`))
	s = o.Finalize(FinalizeInput{})
	wantMs(t, "gap", s.MaxContentGapMs, -1)
	wantMs(t, "text", s.FirstTextMs, 0)
	*s.FirstTextMs = 42
	c.ms = 200
	o.ObserveSignal(SignalToolDelta)
	updated := o.Finalize(FinalizeInput{HasUsage: true, OutputTokens: 5})
	wantMs(t, "text", updated.FirstTextMs, 0)
	wantMs(t, "gap", updated.MaxContentGapMs, 200)
	wantMs(t, "previous last", s.LastContentMs, 0)
	b, err = json.Marshal(updated)
	if err != nil || strings.Contains(string(b), "secret") || strings.Contains(string(b), "private") {
		t.Fatalf("snapshot = %s (%v)", b, err)
	}
}

func BenchmarkObserveFrame(b *testing.B) {
	c := &clock{}
	o := NewObserver(c.now)
	data := []byte(`{"id":"chatcmpl-test","choices":[{"index":0,"delta":{"content":"some streamed words"}}]}`)
	b.ReportAllocs()
	b.SetBytes(int64(len(data)))
	for b.Loop() {
		o.ObserveFrame(ProtocolChat, "", data)
	}
}
