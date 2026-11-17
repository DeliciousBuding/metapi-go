//go:build ignore

// ci-plan is shared by local pushes and CI. It selects packages, never test names.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

const module = "github.com/deliciousbuding/metapi-go"

var packageDir = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*(/[A-Za-z0-9_][A-Za-z0-9_.-]*)*$`)

type plan struct {
	Mode     string          `json:"mode"`
	Reason   []string        `json:"reason"`
	Packages []string        `json:"packages"`
	WebFiles []string        `json:"webFiles"`
	Backend  bool            `json:"backend"`
	Frontend bool            `json:"frontend"`
	Browser  bool            `json:"browser"`
	Full     bool            `json:"full"`
	Jobs     map[string]bool `json:"jobs"`
}

func git(args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %v: %w: %s", args, err, stderr.String())
	}
	return out, nil
}

func names(data []byte) []string {
	return strings.Split(strings.TrimSuffix(string(data), "\x00"), "\x00")
}

func makePlan(base, head, mode string, working bool) (plan, error) {
	p := plan{Mode: mode, Packages: []string{}, WebFiles: []string{}, Reason: []string{}}
	if mode != "change" && mode != "full" && mode != "release" {
		return p, fmt.Errorf("invalid mode %q", mode)
	}
	if mode == "release" {
		p.finish()
		return p, nil
	}
	files, err := git("ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return p, err
	}
	// Parse all build variants and test imports: reverse consumers on another OS
	// or behind the integration tag must not disappear from the impact graph.
	imports := map[string]map[string]bool{}
	for _, file := range names(files) {
		if !strings.HasSuffix(file, ".go") {
			continue
		}
		if _, err := os.Stat(file); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return p, err
		}
		ast, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.SkipObjectResolution)
		if err != nil {
			return p, fmt.Errorf("parse %s: %w", file, err)
		}
		dir := path.Dir(file)
		if !packageDir.MatchString(dir) {
			return p, fmt.Errorf("unsupported Go package directory %q", dir)
		}
		if imports[dir] == nil {
			imports[dir] = map[string]bool{}
		}
		for _, imp := range ast.Imports {
			name, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				return p, err
			}
			if strings.HasPrefix(name, module+"/") {
				imports[dir][strings.TrimPrefix(name, module+"/")] = true
			}
		}
	}
	if len(imports) == 0 {
		return p, fmt.Errorf("no Go packages; refusing empty scan")
	}
	changes := []string{}
	if mode == "change" {
		if base == "" || head == "" {
			return p, fmt.Errorf("change mode requires base and head")
		}
		data, err := git("diff", "--name-only", "--no-renames", "-z", base, head)
		if err != nil {
			return p, err
		}
		if len(data) > 0 {
			changes = append(changes, names(data)...)
		}
		if working {
			for _, args := range [][]string{{"diff", "--name-only", "-z", "HEAD"}, {"ls-files", "-z", "--others", "--exclude-standard"}} {
				data, err := git(args...)
				if err != nil {
					return p, err
				}
				if len(data) > 0 {
					changes = append(changes, names(data)...)
				}
			}
		}
	}
	selected := map[string]bool{}
	p.Full = mode == "full" || len(changes) == 0
	if p.Full {
		p.Reason = append(p.Reason, "full request or no attributable change")
	}
	for _, file := range changes {
		if strings.ContainsAny(file, "\r\n") {
			return p, fmt.Errorf("unsupported newline in changed path")
		}
		switch {
		case strings.HasPrefix(file, "docs/") && strings.HasSuffix(file, ".go"):
			p.Full = true
			p.Reason = append(p.Reason, "contract gate: "+file)
		case strings.HasPrefix(file, "web/") && !strings.HasSuffix(file, ".go"):
			p.Frontend, p.Browser = true, true
			p.WebFiles = append(p.WebFiles, strings.TrimPrefix(file, "web/"))
			selected["web"] = true
			if !strings.HasPrefix(file, "web/src/") && !strings.HasPrefix(file, "web/public/") && !strings.HasSuffix(file, ".md") {
				p.Full = true
				p.Reason = append(p.Reason, "frontend toolchain/config: "+file)
			}
		case strings.HasSuffix(file, ".go") && !strings.HasPrefix(file, "scripts/"):
			dir := path.Dir(file)
			if imports[dir] == nil {
				p.Full = true
				p.Reason = append(p.Reason, "removed/unresolved Go package: "+file)
			} else {
				selected[dir] = true
			}
			if strings.HasPrefix(file, "store/") || strings.HasPrefix(file, "config/") || strings.HasPrefix(file, "auth/") || strings.HasPrefix(file, "router/") || strings.HasPrefix(file, "app/") || strings.HasPrefix(file, "handler/admin/") || strings.HasPrefix(file, "handler/user/") {
				p.Browser = true
			}
		case strings.HasSuffix(file, ".md") || strings.HasPrefix(file, "docs/") && !strings.HasSuffix(file, ".go"):
			// Documentation always runs the complete docs contract suite below.
		default:
			p.Full = true
			p.Reason = append(p.Reason, "shared build/config/fixture or unknown path: "+file)
		}
	}
	// Include transitive reverse dependencies, including external test packages.
	for changed := true; changed; {
		changed = false
		for pkg, deps := range imports {
			if selected[pkg] {
				continue
			}
			for dep := range deps {
				if selected[dep] {
					selected[pkg] = true
					changed = true
					break
				}
			}
		}
	}
	for pkg := range imports {
		if p.Full || selected[pkg] {
			p.Packages = append(p.Packages, module+"/"+pkg)
		}
	}
	sort.Strings(p.Packages)
	p.Backend = len(p.Packages) > 0
	if len(selected) > 0 && !p.Backend {
		return p, fmt.Errorf("changed code selected no packages")
	}
	if !p.Full {
		p.Reason = append(p.Reason, "changed owners plus transitive production/test importers; docs contracts always run")
	}
	if p.Full {
		p.Frontend, p.Browser, p.Backend = true, true, true
	}
	p.finish()
	return p, nil
}

func (p *plan) finish() {
	p.Jobs = map[string]bool{
		"lint": p.Backend, "vet": p.Backend, "vulncheck": p.Full,
		"mod-verify": p.Full, "secret-scan": p.Mode != "release", "docs-hygiene": p.Mode != "release",
		"frontend": p.Frontend, "web-dist": p.Backend || p.Frontend,
		"a11y": p.Browser, "test-sqlite": p.Backend, "test-pg": p.Backend,
		"build": p.Backend, "test-e2e": p.Full, "runtime-smoke-matrix": p.Full,
	}
}

func verify(p plan, input string) error {
	var needs map[string]struct {
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(input), &needs); err != nil {
		return err
	}
	// Scope itself must have succeeded; skipped dependencies are accepted only
	// when the actual plan explicitly marked that check out of scope.
	if needs["scope"].Result != "success" {
		return fmt.Errorf("scope did not succeed")
	}
	if len(p.Jobs) != 14 || len(needs) != 15 {
		return fmt.Errorf("missing or unexpected prerequisites")
	}
	for job, required := range p.Jobs {
		want := "skipped"
		if required {
			want = "success"
		}
		if needs[job].Result != want {
			return fmt.Errorf("%s: got %q, require %s", job, needs[job].Result, want)
		}
	}
	return nil
}

func run() error {
	base := flag.String("base", "", "exact diff base (resolve merge-base before invocation)")
	head := flag.String("head", "HEAD", "diff head")
	mode := flag.String("mode", "change", "change, full or release")
	working := flag.Bool("worktree", false, "include staged, unstaged and untracked changes")
	check := flag.Bool("verify-needs", false, "verify PLAN_JSON and NEEDS_JSON instead of planning")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if *check {
		var p plan
		if err := json.Unmarshal([]byte(os.Getenv("PLAN_JSON")), &p); err != nil {
			return err
		}
		// Rebuild expected jobs; do not trust a caller-supplied jobs map.
		if !slices.Contains([]string{"change", "full", "release"}, p.Mode) {
			return fmt.Errorf("invalid plan mode")
		}
		if p.Mode == "full" && !p.Full || p.Full && (!p.Backend || !p.Frontend || !p.Browser) || p.Backend != (len(p.Packages) > 0) || p.Frontend && !p.Backend || p.Browser && !p.Backend || p.Mode == "release" && (p.Full || p.Backend || p.Frontend || p.Browser) {
			return fmt.Errorf("inconsistent plan scope")
		}
		for _, pkg := range p.Packages {
			if !strings.HasPrefix(pkg, module+"/") || !packageDir.MatchString(strings.TrimPrefix(pkg, module+"/")) {
				return fmt.Errorf("invalid selected package %q", pkg)
			}
		}
		p.finish()
		return verify(p, os.Getenv("NEEDS_JSON"))
	}
	p, err := makePlan(*base, *head, *mode, *working)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(p)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ci-plan:", err)
		os.Exit(1)
	}
}
