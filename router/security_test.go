package router

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/web"
)

// cspNonceSourceRe matches a CSP nonce source: 'nonce-<standard base64>'.
var cspNonceSourceRe = regexp.MustCompile(`^'nonce-[A-Za-z0-9+/]+={0,2}'$`)

// reCSPNonceMeta matches real <meta name="csp-nonce" ...> elements, not the
// HTML-comment documentation about them.
var reCSPNonceMeta = regexp.MustCompile(`(?i)<meta\b[^>]*\bname="csp-nonce"[^>]*>`)

// extractCSPNonce pulls the nonce value out of a Content-Security-Policy
// header value ("" when absent or malformed).
func extractCSPNonce(t *testing.T, csp string) string {
	t.Helper()
	dirs := parseCSPDirectives(t, csp)
	for _, s := range dirs["style-src"] {
		if m := cspNonceSourceRe.FindStringSubmatch(s); m != nil {
			return strings.TrimSuffix(strings.TrimPrefix(s, "'nonce-"), "'")
		}
	}
	return ""
}

// TestCSPNonceRandomPerRequest pins the core nonce contract: every response
// gets a fresh, full-entropy nonce — reusing one across requests would let an
// injected element from page load N pass the policy of page load N+1.
func TestCSPNonceRandomPerRequest(t *testing.T) {
	config.SetRuntime(&config.RuntimeSettings{
		AuthToken:  "admin-token",
		ProxyToken: "proxy-token",
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	cfg := &config.Config{
		RequestBodyLimit: config.DefaultRequestBodyLimit,
	}
	r := New(cfg, web.Dist)

	seen := map[string]bool{}
	for i := 0; i < 25; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/about", nil) // 401, no DB needed
		r.ServeHTTP(rec, req)

		nonce := extractCSPNonce(t, rec.Header().Get("Content-Security-Policy"))
		if nonce == "" {
			t.Fatalf("request %d: no nonce source in style-src: %q", i, rec.Header().Get("Content-Security-Policy"))
		}
		raw, err := base64.StdEncoding.DecodeString(nonce)
		if err != nil {
			t.Fatalf("request %d: nonce %q is not standard base64: %v", i, nonce, err)
		}
		if len(raw) != 16 {
			t.Fatalf("request %d: nonce decodes to %d bytes, want 16 (128 bits)", i, len(raw))
		}
		if seen[nonce] {
			t.Fatalf("request %d: nonce %q repeated across requests", i, nonce)
		}
		seen[nonce] = true
	}
}

// TestSPAFallbackInjectsNonceMetaMatchingHeader verifies the nonce handshake
// with the SPA: the served index.html carries <meta name="csp-nonce"> whose
// content equals the nonce in the CSP header of the same response, so runtime
// <style> injectors (chart colors, dialog scroll lock) can stamp their
// elements. Asserted on the real embedded dist.
func TestSPAFallbackInjectsNonceMetaMatchingHeader(t *testing.T) {
	config.SetRuntime(&config.RuntimeSettings{
		AuthToken:  "admin-token",
		ProxyToken: "proxy-token",
	})
	t.Cleanup(func() { config.SetRuntime(nil) })
	cfg := &config.Config{
		RequestBodyLimit: config.DefaultRequestBodyLimit,
	}
	r := New(cfg, web.Dist)

	for _, path := range []string{"/", "/sites"} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s status = %d", path, rec.Code)
			}
			headerNonce := extractCSPNonce(t, rec.Header().Get("Content-Security-Policy"))
			if headerNonce == "" {
				t.Fatal("no nonce source in CSP header")
			}
			body := rec.Body.String()
			wantMeta := `<meta name="csp-nonce" content="` + headerNonce + `">`
			if !strings.Contains(body, wantMeta) {
				t.Fatalf("served HTML for %s missing %s", path, wantMeta)
			}
			// Count only real <meta> elements. The source template documents
			// the contract in an HTML comment, so a bare attribute substring
			// would over-count; the regex requires the opening tag.
			metas := reCSPNonceMeta.FindAllString(body, -1)
			if len(metas) != 1 {
				t.Fatalf("served HTML for %s contains %d csp-nonce <meta> tags, want exactly 1: %q", path, len(metas), metas)
			}
		})
	}
}

// TestInjectCSPNonceMetaDegrades covers the fallback branches of
// injectCSPNonceMeta directly (no nonce / no <head> / happy path).
func TestInjectCSPNonceMetaDegrades(t *testing.T) {
	html := "<!doctype html><html><head><title>t</title></head><body></body></html>"

	if got := injectCSPNonceMeta(html, ""); got != html {
		t.Fatalf("empty nonce must leave HTML unchanged, got %q", got)
	}
	if got := injectCSPNonceMeta("<html><body>no head</body></html>", "abc"); got != "<html><body>no head</body></html>" {
		t.Fatalf("missing <head> must leave HTML unchanged, got %q", got)
	}
	got := injectCSPNonceMeta(html, "abc123")
	want := `<!doctype html><html><head><meta name="csp-nonce" content="abc123"><title>t</title></head><body></body></html>`
	if got != want {
		t.Fatalf("injectCSPNonceMeta = %q, want %q", got, want)
	}
}

