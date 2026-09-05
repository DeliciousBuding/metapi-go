package docs_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestUnexportedConstGroupMembersAreReferenced covers a blind spot in this
// repository's only dead-code gate.
//
// .golangci.yml enables `unused` (staticcheck U1000). U1000 does report an
// unreferenced unexported standalone `const`, an unreferenced member of a
// `var ( … )` group, an unreferenced member of a `type ( … )` group, and
// unreferenced unexported funcs, types, methods and struct fields.
//
// It does NOT report an unreferenced member of a `const ( … )` group. That was
// measured, not assumed: staticcheck v0.8.1, `staticcheck -checks=U1000` over a
// scratch package holding one dead declaration of each shape, reported
//
//	const deadConst · var deadVar · var(deadBlockVar) · type(deadBlockType) ·
//	type deadType · func deadFunc · func hasDeadMethod.deadMethod · field deadField
//
// and stayed silent on four variants of the grouped form: a dead member after a
// live one, a dead member in FIRST position, an entirely dead group, and a
// time.Duration member sharing a group with a live timeout constant. So the
// silence is the group syntax, not the position, the type, or the block being
// partly live.
//
// Version-exact confirmation from the real gate rather than a local proxy: CI's
// lint job (golangci-lint v1.64.8, `unused` enabled, and no --new-from-rev, so
// any single finding fails the job) passed on pull #1257 head 1acab2b0 — a tree
// that contained two unreferenced unexported const-group members,
// platform/siteProxyCacheTTL and service/oauth/antigravityModelsUserAgent. Both
// were retired in #1258 by a hand-built census in a private directory.
//
// Law 5 admission (a gate must come from a failure, not from tidiness): two dead
// constants shipped; the gate that appears to cover them did not fire; and the
// instrument that found them is a private script nothing runs. A private
// instrument that is not wired to anything lapses silently — the post-merge
// testbed verification entry point did exactly that in the same window, and six
// pull requests landed with no real-environment re-verification as a result.
//
// Scope is the blind spot and nothing wider: standalone consts stay owned by
// `unused`. Two owners for one rule is how the vocabularies folded in #1256 came
// to drift apart in the first place.
//
// Parsing is go/parser + go/ast, never regex. The census that found those two
// constants went through two broken revisions first, both from hand-rolled text
// parsing: a `\b` matcher that scored a field read through a longer identifier
// (h.loadGlobalAllowedModels() reading GlobalAllowedModels) as unreferenced, and
// a comment stripper that ate `sqlite://` inside a string literal, unbalanced a
// top-level `var (` group, and made the parser swallow the rest of the file —
// reporting the keywords `for` and `if` as dead declarations. An AST has neither
// failure mode: a name in a comment or a string literal is not an *ast.Ident.
func TestUnreferencedConstGroupMemberGate(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()

	prodByDir, testByDir, parsed := map[string][]*ast.File{}, map[string][]*ast.File{}, 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "dist", "vendor", ".dev-local", ".worktrees", "web":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			// A file that does not parse fails the gate. Skipping it silently is
			// how the literal gates in #1241 came to report "clean" while
			// scanning nothing at all.
			return fmt.Errorf("parse %s: %w", path, perr)
		}
		parsed++
		dir := filepath.Dir(path)
		if strings.HasSuffix(path, "_test.go") {
			testByDir[dir] = append(testByDir[dir], f)
		} else {
			prodByDir[dir] = append(prodByDir[dir], f)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}
	if parsed == 0 {
		t.Fatal("scanned 0 Go files — the walk is broken, not the repository")
	}

	findings := unreferencedConstGroupMembers(fset, prodByDir, testByDir)
	checked := 0
	for _, ds := range collectConstGroupMembers(fset, prodByDir) {
		checked += len(ds)
	}
	if checked == 0 {
		t.Fatalf("found 0 unexported members of `const ( … )` groups across %d parsed files — "+
			"the collector is broken, not the repository, and this gate would otherwise pass "+
			"forever while checking nothing", parsed)
	}

	t.Logf("gate scope: %d parsed Go files, %d packages with const groups, %d unexported "+
		"const-group members checked", parsed, len(collectConstGroupMembers(fset, prodByDir)), checked)
	if len(findings) > 0 {
		lines := make([]string, 0, len(findings))
		for _, f := range findings {
			lines = append(lines, fmt.Sprintf("%s:%d  %s  (package %s, %d references)",
				mustRel(t, root, f.file), f.line, f.name, filepath.Base(f.pkgDir), f.refs))
		}
		sort.Strings(lines)
		t.Fatalf("%d unexported members of a `const ( … )` group are declared and never "+
			"referenced anywhere in their own package, not even by a test:\n  %s\n\n"+
			"golangci-lint's `unused` will NOT report these (see the comment on this test): "+
			"the grouped const form is its blind spot, so this gate is the only thing that "+
			"notices. Either delete the constant or reference it. If one is deliberately "+
			"unreferenced, add it to allowlistedConstGroupMember with a reason — do not "+
			"widen or disable this gate.",
			len(findings), strings.Join(lines, "\n  "))
	}
}

