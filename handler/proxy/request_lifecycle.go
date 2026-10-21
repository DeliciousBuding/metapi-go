package proxyhandler

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/handler/shared"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/deliciousbuding/metapi-go/store"
)

type requestLifecycleKey struct{}
type gatewayAttemptKey struct{}
type gatewayRequestWriter func(context.Context, store.GatewayRequest) error

// requestLifecycle owns one ingress result. Attempts only attach identity and
// timing; they never decide this result. HTTP commits and the protocol owner's
// terminal observation remain separate, including a 200 followed by interruption.
type requestLifecycle struct {
	userBudget         *requestUserBudget
	mu                 sync.Mutex
	once               sync.Once
	entry              store.GatewayRequest
	started            time.Time
	writer             gatewayRequestWriter
	outcome            string
	rejection          string
	attempted          bool
	dispatched         bool
	failureAfterOutput bool
	writeFailed        bool
	hijacked           bool
	gatewayFailure     string
	noRouteReason      string
	attempts           int
	requestContext     context.Context
	knownFailure       bool
	protocolCompleted  bool
	protocolFailed     bool
}

func defaultGatewayRequestWriter(ctx context.Context, entry store.GatewayRequest) error {
	return EnqueueGatewayRequest(ctx, entry)
}

func newRequestLifecycle(r *http.Request, transport string, writer gatewayRequestWriter) (*requestLifecycle, *http.Request) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		// No client-derived fallback: request identities must never merge calls.
		panic("cannot generate gateway request identity")
	}
	now := time.Now()
	state := &requestLifecycle{started: now, writer: writer, requestContext: r.Context(), entry: store.GatewayRequest{
		ID: "req_" + hex.EncodeToString(id[:]), Transport: transport,
		StartedAt:         now.UTC().Format(time.RFC3339Nano),
		DownstreamPath:    canonicalRequestPath(r.URL.Path),
		ExternalRequestID: safeExternalRequestID(r.Header.Get("X-Request-Id")),
	}}
	return state, r.WithContext(context.WithValue(r.Context(), requestLifecycleKey{}, state))
}

func requestLifecycleFromContext(ctx context.Context) *requestLifecycle {
	state, _ := ctx.Value(requestLifecycleKey{}).(*requestLifecycle)
	return state
}

func safeExternalRequestID(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 256 || strings.IndexFunc(value, unicode.IsControl) >= 0 || strings.HasPrefix(strings.ToLower(value), "sk-") || strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return nil
	}
	return &value
}

func canonicalRequestPath(path string) *string {
	path = strings.TrimRight(strings.SplitN(path, "?", 2)[0], "/")
	switch {
	case path == "/responses/compact":
		path = "/v1/responses/compact"
		return &path
	case path == "/v1/models", path == "/v1/files":
		return &path
	case strings.HasPrefix(path, "/v1/files/"):
		path = "/v1/files/{id}" + requestResourceSuffix(path)
		return &path
	case strings.HasPrefix(path, "/v1/videos/"):
		path = "/v1/videos/{id}" + requestResourceSuffix(path)
		return &path
	}
	return proxyLogDownstreamPath(&Ctx{DownstreamPath: path})
}

func requestResourceSuffix(path string) string {
	for _, suffix := range []string{"/content", "/remix"} {
		if strings.HasSuffix(path, suffix) {
			return suffix
		}
	}
	return ""
}

// RequestLifecycle runs before recovery, body limits and authentication, but
// records only known proxy surfaces. Successful WS upgrades have turn owners;
// an unsuccessful upgrade is still an ordinary HTTP rejection.
func RequestLifecycle(next http.Handler) http.Handler {
	return requestLifecycleMiddleware(next, defaultGatewayRequestWriter)
}

func requestLifecycleMiddleware(next http.Handler, writer gatewayRequestWriter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || canonicalRequestPath(r.URL.Path) == nil {
			next.ServeHTTP(w, r)
			return
		}
		state, request := newRequestLifecycle(r, "http", writer)
		wrapped := &requestResponseWriter{ResponseWriter: w, state: state}
		defer state.finish(request.Context())
		next.ServeHTTP(wrapped, request)
		state.mu.Lock()
		implicit := !state.hijacked && state.entry.HTTPStatus == nil
		state.mu.Unlock()
		if implicit {
			wrapped.WriteHeader(http.StatusOK)
		}
	})
}

func (s *requestLifecycle) bind(pac *auth.ProxyAuthContext, path, model string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if pac != nil {
		s.entry.DownstreamAPIKeyID = pac.KeyID
		s.entry.OwnerUserID, s.entry.UserGroupID = pac.OwnerUserID, pac.UserGroupID
	}
	if canonical := canonicalRequestPath(path); canonical != nil {
		s.entry.DownstreamPath = canonical
	}
	if model != "" && len(model) <= 1024 {
		value := model
		s.entry.ModelRequested = &value
	}
}

