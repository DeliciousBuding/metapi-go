package proxyhandler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
	"github.com/deliciousbuding/metapi-go/platform"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

// One request-local operation, not a durable task or another execution pool.
// Only Read advances it. Close/idle cancellation can close the current network
// body concurrently, so that handle alone is published atomically.
type directImageTaskSession struct {
	op             directImageTaskOperation
	ctx            context.Context
	cancel         context.CancelFunc
	original       *http.Request
	response       *http.Response
	active         atomic.Pointer[imageTaskActiveBody]
	firstByteMs    int64
	usage          ParsedUsage
	taskID         string
	polls          int
	totalBytes     int
	previous       []byte
	needQuery      bool
	done           bool
	frames         [][]byte
	totalOutput    int64
	partialCount   int
	partialDigests [3][32]byte
}

type imageTaskActiveBody struct {
	io.ReadCloser
	once sync.Once
	err  error
}

func (b *imageTaskActiveBody) Close() error {
	b.once.Do(func() { b.err = b.ReadCloser.Close() })
	return b.err
}

type imageTaskPartial struct {
	Index *int   `json:"index"`
	Body  string `json:"body"`
}

func (op directImageTaskOperation) begin(original *http.Request) (*directImageTaskSession, error) {
	ctx, cancel := context.WithTimeout(original.Context(), proxy.RequestCeiling(int((op.budgetMs+999)/1000)))
	session := &directImageTaskSession{op: op, ctx: ctx, cancel: cancel, original: original, usage: ParsedUsage{Source: usageSourceUnknown}}
	started := time.Now()
	submission := original.Clone(platform.WithoutRequestRedirects(ctx))
	submission.GetBody = nil
	if op.streamOptions != nil {
		submission.Header.Set("Accept", "application/json")
	}
	var err error
	session.response, err = sendUpstreamRequest(op.cfg, submission, op.proxyConfig, op.budgetMs, false)
	session.firstByteMs = time.Since(started).Milliseconds()
	if err == nil {
		err = session.adoptResponse(session.response)
	}
	if err != nil {
		session.publishUsage()
		_ = session.Close()
		return nil, &directMediaTransportError{cause: err, usage: session.usage}
	}
	return session, nil
}

func (s *directImageTaskSession) publishUsage() {
	snapshot := s.usage
	s.op.wire.DriverImageUsage = &snapshot
}

func (s *directImageTaskSession) Close() error {
	s.cancel()
	if active := s.active.Swap(nil); active != nil {
		return active.Close()
	}
	return nil
}

// Publish as soon as headers arrive, not on the later first Read. Recheck
// cancellation after publishing: Close may have run before this Store.
func (s *directImageTaskSession) adoptResponse(response *http.Response) error {
	active := &imageTaskActiveBody{ReadCloser: response.Body}
	response.Body = active
	s.response = response
	s.active.Store(active)
	if err := s.ctx.Err(); err != nil {
		s.active.CompareAndSwap(active, nil)
		_ = active.Close()
		return err
	}
	return nil
}

