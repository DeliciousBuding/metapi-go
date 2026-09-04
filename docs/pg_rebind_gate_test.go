package docs

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// pgRebindGate scans handler/ service/ scheduler/ for SQL executed through a
// bare *sqlx.DB with SQLite `?` placeholders.
//
// Background: store.DB wraps every method with an automatic PostgreSQL Rebind,
// but code that receives a bare `*sqlx.DB` (or stores one in a handler struct)
// bypasses the wrapper. On PostgreSQL such queries fail with SQLSTATE 42601.
// Three incidents: balance_history snapshot (v0.8.47), admin_audit_logs insert
// (v0.8.51), and account expiry marking in service/alert (found in the same
// sweep). This gate makes the failure class visible at CI time.
//
// Rule: in any .go file that mentions the bare `*sqlx.DB` type, every
// db.Exec/Get/Select/Query call whose first argument is a string literal
// containing `?` must be wrapped in a Rebind helper (`db.Rebind(...)` or
// `rebindAdminQuery(...)` etc). URLs (`://`) are not placeholders.

var (
	bareSqlxDBRe = regexp.MustCompile(`\*sqlx\.DB`)
	// dbCallRe matches `receiver.Method("...?..."` with an optional Rebind wrapper.
	// Covers non-context variants where the SQL string is the first argument.
	dbCallRe = regexp.MustCompile(
		`\b(\w+)\.(Exec|Get|Select|Query|QueryRow|QueryRowx|Queryx)\(\s*` +
			`(?:(?:db|h\.db|r\.db|cfg\.|tx)\.Rebind\(|rebind[A-Za-z]*\(|` + // allowed wrappers
			`("(?:[^"\\]|\\.)*\?[^"]*"|` + "`" + `[^` + "`" + `]*\?[^` + "`" + `]*` + "`" + `))`,
	)
	// ctxDBCallRe covers ...Context variants where ctx and dest come first and
	// the SQL string literal is the third: db.SelectContext(ctx, &dest, "…?…", …).
	ctxDBCallRe = regexp.MustCompile(
		`\b(\w+)\.(ExecContext|GetContext|SelectContext|QueryContext|QueryRowxContext|QueryxContext)\(` +
			`\s*[^,]+,\s*[^,]+,\s*` +
			`(?:(?:db|h\.db|r\.db|cfg\.|tx)\.Rebind\(|rebind[A-Za-z]*\(|` + // allowed wrappers
			`("(?:[^"\\]|\\.)*\?[^"]*"|` + "`" + `[^` + "`" + `]*\?[^` + "`" + `]*` + "`" + `))`,
	)
)

