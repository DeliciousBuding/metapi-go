package docs_test

// TestA11yShellChromeKeysResolve locks the a11y-checklist §3.1 shell-chrome
// table to the i18n catalog: every i18n key the table names in backticks must
// resolve to a real key in web/src/i18n/locales/en.json. The table once
// documented *rendered* Chinese strings (`打开导航`, `关闭弹框`, …) that had
// silently drifted from the shipped labels for an entire redesign; naming the
// keys instead of the renderings makes the table machine-checkable, and this
// gate is that machine.
//
// Key shapes accepted:
//   - dotted paths (`common.close`) resolved segment-by-segment under
//     the top-level "translation" object
//   - verbatim leaf keys with spaces (`Toggle Sidebar`) found anywhere in the
//     catalog (i18next source-text keys)
//
// Deliberately skipped: `.*` globs (e.g. `common.languageName.*`), paths,
// component names and non-ASCII renderings — those stay review-only prose.
//
// Vacuity guard: the scan must find at least 4 checkable keys in the table,
// so a formatting change that silently empties the scan fails loudly.

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

var a11yBacktickRE = regexp.MustCompile("`([^`\n]+)`")

var a11yFileExtRE = regexp.MustCompile(`\.(tsx?|jsx?|css|md|json|mjs|go|sh)$`)

func loadEnCatalog(t *testing.T) map[string]any {
	t.Helper()
	raw, err := os.ReadFile("../web/src/i18n/locales/en.json")
	if err != nil {
		t.Fatalf("read en.json: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		t.Fatalf("parse en.json: %v", err)
	}
	catalog, ok := root["translation"].(map[string]any)
	if !ok {
		t.Fatal("en.json has no top-level translation object")
	}
	return catalog
}

func resolveDotted(catalog map[string]any, key string) bool {
	node := catalog
	segs := strings.Split(key, ".")
	for i, seg := range segs {
		if i == len(segs)-1 {
			_, ok := node[seg]
			return ok // leaf may be a string, not a nested map
		}
		next, ok := node[seg].(map[string]any)
		if !ok {
			return false
		}
		node = next
	}
	return true
}

func collectLeafKeys(node any, out *map[string]bool) {
	switch v := node.(type) {
	case map[string]any:
		for k, child := range v {
			(*out)[k] = true
			collectLeafKeys(child, out)
		}
	}
}

func TestA11yShellChromeKeysResolve(t *testing.T) {
	raw, err := os.ReadFile("internal/design/a11y-checklist.md")
	if err != nil {
		t.Fatalf("read a11y-checklist.md: %v", err)
	}
	text := string(raw)
	start := strings.Index(text, "### 3.1")
	end := strings.Index(text, "### 3.2")
	if start < 0 || end < 0 || end <= start {
		t.Fatal("a11y-checklist.md: §3.1/§3.2 headings not found — table scan would pass vacuously")
	}
	table := text[start:end]

	catalog := loadEnCatalog(t)
	leafKeys := map[string]bool{}
	collectLeafKeys(catalog, &leafKeys)

	checked := 0
	var failures []string
	for _, m := range a11yBacktickRE.FindAllStringSubmatch(table, -1) {
		tok := strings.TrimSpace(m[1])
		if strings.Contains(tok, "/") || strings.HasSuffix(tok, ".*") {
			continue // paths and glob references stay prose
		}
		if a11yFileExtRE.MatchString(tok) {
			continue // file names (app-header.tsx) stay prose
		}
		if !strings.Contains(tok, ".") && !strings.Contains(tok, " ") {
			continue // single words (component names, renderings) stay prose
		}
		if strings.Contains(tok, ".") && !strings.Contains(tok, " ") {
			if !resolveDotted(catalog, tok) {
				failures = append(failures, tok+" (dotted key not in en.json)")
				continue
			}
			checked++
			continue
		}
		// spaced tokens: verbatim i18next source-text leaf keys
		if !leafKeys[tok] {
			failures = append(failures, tok+" (verbatim key not in en.json)")
			continue
		}
		checked++
	}
	if checked < 4 {
		t.Fatalf("a11y shell-chrome gate scanned only %d keys — would pass vacuously", checked)
	}
	if len(failures) > 0 {
		t.Fatalf("a11y-checklist §3.1 names i18n keys that do not resolve:\n  %s", strings.Join(failures, "\n  "))
	}
}
