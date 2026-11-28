package admin

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/go-chi/chi/v5"
)

// ldohClient returns the shared LDOH proxy client, building it lazily on
// first use. LDOH_PROXY_TIMEOUT_SEC is parsed once at startup (no runtime
// retune), so a sync.Once-built client is safe and keeps one pooled,
// phase-bounded transport instead of a fresh transport-less client per
// proxied request.
func (h *monitorHandler) ldohClient() *http.Client {
	h.ldohClientOnce.Do(func() {
		timeout := time.Duration(h.cfg.LDOHProxyTimeoutSec) * time.Second
		h.ldohHTTPClient = &http.Client{
			Timeout:   timeout,
			Transport: httpclient.NewTransport(httpclient.Options{ResponseHeaderTimeout: timeout}),
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	})
	return h.ldohHTTPClient
}

// monitorProxyCSP is the response policy for the /monitor-proxy/* surface.
// The content is third-party and served from the admin origin; script-src
// 'none' plus sandbox keeps it from acting as same-origin code, while the
// inline-style allowance keeps the LDOH UI readable.
const monitorProxyCSP = "default-src 'none'; style-src 'unsafe-inline'; img-src 'self' data:; script-src 'none'; sandbox"

// containsEncodedPathTraversal reports whether the proxy path reaches a ".."
// segment after up to two extra percent-decode rounds. r.URL.Path has already
// been decoded once by net/http, so "%252e%252e" arrives as "%2e%2e"; an
// upstream that decodes again would turn that into "..". Backslashes are
// treated as separators because some upstream stacks normalize them to "/".
func containsEncodedPathTraversal(path string) bool {
	decoded := path
	for i := 0; i < 2; i++ {
		next, err := url.PathUnescape(decoded)
		if err != nil || next == decoded {
			break
		}
		decoded = next
	}
	for _, segment := range strings.FieldsFunc(decoded, func(r rune) bool {
		return r == '/' || r == '\\'
	}) {
		if segment == ".." {
			return true
		}
	}
	return false
}

// GET/POST/ALL /monitor-proxy/ldoh and /monitor-proxy/ldoh/*
func (h *monitorHandler) ldohProxy(w http.ResponseWriter, r *http.Request) {
	if !h.ensureMonitorAuth(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error": "Missing or invalid monitor session",
		})
		return
	}

	// Third-party LDOH content is served from the Metapi origin. The global
	// policy allows script-src 'self', so proxied pages/scripts would execute
	// as same-origin code and could call /api/* with the admin session
	// (audit M17). This response policy disables scripts entirely and sandboxes
	// the document; setting it here replaces the global header for this
	// response, which is strictly narrower.
	w.Header().Set("Content-Security-Policy", monitorProxyCSP)

	storedCookie := getSettingValue(h.db, ldohCookieSettingKey)
	if storedCookie == "" {
		// Keep plain-text 400 parity with TS for the unconfigured cookie path.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("LDOH cookie not configured"))
		return
	}

	wildcardPath := resolveLdohProxyPath(r)
	// Reject ".." segments before joining the
	// LDOH base URL. The request path arrives percent-decoded, so %2e%2e is
	// caught in the same scan. For the proxy surface a single decode is not
	// enough: upstream frameworks may decode again, so %252e%252e (which
	// reaches us as the literal "%2e%2e") must also be rejected (audit L4).
	if containsEncodedPathTraversal(wildcardPath) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid proxy path"})
		return
	}
	baseURL := h.cfg.LDOHBaseURL
	targetURL, err := url.Parse(baseURL + "/" + wildcardPath)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid proxy path"})
		return
	}
	q := targetURL.Query()
	for key, values := range r.URL.Query() {
		for _, value := range values {
			q.Add(key, value)
		}
	}
	targetURL.RawQuery = q.Encode()

	method := strings.ToUpper(r.Method)
	var body io.Reader
	if method != http.MethodGet && method != http.MethodHead {
		body = r.Body
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), method, targetURL.String(), body)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": "failed to build upstream request"})
		return
	}
	upstreamReq.Header.Set("Cookie", storedCookie)
	upstreamReq.Header.Set("Accept", firstNonEmpty(r.Header.Get("Accept"), "*/*"))
	upstreamReq.Header.Set("Accept-Language", firstNonEmpty(r.Header.Get("Accept-Language"), "zh-CN,zh;q=0.9,en;q=0.8"))
	upstreamReq.Header.Set("User-Agent", firstNonEmpty(r.Header.Get("User-Agent"), "metapiMonitorProxy/1.0"))
	if ct := r.Header.Get("Content-Type"); ct != "" {
		upstreamReq.Header.Set("Content-Type", ct)
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		upstreamReq.Header.Set("Referer", strings.ReplaceAll(referer, "/monitor-proxy/ldoh", ""))
	}

	client := h.ldohClient()
	upstreamResp, err := client.Do(upstreamReq)
	if err != nil {
		slog.Error("LDOH upstream request failed",
			"method", method,
			"url", targetURL.String(),
			"error", err,
		)
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": "LDOH upstream request failed",
		})
		return
	}
	defer upstreamResp.Body.Close()

	contentType := upstreamResp.Header.Get("Content-Type")
	if location := rewriteLocationHeader(upstreamResp.Header.Get("Location"), baseURL); location != "" {
		w.Header().Set("Location", location)
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	if cacheControl := upstreamResp.Header.Get("Cache-Control"); cacheControl != "" {
		w.Header().Set("Cache-Control", cacheControl)
	}

	w.WriteHeader(upstreamResp.StatusCode)

	if shouldRewriteProxyBody(contentType) {
		raw, readErr := io.ReadAll(upstreamResp.Body)
		if readErr != nil {
			return
		}
		_, _ = w.Write([]byte(rewriteProxyText(string(raw), baseURL)))
		return
	}

	_, _ = io.Copy(w, upstreamResp.Body)
}

