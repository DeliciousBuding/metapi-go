package docs_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestPackageBoundaries encodes the forbidden-import hard rules from the
// as-built package map in docs/architecture.md as a machine assertion, so the
// package dependency discipline cannot silently drift.
//
// Scope notes:
//   - Scans all non-test `.go` files not excluded by ownerPackage (every
//     top-level directory outside cmd/, web/, internal/, e2e/, docs/ and any
//     nested testdata/ tree; independent of current GOOS/GOARCH build tags,
//     parsing package headers and imports). The twelve core domain packages in
//     boundaryGroups are additionally asserted to have non-zero scanned counts.
//   - TestProductionBinariesDoNotDependOnTesting separately verifies via
//     `go list -e -json` that `cmd/server`, `cmd/migrate`, and
//     `cmd/channel-plugin-maintain` resolve cleanly (allowing only the missing
//     embedded `web/dist` error in unbuilt worktrees) and do not transitively
//     depend on `testing` or `net/http/httptest` (covering `internal/` packages
//     linked into shipped binaries).

// Rules enforced (denylist from the architecture contract):
//  1. store        ↛ handler, proxy, routing, service, scheduler, router, auth,
//                    or unapproved internal/* business packages
//  2. platform     ↛ store, handler, proxy, router, scheduler   (config + proxy/profiles allowed)
//  3. transform    ↛ handler, store, proxy, routing, service, auth
//  4. routing      ↛ proxy, handler
//  5. service      ↛ handler, router, proxy, scheduler
//  6. scheduler    ↛ handler, router, proxy
//  7. handler      ↛ router                                   (router mounts handlers, not reverse)
//  8. no revived TS-era top-level names proxycore/protocol
//  9. no production file imports "testing" or "net/http/httptest"
// 10. handler/proxy ↛ scheduler; handler/admin ↛ handler/proxy

// Documented architecture exceptions are allowed and excluded from the denylist:
//   - handler/admin → scheduler (admin-ops cron validation only)
//   - handler/admin → app (checkin schedule lifecycle)
//   - app → handler/proxy (ConfigureProxyUpstream composition helper)
//   - platform → proxy/profiles (profile detection)
//   - handler/* → platform (thin admin/verify actions)
//   - handler/proxy → transform/* (one-way protocol wiring)
//   - auth → internal/sharedcount
//   - file-scoped legacy exceptions in legacyBoundaryExceptions (ratcheted:
//     each entry must remain live; removing an edge requires deleting its entry)

// When this test fails: do NOT relax it. Either move the import to an
// allowed edge, or document a new exception in docs/architecture.md + this
// file with justification.

const modulePath = "github.com/deliciousbuding/metapi-go"

// boundaryGroups are the twelve top-level domain packages the denylist below is
// keyed on. TestPackageBoundaries asserts each one was really scanned, so a
// future layout move that forgets this file fails loudly instead of passing
// vacuously. A gate that scans nothing and reports no violations is not a
// lenient gate, it is an absent one: that is the exact shape that let roughly
// thirty release tags go green while their required shards ran zero tests.
var boundaryGroups = []string{
	"app", "auth", "config", "handler", "platform", "proxy", "router",
	"routing", "scheduler", "service", "store", "transform",
}

// storeAllowedInternalImports is the explicit allowlist of internal/* packages
// that store may import (narrow wire/config contract leaves). Any new
// store → internal/* edge must be reviewed instead of slipping past the
// top-level denylist.
var storeAllowedInternalImports = map[string]string{
	"internal/channellimit":           "persisted direct channel rate-limit config types",
	"internal/channelplugin/contract": "persisted channel plugin manifest/version contract",
	"internal/requestoverride":        "persisted DirectRequestConfig operation types",
	"internal/routecondition":         "persisted direct association condition AST",
}

type legacyEdgeKey struct {
	file string
	imp  string
}

