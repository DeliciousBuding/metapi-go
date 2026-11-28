package docs_test

import (
	"fmt"
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

// Coupling ratchets freeze two concrete coupling surfaces so new code cannot
// widen them while existing files are refactored down:
//
//  1. Direct `github.com/jmoiron/sqlx` imports under `handler/admin/...`
//     (file-level ratchet). Note: this only guards direct sqlx imports; files
//     that reuse a *sqlx.DB field declared in another file of handler/admin
//     are not caught until those shared struct fields are retired.
//  2. References to the process-wide `store.GetDB` singleton (per-file
//     occurrence-count ratchet across all ownerPackage-scoped production files,
//     including `package store` same-package references, function values, and
//     dot imports, while respecting local variable/parameter/type shadowing and
//     ignoring struct/interface field declarations).
//
// Scope notes:
//   - Scans all non-test `.go` files not excluded by ownerPackage (every
//     top-level directory outside cmd/, web/, internal/, e2e/, docs/ and any
//     nested testdata/ tree; independent of GOOS/GOARCH build tags). The twelve
//     core domain groups in boundaryGroups are asserted non-zero.
//   - Every baseline entry is monotonic: adding a new file or increasing a
//     file's count fails the test, and shrinking or removing a listed entry
//     also fails until the baseline here is lowered to lock in the gain.

// legacyAdminSQLXFiles freezes handler/admin/... production files that directly
// import github.com/jmoiron/sqlx.
var legacyAdminSQLXFiles = map[string]bool{
	"handler/admin/account_tokens.go":         true,
	"handler/admin/account_tokens_sync.go":    true,
	"handler/admin/accounts.go":               true,
	"handler/admin/accounts_auth.go":          true,
	"handler/admin/accounts_health.go":        true,
	"handler/admin/channel_test_harness.go":   true,
	"handler/admin/downstream_keys.go":        true,
	"handler/admin/helpers.go":                true,
	"handler/admin/model_rates.go":            true,
	"handler/admin/model_redirects.go":        true,
	"handler/admin/model_refresh.go":          true,
	"handler/admin/search.go":                 true,
	"handler/admin/settings.go":               true,
	"handler/admin/settings_apply.go":         true,
	"handler/admin/settings_backup.go":        true,
	"handler/admin/settings_backup_webdav.go": true,
	"handler/admin/site_announcements.go":     true,
	"handler/admin/sites.go":                  true,
	"handler/admin/stats.go":                  true,
	"handler/admin/stats_heatmap.go":          true,
	"handler/admin/stats_helpers.go":          true,
	"handler/admin/tasks.go":                  true,
	"handler/admin/test.go":                   true,
	"handler/admin/token_routes.go":           true,
}

// legacyStoreGetDBCounts freezes the exact per-file count of store.GetDB
// references (including same-package calls inside package store). Increasing
// any count or adding a new file fails the gate; lowering a count fails until
// the ceiling here is lowered too.
var legacyStoreGetDBCounts = map[string]int{}

type couplingScan struct {
	scannedByGroup map[string]int
	adminSQLX      map[string]bool
	storeGetDB     map[string]int
}

func scanCouplingRatchets(root string) (couplingScan, error) {
	res := couplingScan{
		scannedByGroup: map[string]int{},
		adminSQLX:      map[string]bool{},
		storeGetDB:     map[string]int{},
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "dist" ||
				name == ".claude" || name == "worktrees" || name == ".worktrees" ||
				name == ".dev-local" || name == ".local" || name == "vendor" ||
				name == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
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
		fset := token.NewFileSet()
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return fmt.Errorf("parse %s: %w", path, perr)
		}
		topGroup := pkg
		if i := strings.IndexByte(pkg, '/'); i >= 0 {
			topGroup = pkg[:i]
		}
		res.scannedByGroup[topGroup]++
		relFilePath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relFile := filepath.ToSlash(relFilePath)

		storeAliases := map[string]bool{}
		unqualifiedGetDB := pkg == "store"
		for _, imp := range file.Imports {
			ipath, uerr := strconv.Unquote(imp.Path.Value)
			if uerr != nil {
				return fmt.Errorf("unquote import in %s: %w", path, uerr)
			}
			if pkgInSubtree(pkg, "handler/admin") && ipath == "github.com/jmoiron/sqlx" {
				res.adminSQLX[relFile] = true
			}
			if ipath == modulePath+"/store" {
				if imp.Name != nil {
					switch imp.Name.Name {
					case "_":
						continue
					case ".":
						unqualifiedGetDB = true
					default:
						storeAliases[imp.Name.Name] = true
					}
				} else {
					storeAliases["store"] = true
				}
			}
		}

		if len(storeAliases) > 0 || unqualifiedGetDB {
			if count := countStoreGetDBRefs(file, storeAliases, unqualifiedGetDB, pkg == "store"); count > 0 {
				res.storeGetDB[relFile] = count
			}
		}
		return nil
	})
	return res, err
}

