package docs_test

// TestEnvVarDocParity locks the three-way agreement between the environment
// variables a deployment can set and the two places that document them:
//
//	.env.example           — key + inline comment (the machine-readable source)
//	docs/configuration.md  — grouped table (variable / default / description)
//	config/config.go       — what config.Load actually reads, plus clamps
//
// Why a test and not a checklist: the three files drift independently, and a
// drifted key is a support ticket waiting to happen.
//
// What this gate does NOT do, stated plainly because an earlier revision
// claimed otherwise: it compares key *sets* in the four directions below and
// never compares default *values*. Auditing all 83 documented keys against
// .env.example found no value drift, and a naive value comparison would be
// wrong rather than merely strict, because the two columns are not the same
// kind of thing — docs/configuration.md's default column is prose
// ("platform-dependent", "system local time", "on", "falls back to
// `AUTH_TOKEN`"), while .env.example deliberately ships empties and
// placeholders that are not defaults. An empty line is usually consistent with
// a documented non-empty default, because parseBoolean("") and friends return
// the code fallback. The one shipped literal that IS load-bearing is pinned by
// TestShippedCredentialSecretPlaceholderIsTheGuardedOne below.
//
// Directions asserted (all must be empty):
//  1. every key in .env.example is documented in docs/configuration.md
//  2. every variable named in docs/configuration.md exists in .env.example
//  3. every key read by config.Load's get("X") helper exists in .env.example
//  4. every key in .env.example has a real reader somewhere in non-test Go
//     code (get("X"), os.Getenv, os.LookupEnv or a direct env-map index)
//
// Direction 4 is the one that catches dead keys — a variable shipped in
// .env.example that no code path ever reads. There is no allowlist: the fix is
// to drop the key or add the reader, which is what happened to the only key this
// direction ever caught (HOME_PAGE_CONTENT, #1160 → #1166).
//
// Parsing note: the doc side extracts every backticked ALL-CAPS token, not
// just table first-columns, because docs/configuration.md legitimately packs
// several variables into one cell (`A` / `B`). That is over-eager on purpose:
// a prose token like `TLS` in backticks fails the test loudly and is fixed by
// dropping the backticks, whereas a too-narrow parser would silently pass
// while a real variable went undocumented. Failure direction matters more
// than precision here.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestShippedCredentialSecretPlaceholderIsTheGuardedOne pins two files to each
// other. config.Validate reports ACCOUNT_CREDENTIAL_SECRET when it still equals
// the literal shipped in .env.example, and matches it against
// config.EnvExampleAccountCredentialSecret. If either side is retyped alone the
// guard stops matching while every test still passes — the same shape as a
// route-ownership entry naming a file that never mentions the route (#1233) and
// a boundary scan reporting zero violations because it scanned nothing (#1214).
// So the pairing itself is asserted, from the files, not from a copied literal.
func TestShippedCredentialSecretPlaceholderIsTheGuardedOne(t *testing.T) {
	root := repoRoot(t)

	const key = "ACCOUNT_CREDENTIAL_SECRET="
	var shipped string
	for _, line := range strings.Split(readRepoFile(t, root, ".env.example"), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), key); ok {
			shipped = strings.TrimSpace(v)
			break
		}
	}
	if shipped == "" {
		t.Fatalf("parser sanity: no non-empty %s line in .env.example — if the placeholder was removed, delete this test and the config.Validate branch together, do not leave one of them behind", key)
	}

	const decl = "EnvExampleAccountCredentialSecret = "
	var guarded string
	for _, line := range strings.Split(readRepoFile(t, root, "config/defaults.go"), "\n") {
		if i := strings.Index(line, decl); i >= 0 {
			guarded = strings.Trim(strings.TrimSpace(line[i+len(decl):]), `"`)
			break
		}
	}
	if guarded == "" {
		t.Fatal("config.EnvExampleAccountCredentialSecret not found in config/defaults.go — config.Validate's placeholder branch has nothing to match against")
	}

	if shipped != guarded {
		t.Fatalf("the credential-secret placeholder .env.example ships (%q) is not the one config.Validate guards (%q); a deployment copying .env.example would run on a public encryption key with no warning. Change both, or neither.", shipped, guarded)
	}
}