// legacyBoundaryExceptions freezes pre-existing cross-layer edges so they
// cannot grow. Every entry is checked for staleness: once refactored away, the
// gate fails until the obsolete exemption is deleted here.
var legacyBoundaryExceptions = map[legacyEdgeKey]string{
	{file: "service/settingsmigration/service.go", imp: modulePath + "/scheduler"}: "uses scheduler.ScheduleSpec/CronToSchedule until schedule spec lives in a shared leaf",
}

func TestPackageBoundaries(t *testing.T) {
	root := repoRoot(t)
	violations, scanned, matchedLegacy, matchedStoreInternal, err := scanBoundaryViolations(root)
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, group := range boundaryGroups {
		if scanned[group] == 0 {
			missing = append(missing, group)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("gate would pass vacuously: no production Go files scanned for %v. "+
			"The layout moved without updating ownerPackage/boundaryGroups in this file.", missing)
	}
	var stale []string
	for key := range legacyBoundaryExceptions {
		if !matchedLegacy[key] {
			stale = append(stale, fmt.Sprintf("legacy exception %s -> %s", key.file, key.imp))
		}
	}
	for imp := range storeAllowedInternalImports {
		if !matchedStoreInternal[imp] {
			stale = append(stale, fmt.Sprintf("store internal allowlist entry %s", imp))
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Fatalf("stale package boundary exemptions must be removed (ratchet):\n  %s", strings.Join(stale, "\n  "))
	}
	if len(violations) > 0 {
		sort.Slice(violations, func(i, j int) bool {
			if violations[i].pkg != violations[j].pkg {
				return violations[i].pkg < violations[j].pkg
			}
			return violations[i].file < violations[j].file
		})
		var b strings.Builder
		b.WriteString("package boundary violations (docs/architecture.md contract):\n")
		for _, v := range violations {
			b.WriteString("  ")
			b.WriteString(v.file)
			b.WriteString(": package ")
			b.WriteString(v.pkg)
			b.WriteString(" imports forbidden ")
			b.WriteString(v.imp)
			b.WriteString(" (rule: ")
			b.WriteString(v.rule)
			b.WriteString(")\n")
		}
		b.WriteString("\nDo not relax this test. Move the import to an allowed edge or document a new exception in docs/architecture.md + this file.")
		t.Fatal(b.String())
	}
}

type goListPkgError struct {
	ImportStack []string `json:"ImportStack"`
	Err         string   `json:"Err"`
}

type goListPackage struct {
	ImportPath string           `json:"ImportPath"`
	Deps       []string         `json:"Deps"`
	Incomplete bool             `json:"Incomplete"`
	Error      *goListPkgError  `json:"Error"`
	DepsErrors []goListPkgError `json:"DepsErrors"`
}

func isAllowedMissingWebDistError(e goListPkgError) bool {
	if len(e.ImportStack) == 0 || e.ImportStack[len(e.ImportStack)-1] != modulePath+"/web" {
		return false
	}
	return strings.Contains(e.Err, "pattern dist: no matching files found")
}

func validateProductionBinaryDepsJSON(data []byte, wantTargets []string) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var pkgs []goListPackage
	for {
		var p goListPackage
		err := dec.Decode(&p)
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("decode go list json: %w", err)
		}
		pkgs = append(pkgs, p)
	}
	wantSet := make(map[string]bool, len(wantTargets))
	for _, tgt := range wantTargets {
		wantSet[modulePath+"/"+strings.TrimPrefix(tgt, "./")] = true
	}

	seen := map[string]bool{}
	var issues []string
	for _, p := range pkgs {
		if !wantSet[p.ImportPath] {
			continue
		}
		seen[p.ImportPath] = true
		if p.Error != nil {
			issues = append(issues, fmt.Sprintf("%s load error: %s", p.ImportPath, p.Error.Err))
		}
		unapprovedDepErr := false
		for _, de := range p.DepsErrors {
			if isAllowedMissingWebDistError(de) {
				continue
			}
			unapprovedDepErr = true
			issues = append(issues, fmt.Sprintf("%s dep error (%s): %s", p.ImportPath, strings.Join(de.ImportStack, " -> "), de.Err))
		}
		if p.Incomplete && !unapprovedDepErr && p.Error == nil {
			// Verify Incomplete was solely explained by the allowed missing web/dist error.
			if len(p.DepsErrors) == 0 {
				issues = append(issues, fmt.Sprintf("%s marked incomplete with no explained DepsErrors", p.ImportPath))
			}
		}
		if len(p.Deps) == 0 {
			issues = append(issues, fmt.Sprintf("%s resolved empty Deps", p.ImportPath))
		}
		for _, dep := range p.Deps {
			if dep == "testing" || dep == "net/http/httptest" {
				issues = append(issues, fmt.Sprintf("%s transitively imports %s", p.ImportPath, dep))
			}
		}
	}
	for tgt := range wantSet {
		if !seen[tgt] {
			issues = append(issues, fmt.Sprintf("missing go list output for %s", tgt))
		}
	}
	sort.Strings(issues)
	if len(issues) > 0 {
		return fmt.Errorf("production binary dependency check failed:\n  %s", strings.Join(issues, "\n  "))
	}
	return nil
}

