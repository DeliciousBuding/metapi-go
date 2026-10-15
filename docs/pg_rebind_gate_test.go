package docs

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// Check literal SQL arguments in production files mentioning *sqlx.DB, which
// unlike store.DB does not rebind automatically. This file-level heuristic
// recognizes only directly declared *store.DB parameters and store.GetDB locals
// as safe wrappers, not general receiver types, query variables/builders, or
// cross-file handles; PostgreSQL integration tests own those cases.
func TestPgRebindGateNoBareQuestionMarks(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(filepath.Dir(thisFile))
	dirs := []string{"app", "auth", "cmd", "config", "e2e", "handler", "platform", "proxy", "routing", "scheduler", "service", "store", "transform"}
	fset := token.NewFileSet()
	var violations []string
	var scannedFiles, sqlxFiles int
	for _, dir := range dirs {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			file, err := parser.ParseFile(fset, path, src, 0)
			if err != nil {
				return err
			}
			scannedFiles++
			if !mentionsBareSQLXDB(file) {
				return nil
			}
			sqlxFiles++
			for _, pos := range bareSQLPlaceholders(file) {
				position := fset.Position(pos)
				rel, err := filepath.Rel(root, position.Filename)
				if err != nil {
					return err
				}
				violations = append(violations, fmt.Sprintf("%s:%d", filepath.ToSlash(rel), position.Line))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", dir, err)
		}
	}
	if scannedFiles == 0 || sqlxFiles == 0 {
		t.Fatalf("empty scan surface: %d production files, %d mentioning *sqlx.DB", scannedFiles, sqlxFiles)
	}
	if len(violations) > 0 {
		t.Fatalf("literal SQL has bare ? placeholders in files handling *sqlx.DB; use Rebind (verify the receiver is not store.DB):\n%s", strings.Join(violations, "\n"))
	}
}

func mentionsBareSQLXDB(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		star, ok := node.(*ast.StarExpr)
		if !ok {
			return true
		}
		selector, ok := star.X.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != "DB" {
			return true
		}
		ident, ok := selector.X.(*ast.Ident)
		found = found || ok && ident.Name == "sqlx"
		return true
	})
	return found
}

// SQL argument positions differ for execution and destination-taking methods.
// AST calls cover selector receivers and gofmt's multiline call/literal forms.
func bareSQLPlaceholders(file *ast.File) []token.Pos {
	var findings []token.Pos
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if isDirectStoreDB(selector.X) {
			return true
		}
		var index int
		switch selector.Sel.Name {
		case "Exec", "Query", "QueryRow", "QueryRowx", "Queryx":
			index = 0
		case "Get", "Select", "ExecContext", "QueryContext", "QueryRowContext", "QueryRowxContext", "QueryxContext":
			index = 1
		case "GetContext", "SelectContext":
			index = 2
		default:
			return true
		}
		if len(call.Args) <= index {
			return true
		}
		literal, ok := call.Args[index].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		} // Nonliteral SQL (including Rebind calls) is outside this gate.
		query, err := strconv.Unquote(literal.Value)
		if err == nil && hasSQLPlaceholder(query) {
			findings = append(findings, literal.Pos())
		}
		return true
	})
	return findings
}

// Use lexical bindings, not a variable-name allowlist: a sqlx.DB called dbw is
// still checked, while a directly obtained store.DB is not.
func isDirectStoreDB(receiver ast.Expr) bool {
	ident, ok := receiver.(*ast.Ident)
	if !ok || ident.Obj == nil {
		return false
	}
	switch declaration := ident.Obj.Decl.(type) {
	case *ast.Field:
		pointer, ok := declaration.Type.(*ast.StarExpr)
		return ok && isStoreSelector(pointer.X, "DB")
	case *ast.AssignStmt:
		for i, lhs := range declaration.Lhs {
			name, ok := lhs.(*ast.Ident)
			if !ok || name.Obj != ident.Obj || i >= len(declaration.Rhs) {
				continue
			}
			call, ok := declaration.Rhs[i].(*ast.CallExpr)
			return ok && len(call.Args) == 0 && isStoreSelector(call.Fun, "GetDB")
		}
	}
	return false
}

func isStoreSelector(expression ast.Expr, name string) bool {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	ident, ok := selector.X.(*ast.Ident)
	return ok && ident.Name == "store" && ident.Obj == nil
}