// countStoreGetDBRefs counts references to store.GetDB (calls or function
// values, including dot imports and same-package calls inside package store)
// while skipping declarations (struct/interface fields, labels, type names,
// parameter/variable declarations) and identifiers shadowed by local or
// file-level value/type declarations.
func countStoreGetDBRefs(file *ast.File, storeAliases map[string]bool, unqualifiedGetDB, isStorePkg bool) int {
	// Collect top-level package declarations that shadow an import alias or GetDB.
	// Inside package store itself, `func GetDB()` is the target symbol, not a shadow.
	topDeclared := map[string]bool{}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil && d.Name != nil {
				if !(isStorePkg && d.Name.Name == "GetDB") {
					topDeclared[d.Name.Name] = true
				}
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.ValueSpec:
					for _, name := range s.Names {
						topDeclared[name.Name] = true
					}
				case *ast.TypeSpec:
					topDeclared[s.Name.Name] = true
				}
			}
		}
	}

	// Track identifiers that are NOT value references to an unqualified GetDB:
	// selector RHS (`x.GetDB`), composite-literal keys (`T{GetDB: v}`), struct
	// fields, interface methods, and labels.
	nonValueIdent := map[*ast.Ident]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectorExpr:
			nonValueIdent[node.Sel] = true
		case *ast.KeyValueExpr:
			if k, ok := node.Key.(*ast.Ident); ok {
				nonValueIdent[k] = true
			}
		case *ast.Field:
			for _, name := range node.Names {
				nonValueIdent[name] = true
			}
		case *ast.LabeledStmt:
			if node.Label != nil {
				nonValueIdent[node.Label] = true
			}
		case *ast.BranchStmt:
			if node.Label != nil {
				nonValueIdent[node.Label] = true
			}
		case *ast.FuncDecl:
			if node.Name != nil {
				nonValueIdent[node.Name] = true
			}
		case *ast.TypeSpec:
			if node.Name != nil {
				nonValueIdent[node.Name] = true
			}
		case *ast.ValueSpec:
			for _, name := range node.Names {
				nonValueIdent[name] = true
			}
		}
		return true
	})

	count := 0
	var walk func(n ast.Node, scopes []map[string]bool)
	isShadowed := func(name string, scopes []map[string]bool) bool {
		if topDeclared[name] {
			return true
		}
		for i := len(scopes) - 1; i >= 0; i-- {
			if scopes[i][name] {
				return true
			}
		}
		return false
	}
	addFieldNames := func(scope map[string]bool, list *ast.FieldList) {
		if list == nil {
			return
		}
		for _, f := range list.List {
			for _, name := range f.Names {
				if name != nil && name.Name != "_" {
					scope[name.Name] = true
				}
			}
		}
	}
	// addRecvTypeParams registers generic receiver type parameters (e.g.
	// `func (r *Reader[store, T]) ...`) in the method scope so type parameter
	// identifiers shadowing an import alias are not mistaken for the store package.
	addRecvTypeParams := func(scope map[string]bool, list *ast.FieldList) {
		if list == nil {
			return
		}
		for _, f := range list.List {
			typ := f.Type
			if star, ok := typ.(*ast.StarExpr); ok {
				typ = star.X
			}
			switch idx := typ.(type) {
			case *ast.IndexExpr:
				if id, ok := idx.Index.(*ast.Ident); ok && id.Name != "_" {
					scope[id.Name] = true
				}
			case *ast.IndexListExpr:
				for _, elem := range idx.Indices {
					if id, ok := elem.(*ast.Ident); ok && id.Name != "_" {
						scope[id.Name] = true
					}
				}
			}
		}
	}

	walk = func(n ast.Node, scopes []map[string]bool) {
		if n == nil {
			return
		}
		switch node := n.(type) {
		case *ast.FuncDecl:
			scope := map[string]bool{}
			addFieldNames(scope, node.Recv)
			addRecvTypeParams(scope, node.Recv)
			if node.Type != nil {
				addFieldNames(scope, node.Type.TypeParams)
				addFieldNames(scope, node.Type.Params)
				addFieldNames(scope, node.Type.Results)
			}
			next := append(scopes, scope)
			if node.Type != nil {
				walk(node.Type, next)
			}
			if node.Body != nil {
				walk(node.Body, next)
			}
			return
		case *ast.FuncLit:
			scope := map[string]bool{}
			if node.Type != nil {
				addFieldNames(scope, node.Type.Params)
				addFieldNames(scope, node.Type.Results)
			}
			next := append(scopes, scope)
			if node.Type != nil {
				walk(node.Type, next)
			}
			if node.Body != nil {
				walk(node.Body, next)
			}
			return
		case *ast.BlockStmt:
			scope := map[string]bool{}
			next := append(scopes, scope)
			for _, stmt := range node.List {
				walk(stmt, next)
			}
			return
		case *ast.IfStmt:
			scope := map[string]bool{}
			next := append(scopes, scope)
			walk(node.Init, next)
			walk(node.Cond, next)
			if node.Body != nil {
				walk(node.Body, next)
			}
			walk(node.Else, next)
			return
		case *ast.ForStmt:
			scope := map[string]bool{}
			next := append(scopes, scope)
			walk(node.Init, next)
			walk(node.Cond, next)
			walk(node.Post, next)
			if node.Body != nil {
				walk(node.Body, next)
			}
			return
		case *ast.RangeStmt:
			scope := map[string]bool{}
			next := append(scopes, scope)
			walk(node.X, scopes)
			if node.Tok == token.DEFINE {
				if id, ok := node.Key.(*ast.Ident); ok && id.Name != "_" {
					scope[id.Name] = true
				}
				if id, ok := node.Value.(*ast.Ident); ok && id.Name != "_" {
					scope[id.Name] = true
				}
			} else {
				// Non-defining range targets (e.g. `for m[store.GetDB()] = range xs`)
				// are arbitrary LHS expressions and must still be walked.
				walk(node.Key, scopes)
				walk(node.Value, scopes)
			}
			if node.Body != nil {
				walk(node.Body, next)
			}
			return
		case *ast.SwitchStmt:
			scope := map[string]bool{}
			next := append(scopes, scope)
			walk(node.Init, next)
			walk(node.Tag, next)
			if node.Body != nil {
				walk(node.Body, next)
			}
			return
		case *ast.TypeSwitchStmt:
			scope := map[string]bool{}
			next := append(scopes, scope)
			walk(node.Init, next)
			if assign, ok := node.Assign.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE {
				for _, rhs := range assign.Rhs {
					walk(rhs, next)
				}
				for _, lhs := range assign.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
						scope[id.Name] = true
					}
				}
			} else {
				walk(node.Assign, next)
			}
			if node.Body != nil {
				walk(node.Body, next)
			}
			return
		case *ast.CaseClause:
			for _, expr := range node.List {
				walk(expr, scopes)
			}
			scope := map[string]bool{}
			next := append(scopes, scope)
			for _, stmt := range node.Body {
				walk(stmt, next)
			}
			return
		case *ast.CommClause:
			scope := map[string]bool{}
			next := append(scopes, scope)
			walk(node.Comm, next)
			for _, stmt := range node.Body {
				walk(stmt, next)
			}
			return
		case *ast.AssignStmt:
			for _, rhs := range node.Rhs {
				walk(rhs, scopes)
			}
			if node.Tok == token.DEFINE && len(scopes) > 0 {
				top := scopes[len(scopes)-1]
				for _, lhs := range node.Lhs {
					if id, ok := lhs.(*ast.Ident); ok && id.Name != "_" {
						top[id.Name] = true
					}
				}
			} else {
				for _, lhs := range node.Lhs {
					walk(lhs, scopes)
				}
			}
			return
		case *ast.ValueSpec:
			if node.Type != nil {
				walk(node.Type, scopes)
			}
			for _, val := range node.Values {
				walk(val, scopes)
			}
			if len(scopes) > 0 {
				top := scopes[len(scopes)-1]
				for _, name := range node.Names {
					if name != nil && name.Name != "_" {
						top[name.Name] = true
					}
				}
			}
			return
		case *ast.TypeSpec:
			// Local `type store = ...` or `type GetDB ...` shadows the import
			// alias or dot-imported symbol in the enclosing block scope. Still
			// walk TypeParams and Type expressions (e.g. array length
			// expressions like `type T [len([1]func() *store.DB{store.GetDB})]int`).
			if len(scopes) > 0 && node.Name != nil && node.Name.Name != "_" {
				scopes[len(scopes)-1][node.Name.Name] = true
			}
			typeScope := map[string]bool{}
			addFieldNames(typeScope, node.TypeParams)
			next := append(scopes, typeScope)
			if node.TypeParams != nil {
				walk(node.TypeParams, next)
			}
			walk(node.Type, next)
			return
		case *ast.StructType:
			// Field names are excluded via nonValueIdent; still walk field types
			// because array-length expressions inside a field type may reference store.GetDB.
			if node.Fields != nil {
				for _, f := range node.Fields.List {
					walk(f.Type, scopes)
				}
			}
			return
		case *ast.InterfaceType:
			if node.Methods != nil {
				for _, m := range node.Methods.List {
					walk(m.Type, scopes)
				}
			}
			return
		case *ast.SelectorExpr:
			if ident, ok := node.X.(*ast.Ident); ok {
				if node.Sel.Name == "GetDB" && storeAliases[ident.Name] && !isShadowed(ident.Name, scopes) {
					count++
					return
				}
			}
			walk(node.X, scopes)
			return
		case *ast.Ident:
			if unqualifiedGetDB && node.Name == "GetDB" && !nonValueIdent[node] && !isShadowed("GetDB", scopes) {
				count++
			}
			return
		}

		ast.Inspect(n, func(child ast.Node) bool {
			if child == nil || child == n {
				return true
			}
			walk(child, scopes)
			return false
		})
	}

	for _, decl := range file.Decls {
		walk(decl, nil)
	}
	return count
}