// TestProductionBinariesDoNotDependOnTesting checks the actual transitive Go
// package dependency graph of shipped cmd/* binaries so an internal/ package
// linked by production code cannot reintroduce testing or net/http/httptest,
// while failing on any unexpected package/import resolution error.
func TestProductionBinariesDoNotDependOnTesting(t *testing.T) {
	root := repoRoot(t)
	targets := []string{"./cmd/server", "./cmd/migrate", "./cmd/channel-plugin-maintain"}
	// Use -e -json so an unbuilt worktree lacking web/dist still resolves the
	// full Go package graph; validateProductionBinaryDepsJSON strictly permits
	// only the missing embedded web/dist error and rejects every other error.
	args := append([]string{"list", "-e", "-json"}, targets...)
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list %v failed: %v\nstderr:\n%s", targets, err, stderr.String())
	}
	if err := validateProductionBinaryDepsJSON(stdout.Bytes(), targets); err != nil {
		t.Fatal(err)
	}
}

func TestProductionBinaryDepsValidatorRejectsBrokenGraphAndTestDeps(t *testing.T) {
	targets := []string{"./cmd/server", "./cmd/migrate"}
	good := `
{"ImportPath":"github.com/deliciousbuding/metapi-go/cmd/server","Deps":["fmt","net/http"],"Incomplete":true,"DepsErrors":[{"ImportStack":["github.com/deliciousbuding/metapi-go/cmd/server","github.com/deliciousbuding/metapi-go/web"],"Err":"pattern dist: no matching files found"}]}
{"ImportPath":"github.com/deliciousbuding/metapi-go/cmd/migrate","Deps":["fmt"]}
`
	if err := validateProductionBinaryDepsJSON([]byte(good), targets); err != nil {
		t.Fatalf("expected valid graph with allowed missing web/dist to pass, got: %v", err)
	}

	brokenDep := `
{"ImportPath":"github.com/deliciousbuding/metapi-go/cmd/server","Deps":["fmt"]}
{"ImportPath":"github.com/deliciousbuding/metapi-go/cmd/migrate","Deps":["fmt"],"Incomplete":true,"DepsErrors":[{"ImportStack":["github.com/deliciousbuding/metapi-go/cmd/migrate","github.com/deliciousbuding/metapi-go/nonexistent"],"Err":"no required module provides package"}]}
`
	if err := validateProductionBinaryDepsJSON([]byte(brokenDep), targets); err == nil {
		t.Fatal("expected broken dependency graph to fail, got nil")
	}

	leakedTesting := `
{"ImportPath":"github.com/deliciousbuding/metapi-go/cmd/server","Deps":["fmt","net/http/httptest"]}
{"ImportPath":"github.com/deliciousbuding/metapi-go/cmd/migrate","Deps":["fmt"]}
`
	if err := validateProductionBinaryDepsJSON([]byte(leakedTesting), targets); err == nil {
		t.Fatal("expected leaked net/http/httptest dep to fail, got nil")
	}
}