// constFinding is one unreferenced const-group member, with enough context to
// name it in a failure message.
type constFinding struct {
	pkgDir string
	file   string
	name   string
	line   int
	refs   int
}

// unreferencedConstGroupMembers is the comparator. The repository gate above and
// the self-proof below both call THIS function, so the self-proof exercises the
// code that actually runs rather than a parallel restatement of the rule — the
// mistake #1240 found, where an anti-vacuity proof tested a function no gate
// called.
//
// The FileSet must be the one the files were parsed with: positions are offsets
// into it, so a fresh FileSet yields an empty filename and line 0 for every
// finding and the failure message cannot point at anything.
//
// References are counted over production AND test files of the same package
// directory, matching how `unused` treats production code: a helper only tests
// call still compiles and is still exercised, so calling it dead here would turn
// the gate red over legitimate code. Unexported names are package-scoped, so a
// sibling directory cannot reference one and none is consulted.
func unreferencedConstGroupMembers(fset *token.FileSet, prodByDir, testByDir map[string][]*ast.File) []constFinding {
	var out []constFinding
	for dir, ds := range collectConstGroupMembers(fset, prodByDir) {
		all := append(append([]*ast.File{}, prodByDir[dir]...), testByDir[dir]...)
		refs := countIdentRefs(all)
		for _, d := range ds {
			if allowlistedConstGroupMember(d.name) {
				continue
			}
			// refs counts every *ast.Ident occurrence and the declaration's own
			// name is one of them, so <= 1 means "declared and never used".
			if refs[d.name] <= 1 {
				out = append(out, constFinding{
					pkgDir: dir, file: d.file, name: d.name, line: d.line, refs: refs[d.name],
				})
			}
		}
	}
	return out
}

// constGroupMember is one unexported name declared inside a `const ( … )` group.
type constGroupMember struct {
	file string
	name string
	line int
}

// collectConstGroupMembers returns the unexported names declared in grouped
// const form, keyed by package directory.
//
// "Grouped" is decided by the AST, not by text: an ast.GenDecl with Tok ==
// token.CONST and a valid Lparen is exactly `const ( … )`. A standalone
// `const x = 1` has no Lparen and is left to `unused`.
func collectConstGroupMembers(fset *token.FileSet, prodByDir map[string][]*ast.File) map[string][]constGroupMember {
	out := map[string][]constGroupMember{}
	for dir, files := range prodByDir {
		for _, f := range files {
			for _, d := range f.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok || gd.Tok != token.CONST || !gd.Lparen.IsValid() {
					continue
				}
				for _, spec := range gd.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					for _, id := range vs.Names {
						// Exported names are `unused`'s documented non-goal and are
						// part of the package API; this gate is the const-group blind
						// spot for names nothing outside the package can reach. `_` is
						// a discard, not a declaration of anything.
						if id.IsExported() || id.Name == "_" {
							continue
						}
						// An *ast.Ident carries neither a filename nor a line number;
						// both come from the FileSet via NamePos.
						pos := fset.Position(id.NamePos)
						out[dir] = append(out[dir], constGroupMember{
							file: pos.Filename, name: id.Name, line: pos.Line,
						})
					}
				}
			}
		}
	}
	return out
}

// countIdentRefs counts every *ast.Ident occurrence by name across files.
func countIdentRefs(files []*ast.File) map[string]int {
	refs := map[string]int{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok {
				refs[id.Name]++
			}
			return true
		})
	}
	return refs
}

// allowlistedConstGroupMember names constants that are deliberately unreferenced.
//
// Empty on purpose. Every entry must carry the reason it is not simply dead, and
// an entry whose reason stops being true must be deleted — an allowlist that only
// grows is a dead-code ledger with extra steps.
func allowlistedConstGroupMember(name string) bool {
	switch name {
	default:
		return false
	}
}