// envKeyRE matches a KEY=... assignment line in .env.example (comments and
// blank lines are skipped by the caller).
var envKeyRE = regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=`)

// docVarRE matches a backticked ALL-CAPS token inside Markdown — the shape of
// every environment variable in this repository, including short ones without
// underscores (`TZ`, `LOGO`, `FOOTER`, `ABOUT`).
var docVarRE = regexp.MustCompile("`([A-Z][A-Z0-9]*(?:_[A-Z0-9]+)*)`")

// configGetRE matches the get("KEY") helper calls inside config.Load.
var configGetRE = regexp.MustCompile(`get\("([A-Z0-9_]+)"\)`)

// readerREs match every other way a key can be read from the environment in
// this repository.
var readerREs = []*regexp.Regexp{
	regexp.MustCompile(`get\("([A-Z0-9_]+)"\)`),
	regexp.MustCompile(`os\.Getenv\("([A-Z0-9_]+)"\)`),
	regexp.MustCompile(`os\.LookupEnv\("([A-Z0-9_]+)"\)`),
	regexp.MustCompile(`env\["([A-Z0-9_]+)"\]`),
}

func TestEnvVarDocParity(t *testing.T) {
	root := repoRoot(t)

	envContent := readRepoFile(t, root, ".env.example")
	docContent := readRepoFile(t, root, "docs/configuration.md")
	cfgContent := readRepoFile(t, root, "config/config.go")

	envKeys := parseEnvExampleKeys(envContent)
	if len(envKeys) == 0 {
		t.Fatal("parser sanity: .env.example yielded 0 keys — the parser is broken, not the docs")
	}
	docVars := parseDocVars(docContent)
	if len(docVars) == 0 {
		t.Fatal("parser sanity: docs/configuration.md yielded 0 variables — the parser is broken, not the docs")
	}
	cfgKeys := parseConfigGetKeys(cfgContent)
	if len(cfgKeys) == 0 {
		t.Fatal("parser sanity: config/config.go yielded 0 get(\"X\") keys — the parser is broken, not the docs")
	}
	readers := collectEnvReaders(t, root)
	if len(readers) == 0 {
		t.Fatal("parser sanity: no environment readers found in Go sources — the scan is broken")
	}

	// Same comparator the self-proof below feeds violating samples through, so
	// that proof covers the code that actually runs on the real files.
	if drift := detectDrift(envKeys, docVars, cfgKeys, readers); drift != "" {
		t.Fatalf("environment-variable documentation drift:\n%s\nFix the docs (or .env.example) — do not relax this test.", drift)
	}
}

// TestEnvParityGateIsNotAVacuousPass proves the gate above can actually go
// red. It feeds deliberately violating samples through the same parsers and
// comparators used on the real files, and requires each violation to be
// detected. Without this, a typo in a regexp would make TestEnvVarDocParity
// pass forever while documenting nothing.
func TestEnvParityGateIsNotAVacuousPass(t *testing.T) {
	const goodEnv = "PORT=4000\n# a comment\nAUTH_TOKEN=change-me-admin-token\n"
	const goodDoc = "| `PORT` | `4000` | HTTP listen port. |\n| `AUTH_TOKEN` | `change-me-admin-token` | Admin token. |\n"
	const goodCfg = "func Load(env map[string]string) {\n\tcfg.Port = get(\"PORT\")\n\trt.AuthToken = get(\"AUTH_TOKEN\")\n}\n"

	base := detectDrift(parseEnvExampleKeys(goodEnv), parseDocVars(goodDoc), parseConfigGetKeys(goodCfg),
		map[string]bool{"PORT": true, "AUTH_TOKEN": true})
	if base != "" {
		t.Fatalf("control sample must be clean, got:\n%s", base)
	}

	cases := []struct {
		name       string
		env        string
		doc        string
		cfg        string
		readers    map[string]bool
		wantSubstr string
	}{
		{
			name:       "documented variable dropped from the doc",
			env:        goodEnv,
			doc:        "| `PORT` | `4000` | HTTP listen port. |\n",
			cfg:        goodCfg,
			readers:    map[string]bool{"PORT": true, "AUTH_TOKEN": true},
			wantSubstr: "AUTH_TOKEN",
		},
		{
			name:       "doc invents a variable that .env.example does not ship",
			env:        goodEnv,
			doc:        goodDoc + "| `ZZZ_INVENTED_KNOB` | empty | Not real. |\n",
			cfg:        goodCfg,
			readers:    map[string]bool{"PORT": true, "AUTH_TOKEN": true},
			wantSubstr: "ZZZ_INVENTED_KNOB",
		},
		{
			name:       "code reads a key that .env.example never shipped",
			env:        goodEnv,
			doc:        goodDoc,
			cfg:        goodCfg + "\tcfg.X = get(\"ZZZ_UNSHIPPED_KEY\")\n",
			readers:    map[string]bool{"PORT": true, "AUTH_TOKEN": true},
			wantSubstr: "ZZZ_UNSHIPPED_KEY",
		},
		{
			name:       "shipped key has no reader anywhere (dead key)",
			env:        goodEnv + "ZZZ_DOC_PARITY_PROBE=\n",
			doc:        goodDoc + "| `ZZZ_DOC_PARITY_PROBE` | empty | Probe. |\n",
			cfg:        goodCfg,
			readers:    map[string]bool{"PORT": true, "AUTH_TOKEN": true},
			wantSubstr: "ZZZ_DOC_PARITY_PROBE",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := detectDrift(parseEnvExampleKeys(tc.env), parseDocVars(tc.doc), parseConfigGetKeys(tc.cfg), tc.readers)
			if tc.wantSubstr == "" {
				if got != "" {
					t.Fatalf("expected clean, got:\n%s", got)
				}
				return
			}
			if got == "" {
				t.Fatalf("gate did not fire on a deliberate violation (%s) — it is vacuous", tc.name)
			}
			if !strings.Contains(got, tc.wantSubstr) {
				t.Fatalf("gate fired but did not name %q:\n%s", tc.wantSubstr, got)
			}
		})
	}
}

// detectDrift is the pure comparison core shared by the real-file test and
// the self-proof test, so the self-proof exercises exactly the same logic.
func detectDrift(envKeys, docVars, cfgKeys []string, readers map[string]bool) string {
	var b strings.Builder
	reportDiff(&b, "in .env.example but NOT documented in docs/configuration.md", diffKeys(envKeys, docVars))
	reportDiff(&b, "in docs/configuration.md but NOT present in .env.example", diffKeys(docVars, envKeys))
	reportDiff(&b, "read by config.Load but NOT present in .env.example", diffKeys(cfgKeys, envKeys))

	var dead []string
	for _, k := range envKeys {
		if readers[k] {
			continue
		}
		dead = append(dead, k)
	}
	reportDiff(&b, "in .env.example with no reader in non-test Go code (dead key; drop it or add the reader)", dead)
	return b.String()
}

func parseEnvExampleKeys(content string) []string {
	var keys []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if m := envKeyRE.FindStringSubmatch(line); m != nil {
			keys = append(keys, m[1])
		}
	}
	return dedupeSorted(keys)
}

func parseDocVars(content string) []string {
	var keys []string
	for _, m := range docVarRE.FindAllStringSubmatch(content, -1) {
		keys = append(keys, m[1])
	}
	return dedupeSorted(keys)
}

func parseConfigGetKeys(content string) []string {
	return dedupeSorted(firstGroups(configGetRE.FindAllStringSubmatch(content, -1)))
}

func firstGroups(matches [][]string) []string {
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

// collectEnvReaders returns the set of environment keys read anywhere in
// non-test Go source, so a key shipped in .env.example that nothing reads is
// reported instead of silently documented forever.
func collectEnvReaders(t *testing.T, root string) map[string]bool {
	t.Helper()
	readers := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "dist", ".dev-local", ".worktrees", "web":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		src := string(data)
		for _, re := range readerREs {
			for _, m := range re.FindAllStringSubmatch(src, -1) {
				readers[m[1]] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return readers
}

func diffKeys(a, b []string) []string {
	inB := map[string]bool{}
	for _, k := range b {
		inB[k] = true
	}
	var out []string
	for _, k := range a {
		if !inB[k] {
			out = append(out, k)
		}
	}
	return dedupeSorted(out)
}

func reportDiff(b *strings.Builder, label string, keys []string) {
	if len(keys) == 0 {
		return
	}
	b.WriteString("  " + label + " (" + itoa(len(keys)) + "):\n")
	for _, k := range keys {
		b.WriteString("    - " + k + "\n")
	}
}

func dedupeSorted(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func readRepoFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(data)
}
