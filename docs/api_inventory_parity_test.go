package docs_test

// TestAPIRouteInventoryParity locks docs/api/routes-inventory.md to the
// routes the Go code actually registers, so the published inventory cannot
// drift when a handler is added, renamed or removed.
//
// Extraction is pure Go source parsing (go/parser + go/ast) — no shell, no
// ripgrep, no committed fixture that can go stale on its own.
//
// How the live route set is rebuilt:
//
//  1. Every non-test .go file under router/ and handler/ is parsed and all
//     top-level functions are indexed by name.
//  2. The traversal starts at router.New — the only composition root — so a
//     registrar that exists but is never wired is NOT counted as a live
//     route. An unwired handler must not make the docs claim an endpoint.
//  3. Inside each function, chi registrations on a router-shaped receiver
//     are collected: the HTTP verbs (Get/Post/Put/Patch/Delete/Head/Options)
//     plus Handle/HandleFunc (recorded as method ANY). Only string-literal
//     paths that start with "/" count, which is what keeps header lookups
//     such as r.Header.Get("Content-Type") and query lookups such as
//     r.URL.Query().Get("page") out of the route set even though they share
//     the receiver name.
//  4. `r.Route("/prefix", func(r chi.Router){...})` and `r.Mount` scopes
//     prefix everything registered inside them, and `r.Group`/`r.With` do
//     not. Calls into other registrars (admin.RegisterSitesRoutes(r, db),
//     proxyhandler.RegisterProxyRoutes(r), RegisterFilesRoutes(r)) are
//     resolved by function name across files, inheriting the caller's
//     prefix — this is what turns the relative "/chat/completions" inside
//     RegisterProxyRoutes into "/v1/chat/completions" without a hardcoded
//     mount table.
//  5. Registrars are resolved per Go package: a qualified call
//     admin.RegisterSitesRoutes(r, db) resolves through the calling file's
//     import list, an unqualified RegisterFilesRoutes(r) resolves within the
//     caller's own package. Indexing by (package, name) is what keeps common
//     helper names that exist in both handler/admin and handler/proxy
//     (writeJSON and friends) from colliding.
//  6. Two route shapes are deliberately NOT enumerated, and both are static
//     asset serving rather than API contracts (docs/architecture.md):
//     paths built by concatenation (the SPA's root logo/favicon and
//     bootstrap-script handlers) and the embedded dist subtrees mounted by
//     router.mountStaticSubdir, whose route pattern arrives as a function
//     argument ("/assets/*", "/static/*") instead of a literal.
//
// Path parameters are normalized before comparison: chi v5 accepts both
// `:id` and `{id}`, this repository writes `{id}` in code while the
// inventory documents `:id`, and `*` becomes `{wildcard}`.
//
// Scope: the inventory documents the /api admin surface, so parity is
// asserted on /api routes. Every non-/api route still has to be owned by a
// named document via nonAPIRouteAllowlist (TestNonAPIRoutesAreAccountedFor),
// and both allowlists are checked for stale entries, so an allowlist cannot
// accumulate routes that no longer exist.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// routeVerbs are the chi registration methods that create a route.
var routeVerbs = map[string]string{
	"Get": "GET", "Post": "POST", "Put": "PUT", "Patch": "PATCH",
	"Delete": "DELETE", "Head": "HEAD", "Options": "OPTIONS",
	"Handle": "ANY", "HandleFunc": "ANY",
}

// routerIdents are the receiver names used for chi routers in this repo.
var routerIdents = map[string]bool{"r": true, "rr": true, "router": true, "sub": true, "mux": true}

// inventoryAllowlist lists registered /api routes deliberately NOT published
// in docs/api/routes-inventory.md. Each entry needs a reason; an entry that
// no longer matches a live route fails TestAPIRouteInventoryParity, so this
// map cannot rot into a hiding place.
var inventoryAllowlist = map[string]string{}