// Each step consumes exactly one raw response. The first is the billable POST;
// subsequent reads use the same saved GET template and current authorization.
func (s *directImageTaskSession) advance() (*http.Response, error) {
	defer s.publishUsage()
	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	if s.needQuery {
		if err := s.query(); err != nil {
			return nil, err
		}
	}
	active := s.active.Load()
	if err := s.ctx.Err(); err != nil {
		_ = s.Close()
		return nil, err
	}
	body, err := readDirectMediaResponse(s.response)
	s.active.CompareAndSwap(active, nil)
	if err != nil {
		return nil, err
	}
	normalized := proxy.NormalizeUpstreamBufferedBody(s.response.Header, body)
	s.totalBytes += len(normalized.Bytes)
	if !normalized.Readable || s.totalBytes > maxImageTaskResponseBytes {
		return nil, channelplugin.ErrResult
	}
	// The published snapshot is for the downstream relay. It must never be
	// mistaken for the next supplier response's fresh observation.
	rawWire := *s.op.wire
	rawWire.DriverImageUsage = nil
	observed, failed, err := observeDirectDriverBody(&rawWire, normalized.Bytes, false)
	if err != nil || s.taskID != "" && observed.upstreamTaskID != "" && s.taskID != observed.upstreamTaskID {
		return nil, channelplugin.ErrResult
	}
	s.usage = mergeUsagePreferLater(s.usage, observed)
	if s.taskID == "" && observed.upstreamTaskID != "" {
		s.taskID = observed.upstreamTaskID
	}
	if s.response.StatusCode < 200 || s.response.StatusCode >= 300 {
		if s.response.StatusCode >= 300 && s.response.StatusCode < 400 {
			return nil, channelplugin.ErrResult
		}
		if s.op.streamOptions != nil && s.needQuery {
			// Headers were already sent for a successful submit. Preserve an
			// actual JSON query error as the standard SSE error event, rather
			// than turning it into a completion or leaking it into logs.
			var object map[string]json.RawMessage
			var compact bytes.Buffer
			if json.Unmarshal(normalized.Bytes, &object) != nil || object == nil || json.Compact(&compact, normalized.Bytes) != nil {
				return nil, channelplugin.ErrResult
			}
			if err = s.enqueue([]byte("event: error\ndata: " + compact.String() + "\n\n")); err != nil {
				return nil, err
			}
			s.done = true
			return nil, nil
		}
		return replayDirectMediaResponse(s.response, normalized.Bytes, true), nil
	}
	if failed || nativeDriverError(normalized.Bytes) {
		return nil, channelplugin.ErrResult
	}
	stream := s.op.streamOptions != nil
	raw, err := s.op.wire.Driver.Call(s.ctx, "taskResult", channelplugin.DriverInput{Body: string(normalized.Bytes), Model: s.op.wire.PluginModel, TaskID: s.taskID, StatusCode: s.response.StatusCode, Stream: stream})
	if err != nil {
		return nil, err
	}
	var decision struct {
		Status   string             `json:"status"`
		Body     string             `json:"body,omitempty"`
		Partials []imageTaskPartial `json:"partials,omitempty"`
	}
	if requestoverride.Decode(raw, &decision) != nil || len(decision.Partials) > 3 || !stream && len(decision.Partials) > 0 {
		return nil, channelplugin.ErrResult
	}
	if decision.Status != "completed" && (decision.Status != "pending" || decision.Body != "" || s.taskID == "") {
		return nil, channelplugin.ErrResult
	}
	if err = s.partials(decision.Partials); err != nil {
		return nil, err
	}
	if decision.Status == "completed" {
		output, err := s.op.complete(s.ctx, s.response, []byte(decision.Body), s.usage)
		if err != nil {
			return nil, err
		}
		if stream {
			frames, err := s.op.streamOptions.completed(output)
			if err != nil {
				return nil, err
			}
			for _, frame := range frames {
				if err = s.enqueue(frame); err != nil {
					return nil, err
				}
			}
		}
		s.done = true
		result := replayDirectMediaResponse(s.response, output, true)
		result.StatusCode, result.Status = http.StatusOK, "200 OK"
		return result, nil
	}
	s.previous = normalized.Bytes
	s.needQuery = true
	// A valid pending response is activity, not generated output or usage.
	// SSE comments keep the existing idle guard alive during healthy polling.
	if stream && len(s.frames) == 0 {
		if err = s.enqueue([]byte(": image task pending\n\n")); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (s *directImageTaskSession) partials(partials []imageTaskPartial) error {
	if len(partials) == 0 {
		return nil
	}
	for _, partial := range partials {
		if s.op.streamOptions == nil || partial.Index == nil || *partial.Index < 0 || *partial.Index >= s.op.streamOptions.partialLimit || *partial.Index > s.partialCount {
			return channelplugin.ErrResult
		}
		index := *partial.Index
		digest := sha256.Sum256([]byte(partial.Body))
		if index < s.partialCount {
			if s.partialDigests[index] != digest {
				return channelplugin.ErrResult
			}
			continue
		}
		frame, err := s.op.streamOptions.partial(index, []byte(partial.Body))
		if err != nil {
			return err
		}
		if err = s.enqueue(frame); err != nil {
			return err
		}
		s.partialDigests[index] = digest
		s.partialCount++
	}
	return nil
}

func (s *directImageTaskSession) enqueue(frame []byte) error {
	if int64(len(frame)) > streamResponseByteLimit()-s.totalOutput {
		return errMessagesChatStreamLimit
	}
	s.totalOutput += int64(len(frame))
	s.frames = append(s.frames, frame)
	return nil
}

func (s *directImageTaskSession) query() error {
	if s.polls > 0 {
		timer := time.NewTimer(s.op.pollInterval)
		defer timer.Stop()
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		case <-timer.C:
		}
	}
	address, err := contract.ImageQueryURL(s.op.wire.DriverImageQuery.URL, s.taskID)
	if err != nil {
		return channelplugin.ErrResult
	}
	raw, err := s.op.wire.Driver.Call(s.ctx, "query", channelplugin.DriverInput{Body: string(s.previous), Model: s.op.wire.PluginModel, TaskID: s.taskID, Stream: s.op.streamOptions != nil})
	if err != nil {
		return err
	}
	var descriptor channelplugin.RequestDescriptor
	if requestoverride.Decode(raw, &descriptor) != nil || descriptor.Method != http.MethodGet || descriptor.Body != "" || descriptor.Parts != nil || descriptor.ContentType != "" && descriptor.ContentType != "application/json" || validateDriverOperationURL(descriptor, address) != nil {
		return channelplugin.ErrResult
	}
	token, err := s.op.beforeQuery(s.ctx)
	if err != nil {
		return err
	}
	query, err := http.NewRequestWithContext(platform.WithoutRequestRedirects(s.ctx), http.MethodGet, address, nil)
	if err != nil {
		return channelplugin.ErrResult
	}
	if s.op.wire.DriverImageQuery.Auth != store.DirectAuthNone {
		query.Header = s.original.Header.Clone()
	}
	for _, name := range []string{"Content-Type", "Content-Length", "Range", "If-Range", "X-ModelScope-Async-Mode"} {
		query.Header.Del(name)
	}
	for name, value := range descriptor.Headers {
		query.Header.Set(name, value)
	}
	s.op.wire.Driver.ClearAuthentication(query.Header)
	query.Header.Set("Accept", "application/json")
	if s.op.wire.DriverImageQuery.Auth != store.DirectAuthNone {
		switch s.op.wire.DriverImageQuery.Auth {
		case store.DirectAuthAPIKey:
			query.Header.Set("X-Api-Key", token)
		case store.DirectAuthGoogle:
			query.Header.Set("X-Goog-Api-Key", token)
		default:
			query.Header.Set("Authorization", "Bearer "+token)
		}
		if err = s.op.wire.Driver.ApplyAuthentication(query.Header, token); err != nil {
			return err
		}
	}
	queryProxy := s.op.proxyConfig
	if s.op.wire.DriverImageQuery.Auth == store.DirectAuthNone && queryProxy != nil {
		clean := *queryProxy
		clean.CustomHeaders, clean.ClearanceCookie = nil, ""
		queryProxy = &clean
	}
	s.response, err = sendUpstreamRequest(s.op.cfg, query, queryProxy, s.op.budgetMs, false)
	if err != nil {
		return err
	}
	if err = s.adoptResponse(s.response); err != nil {
		return err
	}
	s.polls++
	return nil
}

type directImageTaskBody struct {
	session *directImageTaskSession
	pending []byte
}

func (b *directImageTaskBody) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if err := b.session.ctx.Err(); err != nil {
		_ = b.Close()
		b.pending, b.session.frames = nil, nil
		return 0, &directMediaTransportError{cause: err, usage: b.session.usage}
	}
	for len(b.pending) == 0 {
		if err := b.session.ctx.Err(); err != nil {
			_ = b.Close()
			b.session.frames = nil
			return 0, &directMediaTransportError{cause: err, usage: b.session.usage}
		}
		if len(b.session.frames) > 0 {
			b.pending = b.session.frames[0]
			b.session.frames = b.session.frames[1:]
			break
		}
		if b.session.done {
			_ = b.Close()
			return 0, io.EOF
		}
		response, err := b.session.advance()
		if response != nil {
			_ = response.Body.Close()
			if response.StatusCode != http.StatusOK {
				err = channelplugin.ErrResult
			}
		}
		if err != nil {
			_ = b.Close()
			b.session.frames = nil
			return 0, &directMediaTransportError{cause: err, usage: b.session.usage}
		}
	}
	count := copy(buffer, b.pending)
	b.pending = b.pending[count:]
	return count, nil
}

func (b *directImageTaskBody) Close() error { return b.session.Close() }