func (s *requestLifecycle) observe(status string, stream bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcome = status
	if status == shared.OutcomeSuccess && !stream && s.requestContext.Err() == nil && !s.writeFailed {
		s.protocolCompleted = true
	}
	s.knownFailure = status != shared.OutcomeSuccess && s.requestContext.Err() == nil
	if status != shared.OutcomeSuccess && status != shared.OutcomeClientError {
		s.failureAfterOutput = s.entry.OutputStarted && (s.entry.Transport == "websocket" || s.entry.HTTPStatus != nil && *s.entry.HTTPStatus < 400)
	}
}

func (s *requestLifecycle) firstOutput() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.entry.Transport != "websocket" && s.entry.FirstOutputLatencyMs == nil {
		value := time.Since(s.started).Milliseconds()
		s.entry.FirstOutputLatencyMs = &value
	}
}

func (s *requestLifecycle) finish(ctx context.Context) {
	s.once.Do(func() {
		s.mu.Lock()
		if s.hijacked {
			s.mu.Unlock()
			return
		}
		s.entry.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
		s.entry.LatencyMs = time.Since(s.started).Milliseconds()
		s.entry.AttemptCount = s.attempts
		status, reason := s.terminal(ctx)
		s.entry.TerminalStatus, s.entry.ReasonCode = status, &reason
		entry := s.entry
		s.mu.Unlock()
		// A disconnected caller must not cancel its own completion record. The
		// bounded write cannot rewrite the response or start a new retry.
		writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer cancel()
		s.settleUserBudget(writeCtx)
		if err := s.writer(writeCtx, entry); err != nil {
			slog.Warn("gateway request record failed", "gateway_request_id", entry.ID, "err", err)
		}
	})
}

// Called with mu held. A protocol failure already observed wins over a later
// socket close; a mere write failure is not evidence of user-initiated cancel.
func (s *requestLifecycle) terminal(ctx context.Context) (string, string) {
	if s.gatewayFailure != "" {
		if s.failureAfterOutput {
			return "output_interrupted", s.gatewayFailure
		}
		return "gateway_failed", s.gatewayFailure
	}
	if s.failureAfterOutput || s.protocolFailed && s.entry.Transport != "websocket" && s.entry.HTTPStatus != nil && *s.entry.HTTPStatus < 400 {
		return "output_interrupted", "protocol_interrupted"
	}
	if s.protocolCompleted && !s.protocolFailed && !s.writeFailed && s.outcome == shared.OutcomeSuccess {
		return "completed", "normal_completion"
	}
	if errors.Is(ctx.Err(), context.Canceled) && !s.knownFailure && s.rejection == "" {
		return "client_cancelled", "downstream_cancelled"
	}
	if s.writeFailed {
		return "output_interrupted", "downstream_write_failed"
	}
	if s.rejection != "" {
		if s.entry.HTTPStatus != nil && *s.entry.HTTPStatus >= 500 {
			return "gateway_failed", s.rejection
		}
		return "rejected_policy", s.rejection
	}
	if s.protocolFailed && s.entry.Transport == "websocket" {
		return "upstream_failed", "protocol_interrupted"
	}
	if s.outcome == shared.OutcomeSuccess {
		return "completed", "normal_completion"
	}
	if s.outcome == shared.OutcomeUnavailable && !s.attempted {
		if s.noRouteReason != "" {
			return "rejected_no_route", s.noRouteReason
		}
		return "rejected_no_route", "no_available_candidate"
	}
	if s.outcome == shared.OutcomeClientError {
		if s.attempted {
			return "upstream_failed", "upstream_rejected"
		}
		return "rejected_policy", "invalid_request"
	}
	if s.outcome != "" {
		return "upstream_failed", s.outcome
	}
	if s.entry.HTTPStatus != nil {
		status := *s.entry.HTTPStatus
		if status >= 400 && status < 500 {
			return "rejected_policy", "invalid_request"
		}
		if status >= 500 {
			return "gateway_failed", "internal_error"
		}
		if status >= 200 && status < 300 && !s.dispatched {
			return "completed", "local_response"
		}
	}
	return "gateway_failed", "completion_unobserved"
}

type requestResponseWriter struct {
	http.ResponseWriter
	state    *requestLifecycle
	stream   *incrementalSseAnalyzer
	terminal *nativeTerminalBody
}

