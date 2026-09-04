package docs_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	windowsLocalPathRE = regexp.MustCompile(`(?i)\b[A-Z]:\\(?:Users|Code)\\[^\s` + "`" + `)]+`)
	unixHomePathRE     = regexp.MustCompile(`/(?:Users|home)/[^/\s` + "`" + `]+`)
	dsnWithPasswordRE  = regexp.MustCompile(`(?i)\b(?:postgres(?:ql)?|mysql)://([^:\s` + "`" + `/]+):([^@\s` + "`" + `]+)@([^\s` + "`" + `/]+)`)
	aiArtifactRE       = regexp.MustCompile(`(?i)(contentReference\[oaicite:|oai_citation|citeturn\d+search\d+|grok_card|utm_source=(?:chatgpt\.com|copilot\.com|openai|claude\.ai|perplexity\.ai))`)
	// Public markdown must only link public repositories under this owner.
	// Private deployment forks (e.g. tokendance-gateway) must never be cited.
	ownerRepoLinkRE = regexp.MustCompile(`(?i)github\.com/DeliciousBuding/([A-Za-z0-9_.-]+)`)
	// Inline markdown link target: [text](target). Targets containing spaces
	// (optional-title form) and HTML tags are deliberately out of scope.
	markdownLinkRE = regexp.MustCompile(`\[[^\]]*\]\(([^)\s]+)\)`)
)

// storefrontMarkdown names the README files: the public storefront of the
// repository. They must never deep-link into docs/internal/ — maintainer
// process context (roadmap, audit waves, state tables) stays internal and
// user-facing facts are stated inline instead.
var storefrontMarkdown = map[string]bool{
	"README.md":    true,
	"README_EN.md": true,
}

