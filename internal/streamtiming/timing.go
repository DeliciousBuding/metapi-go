// Package streamtiming measures arrival of streamed model output at the receiver.
package streamtiming

import "time"

type Protocol string

const (
	ProtocolChat      Protocol = "chat"
	ProtocolResponses Protocol = "responses"
	ProtocolMessages  Protocol = "messages"
	ProtocolGemini    Protocol = "gemini"
)

type SignalKind uint8

const (
	SignalReasoningStart SignalKind = 1 << iota
	SignalReasoningDelta
	SignalHiddenReasoning
	SignalReasoningSummary
	SignalTextDelta
	SignalToolDelta
)

const (
	StatusMeasured             = "measured"
	StatusBufferedBurst        = "bufferedBurst"
	StatusHiddenReasoning      = "hiddenReasoning"
	StatusInsufficientEvidence = "insufficientEvidence"
	StatusInterrupted          = "interrupted"
)

type FinalizeInput struct {
	// OutputTokens includes ReasoningTokens. A missing reasoning split is zero.
	OutputTokens, ReasoningTokens int64
	HasUsage, Interrupted         bool
	// EndedAt is the last byte's arrival time. Zero samples the observer clock.
	EndedAt time.Time
}

// Snapshot owns its values. Missing observations marshal as null, including
// the maximum gap before two content frames have arrived.
type Snapshot struct {
	HeaderMs          *int64   `json:"headerMs"`
	FirstReasoningMs  *int64   `json:"firstReasoningMs"`
	FirstTextMs       *int64   `json:"firstTextMs"`
	LastContentMs     *int64   `json:"lastContentMs"`
	MaxContentGapMs   *int64   `json:"maxContentGapMs"`
	ReasoningEvidence string   `json:"reasoningEvidence"`
	DecodeTokens      int64    `json:"decodeTokens"`
	DecodeWindowMs    *int64   `json:"decodeWindowMs"`
	DecodeTPS         *float64 `json:"decodeTps"`
	DecodeStatus      string   `json:"decodeStatus"`
}

const unseen time.Duration = -1

// Observer belongs to one stream. Its owner calls the methods serially.
type Observer struct {
	now                                      func() time.Time
	start                                    time.Time
	header, reasoning, firstDelta, firstText time.Duration
	firstContent, lastContent, maxGap        time.Duration
	hidden, summary                          bool
	flow                                     []flowMark
	flowFull                                 bool
}

// NewObserver starts timing now. Pass nil to use time.Now.
func NewObserver(now func() time.Time) *Observer {
	if now == nil {
		now = time.Now
	}
	return &Observer{now: now, start: now(), header: unseen, reasoning: unseen,
		firstDelta: unseen, firstText: unseen, firstContent: unseen, lastContent: unseen, maxGap: unseen}
}

func (o *Observer) MarkHeader() {
	if o.header == unseen {
		o.header = o.now().Sub(o.start)
	}
}

// ObserveFrame accepts the JSON data of one SSE event (or Gemini chunk).
// Multiple signals in a frame share one arrival and one gap observation.
func (o *Observer) ObserveFrame(protocol Protocol, eventName string, rawJSON []byte) {
	at := o.now().Sub(o.start)
	f := classify(protocol, eventName, rawJSON)
	o.observe(at, f.kinds, f.bytes)
}

// ObserveSignal observes a preclassified event. Without payload sizes the
// concentration check weights each content event equally.
func (o *Observer) ObserveSignal(kind SignalKind) {
	o.observe(o.now().Sub(o.start), kind, 1)
}

func (o *Observer) observe(at time.Duration, kinds SignalKind, weight int64) {
	if kinds&SignalHiddenReasoning != 0 {
		o.hidden = true
	}
	if kinds&SignalReasoningSummary != 0 {
		o.summary = true
	}
	if kinds&SignalReasoningStart != 0 && o.reasoning == unseen {
		o.reasoning = at
	}
	if kinds&SignalReasoningDelta != 0 {
		if o.firstDelta == unseen {
			o.firstDelta = at
		}
		if at-o.firstDelta >= 100*time.Millisecond && (o.reasoning == unseen || o.firstDelta < o.reasoning) {
			o.reasoning = o.firstDelta
		}
	}
	if kinds&SignalTextDelta != 0 && o.firstText == unseen {
		o.firstText = at
	}
	if kinds&(SignalReasoningDelta|SignalReasoningSummary|SignalTextDelta|SignalToolDelta) == 0 {
		return
	}
	if o.lastContent == unseen {
		o.firstContent = at
	} else {
		o.maxGap = max(o.maxGap, at-o.lastContent)
	}
	o.lastContent = at
	o.markFlow(at.Milliseconds(), max(weight, 1))
}

type flowMark struct{ ms, bytes int64 }

