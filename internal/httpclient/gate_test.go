package httpclient

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Outbound HTTP client gate.
//
// Background: outbound requests that ride http.DefaultClient /
// http.DefaultTransport (or an http.Transport literal without phase
// timeouts) inherit no response-header bound and no explicit dial/TLS/idle
// bounds, so a silently-accepting upstream can hold connections and
// goroutines indefinitely. The baseline adopted for this repo mirrors the
// axonhub HTTP client baseline: dial 30s, TLS handshake 10s, MaxIdleConns
// 100, IdleConnTimeout 90s. Control-plane transports are constructed in
// internal/httpclient, with named exceptions that keep their own literals
// by design (all still satisfy R3/R4):
//
//   - platform/site_proxy.go — pooled transport cache for platform adapter
//     traffic, operator-tunable via PROXY_*_TIMEOUT_SEC;
//   - SSRF-hardened clients whose DialContext enforces dial-level target
//     guards internal/httpclient does not model: notifyHTTPClient
//     (service/notify, all webhook-style channels) and the WebDAV backup
//     clients in handler/admin and scheduler. Transports built here opt into
//     the shared site dial guard (internal/ssrf) with Options.SiteDialGuard —
//     used by the proxy data plane and channel health probes;
//   - proxy.NewStreamTransport — SSE data-plane stream relay; header phase
//     bounded, whole-request timeout owned by the relay's idle guard.
//
// Rules enforced on every non-test .go file in the repository:
//
//	R1  http.DefaultClient / http.DefaultTransport are forbidden.
//	R2  package-level http.Get/Post/PostForm/Head are forbidden (they ride
//	    http.DefaultClient, which has no timeout at all).
//	R3  every http.Client literal must set an explicit Transport or a
//	    non-zero Timeout.
//	R4  every http.Transport literal must set DialContext/DialTLSContext,
//	    TLSHandshakeTimeout and IdleConnTimeout. ResponseHeaderTimeout is
//	    deliberately not required: context-driven paths (SSE relays, probe
//	    and channel-test fallbacks) leave the header phase to the caller
//	    deadline on purpose.
//
// The matcher is AST-based (go/parser), so comments mentioning the
// forbidden identifiers cannot trip it, and import aliases of net/http are
// honored. This mirrors the docs/pg_rebind_gate_test.go pattern: a repo
// scan plus a sanity test that locks matcher behaviour with counter-examples.

// auditSource inspects one Go source file for outbound HTTP constructions
// that bypass the timeout/pool baseline. It parses without type checking,
// so it runs on any syntactically valid file regardless of dependencies.
func auditSource(filename string, src []byte) ([]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, 0)
	if err != nil {
		return nil, err
	}

	// Resolve the local name of the net/http import; files without one
	// cannot construct http.* values.
	httpName := ""
	for _, imp := range file.Imports {
		if imp.Path.Value == `"net/http"` {
			httpName = "http"
			if imp.Name != nil {
				httpName = imp.Name.Name
			}
		}
	}
	if httpName == "" {
		return nil, nil
	}

	var violations []string
	report := func(n ast.Node, msg string) {
		violations = append(violations, fmt.Sprintf("%s: %s", fset.Position(n.Pos()), msg))
	}

	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectorExpr:
			ident, ok := node.X.(*ast.Ident)
			if !ok || ident.Name != httpName {
				return true
			}
			switch node.Sel.Name {
			case "DefaultClient", "DefaultTransport":
				report(node, fmt.Sprintf(
					"http.%s carries no outbound phase bounds; build the client/transport via internal/httpclient or the platform site-proxy pool",
					node.Sel.Name))
			}
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok || ident.Name != httpName {
				return true
			}
			switch sel.Sel.Name {
			case "Get", "Post", "PostForm", "Head":
				report(node, fmt.Sprintf(
					"http.%s uses http.DefaultClient (no timeout); construct a client via internal/httpclient instead",
					sel.Sel.Name))
			}
		case *ast.CompositeLit:
			sel, ok := node.Type.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			ident, ok := sel.X.(*ast.Ident)
			if !ok || ident.Name != httpName {
				return true
			}
			switch sel.Sel.Name {
			case "Client":
				hasTransport, hasTimeout := false, false
				for _, elt := range node.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					key, ok := kv.Key.(*ast.Ident)
					if !ok {
						continue
					}
					switch key.Name {
					case "Transport":
						hasTransport = true
					case "Timeout":
						if !isZeroLiteral(kv.Value) {
							hasTimeout = true
						}
					}
				}
				if !hasTransport && !hasTimeout {
					report(node, "http.Client literal without Transport and without a non-zero Timeout rides http.DefaultTransport with no deadline; set an explicit Transport (internal/httpclient) and/or Timeout")
				}
			case "Transport":
				var missing []string
				if !transportHasField(node, "DialContext", "DialTLSContext") {
					missing = append(missing, "DialContext/DialTLSContext")
				}
				if !transportHasField(node, "TLSHandshakeTimeout") {
					missing = append(missing, "TLSHandshakeTimeout")
				}
				if !transportHasField(node, "IdleConnTimeout") {
					missing = append(missing, "IdleConnTimeout")
				}
				if len(missing) > 0 {
					report(node, fmt.Sprintf(
						"http.Transport literal missing %s; build it via internal/httpclient.NewTransport or mirror the baseline (dial 30s / TLS 10s / idle 90s / pool 100)",
						strings.Join(missing, ", ")))
				}
			}
		}
		return true
	})
	return violations, nil
}

