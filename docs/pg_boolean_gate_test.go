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
//	<bool-col> != 0 | 1
//
// where <bool-col> is one of the known BOOLEAN columns declared in
// store/schema_ddl.go (the full 16-column inventory is in knownBooleanColumns
// below). Matching is case-insensitive so uppercase identifiers (AVAILABLE,
// COALESCE) are also caught. Use COALESCE(<col>, false) = true /
// <col> = true / IS TRUE instead — these are portable across both SQLite
// and PostgreSQL.

var (
	// knownBooleanColumns is the set of column names declared BOOLEAN in
	// store/schema_ddl.go. Adding a new BOOLEAN column here extends the gate.
	// The list is the complete inventory from schema_ddl.go as of 2026-08-17;
	// when adding a new BOOLEAN column to the schema, add it here too.
	knownBooleanColumns = []string{
		"available",
		"checkin_enabled",
		"custom_headers_override_request_headers",
		"downgrade_decision",
		"enabled",
		"is_default",
		"is_manual",
		"is_pinned",
		"is_stream",
		"manual_override",
		"post_refresh_probe_enabled",
		"read",
		"recover_applied",
		"resin_enabled",
		"use_system_proxy",
		"use_utls",
	}

	// booleanColumnAlternation builds `(?:available|enabled|is_default|...)`
	// from the list above, used inside the shared regex.
	booleanColumnAlternation = "(?:" + strings.Join(knownBooleanColumns, "|") + ")"

	// coalesceBoolIntRe matches COALESCE(<optional-alias.><bool-col>, 0) or
	// COALESCE(<bool-col>, 0). The column may be prefixed by a table alias
	// like "tma." or "at.". Case-insensitive (?i) so uppercase identifiers
	// (AVAILABLE, COALESCE) are also caught — SQL is case-insensitive.
	coalesceBoolIntRe = regexp.MustCompile(
		`(?i)COALESCE\(\s*(?:[a-z_]+\.)?` + booleanColumnAlternation + `\s*,\s*0\s*\)`,
	)

	// boolEqualsIntRe matches <optional-alias.><bool-col> = 0|1 or <> 0|1 or != 0|1.
	// Word boundaries prevent matching substrings like "not_available".
	// Case-insensitive (?i) so uppercase identifiers are also caught.
	boolEqualsIntRe = regexp.MustCompile(
		`(?i)(?:[a-z_]+\.)?\b` + booleanColumnAlternation + `\b\s*(?:=|<>|!=)\s*[01]\b`,
	)
)

func TestPgBooleanLiteralGateNoIntComparison(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Dir(filepath.Dir(thisFile)) // docs/ → repo root
	dirs := []string{"app", "auth", "cmd", "config", "e2e", "handler", "platform", "proxy", "routing", "scheduler", "service", "store", "transform"}
	var violations []string
	var scannedFiles, examinedLines int
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
			saw[rel] = true

			content := string(src)
			for _, line := range strings.Split(content, "\n") {
				// Skip comment lines — doc comments may legitimately mention
				// "available=1" as prose (e.g. service/model_redirects.go:189).
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "//") {
					continue
				}
				if trimmed == "" {
					continue
				}
				examinedLines++
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

	// A renamed directory (WalkDir on a missing root hands the callback an error,
	// which the callback swallows), a broadened `_test.go` filter, or a comment
	// predicate that swallows real code all narrow this gate to silence while it
	// stays green. Count the input before trusting the verdict — docs/testing.md.
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
	if examinedLines < 10000 {
		t.Fatalf("examined %d non-comment lines; the comment predicate is swallowing real code, not skipping prose", examinedLines)
	}
	for _, probe := range []string{"service/model_redirects.go", "store/setting_store.go"} {
		if !saw[probe] {
			t.Fatalf("walk never examined %s; the scan surface is broken", probe)
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

	// Direct = 1 / = 0 / <> 1 / != 1 must be flagged.
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
	bangNotEquals := "AND at.enabled != 1"
	if !boolEqualsIntRe.MatchString(bangNotEquals) {
		t.Fatalf("regexp must match <bool> != 1: %s", bangNotEquals)
	}

	// Case-insensitivity: uppercase identifiers must be flagged.
	upperCase := "WHERE AVAILABLE = 1"
	if !boolEqualsIntRe.MatchString(upperCase) {
		t.Fatalf("regexp must match uppercase AVAILABLE = 1: %s", upperCase)
	}
	upperCoalesce := "WHERE COALESCE(TMA.AVAILABLE, 0) = 1"
	if !coalesceBoolIntRe.MatchString(upperCoalesce) {
		t.Fatalf("regexp must match uppercase COALESCE: %s", upperCoalesce)
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