// TestSonnerStyleHashMatchesEmbeddedBundle is the drift guard for
// sonnerToastStyleHash: it re-derives the sha256 from the sonner __insertCSS
// call embedded in the built bundle and fails if a sonner upgrade changes
// the injected string without the constant following. Skips when the
// embedded dist is a compile-time placeholder (no real frontend build).
func TestSonnerStyleHashMatchesEmbeddedBundle(t *testing.T) {
	const marker = "[data-sonner-toaster][dir=ltr]"

	sub, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		t.Skipf("embedded dist unavailable: %v", err)
	}
	var found []string
	err = fs.WalkDir(sub, "static/js", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".js") {
			return nil
		}
		data, readErr := fs.ReadFile(sub, path)
		if readErr != nil {
			return readErr
		}
		if strings.Count(string(data), marker) > 0 {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		t.Skipf("cannot walk embedded dist (placeholder build?): %v", err)
	}
	if len(found) == 0 {
		t.Skip("sonner marker not present in embedded dist (placeholder build?)")
	}
	if len(found) != 1 {
		t.Fatalf("sonner toast CSS marker found in %d bundles %q, want exactly 1", len(found), found)
	}

	data, err := fs.ReadFile(sub, found[0])
	if err != nil {
		t.Fatalf("read %s: %v", found[0], err)
	}
	literal, err := extractJSStringLiteralContaining(string(data), marker)
	if err != nil {
		t.Fatalf("extract sonner CSS literal from %s: %v", found[0], err)
	}
	value, err := unescapeJSLiteral(literal)
	if err != nil {
		t.Fatalf("unescape sonner CSS literal: %v", err)
	}
	sum := sha256.Sum256([]byte(value))
	got := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	if got != sonnerToastStyleHash {
		t.Fatalf("sonnerToastStyleHash = %s, but the embedded bundle injects CSS hashing to %s — update the constant (see its doc comment)", sonnerToastStyleHash, got)
	}
}

// extractJSStringLiteralContaining finds the double-quoted JS string literal
// containing marker and returns its raw content (between the quotes, escapes
// preserved).
func extractJSStringLiteralContaining(src, marker string) (string, error) {
	mi := strings.Index(src, marker)
	if mi < 0 {
		return "", errMarkerMissing
	}
	open := strings.LastIndex(src[:mi], `"`)
	if open < 0 {
		return "", errNoOpeningQuote
	}
	var sb strings.Builder
	for i := open + 1; i < len(src); i++ {
		ch := src[i]
		if ch == '\\' && i+1 < len(src) {
			sb.WriteByte(ch)
			sb.WriteByte(src[i+1])
			i++
			continue
		}
		if ch == '"' {
			return sb.String(), nil
		}
		sb.WriteByte(ch)
	}
	return "", errUnterminatedLiteral
}

// unescapeJSLiteral decodes a JS string literal body (the text between the
// quotes) into its value, covering the escape forms a CSS string can
// realistically contain.
func unescapeJSLiteral(raw string) (string, error) {
	var sb strings.Builder
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if ch != '\\' {
			sb.WriteByte(ch)
			continue
		}
		if i+1 >= len(raw) {
			return "", errTrailingEscape
		}
		i++
		switch raw[i] {
		case 'n':
			sb.WriteByte('\n')
		case 't':
			sb.WriteByte('\t')
		case 'r':
			sb.WriteByte('\r')
		case 'b':
			sb.WriteByte('\b')
		case 'f':
			sb.WriteByte('\f')
		case 'v':
			sb.WriteByte('\v')
		case '0':
			sb.WriteByte(0)
		case 'x':
			if i+2 >= len(raw) {
				return "", errTrailingEscape
			}
			v, err := hexByte(raw[i+1 : i+3])
			if err != nil {
				return "", err
			}
			sb.WriteByte(v)
			i += 2
		case 'u':
			if i+4 >= len(raw) {
				return "", errTrailingEscape
			}
			var r rune
			for k := 1; k <= 4; k++ {
				v, err := hexNibble(raw[i+k])
				if err != nil {
					return "", err
				}
				r = r<<4 | rune(v)
			}
			sb.WriteRune(r)
			i += 4
		default:
			// JS: any other escaped character is itself (\", \\, \').
			sb.WriteByte(raw[i])
		}
	}
	return sb.String(), nil
}

var (
	errMarkerMissing       = errors.New("sonner marker not found in bundle")
	errNoOpeningQuote      = errors.New("no opening quote before marker")
	errUnterminatedLiteral = errors.New("unterminated string literal")
	errTrailingEscape      = errors.New("trailing escape sequence")
)

func hexNibble(c byte) (byte, error) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', nil
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, nil
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, nil
	}
	return 0, errors.New("invalid hex digit")
}

func hexByte(s string) (byte, error) {
	hi, err := hexNibble(s[0])
	if err != nil {
		return 0, err
	}
	lo, err := hexNibble(s[1])
	if err != nil {
		return 0, err
	}
	return hi<<4 | lo, nil
}