func TestPublicMarkdownHygiene(t *testing.T) {
	root := repoRoot(t)
	var findings []string
	scanned := 0

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			// Skip VCS/build caches and local evidence/worktrees so hygiene only
			// covers published tree paths. CI clones are clean, while ignored
			// .dev-local evidence may contain absolute capture paths by design.
			if name == ".git" || name == "node_modules" || name == "dist" ||
				name == ".claude" || name == ".dev-local" || name == ".worktrees" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		scanned++

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			lineNo := i + 1
			switch {
			case windowsLocalPathRE.MatchString(line):
				findings = append(findings, formatFinding(rel, lineNo, "local Windows path", line))
			case hasDisallowedUnixHomePath(line):
				findings = append(findings, formatFinding(rel, lineNo, "local Unix home path", line))
			case aiArtifactRE.MatchString(line):
				findings = append(findings, formatFinding(rel, lineNo, "AI citation or tracking artifact", line))
			case hasPrivateRepoLink(line):
				findings = append(findings, formatFinding(rel, lineNo, "link to a non-public repository under DeliciousBuding", line))
			case dsnWithPasswordRE.MatchString(line) && !isAllowedExampleDSN(line):
				findings = append(findings, formatFinding(rel, lineNo, "credential-bearing DSN without placeholders", line))
			case looksLikeRedisSupportedClaim(line):
				findings = append(findings, formatFinding(rel, lineNo, "unsupported Redis runtime claim", line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// A hygiene gate that scans nothing and reports no violations is not a
	// lenient gate, it is an absent one: the skip list above or the .md
	// extension check could each narrow it to silence. Same invariant as
	// TestPackageBoundaries — see docs/testing.md.
	if scanned == 0 {
		t.Fatal("gate would pass vacuously: no markdown files were scanned")
	}
	if len(findings) > 0 {
		t.Fatalf("public Markdown hygiene violations:\n%s", strings.Join(findings, "\n"))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate repo root")
		}
		dir = parent
	}
}

func mustRel(t *testing.T, root, path string) string {
	t.Helper()
	rel, err := filepath.Rel(root, path)
	if err != nil {
		t.Fatal(err)
	}
	return rel
}

func formatFinding(path string, line int, kind string, text string) string {
	return path + ":" + itoa(line) + ": " + kind + ": " + strings.TrimSpace(text)
}

func hasPrivateRepoLink(line string) bool {
	matches := ownerRepoLinkRE.FindAllStringSubmatch(line, -1)
	for _, match := range matches {
		// metapi-go itself is the only public repository under this owner.
		// Strip a trailing ".git" (clone URLs) before comparing.
		repo := strings.TrimSuffix(strings.ToLower(match[1]), ".git")
		if repo != "metapi-go" {
			return true
		}
	}
	return false
}

func isAllowedExampleDSN(line string) bool {
	matches := dsnWithPasswordRE.FindAllStringSubmatch(line, -1)
	if len(matches) == 0 {
		return true
	}
	for _, match := range matches {
		user, password, host := strings.ToLower(match[1]), strings.ToLower(match[2]), strings.ToLower(match[3])
		if strings.Contains(match[0], "<") || strings.Contains(password, "***") {
			continue
		}
		if host == "localhost:5432" && user == "postgres" && password == "test" {
			continue
		}
		if user == "user" && (password == "pass" || password == "password") && strings.HasPrefix(host, "host") {
			continue
		}
		return false
	}
	return true
}

func hasDisallowedUnixHomePath(line string) bool {
	matches := unixHomePathRE.FindAllString(line, -1)
	for _, match := range matches {
		normalized := strings.ToLower(match)
		if normalized == "/home/runner" || normalized == "/home/app" {
			continue
		}
		return true
	}
	return false
}

func looksLikeRedisSupportedClaim(line string) bool {
	lower := strings.ToLower(line)
	if !strings.Contains(lower, "redis") {
		return false
	}
	hasClaim := strings.Contains(lower, "supported") ||
		strings.Contains(lower, "integrated") ||
		strings.Contains(lower, "required") ||
		strings.Contains(lower, "dependency") ||
		strings.Contains(lower, "runtime") ||
		strings.Contains(lower, "兼容") ||
		strings.Contains(lower, "支持") ||
		strings.Contains(lower, "集成")
	if !hasClaim {
		return false
	}
	hasNegation := strings.Contains(lower, "not ") ||
		strings.Contains(lower, "not*") || // markdown **not** / *not*
		strings.Contains(lower, "*not*") ||
		strings.Contains(lower, "never") ||
		strings.Contains(lower, " no ") ||
		strings.Contains(lower, "no `redis_url`") ||
		strings.Contains(lower, "no live redis") ||
		strings.Contains(lower, "without redis") ||
		strings.Contains(lower, "optional redis") ||
		strings.Contains(lower, "fail-open") ||
		strings.Contains(lower, "fail open") ||
		strings.Contains(lower, "尚未") ||
		strings.Contains(lower, "没有") ||
		strings.Contains(lower, "无 redis")
	return !hasNegation
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// TestStorefrontDoesNotLinkInternalDocs pins the public/internal docs split:
// the README files are the storefront and may only link user-facing material.
// Maintainer process docs under docs/internal/ (roadmap, state, audits,
// design notes) are discoverable through docs/README.md, never through the
// storefront.
func TestStorefrontDoesNotLinkInternalDocs(t *testing.T) {
	root := repoRoot(t)
	var findings []string
	linksChecked := 0

	for name := range storefrontMarkdown {
		path := filepath.Join(root, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			for _, match := range markdownLinkRE.FindAllStringSubmatch(line, -1) {
				target := match[1]
				if isExternalLinkTarget(target) {
					continue
				}
				linksChecked++
				resolved := resolveLinkTarget(root, name, target)
				if strings.HasPrefix(resolved, "docs/internal/") {
					findings = append(findings, formatFinding(name, i+1, "storefront link into docs/internal/", line))
				}
			}
		}
	}

	// Reading two named files defends the *input* surface — a missing README is
	// already fatal above. It says nothing about the *predicate* surface: this
	// gate shares markdownLinkRE and isExternalLinkTarget with
	// TestRelativeMarkdownLinksResolve, and narrowing either one empties this
	// gate while both files still read fine. Probed both ways before adding the
	// counter; measured 23 + 24 links examined across the two READMEs. Same
	// reason the sibling counts links rather than files (#1233, #1238).
	if linksChecked == 0 {
		t.Fatal("gate would pass vacuously: no storefront links were examined — markdownLinkRE or isExternalLinkTarget changed shape")
	}
	if len(findings) > 0 {
		t.Fatalf("storefront docs must not link maintainer-internal docs:\n%s", strings.Join(findings, "\n"))
	}
}

// TestRelativeMarkdownLinksResolve catches dead relative links in tracked
// markdown (dead links are the defect readers report most often, and they are
// trivially automatable). Links inside fenced code blocks and external URLs
// are out of scope.
func TestRelativeMarkdownLinksResolve(t *testing.T) {
	root := repoRoot(t)
	var findings []string
	scanned, linksChecked := 0, 0

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "node_modules" || name == "dist" ||
				name == ".claude" || name == ".worktrees" || name == ".dev-local" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		scanned++

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(mustRel(t, root, path))
		inFence := false
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "```") {
				inFence = !inFence
				continue
			}
			if inFence {
				continue
			}
			for _, match := range markdownLinkRE.FindAllStringSubmatch(line, -1) {
				target := match[1]
				if isExternalLinkTarget(target) {
					continue
				}
				linksChecked++
				resolved := filepath.Join(root, filepath.FromSlash(resolveLinkTarget(root, rel, target)))
				if _, err := os.Stat(resolved); err != nil {
					findings = append(findings, formatFinding(rel, i+1, "dead relative link -> "+target, line))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// Two counters, not one: scanning files but examining no links is the
	// failure mode of a regex or an exclusion predicate that quietly changed
	// shape, which is how an ownership entry ends up naming a file that never
	// mentions the thing it owns (#1233). See docs/testing.md.
	if scanned == 0 {
		t.Fatal("gate would pass vacuously: no markdown files were scanned")
	}
	if linksChecked == 0 {
		t.Fatal("gate would pass vacuously: no relative markdown links were examined — markdownLinkRE or isExternalLinkTarget changed shape")
	}
	if len(findings) > 0 {
		t.Fatalf("dead markdown links:\n%s", strings.Join(findings, "\n"))
	}
}

// isExternalLinkTarget reports link targets that are not local file paths.
func isExternalLinkTarget(target string) bool {
	return strings.Contains(target, "://") ||
		strings.HasPrefix(target, "mailto:") ||
		strings.HasPrefix(target, "#") ||
		strings.HasPrefix(target, "<") ||
		strings.HasPrefix(target, "{")
}

// resolveLinkTarget resolves a relative markdown link target against the
// directory of the linking file and returns a repo-root-relative slash path.
// Anchors are stripped before resolution.
func resolveLinkTarget(root, linkingFileRel, target string) string {
	if idx := strings.Index(target, "#"); idx >= 0 {
		target = target[:idx]
	}
	if target == "" {
		return ""
	}
	dir := filepath.Dir(filepath.FromSlash(linkingFileRel))
	resolved := filepath.ToSlash(filepath.Clean(filepath.Join(dir, filepath.FromSlash(target))))
	return resolved
}