func transportHasField(lit *ast.CompositeLit, names ...string) bool {
	for _, elt := range lit.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		key, ok := kv.Key.(*ast.Ident)
		if !ok {
			continue
		}
		for _, name := range names {
			if key.Name == name {
				return true
			}
		}
	}
	return false
}

func isZeroLiteral(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	return ok && lit.Kind == token.INT && lit.Value == "0"
}

func TestOutboundHTTPClientGateNoBareClients(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/httpclient -> internal -> repo root
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
	skipDirs := map[string]bool{
		".git": true, ".github": true, ".worktrees": true,
		"web": true, "docs": true, "vendor": true, "node_modules": true,
	}

	var violations []string
	scanned := 0
	err := filepath.WalkDir(repoRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		rel, relErr := filepath.Rel(repoRoot, path)
		if relErr != nil {
			rel = path
		}
		v, auditErr := auditSource(rel, src)
		if auditErr != nil {
			// Unparseable files already fail the build before this gate runs.
			return nil
		}
		scanned++
		violations = append(violations, v...)
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo root: %v", err)
	}
	if scanned == 0 {
		t.Fatalf("gate scanned no files; repo root resolution is broken (%s)", repoRoot)
	}
	if len(violations) > 0 {
		t.Fatalf(
			"outbound HTTP construction bypasses the timeout/pool baseline "+
				"(build clients/transports via internal/httpclient or the platform site-proxy pool):\n%s",
			strings.Join(violations, "\n"),
		)
	}
}

// TestOutboundHTTPClientGateMatcherSanity locks the matcher behaviour with
// counter-examples: bare constructions must be flagged, bounded ones must
// pass. This is the reverse-validation half of the gate (the repo scan
// above is the forward half).
func TestOutboundHTTPClientGateMatcherSanity(t *testing.T) {
	cases := []struct {
		name string
		src  string
		flag bool
	}{
		{
			name: "bare http.DefaultClient",
			src: `package p

import "net/http"

var c = http.DefaultClient
`,
			flag: true,
		},
		{
			name: "bare http.DefaultTransport",
			src: `package p

import "net/http"

var tr = http.DefaultTransport
`,
			flag: true,
		},
		{
			name: "aliased net/http DefaultClient",
			src: `package p

import nhttp "net/http"

var c = nhttp.DefaultClient
`,
			flag: true,
		},
		{
			name: "package-level http.Get",
			src: `package p

import "net/http"

func f() { _, _ = http.Get("http://example.com") }
`,
			flag: true,
		},
		{
			name: "client literal without timeout or transport",
			src: `package p

import "net/http"

var c = &http.Client{}
`,
			flag: true,
		},
		{
			name: "client literal with zero timeout",
			src: `package p

import "net/http"

var c = &http.Client{Timeout: 0}
`,
			flag: true,
		},
		{
			name: "transport literal missing idle bound",
			src: `package p

import "net/http"

var tr = &http.Transport{DialContext: d, TLSHandshakeTimeout: t}
`,
			flag: true,
		},
		{
			name: "client with timeout",
			src: `package p

import ("net/http"; "time")

var c = &http.Client{Timeout: 30 * time.Second}
`,
			flag: false,
		},
		{
			name: "client with transport and zero timeout",
			src: `package p

import "net/http"

var c = &http.Client{Transport: tr, Timeout: 0}
`,
			flag: false,
		},
		{
			name: "transport with DialTLSContext instead of DialContext",
			src: `package p

import "net/http"

var tr = &http.Transport{DialTLSContext: d, TLSHandshakeTimeout: t, IdleConnTimeout: i}
`,
			flag: false,
		},
		{
			name: "complete transport",
			src: `package p

import "net/http"

var tr = &http.Transport{DialContext: d, TLSHandshakeTimeout: t, IdleConnTimeout: i}
`,
			flag: false,
		},
	}
	for _, tc := range cases {
		violations, err := auditSource("sanity.go", []byte(tc.src))
		if err != nil {
			t.Fatalf("%s: audit parse error: %v", tc.name, err)
		}
		if tc.flag && len(violations) == 0 {
			t.Errorf("%s: matcher must flag this construction, got clean", tc.name)
		}
		if !tc.flag && len(violations) > 0 {
			t.Errorf("%s: matcher must not flag this construction, got: %v", tc.name, violations)
		}
	}
}
