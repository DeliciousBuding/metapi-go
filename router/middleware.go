package router

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"runtime/debug"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/proxy"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// CORS returns a CORS middleware handler configured for Metapi.
// It is intentionally permissive for public health and proxy endpoints.
func CORS() func(http.Handler) http.Handler {
	return cors.Handler(corsOptions([]string{"*"}))
}

// AdminCORS returns a CORS middleware for admin API routes. By default it does
// not allow cross-origin browser access; operators can opt in with
// ADMIN_CORS_ALLOWED_ORIGINS for separately hosted admin frontends.
func AdminCORS(cfg *config.Config) func(http.Handler) http.Handler {
	if cfg == nil || len(cfg.AdminCorsAllowedOrigins) == 0 {
		return func(next http.Handler) http.Handler {
			return next
		}
	}
	return cors.Handler(corsOptions(cfg.AdminCorsAllowedOrigins))
}

func corsOptions(allowedOrigins []string) cors.Options {
	return cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		ExposedHeaders:   []string{"Link", "X-Request-Id"},
		AllowCredentials: false,
		MaxAge:           300,
	}
}

// ProxyWriteDeadline re-arms the connection write deadline for the proxy
// surface (/v1 plus the non-/v1 proxy aliases).
//
// net/http arms that deadline from Server.WriteTimeout (60s, see
// app.newHTTPServer) immediately after the request header is read, so it also
// covers the time the handler spends waiting for the upstream. That inverted
// against the executor's whole-request ceiling (90s, or 2x the configured
// first-byte window): a buffered response arriving between 61s and 90s was
// killed while being written back to the client. proxy.WriteBudget is derived
// from the same SSOT as the executor ceiling, so the write side can no longer
// be shorter than the request side, while admin routes keep the strict 60s.
//
// SSE relays still clear the deadline outright (handler/proxy
// relayUpstreamStream): a healthy stream is bounded per chunk by
// PROXY_STREAM_IDLE_TIMEOUT_SEC rather than by total elapsed time.
func ProxyWriteDeadline(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(proxyWriteBudget()))
		next.ServeHTTP(w, r)
	})
}

// proxyWriteBudget reads the first-byte window from the published runtime
// settings. RuntimeSafe (not Runtime) because the middleware can be exercised
// by tests that never publish config.
func proxyWriteBudget() time.Duration {
	firstByteSec := 0
	if rt := config.RuntimeSafe(); rt != nil {
		firstByteSec = rt.ProxyFirstByteTimeoutSec
	}
	return proxy.WriteBudget(firstByteSec)
}

// RequestLogger logs every incoming request using slog after it completes,
// including status, duration, and bytes written for latency/error triage.
// Includes request_id for log correlation when RequestID middleware is active.
// Equivalent to TS Fastify `logger: true`, extended with outcome fields.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := middleware.GetReqID(r.Context())
		sw := &statusRecorder{
			WrapResponseWriter: middleware.NewWrapResponseWriter(w, r.ProtoMajor),
		}
		next.ServeHTTP(sw, r)

		status := sw.Status()
		if status == 0 {
			status = http.StatusOK
		}
		slog.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
			"request_id", reqID,
			"status", status,
			"bytes", sw.BytesWritten(),
			"duration_ms", float64(time.Since(start).Microseconds())/1000.0,
		)
	})
}

// statusRecorder wraps a chi WrapResponseWriter and forwards the optional
// http.ResponseWriter interfaces that chi's interface does not expose. The
// proxy SSE path opts out of the server-level WriteTimeout via SetWriteDeadline
// (handler/proxy/upstream.go), SSE needs Flush, and WebSocket upgrades need
// Hijack — dropping any of these would silently break streaming.
type statusRecorder struct {
	middleware.WrapResponseWriter
}

func (s *statusRecorder) Flush() {
	if f, ok := s.WrapResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := s.WrapResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("statusRecorder: underlying writer does not support hijacking")
}