// nonAPIRouteAllowlist maps each registered non-/api route to the document
// that owns its contract.
var nonAPIRouteAllowlist = map[string]string{
	// Public liveness / readiness / metrics — docs/deployment.md.
	"GET /health":  "docs/deployment.md",
	"GET /ready":   "docs/deployment.md",
	"GET /metrics": "docs/deployment.md",

	// /v1 proxy data plane — docs/api/proxy.md owns the contract.
	"POST /v1/chat/completions":      "docs/api/proxy.md",
	"POST /v1/messages":              "docs/api/proxy.md",
	"POST /v1/messages/count_tokens": "docs/api/proxy.md",
	"POST /v1/completions":           "docs/api/proxy.md",
	"POST /v1/responses":             "docs/api/proxy.md",
	"GET /v1/responses":              "docs/api/proxy.md",
	"POST /v1/responses/compact":     "docs/api/proxy.md",
	"GET /v1/models":                 "docs/api/proxy.md",
	"POST /v1/embeddings":            "docs/api/proxy.md",
	"POST /v1/rerank":                "docs/api/proxy.md",
	"POST /v1/images/generations":    "docs/api/proxy.md",
	"POST /v1/images/edits":          "docs/api/proxy.md",
	"POST /v1/images/variations":     "docs/api/proxy.md",
	"POST /v1/videos":                "docs/api/proxy.md",
	"GET /v1/videos/{id}":            "docs/api/proxy.md",
	"DELETE /v1/videos/{id}":         "docs/api/proxy.md",
	"POST /v1/search":                "docs/api/proxy.md",
	"POST /v1/files":                 "docs/api/proxy.md",
	"GET /v1/files":                  "docs/api/proxy.md",
	"GET /v1/files/{fileId}":         "docs/api/proxy.md",
	"GET /v1/files/{fileId}/content": "docs/api/proxy.md",
	"DELETE /v1/files/{fileId}":      "docs/api/proxy.md",
	// Downstream-key-visible price catalog mounted under /v1 (inherits
	// proxy auth), reusing the admin model-price data surface.
	"GET /v1/models/price-compare": "docs/api/proxy.md",
	"GET /v1/pricing":              "docs/api/proxy.md",

	// Non-/v1 proxy aliases: Codex native paths and the Gemini surface.
	"POST /chat/completions":                            "docs/api/proxy.md",
	"POST /responses":                                   "docs/api/proxy.md",
	"GET /responses":                                    "docs/api/proxy.md",
	"POST /responses/{wildcard}":                        "docs/api/proxy.md",
	"GET /responses/{wildcard}":                         "docs/api/proxy.md",
	"GET /v1beta/models":                                "docs/api/proxy.md",
	"POST /v1beta/models/{wildcard}":                    "docs/api/proxy.md",
	"GET /gemini/{geminiApiVersion}/models":             "docs/api/proxy.md",
	"POST /gemini/{geminiApiVersion}/models/{wildcard}": "docs/api/proxy.md",
	"POST /v1internal::generateContent":                 "docs/api/proxy.md",
	"POST /v1internal::streamGenerateContent":           "docs/api/proxy.md",
	"POST /v1internal::countTokens":                     "docs/api/proxy.md",

	// LDOH iframe proxy: cookie-authenticated, deliberately outside the
	// bearer-token admin group — docs/api/monitor.md owns it.
	"ANY /monitor-proxy/ldoh":            "docs/api/monitor.md",
	"ANY /monitor-proxy/ldoh/":           "docs/api/monitor.md",
	"ANY /monitor-proxy/ldoh/{wildcard}": "docs/api/monitor.md",
}