// Ignore question marks in quoted SQL values/identifiers and comments, not the
// entire query: a URL value must not exempt a later real bind parameter.
func hasSQLPlaceholder(query string) bool {
	for i := 0; i < len(query); i++ {
		switch query[i] {
		case '\'', '"', '`':
			quote := query[i]
			for i++; i < len(query); i++ {
				if query[i] != quote {
					continue
				}
				if i+1 < len(query) && query[i+1] == quote {
					i++
					continue
				}
				break
			}
		case '-':
			if i+1 < len(query) && query[i+1] == '-' {
				for i < len(query) && query[i] != '\n' {
					i++
				}
			}
		case '/':
			if i+1 < len(query) && query[i+1] == '*' {
				i += 2
				for i+1 < len(query) && (query[i] != '*' || query[i+1] != '/') {
					i++
				}
				i++
			}
		case '?':
			return true
		}
	}
	return false
}

func TestPgRebindGateLiteralCalls(t *testing.T) {
	cases := []struct {
		name, call string
		want       int
	}{
		{"exec", `db.Exec("UPDATE accounts SET enabled = ?", true)`, 1},
		{"get destination", `db.Get(&out, "SELECT id FROM accounts WHERE id = ?", id)`, 1},
		{"select destination", `db.Select(&out, "SELECT id FROM accounts WHERE id = ?", id)`, 1},
		{"exec context", `db.ExecContext(ctx, "DELETE FROM accounts WHERE id = ?", id)`, 1},
		{"query context", `db.QueryContext(ctx, "SELECT id FROM accounts WHERE id = ?", id)`, 1},
		{"query row context", `db.QueryRowContext(ctx, "SELECT id FROM accounts WHERE id = ?", id)`, 1},
		{"get context destination", `db.GetContext(ctx, &out, "SELECT id FROM accounts WHERE id = ?", id)`, 1},
		{"select context destination", `db.SelectContext(ctx, &out, "SELECT id FROM accounts WHERE id = ?", id)`, 1},
		{"selector multiline raw literal", "h.db.SelectContext(\nctx,\n&out,\n`SELECT id\nFROM accounts WHERE id = ?`,\nid,\n)", 1},
		{"rebound", `h.db.Get(&out, h.db.Rebind("SELECT id FROM accounts WHERE id = ?"), id)`, 0},
		{"rebound context", `db.ExecContext(ctx, db.Rebind("DELETE FROM accounts WHERE id = ?"), id)`, 0},
		{"helper rebound", `db.Exec(rebindAdminQuery("DELETE FROM accounts WHERE id = ?"), id)`, 0},
		{"url value", `db.Get(&out, "SELECT id FROM logs WHERE url = 'https://x.com?q=1'")`, 0},
		{"url and parameter", `db.Get(&out, "SELECT id FROM logs WHERE url = 'https://x.com?q=1' AND id = ?", id)`, 1},
		{"quoted and commented", "db.Exec(`SELECT '?' AS \"?\" /* ? */ -- ?\n`)", 0},
		{"escaped SQL quote", `db.Exec("SELECT 'it''s ?' WHERE id = ?", id)`, 1},
		{"nonliteral scope", `db.Exec(query, id)`, 0},
		{"store getter", `db := store.GetDB(); db.Exec("SELECT ?", id)`, 0},
		{"store local shadow", `db := store.GetDB(); { db := rawDB; db.Exec("SELECT ?", id) }; _ = db`, 1},
		{"store name shadow", `store := other; db := store.GetDB(); db.Exec("SELECT ?", id)`, 1},
		{"store parameter", `func(db *store.DB) { db.Query("SELECT ?", id) }(wrappedDB)`, 0},
		{"sqlx parameter same name", `func(dbw *sqlx.DB) { dbw.Query("SELECT ?", id) }(rawDB)`, 1},
		{"comment not call", `// db.Exec("SELECT ?")` + "\n", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", "package fixture\nfunc check() {\n"+tc.call+"\n}", 0)
			if err != nil {
				t.Fatal(err)
			}
			if got := bareSQLPlaceholders(file); len(got) != tc.want {
				t.Fatalf("findings = %d, want %d for %s", len(got), tc.want, tc.call)
			}
		})
	}
}

func TestPgRebindGateSQLXSurface(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   bool
	}{
		{"package p\nfunc f(db *sqlx.DB) {}", true},
		{"package p\ntype H struct { db *sqlx.DB }", true},
		{"package p\nfunc f(db *store.DB) {}", false},
		{"package p\n// *sqlx.DB\nvar text = \"*sqlx.DB\"", false},
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", tc.source, 0)
		if err != nil {
			t.Fatal(err)
		}
		if got := mentionsBareSQLXDB(file); got != tc.want {
			t.Errorf("mentionsBareSQLXDB = %v, want %v for %s", got, tc.want, tc.source)
		}
	}
}