type violation struct {
	pkg  string
	file string
	imp  string
	rule string
}

// pkgInSubtree reports whether pkg is equal to prefix or nested under prefix/.
func pkgInSubtree(pkg, prefix string) bool {
	return pkg == prefix || strings.HasPrefix(pkg, prefix+"/")
}

// ownerPackage maps a directory (relative to repo root, slash-separated)
// to its owning Go package path suffix (e.g. "handler/admin"). Returns ""
// for directories out of scope (testdata, cmd, web, internal, e2e, docs).
func ownerPackage(relDir string) (string, bool) {
	if relDir == "" || relDir == "." {
		return "", false
	}
	for _, seg := range strings.Split(relDir, "/") {
		if seg == "testdata" {
			return "", false
		}
	}
	// Out-of-scope trees.
	top := relDir
	if i := strings.IndexByte(relDir, '/'); i >= 0 {
		top = relDir[:i]
	}
	switch top {
	case "cmd", "web", "internal", "e2e", "docs", "node_modules", ".git",
		".claude", "worktrees", "dist":
		return "", false
	}
	return relDir, true
}

// forbiddenImport returns the rule a package violates by importing `imp`,
// or "" if the import is allowed (or not an internal module import).
func forbiddenImport(pkg, imp string) string {
	// Rule 9 is checked before the module-path guard below because "testing" and
	// "net/http/httptest" are stdlib, and that guard returns "" for every stdlib
	// import.
	//
	// A test helper or httptest call living in a production file puts `testing`
	// into the production dependency graph of every package that imports it, and
	// the linker then carries it into the shipped binary. Measured twice:
	// platform/ put `testing` into six graphs (+11,724 bytes), and
	// handler/proxy/responses_ws.go importing net/http/httptest pulled both
	// net/http/httptest and testing back into cmd/server until replaced with an
	// in-process response recorder.
	//
	// internal/pgtest and internal/golden are test-only packages; shipped
	// binaries are separately checked by TestProductionBinariesDoNotDependOnTesting.
	if imp == "testing" || imp == "net/http/httptest" {
		return "rule 9: a production file must not import testing or net/http/httptest — move helpers to _test.go or use stdlib net/http"
	}
	if !strings.HasPrefix(imp, modulePath+"/") {
		return "" // external or stdlib
	}
	suffix := strings.TrimPrefix(imp, modulePath+"/")
	if suffix == pkg {
		return "" // self-import (same package)
	}
	// Top-level category of the importer.
	pkgTop := pkg
	if i := strings.IndexByte(pkg, '/'); i >= 0 {
		pkgTop = pkg[:i]
	}
	impTop := suffix
	if i := strings.IndexByte(suffix, '/'); i >= 0 {
		impTop = suffix[:i]
	}

	// Rule 8: no revived TS-era top-level names.
	if impTop == "proxycore" || impTop == "protocol" {
		return "rule 8: no revived proxycore/protocol top-level names"
	}

	// Helper: does imp target one of these top-level groups?
	inGroups := func(groups ...string) bool {
		for _, g := range groups {
			if pkgInSubtree(suffix, g) {
				return true
			}
		}
		return false
	}

	switch pkgTop {
	case "store":
		if inGroups("handler", "proxy", "routing", "service", "scheduler", "router", "auth") {
			return "rule 1: store ↛ upper layers"
		}
		if impTop == "internal" {
			if _, ok := storeAllowedInternalImports[suffix]; !ok {
				return "rule 1: store ↛ unapproved internal/* package (keep persistence leaf free of domain logic)"
			}
		}
	case "platform":
		// Allowed: config, proxy/profiles. Denied: store/handler/proxy/router/scheduler.
		if suffix == "config" || suffix == "proxy/profiles" || suffix == "proxy/types" {
			return ""
		}
		if inGroups("store", "handler", "proxy", "router", "scheduler") {
			return "rule 2: platform ↛ store/handler/proxy/router/scheduler (only config + proxy/profiles)"
		}
	case "transform":
		// Allowed: transform/shared, and same-protocol siblings.
		if impTop == "transform" {
			return "" // leaf cluster
		}
		if inGroups("handler", "store", "proxy", "routing", "service", "auth") {
			return "rule 3: transform ↛ upper layers (leaf protocol cluster)"
		}
	case "routing":
		if inGroups("proxy", "handler") {
			return "rule 4: routing ↛ proxy/handler (selection stays pure)"
		}
	case "service":
		if inGroups("handler", "router", "proxy", "scheduler") {
			return "rule 5: service ↛ handler/router/proxy/scheduler (domain free of HTTP/orchestration/scheduling)"
		}
	case "scheduler":
		// scheduler must not depend on the HTTP/handler or routing layers.
		// DB-connection error counters live in app/observability, a stdlib-only
		// leaf, so scheduler/lease.go needs no handler/shared import.
		if inGroups("handler", "router", "proxy") {
			return "rule 6: scheduler ↛ handler/router/proxy (DB-conn counters go through app/observability)"
		}
	case "handler":
		// handler/admin documented exceptions: → scheduler (admin-ops), → app (lifecycle).
		if pkgInSubtree(pkg, "handler/admin") && impTop == "scheduler" {
			return "" // admin-ops cron validation exception
		}
		if pkgInSubtree(pkg, "handler/admin") && impTop == "app" {
			return "" // checkin schedule lifecycle exception
		}
		if pkgInSubtree(pkg, "handler/proxy") && impTop == "scheduler" {
			return "rule 10: handler/proxy ↛ scheduler (data-plane HTTP surface must not depend on cron layer)"
		}
		if pkgInSubtree(pkg, "handler/admin") && pkgInSubtree(suffix, "handler/proxy") {
			return "rule 10: handler/admin ↛ handler/proxy (extract shared policy/helpers to a lower layer)"
		}
		// handler → router forbidden (router mounts handlers, not reverse).
		if impTop == "router" {
			return "rule 7: handler ↛ router (router mounts handlers, not reverse)"
		}
	}
	return ""
}

