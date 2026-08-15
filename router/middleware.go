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
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// CORS returns a CORS middleware handler configured for MetAPI.
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
// configured proxy CIDRs. Direct clients cannot spoof rate-limit or admin
// allowlist identity by sending forwarded headers themselves.
func TrustedRealIP(cfg *config.Config) func(http.Handler) http.Handler {
	var prefixes []netip.Prefix
	if cfg != nil {
		prefixes = parseTrustedProxyPrefixes(cfg.TrustedProxyCidrs)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(prefixes) > 0 && isTrustedProxyPeer(r.RemoteAddr, prefixes) {
				if ip := forwardedClientIP(r); ip != "" {
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
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, prefix := range prefixes {
		if prefix.Contains(addr) {
			return true
		}
	}
	return false
}

func forwardedClientIP(r *http.Request) string {
	for _, xff := range r.Header.Values("X-Forwarded-For") {
		part, _, _ := strings.Cut(xff, ",")
		if ip := normalizeForwardedIP(part); ip != "" {
			return ip
		}
	}
	return normalizeForwardedIP(r.Header.Get("X-Real-IP"))
}

func normalizeForwardedIP(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	addr, err := netip.ParseAddr(raw)
	if err != nil {
		return ""
	}
	return addr.Unmap().String()
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
// truncated body.
func writeBodyLimitError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusRequestEntityTooLarge)
	_, _ = w.Write([]byte(`{"error":"Request body too large"}`))
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
					writeBodyLimitError(w)
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, int64(limit))
			}
			next.ServeHTTP(w, r)
		})
	}
}