func (w *requestResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *requestResponseWriter) WriteHeader(status int) {
	if status >= 200 {
		w.state.mu.Lock()
		if w.state.entry.HTTPStatus == nil {
			value := status
			elapsed := time.Since(w.state.started).Milliseconds()
			w.state.entry.HTTPStatus = &value
			w.state.entry.FirstByteLatencyMs = &elapsed
		}
		w.state.mu.Unlock()
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *requestResponseWriter) Write(body []byte) (int, error) {
	w.state.mu.Lock()
	committed := w.state.entry.HTTPStatus != nil
	w.state.mu.Unlock()
	if !committed {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(body)
	if err == nil && n != len(body) {
		err = io.ErrShortWrite
	}
	w.state.mu.Lock()
	w.state.entry.OutputStarted = w.state.entry.OutputStarted || n > 0
	w.state.writeFailed = w.state.writeFailed || err != nil
	w.state.mu.Unlock()
	if err == nil {
		// Observe only bytes accepted by the downstream writer, not upstream
		// bytes read ahead of a failed client write.
		if w.stream == nil && (strings.Contains(w.Header().Get("Content-Type"), "text/event-stream") || strings.HasPrefix(string(body), "data:") || strings.HasPrefix(string(body), "event:")) {
			w.stream = newIncrementalSseAnalyzer()
			w.terminal = &nativeTerminalBody{}
			if w.state.entry.DownstreamPath != nil {
				w.terminal.completionProtocol, _ = proxy.EndpointFromPath(*w.state.entry.DownstreamPath)
			}
			w.stream.onEvent = func(event SseEvent) {
				w.terminal.observeTerminalEvent(&event)
				w.state.mu.Lock()
				w.state.protocolCompleted = w.terminal.terminalSeen && !w.terminal.terminalFailed
				w.state.protocolFailed = w.state.protocolFailed || w.terminal.terminalFailed || IsSseErrorEvent(event)
				w.state.mu.Unlock()
			}
		}
		if w.stream != nil {
			w.stream.Push(body[:n])
		}
	}
	return n, err
}
func (w *requestResponseWriter) Flush() {
	_ = w.FlushError()
}
func (w *requestResponseWriter) FlushError() error {
	w.state.mu.Lock()
	committed := w.state.entry.HTTPStatus != nil
	w.state.mu.Unlock()
	if !committed {
		w.WriteHeader(http.StatusOK)
	}
	err := http.NewResponseController(w.ResponseWriter).Flush()
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		w.state.mu.Lock()
		w.state.writeFailed = true
		w.state.mu.Unlock()
	}
	return err
}
func (w *requestResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	conn, rw, err := http.NewResponseController(w.ResponseWriter).Hijack()
	if err == nil {
		w.state.mu.Lock()
		w.state.hijacked = true
		w.state.mu.Unlock()
	}
	return conn, rw, err
}
func (w *requestResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	return io.Copy(struct{ io.Writer }{w}, r)
}

// These narrow writer capabilities let auth report safe rejection metadata
// without importing handlers or exposing credential values.
func (w *requestResponseWriter) ObserveProxyKey(id int64) {
	w.state.mu.Lock()
	w.state.entry.DownstreamAPIKeyID = &id
	w.state.mu.Unlock()
}
func (w *requestResponseWriter) ObserveProxyUser(userID, groupID *int64) {
	w.state.mu.Lock()
	w.state.entry.OwnerUserID, w.state.entry.UserGroupID = userID, groupID
	w.state.mu.Unlock()
}
func (w *requestResponseWriter) ObserveProxyCredential(token string) {
	w.state.mu.Lock()
	defer w.state.mu.Unlock()
	if w.state.entry.ExternalRequestID != nil && *w.state.entry.ExternalRequestID == token {
		w.state.entry.ExternalRequestID = nil
	}
}
func (w *requestResponseWriter) ObserveProxyRejection(code string) {
	w.state.mu.Lock()
	w.state.rejection = code
	w.state.mu.Unlock()
}
func (w *requestResponseWriter) ObserveProxyPanic() {
	w.state.mu.Lock()
	w.state.gatewayFailure = "handler_panic"
	w.state.failureAfterOutput = w.state.entry.OutputStarted
	w.state.mu.Unlock()
}

func beginGatewayAttempt(ctx context.Context) context.Context {
	s := requestLifecycleFromContext(ctx)
	if s == nil {
		return ctx
	}
	s.mu.Lock()
	ordinal := s.attempts
	s.attempts++
	s.attempted = true
	s.mu.Unlock()
	return context.WithValue(ctx, gatewayAttemptKey{}, ordinal)
}

func (s *requestLifecycle) recordAttempt(ctx context.Context, entry *proxy.ProxyLogEntry) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entry.GatewayRequestID = s.entry.ID
	ordinal, hasOrdinal := ctx.Value(gatewayAttemptKey{}).(int)
	if !hasOrdinal {
		ordinal = s.attempts
	}
	if budget := s.userBudget; budget != nil && !budget.seenAttempts[ordinal] {
		if budget.seenAttempts == nil {
			budget.seenAttempts = map[int]bool{}
		}
		budget.seenAttempts[ordinal] = true
		if entry.TotalTokens != nil && *entry.TotalTokens >= 0 && *entry.TotalTokens <= math.MaxInt64-budget.tokens {
			budget.tokens += *entry.TotalTokens
			budget.usageFound = true
		} else {
			budget.usageUnknown = true
		}
	}
	if ordinal, ok := ctx.Value(gatewayAttemptKey{}).(int); ok {
		entry.RetryCount = ordinal
		return
	}
	// Non-dispatch surfaces retain their existing log-driven observation.
	if s.entry.Transport == "websocket" {
		entry.RetryCount = s.attempts
	}
	s.attempts++
	s.attempted = true
}
