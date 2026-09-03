package docs_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestPackageBoundaries encodes the forbidden-import hard rules from the
// as-built package map in docs/architecture.md as a machine assertion, so the
// package dependency discipline cannot silently drift.

// This is the Go analogue of a grep-based architecture boundary check
// (test.yml:134-148 "web handlers must route through commands/*_core"):
// every architecture-level boundary decision is frozen into a CI test.

// Rules enforced (denylist from the architecture contract):
//  1. store        ↛ handler, proxy, routing, service, scheduler, router, auth
//  2. platform     ↛ store, handler, proxy, router, scheduler   (config + proxy/profiles allowed)
//  3. transform    ↛ handler, store, proxy, routing, service, auth
//  4. routing      ↛ proxy, handler
//  5. service      ↛ handler, router, proxy
//  6. scheduler    ↛ handler, router, proxy
//  7. handler      ↛ router                                   (router mounts handlers, not reverse)
//  8. no revived TS-era top-level names proxycore/protocol

// Documented architecture exceptions are
// allowed and excluded from the denylist:
//   - handler/admin → scheduler (admin-ops cron validation only, §5.1)
//   - handler/admin → app (checkin schedule lifecycle, §5.2)
//   - app → handler/proxy (ConfigureProxyUpstream composition helper, §5.3)
//   - platform → proxy/profiles (profile detection, §3.2)
//   - handler/* → platform (thin admin/verify actions, §2.2 table)
//   - handler/proxy → transform/* (one-way protocol wiring, §5.4)
//   - auth → internal/sharedcount
//   - (scheduler → handler/shared §5.11 exception RESOLVED 2026-07-31:
//     scheduler now records DB-conn errors via the `app` facade, no direct
//     handler/shared import)

// cmd/server is the composition root and may import anything.
// cmd/migrate, e2e, internal, docs, web are out of scope.

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

func TestPackageBoundaries(t *testing.T) {
	root := repoRoot(t)
	violations, scanned := scanBoundaryViolations(t, root)
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

type violation struct {
	pkg  string
	file string
	imp  string
	rule string
}

// ownerPackage maps a directory (relative to repo root, slash-separated)
// to its owning Go package path suffix (e.g. "handler/admin"). Returns ""
// for directories out of scope (tests, cmd, web, internal, e2e, docs).
func ownerPackage(relDir string) (string, bool) {
	if relDir == "" || relDir == "." {
		return "", false
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
			if impTop == g || strings.HasPrefix(suffix, g+"/") {
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
		if inGroups("handler", "router", "proxy") {
			return "rule 5: service ↛ handler/router/proxy (domain free of HTTP/orchestration)"
		}
	case "scheduler":
		// scheduler must not depend on the HTTP/handler or routing layers.
		// DB-connection error metrics are recorded via the `app` facade
		// (app.RecordDBConnError delegates to handler/shared), not by
		// importing handler/shared directly — this resolved the §5.11
		// exception (was: scheduler/lease.go imported handler/shared).
		if inGroups("handler", "router", "proxy") {
			return "rule 6: scheduler ↛ handler/router/proxy (DB-conn metric via app facade, not handler/shared)"
		}
	case "handler":
		// handler/admin documented exceptions: → scheduler (admin-ops), → app (lifecycle).
		if pkg == "handler/admin" && (suffix == "scheduler" || impTop == "scheduler") {
			return "" // §5.1 admin-ops cron validation exception
		}
		if pkg == "handler/admin" && impTop == "app" {
			return "" // §5.2 checkin schedule lifecycle exception
		}
		// handler → router forbidden (router mounts handlers).
		if impTop == "router" {
			return "rule 7: handler ↛ router (router mounts handlers, not reverse)"
		}
	}
	return ""
}

func scanBoundaryViolations(t *testing.T, root string) ([]violation, map[string]int) {
	t.Helper()
	var out []violation
	scanned := map[string]int{}
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
				name == ".dev-local" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		// Only production (non-test) .go files.
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relDir := filepath.ToSlash(mustRel(t, root, filepath.Dir(path)))
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
			// Unparseable (build-tagged, generated): skip rather than fail the gate.
			return nil
		}
		for _, imp := range f.Imports {
			ipath := strings.Trim(imp.Path.Value, `"`)
			if rule := forbiddenImport(pkg, ipath); rule != "" {
				out = append(out, violation{
					pkg:  pkg,
					file: filepath.ToSlash(mustRel(t, root, path)),
					imp:  ipath,
					rule: rule,
				})
			}
		}
		_ = ast.Print // keep ast import used if extended later
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out, scanned
}
