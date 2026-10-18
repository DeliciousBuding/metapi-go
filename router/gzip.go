// gzip.go — gzip response compression for the embedded SPA static assets.
//
// The SPA static asset pipeline used to serve every JS/CSS/HTML/SVG payload
// uncompressed. This middleware compresses compressible text responses when
// the client advertises gzip support.
//
// The compression decision is made lazily in WriteHeader based on the
// Content-Type the inner handler set (http.FileServer / ServeContent set it
// before writing), so already-compressed binary formats (png/jpg/webp/woff2…)
// pass through untouched and no response buffering is needed. The middleware
// only touches Content-Encoding / Vary / Content-Length — headers set earlier
// in the chain (e.g. the SecurityHeaders CSP) are left intact.
//
// Scope: applied only to the static asset handlers in setupSPAFallback. API
// routes keep their own (often streaming) response semantics.

package router

import (
	"compress/gzip"
	"mime"
	"net/http"
	"strconv"
	"strings"
)

// compressibleContentTypes lists the MIME types worth compressing for SPA
// assets. Binary formats that are already compressed are deliberately absent.
var compressibleContentTypes = map[string]bool{
	"text/html":                 true,
	"text/css":                  true,
	"text/plain":                true,
	"text/javascript":           true,
	"text/xml":                  true,
	"application/javascript":    true,
	"application/json":          true,
	"application/xml":           true,
	"application/manifest+json": true,
	"image/svg+xml":             true,
}

// isCompressibleContentType reports whether a response Content-Type (possibly
// carrying charset params) should be gzip-compressed. Structured syntax
// suffixes (+json / +xml) are matched beyond the explicit allowlist.
func isCompressibleContentType(contentType string) bool {
	if contentType == "" {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0])
	}
	if compressibleContentTypes[mediaType] {
		return true
	}
	_, suffix, ok := strings.Cut(mediaType, "+")
	return ok && (suffix == "json" || suffix == "xml")
}

// acceptsGzip reports whether the client accepts gzip with a non-zero
// q-value. The header is parsed per RFC 9110 instead of substring-matched:
// "gzip;q=0" is an explicit refusal and must not be compressed (audit L29).
// An explicit gzip/x-gzip entry overrides a wildcard; "*" alone accepts gzip
// when its q-value is positive.
func acceptsGzip(r *http.Request) bool {
	header := r.Header.Get("Accept-Encoding")
	if header == "" {
		return false
	}
	explicit := false
	accepted := false
	wildcard := -1.0
	for _, part := range strings.Split(header, ",") {
		coding, q := parseAcceptEncodingPart(part)
		switch coding {
		case "gzip", "x-gzip":
			explicit = true
			accepted = q > 0
		case "*":
			wildcard = q
		}
	}
	if explicit {
		return accepted
	}
	return wildcard > 0
}

// parseAcceptEncodingPart splits one Accept-Encoding list member into its
// lowercase coding name and q-value (default 1). Malformed parameters are
// ignored; an unparseable q-value falls back to the default.
func parseAcceptEncodingPart(part string) (string, float64) {
	fields := strings.Split(part, ";")
	coding := strings.ToLower(strings.TrimSpace(fields[0]))
	q := 1.0
	for _, param := range fields[1:] {
		key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
		if !ok || !strings.EqualFold(strings.TrimSpace(key), "q") {
			continue
		}
		if parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
			q = parsed
		}
	}
	if q < 0 {
		q = 0
	}
	return coding, q
}

// gzipResponseWriter wraps a ResponseWriter and transparently compresses the
// body when the final Content-Type is compressible. The decision runs in
// WriteHeader so the handler can set Content-Type / Content-Length first and
// no buffering of the response body is required.
type gzipResponseWriter struct {
	http.ResponseWriter
	gzipWriter *gzip.Writer
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	if w.gzipWriter == nil &&
		status == http.StatusOK &&
		w.Header().Get("Content-Encoding") == "" &&
		isCompressibleContentType(w.Header().Get("Content-Type")) {
		// The body will be compressed, so the uncompressed Content-Length
		// (set by ServeContent) no longer applies — the chunked transfer
		// fallback takes over. Content-Encoding + Vary keep the response
		// correct for caches that key on Accept-Encoding.
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		w.Header().Del("Content-Length")
		w.gzipWriter = gzip.NewWriter(w.ResponseWriter)
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(p []byte) (int, error) {
	if w.gzipWriter != nil {
		return w.gzipWriter.Write(p)
	}
	// Implicit WriteHeader(http.StatusOK): run the compression decision before
	// the first direct write so headers stay consistent.
	w.WriteHeader(http.StatusOK)
	if w.gzipWriter != nil {
		return w.gzipWriter.Write(p)
	}
	return w.ResponseWriter.Write(p)
}

// Close flushes and closes the gzip stream. The middleware defers it so the
// trailer is written after the handler returns.
func (w *gzipResponseWriter) Close() error {
	if w.gzipWriter == nil {
		return nil
	}
	return w.gzipWriter.Close()
}

// withGzip compresses the response of next when the client advertises gzip
// support. Whether compression actually engages is decided per-response by
// gzipResponseWriter (Content-Type based), so binary assets pass through
// unchanged.
func withGzip(next http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !acceptsGzip(r) {
			next.ServeHTTP(w, r)
			return
		}
		compressed := &gzipResponseWriter{ResponseWriter: w}
		defer compressed.Close()
		next.ServeHTTP(compressed, r)
	})
}