func scanBoundaryViolations(root string) ([]violation, map[string]int, map[legacyEdgeKey]bool, map[string]bool, error) {
	var out []violation
	scanned := map[string]int{}
	matchedLegacy := map[legacyEdgeKey]bool{}
	matchedStoreInternal := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			// .worktrees holds full checkouts of this same repository, so scanning
			// it means parsing a second copy of every backend package and reporting
			// a work-in-progress branch's violations under a confusing path: with
			// two worktrees checked out the walk covered 732 production files
			// against 366 in the repository proper. .dev-local is the private
			// QA/runbook area, where a stray .go file is not production
			// architecture and must not be treated as one.
			if name == ".git" || name == "node_modules" || name == "dist" ||
				name == ".claude" || name == "worktrees" || name == ".worktrees" ||
				name == ".dev-local" || name == ".local" || name == "vendor" ||
				name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		// Only production (non-test) .go files.
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relDirPath, err := filepath.Rel(root, filepath.Dir(path))
		if err != nil {
			return err
		}
		relDir := filepath.ToSlash(relDirPath)
		pkg, ok := ownerPackage(relDir)
		if !ok {
			return nil
		}
		if i := strings.IndexByte(pkg, '/'); i >= 0 {
			scanned[pkg[:i]]++
		} else {
			scanned[pkg]++
		}
		fset := token.NewFileSet()
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", path, perr)
		}
		relFilePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relFile := filepath.ToSlash(relFilePath)
		for _, imp := range f.Imports {
			ipath, uerr := strconv.Unquote(imp.Path.Value)
			if uerr != nil {
				return fmt.Errorf("unquote import in %s: %w", path, uerr)
			}
			if pkgInSubtree(pkg, "store") && strings.HasPrefix(ipath, modulePath+"/internal/") {
				suffix := strings.TrimPrefix(ipath, modulePath+"/")
				if _, allowed := storeAllowedInternalImports[suffix]; allowed {
					matchedStoreInternal[suffix] = true
				}
			}
			if rule := forbiddenImport(pkg, ipath); rule != "" {
				key := legacyEdgeKey{file: relFile, imp: ipath}
				if _, ok := legacyBoundaryExceptions[key]; ok {
					matchedLegacy[key] = true
					continue
				}
				out = append(out, violation{
					pkg:  pkg,
					file: relFile,
					imp:  ipath,
					rule: rule,
				})
			}
		}
		return nil
	})
	return out, scanned, matchedLegacy, matchedStoreInternal, err
}

