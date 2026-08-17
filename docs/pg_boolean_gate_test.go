package docs

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// pgBooleanLiteralGate scans handler/ service/ scheduler/ for SQL that
// compares BOOLEAN columns against integer literals (0/1) or wraps them in
// COALESCE(..., 0). Both patterns are SQLite-only: SQLite stores booleans as
// 0/1 integers so the comparison type-checks, but PostgreSQL's BOOLEAN type
// rejects them with SQLSTATE 42804 ("COALESCE types boolean and integer
// cannot be matched" / "operator does not exist: boolean = integer").
//
// Background: the 2026-08-17 v0.14.0 deployment surfaced this on the
// /api/models/token-candidates endpoint — four builder queries in
// stats_marketplace.go used COALESCE(<bool>, 0) = 1 and <bool> = 1, which
// PostgreSQL rejected. The bug shipped because (a) no test exercised those
// builders on PG, and (b) the existing pg_rebind_gate_test.go only catches
// ?-placeholder dialect drift (SQLSTATE 42601), not boolean-literal drift.
// This gate generalizes dialect-drift detection to the boolean-literal class.
//
// Rule: in any .go file (excluding _test.go), no SQL string literal may
// contain:
//
//	COALESCE(<optional-table-alias.><bool-col>, 0)
//	<bool-col> = 0 | 1
//	<bool-col> <> 0 | 1
//
// where <bool-col> is one of the known BOOLEAN columns: available, enabled,
// is_default, resin_enabled, use_utls, post_refresh_probe_enabled. Use
// COALESCE(<col>, false) = true / <col> = true / IS TRUE instead — these are
// portable across both SQLite and PostgreSQL.

var (
	// knownBooleanColumns is the set of column names declared BOOLEAN in
	// store/schema_ddl.go. Adding a new BOOLEAN column here extends the gate.
	knownBooleanColumns = []string{
		"available",
		"enabled",
		"is_default",
		"resin_enabled",
		"use_utls",
		"post_refresh_probe_enabled",
	}

	// booleanColumnAlternation builds `(?:available|enabled|is_default|...)`
	// from the list above, used inside the shared regex.
	booleanColumnAlternation = "(?:" + strings.Join(knownBooleanColumns, "|") + ")"

	// coalesceBoolIntRe matches COALESCE(<optional-alias.><bool-col>, 0) or
	// COALESCE(<bool-col>, 0). The column may be prefixed by a table alias
	// like "tma." or "at.". Captures the column name for the violation
	// message.
	coalesceBoolIntRe = regexp.MustCompile(
		`COALESCE\(\s*(?:[a-z_]+\.)?` + booleanColumnAlternation + `\s*,\s*0\s*\)`,
	)

	// boolEqualsIntRe matches <optional-alias.><bool-col> = 0|1 or <> 0|1.
	// Word boundaries prevent matching substrings like "not_available".
	boolEqualsIntRe = regexp.MustCompile(
		`(?:[a-z_]+\.)?\b` + booleanColumnAlternation + `\b\s*(?:=|<>)\s*[01]\b`,
	)
)

func TestPgBooleanLiteralGateNoIntComparison(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Dir(filepath.Dir(thisFile)) // docs/ → repo root
	dirs := []string{"handler", "service", "scheduler"}
	var violations []string

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
			content := string(src)
			for _, line := range strings.Split(content, "\n") {
				// Skip comment lines — doc comments may legitimately mention
				// "available=1" as prose (e.g. service/model_redirects.go:189).
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "//") {
					continue
				}
				for _, m := range coalesceBoolIntRe.FindAllString(line, -1) {
					violations = append(violations, path+": "+m+
						" — use COALESCE(<col>, false) = true instead (PG 42804)")
				}
				for _, m := range boolEqualsIntRe.FindAllString(line, -1) {
					violations = append(violations, path+": "+m+
						" — use <col> = true / false instead (PG 42804)")
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}

	if len(violations) > 0 {
		t.Fatalf(
			"boolean column compared against integer literal (PostgreSQL would 42804). "+
				"Use COALESCE(<col>, false) = true or <col> = true / false:\n%s",
			strings.Join(violations, "\n"),
		)
	}
}

// TestPgBooleanLiteralGateRegexpSanity locks the matcher behaviour so future
// column additions don't silently weaken the gate.
func TestPgBooleanLiteralGateRegexpSanity(t *testing.T) {
	// COALESCE with integer default must be flagged — prefixed and bare.
	coalescePrefixed := "WHERE COALESCE(tma.available, 0) = 1"
	if !coalesceBoolIntRe.MatchString(coalescePrefixed) {
		t.Fatalf("regexp must match COALESCE with prefixed boolean column: %s", coalescePrefixed)
	}
	coalesceBare := "WHERE COALESCE(available, 0) = 1"
	if !coalesceBoolIntRe.MatchString(coalesceBare) {
		t.Fatalf("regexp must match COALESCE with bare boolean column: %s", coalesceBare)
	}

	// COALESCE with false default must NOT be flagged (dialect-safe).
	coalesceSafe := "WHERE COALESCE(tma.available, false) = true"
	if coalesceBoolIntRe.MatchString(coalesceSafe) {
		t.Fatalf("regexp must not flag dialect-safe COALESCE: %s", coalesceSafe)
	}

	// Direct = 1 / = 0 / <> 1 must be flagged.
	equalsOne := "AND at.enabled = 1"
	if !boolEqualsIntRe.MatchString(equalsOne) {
		t.Fatalf("regexp must match <bool> = 1: %s", equalsOne)
	}
	equalsZero := "WHERE enabled = 0"
	if !boolEqualsIntRe.MatchString(equalsZero) {
		t.Fatalf("regexp must match <bool> = 0: %s", equalsZero)
	}
	notEqualsOne := "AND at.enabled <> 1"
	if !boolEqualsIntRe.MatchString(notEqualsOne) {
		t.Fatalf("regexp must match <bool> <> 1: %s", notEqualsOne)
	}

	// = true / = false must NOT be flagged (dialect-safe).
	equalsTrue := "AND at.enabled = true"
	if boolEqualsIntRe.MatchString(equalsTrue) {
		t.Fatalf("regexp must not flag <bool> = true: %s", equalsTrue)
	}

	// Comment lines must be skipped by the walker (not by the regex — the
	// walker checks strings.HasPrefix(trimmed, "//") before applying the
	// regex). Verify the regex itself is agnostic: a comment-style string
	// would match, but the walker excludes it. This test documents that the
	// exclusion is in the walker, not the regex.
	commentLine := "// available=1 in a comment is fine"
	if !boolEqualsIntRe.MatchString(commentLine) {
		t.Fatalf("regexp matches comment content; walker excludes comment lines — this is intentional")
	}
}