func diffBoolRatchet(observed, allowed map[string]bool) (added, stale []string) {
	for f := range observed {
		if !allowed[f] {
			added = append(added, f)
		}
	}
	for f := range allowed {
		if !observed[f] {
			stale = append(stale, f)
		}
	}
	sort.Strings(added)
	sort.Strings(stale)
	return added, stale
}

func diffCountRatchet(observed, allowed map[string]int) (increased, stale []string) {
	for f, obs := range observed {
		lim, ok := allowed[f]
		if !ok {
			increased = append(increased, fmt.Sprintf("%s (observed %d, allowed 0)", f, obs))
		} else if obs > lim {
			increased = append(increased, fmt.Sprintf("%s (observed %d, allowed %d)", f, obs, lim))
		}
	}
	for f, lim := range allowed {
		obs := observed[f]
		if obs < lim {
			stale = append(stale, fmt.Sprintf("%s (observed %d, baseline %d)", f, obs, lim))
		}
	}
	sort.Strings(increased)
	sort.Strings(stale)
	return increased, stale
}

func TestCouplingRatchets(t *testing.T) {
	root := repoRoot(t)
	scan, err := scanCouplingRatchets(root)
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, group := range boundaryGroups {
		if scan.scannedByGroup[group] == 0 {
			missing = append(missing, group)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("gate would pass vacuously: no production Go files scanned for %v", missing)
	}

	if added, stale := diffBoolRatchet(scan.adminSQLX, legacyAdminSQLXFiles); len(added) > 0 || len(stale) > 0 {
		if len(added) > 0 {
			t.Errorf("handler/admin direct sqlx import ratchet tripped (%d new file(s)):\n  %s\nMove SQL queries into store/ or service/ instead of adding sqlx imports under handler/admin.",
				len(added), strings.Join(added, "\n  "))
		}
		if len(stale) > 0 {
			t.Errorf("handler/admin direct sqlx import ratchet has stale entries (%d file(s)) — remove them to lock in the improvement:\n  %s",
				len(stale), strings.Join(stale, "\n  "))
		}
	}

	if increased, stale := diffCountRatchet(scan.storeGetDB, legacyStoreGetDBCounts); len(increased) > 0 || len(stale) > 0 {
		if len(increased) > 0 {
			t.Errorf("store.GetDB singleton ratchet tripped (%d file(s)):\n  %s\nPass *store.DB explicitly via constructor or config struct instead of adding store.GetDB references.",
				len(increased), strings.Join(increased, "\n  "))
		}
		if len(stale) > 0 {
			t.Errorf("store.GetDB singleton ratchet has stale ceilings (%d file(s)) — lower or delete them to lock in the improvement:\n  %s",
				len(stale), strings.Join(stale, "\n  "))
		}
	}
}

func TestCouplingRatchetScannerDetectsEdgeCases(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"handler/admin/bad_sqlx.go": `package admin
import "github.com/jmoiron/sqlx"
var _ *sqlx.DB
`,
		"handler/admin/payloads/raw_sqlx.go": "package payloads\nimport _ `github.com/jmoiron/sqlx`\n",
		"handler/admin/clean.go": `package admin
func ok() {}
`,
		"service/alias_and_funcval.go": `package service
import dbstore "github.com/deliciousbuding/metapi-go/store"
var getter = dbstore.GetDB
func f(xs []int) {
	m := map[*dbstore.DB]int{}
	for m[dbstore.GetDB()] = range xs {}
	_ = dbstore.GetDB()
	_ = getter()
}
`,
		"service/dot_import.go": `package service
import . "github.com/deliciousbuding/metapi-go/store"
type Holder struct { GetDB int }
type Getter interface { GetDB() int }
func f() {
	h := Holder{GetDB: 1}
	_ = h.GetDB
	_ = GetDB()
	fn := GetDB
	_ = fn()
}
`,
		"service/shadowed.go": `package service
import "github.com/deliciousbuding/metapi-go/store"
type fakeStore struct{}
func (fakeStore) GetDB() int { return 0 }
type Reader[T interface{ GetDB() int }] struct{ value T }
func (r *Reader[store]) Read() int { return store.GetDB(r.value) }
func shadowedParam(store fakeStore) int { return store.GetDB() }
func shadowedLocal(s fakeStore) int {
	store := s
	return store.GetDB()
}
func shadowedTypeAlias() any {
	type store = fakeStore
	return store.GetDB
}
func realCall() { _ = store.GetDB() }
`,
		"service/type_expr_ref.go": `package service
import "github.com/deliciousbuding/metapi-go/store"
type Table [len([1]func() *store.DB{store.GetDB})]int
`,
		"store/internal_caller.go": `package store
func LoadSomething() { _ = GetDB() }
`,
		"store/bootstrap_def.go": `package store
func GetDB() *DB { return nil }
`,
		"service/testdata/ignored.go": `package testdata
import "github.com/deliciousbuding/metapi-go/store"
func f() { _ = store.GetDB() }
`,
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

	scan, err := scanCouplingRatchets(root)
	if err != nil {
		t.Fatal(err)
	}
	if scan.scannedByGroup["handler"] != 3 || scan.scannedByGroup["service"] != 4 || scan.scannedByGroup["store"] != 2 {
		t.Fatalf("unexpected scannedByGroup: %v", scan.scannedByGroup)
	}
	if !scan.adminSQLX["handler/admin/bad_sqlx.go"] || !scan.adminSQLX["handler/admin/payloads/raw_sqlx.go"] || scan.adminSQLX["handler/admin/clean.go"] {
		t.Fatalf("unexpected adminSQLX: %v", scan.adminSQLX)
	}
	if got := scan.storeGetDB["service/alias_and_funcval.go"]; got != 3 {
		t.Fatalf("alias_and_funcval count = %d, want 3 (var + range LHS + direct call)", got)
	}
	if got := scan.storeGetDB["service/dot_import.go"]; got != 2 {
		t.Fatalf("dot_import count = %d, want 2 (struct/interface field/key ignored)", got)
	}
	if got := scan.storeGetDB["service/shadowed.go"]; got != 1 {
		t.Fatalf("shadowed count = %d, want 1 (param/local/type-alias/generic-recv shadows ignored)", got)
	}
	if got := scan.storeGetDB["service/type_expr_ref.go"]; got != 1 {
		t.Fatalf("type_expr_ref count = %d, want 1 (array-len type expr detected)", got)
	}
	if got := scan.storeGetDB["store/internal_caller.go"]; got != 1 {
		t.Fatalf("store/internal_caller count = %d, want 1", got)
	}
	if got := scan.storeGetDB["store/bootstrap_def.go"]; got != 0 {
		t.Fatalf("store/bootstrap_def count = %d, want 0 (definition is not a call)", got)
	}
	if _, ok := scan.storeGetDB["service/testdata/ignored.go"]; ok {
		t.Fatalf("testdata file must be ignored: %v", scan.storeGetDB)
	}

	inc, stale := diffCountRatchet(
		map[string]int{"a.go": 3, "b.go": 1, "new.go": 1},
		map[string]int{"a.go": 2, "b.go": 2, "gone.go": 1},
	)
	if len(inc) != 2 || len(stale) != 2 {
		t.Fatalf("unexpected count ratchet diff: inc=%v stale=%v", inc, stale)
	}

	badRoot := t.TempDir()
	badFile := filepath.Join(badRoot, "service", "broken.go")
	if err := os.MkdirAll(filepath.Dir(badFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badFile, []byte("package service\nfunc broken(\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := scanCouplingRatchets(badRoot); err == nil {
		t.Fatal("expected parse error for malformed Go source, got nil")
	}
}