func TestPackageBoundaryScanIgnoresLocalDrafts(t *testing.T) {
	for _, dir := range []string{".local", "service/testdata", "store", "local"} {
		t.Run(dir, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(dir), "draft.go")
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("package draft\nimport _ \"testing\"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			violations, scanned, _, _, err := scanBoundaryViolations(root)
			if err != nil {
				t.Fatal(err)
			}
			if dir == ".local" || dir == "service/testdata" {
				if len(violations) != 0 || len(scanned) != 0 {
					t.Fatalf("out-of-scope dir %s entered the production scan: violations=%v scanned=%v", dir, violations, scanned)
				}
				return
			}
			if len(violations) != 1 || violations[0].imp != "testing" || scanned[dir] != 1 {
				t.Fatalf("public copy must trigger the production-import gate: violations=%v scanned=%v", violations, scanned)
			}
		})
	}
}

func TestPackageBoundaryRulesCatchKnownBadEdges(t *testing.T) {
	cases := []struct {
		name    string
		pkg     string
		imp     string
		wantHit bool
	}{
		{name: "production httptest rejected", pkg: "handler/proxy", imp: "net/http/httptest", wantHit: true},
		{name: "production testing rejected", pkg: "store", imp: "testing", wantHit: true},
		{name: "handler proxy to scheduler rejected", pkg: "handler/proxy", imp: modulePath + "/scheduler", wantHit: true},
		{name: "handler admin to handler proxy rejected", pkg: "handler/admin", imp: modulePath + "/handler/proxy", wantHit: true},
		{name: "handler admin subpackage to handler proxy rejected", pkg: "handler/admin/payloads", imp: modulePath + "/handler/proxy", wantHit: true},
		{name: "service to scheduler rejected", pkg: "service/foo", imp: modulePath + "/scheduler", wantHit: true},
		{name: "store to unapproved internal rejected", pkg: "store", imp: modulePath + "/internal/httpclient", wantHit: true},
		{name: "store to approved internal contract allowed", pkg: "store", imp: modulePath + "/internal/requestoverride", wantHit: false},
		{name: "handler proxy to transform allowed", pkg: "handler/proxy", imp: modulePath + "/transform/shared", wantHit: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := forbiddenImport(tc.pkg, tc.imp)
			if tc.wantHit && got == "" {
				t.Fatalf("expected forbiddenImport(%q, %q) to reject, got allowed", tc.pkg, tc.imp)
			}
			if !tc.wantHit && got != "" {
				t.Fatalf("expected forbiddenImport(%q, %q) to allow, got %q", tc.pkg, tc.imp, got)
			}
		})
	}
}

func TestPackageBoundaryScanHandlesRawStringImportsAndSubpackages(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"handler/admin/payloads/bad.go": "package payloads\nimport _ `" + modulePath + "/handler/proxy`\n",
		"store/raw_httptest.go":         "package store\nimport _ `net/http/httptest`\n",
	}
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	violations, _, _, _, err := scanBoundaryViolations(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 2 {
		t.Fatalf("expected 2 violations for raw-string / subpackage imports, got %v", violations)
	}
}

func TestPackageBoundaryScanFailsOnMalformedSource(t *testing.T) {
	root := t.TempDir()
	badPath := filepath.Join(root, "store", "broken.go")
	if err := os.MkdirAll(filepath.Dir(badPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badPath, []byte("package store\nimport (\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, _, err := scanBoundaryViolations(root); err == nil {
		t.Fatal("expected parse error for malformed Go source, got nil")
	}
}