// Keep only cumulative byte counts, coalesced per millisecond. At the bound
// the exact percentiles become unknown, but content timing keeps working.
func (o *Observer) markFlow(ms, weight int64) {
	if o.flowFull {
		return
	}
	if n := len(o.flow); n > 0 {
		last := &o.flow[n-1]
		if last.ms == ms {
			last.bytes += weight
			return
		}
		weight += last.bytes
	}
	if len(o.flow) == 1<<16 {
		o.flow, o.flowFull = nil, true
		return
	}
	o.flow = append(o.flow, flowMark{ms, weight})
}

// Middle 80% of arriving content, scaled back to its whole span (Magpie).
func (o *Observer) pacedSpan(from int64) float64 {
	var base int64
	marks := o.flow
	for len(marks) > 0 && marks[0].ms < from {
		base = marks[0].bytes
		marks = marks[1:]
	}
	if len(marks) == 0 {
		return 0
	}
	total := marks[len(marks)-1].bytes - base
	// The first byte at or beyond each percentile.
	low, high := (total-1)/10+1, total-total/10
	first := int64(-1)
	for _, m := range marks {
		if m.bytes-base < low {
			continue
		}
		if first < 0 {
			first = m.ms
		}
		if m.bytes-base >= high {
			return float64(m.ms-first) * 1.25
		}
	}
	return 0
}

func milliseconds(d time.Duration) *int64 {
	if d < 0 {
		return nil
	}
	ms := d.Milliseconds()
	return &ms
}

// Finalize returns a detached snapshot. DecodeTokens and DecodeWindowMs
// describe the candidate interval even when its speed is rejected as a burst.
func (o *Observer) Finalize(in FinalizeInput) Snapshot {
	s := Snapshot{HeaderMs: milliseconds(o.header), FirstTextMs: milliseconds(o.firstText),
		LastContentMs: milliseconds(o.lastContent), MaxContentGapMs: milliseconds(o.maxGap),
		DecodeStatus: StatusInsufficientEvidence, ReasoningEvidence: "none"}
	switch {
	case o.hidden:
		s.ReasoningEvidence = "hidden"
	case o.reasoning != unseen || o.firstDelta != unseen:
		s.ReasoningEvidence = "visible"
		s.FirstReasoningMs = milliseconds(o.reasoning)
	case o.summary:
		s.ReasoningEvidence = "summaryOnly"
	case in.HasUsage && in.ReasoningTokens > 0:
		s.ReasoningEvidence = "hidden"
	}
	if in.Interrupted {
		s.DecodeStatus = StatusInterrupted
		return s
	}
	if !in.HasUsage || in.OutputTokens <= 0 || in.ReasoningTokens < 0 || in.ReasoningTokens > in.OutputTokens {
		return s
	}
	start, from, tokens := o.firstContent, o.firstContent, in.OutputTokens
	if s.ReasoningEvidence != "none" && s.FirstReasoningMs == nil {
		if o.firstText == unseen || in.ReasoningTokens == 0 {
			s.DecodeStatus = StatusHiddenReasoning
			if s.ReasoningEvidence == "visible" {
				s.DecodeStatus = StatusBufferedBurst
			}
			return s
		}
		start, tokens = o.firstText, tokens-in.ReasoningTokens
	} else if o.reasoning != unseen && (start == unseen || o.reasoning < start) {
		start = o.reasoning
	}
	if in.ReasoningTokens > 0 && o.firstText != unseen {
		from = o.firstText
	}
	if o.lastContent == unseen || start == unseen || tokens == 0 || o.flowFull {
		return s
	}
	end := in.EndedAt
	if end.IsZero() {
		end = o.now()
	}
	endOffset := end.Sub(o.start)
	if endOffset < o.lastContent || endOffset < start {
		return s
	}
	window, tail := endOffset-start, endOffset-from
	s.DecodeTokens, s.DecodeWindowMs = tokens, milliseconds(window)
	// Magpie receive-side limits: >=100 ms, <=10k tok/s, no held burst,
	// no middle 80% in <1/4 of the tail, and no >20x reasoning/answer pace jump.
	answer := in.OutputTokens - in.ReasoningTokens
	held := window < time.Second && start > 4*window
	concentrated := tail <= 0 || o.pacedSpan(from.Milliseconds())*4 < float64(tail)/float64(time.Millisecond)
	paceJump := in.ReasoningTokens > 0 && o.firstText != unseen &&
		float64(answer)*(o.firstText-o.firstContent).Seconds() > 20*float64(in.ReasoningTokens)*tail.Seconds()
	if window < 100*time.Millisecond || float64(tokens) > 10000*window.Seconds() || held || concentrated || paceJump {
		s.DecodeStatus = StatusBufferedBurst
		return s
	}
	tps := float64(tokens) / window.Seconds()
	s.DecodeStatus, s.DecodeTPS = StatusMeasured, &tps
	return s
}