func TestAPIRouteInventoryParity(t *testing.T) {
	root := repoRoot(t)
	code := extractRegisteredRoutes(t, root)
	codeRoutes, nonAPI := splitByScope(code)
	if len(codeRoutes) == 0 {
		t.Fatal("extractor sanity: no /api routes reachable from router.New — the extractor is broken, not the docs")
	}
	parsedDoc := parseRouteInventory(t, root)
	if len(parsedDoc) == 0 {
		t.Fatal("extractor sanity: docs/api/routes-inventory.md yielded no routes — the parser is broken")
	}
	// The inventory documents path parameters as :param while chi registers
	// {param}; normalize before comparing (see normalizeRoutePath).
	doc := normalizeDocKeys(parsedDoc)

	if got := compareRouteSets(codeRoutes, doc); got != "" {
		t.Fatalf("API route inventory drift:\n%s\nDocument the route (method, path, auth surface, request/response shape) or remove the stale entry. Do not relax this test.", got)
	}

	// Allowlist hygiene: an entry that matches no live route is dead weight
	// and a possible hiding place for a removed-but-still-claimed exception.
	var stale []string
	for key := range inventoryAllowlist {
		if _, ok := codeRoutes[key]; !ok {
			stale = append(stale, key)
		}
	}
	for key := range nonAPIRouteAllowlist {
		if _, ok := nonAPI[key]; !ok {
			stale = append(stale, key)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Fatalf("allowlist entries that match no registered route (remove them or fix the path):\n  %s",
			strings.Join(stale, "\n  "))
	}
}

// TestNonAPIRoutesAreAccountedFor keeps the extractor honest about routes the
// admin inventory deliberately does not cover: every non-/api route reachable
// from router.New must name the document that owns it.
func TestNonAPIRoutesAreAccountedFor(t *testing.T) {
	root := repoRoot(t)
	_, nonAPI := splitByScope(extractRegisteredRoutes(t, root))
	var unowned []string
	for key, where := range nonAPI {
		if _, ok := nonAPIRouteAllowlist[key]; ok {
			continue
		}
		unowned = append(unowned, key+"  (registered in "+where+")")
	}
	if len(unowned) > 0 {
		sort.Strings(unowned)
		t.Fatalf("non-/api routes with no owning document:\n  %s\nDocument them (docs/api/proxy.md for data-plane routes) and record the owner in nonAPIRouteAllowlist with a reason.",
			strings.Join(unowned, "\n  "))
	}
}

// TestRouteInventoryGateIsNotAVacuousPass proves the parity comparison can go
// red in both directions, using the same compareRouteSets/normalizeRoutePath
// code paths the real test uses. Without this, a broken inventory parser
// would make TestAPIRouteInventoryParity pass forever while asserting nothing.
func TestRouteInventoryGateIsNotAVacuousPass(t *testing.T) {
	code := map[string]string{
		"GET /api/sites":         "handler/admin/sites.go:RegisterSitesRoutes",
		"POST /api/sites":        "handler/admin/sites.go:RegisterSitesRoutes",
		"DELETE /api/sites/{id}": "handler/admin/sites.go:RegisterSitesRoutes",
	}
	clean := map[string]bool{
		"GET /api/sites":        true,
		"POST /api/sites":       true,
		"DELETE /api/sites/:id": true, // inventory uses :param notation
	}
	if got := compareRouteSets(code, normalizeDocKeys(clean)); got != "" {
		t.Fatalf("control sample must be clean, got:\n%s", got)
	}

	missing := normalizeDocKeys(map[string]bool{"GET /api/sites": true, "POST /api/sites": true})
	got := compareRouteSets(code, missing)
	if !strings.Contains(got, "DELETE /api/sites/{id}") || !strings.Contains(got, "MISSING") {
		t.Fatalf("gate did not report an undocumented registered route:\n%s", got)
	}

	ghost := normalizeDocKeys(map[string]bool{
		"GET /api/sites":        true,
		"POST /api/sites":       true,
		"DELETE /api/sites/:id": true,
		"GET /api/sites/ghost":  true,
	})
	got = compareRouteSets(code, ghost)
	if !strings.Contains(got, "GET /api/sites/ghost") || !strings.Contains(got, "NOT registered") {
		t.Fatalf("gate did not report an invented documented route:\n%s", got)
	}

	// Notation normalization itself must be load-bearing.
	if normalizeRoutePath("/api/sites/:id") != normalizeRoutePath("/api/sites/{id}") {
		t.Fatal("normalizeRoutePath no longer treats :param and {param} as the same route")
	}
	if normalizeRoutePath("/responses/*") != "/responses/{wildcard}" {
		t.Fatal("normalizeRoutePath no longer maps a chi wildcard to {wildcard}")
	}
}

// compareRouteSets reports both drift directions: registered-but-undocumented
// and documented-but-unregistered.
func compareRouteSets(code map[string]string, doc map[string]bool) string {
	var undocumented, invented []string
	for key, where := range code {
		if doc[key] {
			continue
		}
		if _, ok := inventoryAllowlist[key]; ok {
			continue
		}
		undocumented = append(undocumented, key+"  (registered in "+where+")")
	}
	for key := range doc {
		if _, ok := code[key]; !ok {
			invented = append(invented, key)
		}
	}
	var b strings.Builder
	if len(undocumented) > 0 {
		sort.Strings(undocumented)
		b.WriteString("  MISSING from docs/api/routes-inventory.md but registered in code (" +
			strconv.Itoa(len(undocumented)) + "):\n    " + strings.Join(undocumented, "\n    ") + "\n")
	}
	if len(invented) > 0 {
		sort.Strings(invented)
		b.WriteString("  NOT registered in code but listed in docs/api/routes-inventory.md (" +
			strconv.Itoa(len(invented)) + "):\n    " + strings.Join(invented, "\n    ") + "\n")
	}
	return b.String()
}

func normalizeDocKeys(doc map[string]bool) map[string]bool {
	out := make(map[string]bool, len(doc))
	for key := range doc {
		verb, path, ok := strings.Cut(key, " ")
		if !ok {
			continue
		}
		out[verb+" "+normalizeRoutePath(path)] = true
	}
	return out
}

// splitByScope separates the /api admin surface from everything else.
func splitByScope(routes map[registeredRoute]bool) (api map[string]string, other map[string]string) {
	api = map[string]string{}
	other = map[string]string{}
	for route := range routes {
		key := route.method + " " + normalizeRoutePath(route.path)
		if strings.HasPrefix(route.path, "/api/") || route.path == "/api" {
			api[key] = route.where
			continue
		}
		other[key] = route.where
	}
	return api, other
}

type registeredRoute struct {
	method string
	path   string
	where  string
}

// pkgIndex holds every top-level function under router/ and handler/, keyed
// by package directory then function name, plus each file's internal import
// aliases so a qualified registrar call can be resolved to a package.
type pkgIndex struct {
	funcs   map[string]map[string]*ast.FuncDecl // dir -> func name -> decl
	pkgName map[string]string                   // dir -> Go package name
	imports map[string]map[string]string        // dir -> alias -> imported dir
	file    map[string]map[string]string        // dir -> func name -> relative file
}

func extractRegisteredRoutes(t *testing.T, root string) map[registeredRoute]bool {
	t.Helper()
	idx := buildPkgIndex(t, root)
	if len(idx.funcs) == 0 {
		t.Fatal("extractor sanity: no Go functions indexed under router/ or handler/")
	}
	if idx.funcs["router"]["New"] == nil {
		t.Fatal("extractor sanity: router.New not found — the composition root moved")
	}

	var out []registeredRoute
	visited := map[string]bool{}
	var walk func(dir, name, prefix string, depth int)
	walk = func(dir, name, prefix string, depth int) {
		key := dir + "|" + name + "|" + prefix
		if depth > 16 || visited[key] {
			return
		}
		visited[key] = true
		fn := idx.funcs[dir][name]
		if fn == nil || fn.Body == nil {
			return
		}
		where := idx.file[dir][name] + ":" + name
		scanBlock(fn.Body, prefix, where, idx, dir,
			func(method, path, w string) {
				out = append(out, registeredRoute{method: method, path: path, where: w})
			},
			walk)
	}
	walk("router", "New", "", 0)

	set := map[registeredRoute]bool{}
	for _, r := range out {
		set[r] = true
	}
	return set
}

// scanBlock walks a block in lexical order, threading the chi Route/Mount
// prefix through nested function literals and following calls into other
// registrars.
func scanBlock(block *ast.BlockStmt, prefix, where string, idx *pkgIndex, dir string,
	emit func(method, path, where string), walk func(dir, name, prefix string, depth int)) {
	for _, stmt := range block.List {
		scanStmt(stmt, prefix, where, idx, dir, emit, walk, 0)
	}
}

func scanStmt(n ast.Node, prefix, where string, idx *pkgIndex, dir string,
	emit func(method, path, where string), walk func(dir, name, prefix string, depth int), depth int) {
	ast.Inspect(n, func(node ast.Node) bool {
		switch v := node.(type) {
		case *ast.FuncLit:
			// A nested handler literal cannot register routes at this level;
			// Route/Group bodies are descended into explicitly instead.
			return false
		case *ast.CallExpr:
			handleCall(v, prefix, where, idx, dir, emit, walk, depth)
		}
		return true
	})
}

func handleCall(ce *ast.CallExpr, prefix, where string, idx *pkgIndex, dir string,
	emit func(method, path, where string), walk func(dir, name, prefix string, depth int), depth int) {
	sel, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok {
		// Unqualified call: same package, e.g. RegisterFilesRoutes(r).
		if id, ok := ce.Fun.(*ast.Ident); ok && hasRouterArg(ce) {
			if idx.funcs[dir][id.Name] != nil {
				walk(dir, id.Name, prefix, depth+1)
			}
		}
		return
	}
	name := sel.Sel.Name

	// Package-qualified call: admin.RegisterSitesRoutes(r, db.DB).
	if pkgIdent, ok := sel.X.(*ast.Ident); ok {
		if target, isPkg := idx.imports[dir][pkgIdent.Name]; isPkg {
			if hasRouterArg(ce) && idx.funcs[target][name] != nil {
				walk(target, name, prefix, depth+1)
			}
			return
		}
	}

	if !isRouterExpr(sel.X) {
		return
	}
	path, literal := firstStringArg(ce)
	switch {
	case (name == "Route" || name == "Mount") && literal && len(ce.Args) >= 2:
		if fl, ok := ce.Args[1].(*ast.FuncLit); ok && fl.Body != nil {
			inner := joinRoutePath(prefix, path)
			for _, stmt := range fl.Body.List {
				scanStmt(stmt, inner, where, idx, dir, emit, walk, depth)
			}
		}
	case (name == "Group" || name == "With" || name == "Use") && len(ce.Args) >= 1:
		if fl, ok := ce.Args[0].(*ast.FuncLit); ok && fl.Body != nil {
			for _, stmt := range fl.Body.List {
				scanStmt(stmt, prefix, where, idx, dir, emit, walk, depth)
			}
		}
	default:
		verb, isVerb := routeVerbs[name]
		// Only literal paths starting with "/" are routes; this excludes
		// r.Header.Get("Content-Type") and r.URL.Query().Get("page").
		if isVerb && literal && strings.HasPrefix(path, "/") {
			emit(verb, joinRoutePath(prefix, path), where)
		}
	}
}

func hasRouterArg(ce *ast.CallExpr) bool {
	return len(ce.Args) > 0 && isRouterExpr(ce.Args[0])
}

// isRouterExpr reports whether an expression bottoms out in a router-named
// identifier, so chained forms like r.With(CORS()).Get("/health", h) count.
func isRouterExpr(e ast.Expr) bool {
	for {
		switch v := e.(type) {
		case *ast.Ident:
			return routerIdents[v.Name]
		case *ast.SelectorExpr:
			e = v.X
		case *ast.CallExpr:
			e = v.Fun
		case *ast.ParenExpr:
			e = v.X
		default:
			return false
		}
	}
}

func firstStringArg(ce *ast.CallExpr) (string, bool) {
	if len(ce.Args) == 0 {
		return "", false
	}
	lit, ok := ce.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}

func joinRoutePath(prefix, path string) string {
	if prefix == "" {
		return path
	}
	if path == "" || path == "/" {
		return prefix
	}
	return strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(path, "/")
}

// normalizeRoutePath collapses :param and {param} to {param} and a chi
// wildcard to {wildcard} so doc notation and code notation compare equal.
func normalizeRoutePath(p string) string {
	p = strings.ReplaceAll(p, "*", "{wildcard}")
	parts := strings.Split(p, "/")
	for i, seg := range parts {
		if strings.HasPrefix(seg, ":") && len(seg) > 1 {
			parts[i] = "{" + seg[1:] + "}"
		}
	}
	return strings.Join(parts, "/")
}

// buildPkgIndex parses every non-test .go file under router/ and handler/
// (one level of subpackages, which is where every registrar lives) and
// indexes top-level functions per package directory.
func buildPkgIndex(t *testing.T, root string) *pkgIndex {
	t.Helper()
	fset := token.NewFileSet()
	idx := &pkgIndex{
		funcs:   map[string]map[string]*ast.FuncDecl{},
		pkgName: map[string]string{},
		imports: map[string]map[string]string{},
		file:    map[string]map[string]string{},
	}

	var files []string
	for _, dir := range []string{"router", "handler"} {
		for _, pattern := range []string{filepath.Join(root, dir, "*.go"), filepath.Join(root, dir, "*", "*.go")} {
			matches, err := filepath.Glob(pattern)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, matches...)
		}
	}
	if len(files) == 0 {
		t.Fatal("extractor sanity: no Go files found under router/ or handler/")
	}

	type parsedFile struct {
		dir string
		rel string
		ast *ast.File
	}
	var parsed []parsedFile
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		rel := strings.TrimPrefix(filepath.ToSlash(path), filepath.ToSlash(root)+"/")
		dir := filepath.ToSlash(filepath.Dir(rel))
		idx.pkgName[dir] = f.Name.Name
		parsed = append(parsed, parsedFile{dir: dir, rel: rel, ast: f})
	}

	// Import aliases resolve after every package name is known, because the
	// alias defaults to the imported package's name (handler/proxy declares
	// package proxyhandler, not proxy).
	for _, pf := range parsed {
		if idx.imports[pf.dir] == nil {
			idx.imports[pf.dir] = map[string]string{}
		}
		for _, spec := range pf.ast.Imports {
			ipath, err := strconv.Unquote(spec.Path.Value)
			if err != nil || !strings.HasPrefix(ipath, modulePath+"/") {
				continue
			}
			target := strings.TrimPrefix(ipath, modulePath+"/")
			alias := target[strings.LastIndex(target, "/")+1:]
			if spec.Name != nil {
				alias = spec.Name.Name
			} else if known, ok := idx.pkgName[target]; ok {
				alias = known
			}
			idx.imports[pf.dir][alias] = target
		}
	}

	for _, pf := range parsed {
		if idx.funcs[pf.dir] == nil {
			idx.funcs[pf.dir] = map[string]*ast.FuncDecl{}
			idx.file[pf.dir] = map[string]string{}
		}
		for _, decl := range pf.ast.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			// Methods are skipped: in this repository every chi registration
			// literal lives in a top-level Register* function or router.New.
			if !ok || fn.Recv != nil || fn.Body == nil {
				continue
			}
			// A package may declare several init functions; they never
			// register routes.
			if fn.Name.Name == "init" || fn.Name.Name == "main" {
				continue
			}
			idx.funcs[pf.dir][fn.Name.Name] = fn
			idx.file[pf.dir][fn.Name.Name] = pf.rel
		}
	}
	return idx
}

// parseRouteInventory reads the `### VERB` + "- `path`" bullet structure of
// docs/api/routes-inventory.md into a "VERB /path" set.
func parseRouteInventory(t *testing.T, root string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "docs/api/routes-inventory.md"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	verb := ""
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### ") {
			verb = strings.ToUpper(strings.TrimSpace(trimmed[4:]))
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			verb = ""
			continue
		}
		if verb == "" || !strings.HasPrefix(trimmed, "- ") {
			continue
		}
		entry := strings.TrimSpace(trimmed[2:])
		if !strings.HasPrefix(entry, "`") {
			continue
		}
		end := strings.Index(entry[1:], "`")
		if end < 0 {
			continue
		}
		path := entry[1 : 1+end]
		if strings.HasPrefix(path, "/") {
			out[verb+" "+path] = true
		}
	}
	return out
}