func resolveLdohProxyPath(r *http.Request) string {
	cleanPath := r.URL.Path
	prefix := "/monitor-proxy/ldoh"
	if cleanPath == prefix || cleanPath == prefix+"/" {
		return ""
	}
	if strings.HasPrefix(cleanPath, prefix+"/") {
		return strings.TrimPrefix(cleanPath, prefix+"/")
	}
	if star := chi.URLParam(r, "*"); star != "" {
		return strings.TrimPrefix(star, "/")
	}
	return ""
}

func rewriteProxyText(text, baseURL string) string {
	// Build the URL-based replacer entries dynamically so operators who
	// override LDOH_BASE_URL get correct body rewrites for their instance.
	// The escaped variant handles JSON-encoded source maps / inline scripts
	// where "/" is written as "\/".
	baseURLPrefix := baseURL + "/"
	escapedBaseURLPrefix := strings.ReplaceAll(baseURLPrefix, "/", `\/`)
	replacer := strings.NewReplacer(
		baseURLPrefix, "/monitor-proxy/ldoh/",
		escapedBaseURLPrefix, `\/monitor-proxy\/ldoh\/`,
		`src="/`, `src="/monitor-proxy/ldoh/`,
		`src='/`, `src='/monitor-proxy/ldoh/`,
		`href="/`, `href="/monitor-proxy/ldoh/`,
		`href='/`, `href='/monitor-proxy/ldoh/`,
		`action="/`, `action="/monitor-proxy/ldoh/`,
		`action='/`, `action='/monitor-proxy/ldoh/`,
		`"/_next/`, `"/monitor-proxy/ldoh/_next/`,
		`'/_next/`, `'/monitor-proxy/ldoh/_next/`,
		`"\/api/`, `"\/monitor-proxy\/ldoh\/api/`,
		`'/api/`, `'/monitor-proxy/ldoh/api/`,
		`"/api/`, `"/monitor-proxy/ldoh/api/`,
	)
	return replacer.Replace(text)
}

func rewriteLocationHeader(location, baseURL string) string {
	if location == "" {
		return ""
	}
	if strings.HasPrefix(location, baseURL+"/") {
		return "/monitor-proxy/ldoh/" + strings.TrimPrefix(location, baseURL+"/")
	}
	if strings.HasPrefix(location, "/") {
		return "/monitor-proxy/ldoh" + location
	}
	return location
}

func shouldRewriteProxyBody(contentType string) bool {
	ct := strings.ToLower(contentType)
	return strings.Contains(ct, "text/html") ||
		strings.Contains(ct, "application/javascript") ||
		strings.Contains(ct, "text/javascript") ||
		strings.Contains(ct, "text/css") ||
		strings.Contains(ct, "application/json")
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