// TestConstGroupGateIsNotAVacuousPass proves the gate above can go red, and pins
// its scope from both sides: it must catch the grouped form `unused` misses, and
// must NOT reach into what `unused` already owns or into legitimately-referenced
// code. Without the second half a gate that flags everything also "passes" this.
//
// Every case goes through unreferencedConstGroupMembers — the same function the
// repository gate calls.
func TestConstGroupGateIsNotAVacuousPass(t *testing.T) {
	// Control first: a clean package must come back empty, otherwise every
	// "detected" below could be a gate that flags everything.
	const cleanProd = `package p

const (
	liveA = 1
	liveB = 2
)

func Use() int { return liveA + liveB }
`
	if got := runComparator(t, map[string]string{"p/prod.go": cleanProd}, nil); len(got) != 0 {
		t.Fatalf("control sample must be clean, got %v", got)
	}

	cases := []struct {
		name string
		// filename -> source. A name ending in _test.go lands in the test set.
		files map[string]string
		// names that must be reported; both empty means "must be clean"
		wantNames []string
		// file suffixes a finding must appear in (package-scoping assertions)
		wantFiles []string
	}{
		{
			name: "dead member of a const group is the blind spot this gate exists for",
			files: map[string]string{"p/prod.go": `package p

const (
	liveA   = 1
	deadTTL = 3
)

func Use() int { return liveA }
`},
			wantNames: []string{"deadTTL"},
		},
		{
			name: "dead member in FIRST position, live one after it",
			files: map[string]string{"p/prod.go": `package p

const (
	deadFirst = 1
	liveAfter = 2
)

func Use() int { return liveAfter }
`},
			wantNames: []string{"deadFirst"},
		},
		{
			name: "an entirely dead group reports every member",
			files: map[string]string{"p/prod.go": `package p

const (
	deadAll1 = 1
	deadAll2 = 2
)

func Use() int { return 0 }
`},
			wantNames: []string{"deadAll1", "deadAll2"},
		},
		{
			name: "referenced only by a same-package test counts as referenced",
			files: map[string]string{
				"p/prod.go": `package p

const (
	seamOnly = 7
)
`,
				"p/prod_test.go": `package p

import "testing"

func TestSeam(t *testing.T) {
	if seamOnly != 7 {
		t.Fatal("x")
	}
}
`,
			},
			wantNames: nil,
		},
		{
			name: "a dead STANDALONE const is out of scope: unused already reports it",
			files: map[string]string{"p/prod.go": `package p

const deadStandalone = 1

func Use() int { return 0 }
`},
			wantNames: nil,
		},
		{
			name: "an exported group member is out of scope: it is package API",
			files: map[string]string{"p/prod.go": `package p

const (
	ExportedAndUnreferenced = 1
)
`},
			wantNames: nil,
		},
		{
			name: "the blank identifier in a group is a discard, not a declaration",
			files: map[string]string{"p/prod.go": `package p

const (
	_ = iota
	liveC
)

func Use() int { return liveC }
`},
			wantNames: nil,
		},
		{
			// This is the case hand-rolled text parsing got wrong twice in the
			// census this gate came from: a name inside a comment or a string
			// literal is not a reference. With an AST it is not an *ast.Ident,
			// so the member is correctly reported dead.
			name: "a name only in a comment and a string literal is still dead",
			files: map[string]string{"p/prod.go": `package p

const (
	// ghostConst is documented right here, and "ghostConst" also appears in
	// the string below, and sqlite:// must not be mistaken for a comment.
	ghostConst = "sqlite://ghostConst"
	liveD      = 1
)

func Use() int { return liveD }
`},
			wantNames: []string{"ghostConst"},
		},
		{
			name: "package scoping: the same name dead in one package, live in another",
			files: map[string]string{
				"a/prod.go": `package a

const (
	shared = 1
)

func Use() int { return 0 }
`,
				"b/prod.go": `package b

const (
	shared = 2
)

func Use() int { return shared }
`,
			},
			wantNames: []string{"shared"},
			wantFiles: []string{"a/prod.go"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runComparator(t, tc.files, nil)
			names := make([]string, 0, len(got))
			files := make([]string, 0, len(got))
			for _, f := range got {
				names = append(names, f.name)
				files = append(files, f.file)
			}
			if len(tc.wantNames) == 0 && len(tc.wantFiles) == 0 {
				if len(got) != 0 {
					t.Fatalf("expected a clean result, got findings: %v in %v", names, files)
				}
				return
			}
			// Exactly the expected names, and no others: a gate that flags
			// everything also "finds" the expected name, so the count matters.
			if len(got) != len(tc.wantNames) {
				t.Fatalf("expected exactly %d finding(s) %v, got %d: %v", len(tc.wantNames), tc.wantNames, len(got), names)
			}
			for _, w := range tc.wantNames {
				if !contains(names, w) {
					t.Fatalf("expected %q to be reported, got %v", w, names)
				}
			}
			for _, w := range tc.wantFiles {
				hit := false
				for _, f := range files {
					if strings.HasSuffix(f, w) {
						hit = true
					}
				}
				if !hit {
					t.Fatalf("expected the finding to be in %s, got %v", w, files)
				}
			}
		})
	}
}

// runComparator parses synthetic sources and feeds them through the same
// unreferencedConstGroupMembers the repository gate calls.
func runComparator(t *testing.T, files map[string]string, _ []string) []constFinding {
	t.Helper()
	fset := token.NewFileSet()
	prodByDir, testByDir := map[string][]*ast.File{}, map[string][]*ast.File{}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		f, err := parser.ParseFile(fset, name, files[name], 0)
		if err != nil {
			t.Fatalf("synthetic source %s does not parse: %v", name, err)
		}
		dir := filepath.Dir(name)
		if strings.HasSuffix(name, "_test.go") {
			testByDir[dir] = append(testByDir[dir], f)
		} else {
			prodByDir[dir] = append(prodByDir[dir], f)
		}
	}
	return unreferencedConstGroupMembers(fset, prodByDir, testByDir)
}

func contains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}