func (s *statusRecorder) ReadFrom(r io.Reader) (int64, error) {
	if rf, ok := s.WrapResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	return io.Copy(s.WrapResponseWriter, r)
}

func (s *statusRecorder) SetWriteDeadline(t time.Time) error {
	if rc, ok := s.Unwrap().(interface{ SetWriteDeadline(time.Time) error }); ok {
		return rc.SetWriteDeadline(t)
	}
	return nil
}

// TrustedRealIP reads X-Forwarded-For / X-Real-IP only from explicitly
// configured proxy CIDRs, and resolves the client by walking the forwarded
// chain from the right, skipping the hops that belong to those CIDRs. Direct
// clients cannot spoof rate-limit or admin allowlist identity by sending
// forwarded headers themselves.
func TrustedRealIP(cfg *config.Config) func(http.Handler) http.Handler {
	var prefixes []netip.Prefix
	if cfg != nil {
		prefixes = parseTrustedProxyPrefixes(cfg.TrustedProxyCidrs)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(prefixes) > 0 && isTrustedProxyPeer(r.RemoteAddr, prefixes) {
				if ip := forwardedClientIP(r, prefixes); ip != "" {
					r.RemoteAddr = ip
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func parseTrustedProxyPrefixes(raw []string) []netip.Prefix {
	prefixes := make([]netip.Prefix, 0, len(raw))
	for _, item := range raw {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(item))
		if err == nil {
			prefixes = append(prefixes, prefix)
		}
	}
	return prefixes
}

func isTrustedProxyPeer(remoteAddr string, prefixes []netip.Prefix) bool {
	addr, ok := remoteAddrIP(remoteAddr)
	if !ok {
		return false
	}
	return addrInPrefixes(addr, prefixes)
}

// remoteAddrIP extracts the peer address from http.Request.RemoteAddr, which
// carries "host:port" for IPv4 and "[host]:port" for IPv6.
func remoteAddrIP(remoteAddr string) (netip.Addr, bool) {
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func addrInPrefixes(addr netip.Addr, prefixes []netip.Prefix) bool {
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

// forwardedClientIP resolves the client address out of the forwarded headers of
// a request that already passed the trusted-peer gate.
//
// Reverse proxies append to X-Forwarded-For (nginx: proxy_set_header
// X-Forwarded-For $proxy_add_x_forwarded_for), so the chain reads
// [values the client injected..., real client, proxy hops...] and its left-most
// entry is attacker controlled input. It is therefore walked from the right the
// way nginx real_ip_recursive and RFC 7239 consumers do: entries inside
// TRUSTED_PROXY_CIDRS are hops and get skipped, and the first entry outside them
// is the client.
func forwardedClientIP(r *http.Request, prefixes []netip.Prefix) string {
	chain := forwardedAddrChain(r.Header.Values("X-Forwarded-For"))
	if len(chain) == 0 {
		// X-Forwarded-For carried no address at all: keep the single-value
		// fallback, still behind the trusted-peer gate.
		return normalizeForwardedIP(r.Header.Get("X-Real-IP"))
	}
	// The direct peer terminates the chain; the caller already proved it is a
	// trusted proxy, so it is one more hop to skip rather than the client.
	if peer, ok := remoteAddrIP(r.RemoteAddr); ok {
		chain = append(chain, peer)
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if !addrInPrefixes(chain[i], prefixes) {
			return chain[i].String()
		}
	}
	// Every hop is trusted, e.g. internal proxies in front of an internal client.
	// Return the left-most entry instead of nothing so per-IP rate limits keep a
	// stable key rather than collapsing every such caller into one bucket.
	return chain[0].String()
}

// forwardedAddrChain expands every X-Forwarded-For header into the hop list it
// describes, keeping wire order: proxies append, so header order followed by
// in-header order runs from the original client to the closest proxy. Entries
// that are not addresses are dropped.
func forwardedAddrChain(values []string) []netip.Addr {
	chain := make([]netip.Addr, 0, len(values)+1)
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if addr := parseForwardedAddr(part); addr.IsValid() {
				chain = append(chain, addr)
			}
		}
	}
	return chain
}

func normalizeForwardedIP(raw string) string {
	if addr := parseForwardedAddr(raw); addr.IsValid() {
		return addr.String()
	}
	return ""
}

// parseForwardedAddr parses one forwarded-header entry, tolerating the bracketed
// IPv6 form some proxies emit, and unmaps IPv4-mapped addresses so chain entries
// match TRUSTED_PROXY_CIDRS exactly the way the peer does.
func parseForwardedAddr(raw string) netip.Addr {
	raw = strings.Trim(strings.TrimSpace(raw), "[]")
	if raw == "" {
		return netip.Addr{}
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}
	}
	return addr.Unmap()
}

// Recoverer catches panics, logs the panic + stack via slog (with request_id
// for correlation), and returns 500. Equivalent to chi middleware.Recoverer but
// keeps panic diagnostics in the structured log instead of stderr.
func Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rv := recover(); rv != nil {
				if rv == http.ErrAbortHandler {
					panic(rv)
				}
				slog.Error("panic recovered",
					"panic", rv,
					"method", r.Method,
					"path", r.URL.Path,
					"request_id", middleware.GetReqID(r.Context()),
					"stack", string(debug.Stack()),
				)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// BodyLimit enforces a maximum request body size using http.MaxBytesReader.
// Returns a middleware that wraps the request body so that reads beyond the
// limit return an error, causing the handler to receive a closed body.
func BodyLimit(limitBytes int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limitBytes > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, int64(limitBytes))
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isFileUploadPath reports whether the given request path targets a multipart
// upload route that should use the higher file-upload body limit instead of
// the general RequestBodyLimit. Covers /v1/files (POST upload) and
// /v1/images/* (POST edits/variations may carry multipart image payloads).
func isFileUploadPath(path string) bool {
	if path == "/v1/files" {
		return true
	}
	if strings.HasPrefix(path, "/v1/images/") {
		return true
	}
	return false
}

// writeBodyLimitError emits a 413 Request Entity Too Large response with a
// JSON error body, so clients receive a clean rejection instead of a silently
// truncated body. The body limit is mounted globally, so the shape follows
// the surface: /api and /auth keep the flat {"error": "..."} admin contract,
// everything else (the /v1 proxy surface and its aliases) gets the
// OpenAI-shaped envelope the proxy auth middleware and handlers emit.
func writeBodyLimitError(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	if strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/auth") {
		_, _ = w.Write([]byte(`{"error":"Request body too large"}`))
		return
	}
	_, _ = w.Write([]byte(`{"error":{"message":"Request body too large","type":"invalid_request_error"}}`))
}

// BodyLimitPathAware enforces a maximum request body size, selecting between a
// general limit and a higher file-upload limit based on the request path.
// Multipart upload routes (/v1/files, /v1/images/*) use the higher
// fileUploadLimitBytes; all other paths use defaultLimitBytes.
//
// A Content-Length that already exceeds the selected limit is rejected early
// with a 413 JSON response before the handler runs, so the client gets a
// clean error instead of a truncated read. For chunked requests (no
// Content-Length) http.MaxBytesReader still caps the body as a safety net.
func BodyLimitPathAware(defaultLimitBytes, fileUploadLimitBytes int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := defaultLimitBytes
			if isFileUploadPath(r.URL.Path) && fileUploadLimitBytes > 0 {
				limit = fileUploadLimitBytes
			}
			if limit > 0 {
				if r.ContentLength > int64(limit) {
					writeBodyLimitError(w, r)
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, int64(limit))
			}
			next.ServeHTTP(w, r)
		})
	}
}