func TestPgRebindGateNoBareQuestionMarks(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Dir(filepath.Dir(thisFile)) // docs/ → repo root
	dirs := []string{"app", "auth", "cmd", "config", "e2e", "handler", "platform", "proxy", "routing", "scheduler", "service", "store", "transform"}
	var violations []string
	var scannedFiles, sqlxFiles int
	dirsReached := map[string]bool{}
	saw := map[string]bool{}

	for _, dir := range dirs {
		root := filepath.Join(repoRoot, dir)
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			src, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			rel, relErr := filepath.Rel(repoRoot, path)
			if relErr != nil {
				return nil
			}
			scannedFiles++
			dirsReached[dir] = true
			// Only files that handle a bare *sqlx.DB can hit the wrapper bypass.
			if !bareSqlxDBRe.Match(src) {
				return nil
			}
			sqlxFiles++
			saw[rel] = true
			content := string(src)
			for _, line := range strings.Split(content, "\n") {
				for _, re := range []*regexp.Regexp{dbCallRe, ctxDBCallRe} {
					for _, m := range re.FindAllStringSubmatch(line, -1) {
						if m[3] == "" {
							continue // Rebind-wrapped call, safe
						}
						call := m[0]
						if strings.Contains(call, "://") {
							continue // URL query string, not a placeholder
						}
						violations = append(violations, path+":"+call)
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}

	// Three surfaces can narrow this gate to silence while it stays green: a
	// renamed directory (WalkDir on a missing root hands the callback an error,
	// which the callback swallows), a broadened `_test.go` filter, and
	// bareSqlxDBRe itself, which decides the scan surface. Assert all three
	// before trusting a clean result — docs/testing.md, "count the input".
	if len(dirsReached) != len(dirs) {
		var missing []string
		for _, d := range dirs {
			if !dirsReached[d] {
				missing = append(missing, d)
			}
		}
		t.Fatalf("walk never reached %s — a renamed or missing directory silently empties this gate", strings.Join(missing, ", "))
	}
	if scannedFiles < 100 {
		t.Fatalf("walk examined %d production .go files; the scan surface is broken, not clean", scannedFiles)
	}
	if sqlxFiles < 20 {
		t.Fatalf("%d files matched bareSqlxDBRe; the scan surface is broken, not clean", sqlxFiles)
	}
	for _, probe := range []string{"service/model_redirects.go", "store/open.go", "store/factory_reset.go"} {
		if !saw[probe] {
			t.Fatalf("scan surface never admitted %s, which handles a bare *sqlx.DB", probe)
		}
	}

	if len(violations) > 0 {
		t.Fatalf(
			"bare `?` placeholder through *sqlx.DB (PostgreSQL would 42601). "+
				"Wrap each query in db.Rebind(...) or a rebind* helper:\n%s",
			strings.Join(violations, "\n"),
		)
	}
}

// TestPgRebindGateRegexpSanity locks the matcher behaviour: a bare call must be
// flagged, a Rebind-wrapped call must not, and URL query strings must not.
//
// It also locks bareSqlxDBRe, which the gate above uses to decide *which files it
// looks at*. The two violation regexps were already locked; the scan-surface
// filter was not, so narrowing it emptied the gate while both this test and the
// gate stayed green — the same split that let two doc gates pass having scanned
// nothing (#1238).
func TestPgRebindGateRegexpSanity(t *testing.T) {
	if !bareSqlxDBRe.MatchString("func (s *Store) expire(db *sqlx.DB, id int64) error {") {
		t.Fatal("bareSqlxDBRe must match a *sqlx.DB parameter: it decides which files the gate scans")
	}
	if bareSqlxDBRe.MatchString("package store\n\nfunc now() time.Time { return time.Now() }\n") {
		t.Fatal("bareSqlxDBRe matches a file with no database handle; the scan surface is not what the gate claims")
	}

	bare := `db.Exec("UPDATE accounts SET status = 'expired' WHERE id = ?", 1)`
	if !dbCallRe.MatchString(bare) {
		t.Fatalf("regexp must match bare call: %s", bare)
	}

	wrapped := `db.Exec(db.Rebind("UPDATE accounts SET status = 'expired' WHERE id = ?"), 1)`
	if m := dbCallRe.FindStringSubmatch(wrapped); m != nil && m[3] != "" {
		t.Fatalf("regexp must treat Rebind-wrapped call as safe: %s", wrapped)
	}

	url := `h.db.Get(&x, "SELECT * FROM logs WHERE url LIKE 'https://x.com?q=1'")`
	if m := dbCallRe.FindStringSubmatch(url); m != nil && m[3] != "" {
		t.Fatalf("URL query string must not be flagged: %s", url)
	}

	// …Context variants put ctx first; the SQL string is the second argument.
	ctxBare := `db.SelectContext(ctx, &routes, ` + "`" + `SELECT model_pattern FROM token_routes WHERE enabled = ?` + "`" + `, true)`
	if !ctxDBCallRe.MatchString(ctxBare) {
		t.Fatalf("regexp must match bare ctx-variant call: %s", ctxBare)
	}

	ctxWrapped := `db.SelectContext(ctx, &routes, db.Rebind(` + "`" + `SELECT model_pattern FROM token_routes WHERE enabled = ?` + "`" + `), true)`
	if m := ctxDBCallRe.FindStringSubmatch(ctxWrapped); m != nil && m[3] != "" {
		t.Fatalf("regexp must treat Rebind-wrapped ctx call as safe: %s", ctxWrapped)
	}
}
