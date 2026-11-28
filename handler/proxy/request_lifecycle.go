package proxyhandler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/store"
)

type proxyScopeKey struct{}

// The database belongs to the router even when this path has no request record.
// Each WebSocket turn inherits that database and starts its own lifecycle.
type proxyRequestScope struct {
	db        *store.DB
	lifecycle *requestLifecycle
}

func proxyScopeFromContext(ctx context.Context) proxyRequestScope {
	scope, _ := ctx.Value(proxyScopeKey{}).(proxyRequestScope)
	return scope
}

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
	firstFailure       string
	attempts           int
	requestContext     context.Context
	knownFailure       bool
	protocolCompleted  bool
	protocolFailed     bool
}

func defaultGatewayRequestWriter(db *store.DB) gatewayRequestWriter {
	return func(ctx context.Context, entry store.GatewayRequest) error {
		return EnqueueGatewayRequest(ctx, db, entry)
	}
}

func newRequestLifecycle(db *store.DB, r *http.Request, transport string, writer gatewayRequestWriter) (*requestLifecycle, *http.Request) {
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
	return state, r.WithContext(context.WithValue(r.Context(), proxyScopeKey{}, proxyRequestScope{db: db, lifecycle: state}))
}

func requestLifecycleFromContext(ctx context.Context) *requestLifecycle {
	return proxyScopeFromContext(ctx).lifecycle
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
	case path == "/v1/models", path == "/v1beta/models", path == "/v1/files":
		return &path
	case strings.HasPrefix(path, "/gemini/") && strings.HasSuffix(path, "/models"):
		version := strings.TrimSuffix(strings.TrimPrefix(path, "/gemini/"), "/models")
		if version != "" && !strings.Contains(version, "/") {
			normalized := "/gemini/{version}/models"
			return &normalized
		}
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
func RequestLifecycle(db *store.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return requestLifecycleMiddleware(db, next, defaultGatewayRequestWriter(db))
	}
}

func requestLifecycleMiddleware(db *store.DB, next http.Handler, writer gatewayRequestWriter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions || canonicalRequestPath(r.URL.Path) == nil {
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), proxyScopeKey{}, proxyRequestScope{db: db})))
			return
		}
		state, request := newRequestLifecycle(db, r, "http", writer)
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
