package store

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Settings rehydration gate.
//
// A runtime setting is only real if it survives a restart: the admin API
// persists it to the settings table, and the next boot must read it back into
// the config snapshot the consumers use. When one half is missing the operator
// sees a 200, sees the new value in GET /api/settings/runtime, and then loses
// it on the next deploy/crash/OOM restart — with no log line to explain it.
// The security-relevant instance of this defect was admin_ip_allowlist, which
// fell back to the (empty = allow every IP) env value after a restart.
//
// Rules enforced here:
//
//	R1  every settings key the admin write side can persist must either have a
//	    case in ApplyRuntimeSettings or be listed in nonHydratedSettingKeys
//	    with a reason;
//	R2  a key must not be both hydrated and allowlisted (a stale allowlist
//	    entry hides the case that actually applies it);
//	R3  every allowlist entry carries a non-empty reason.
//
// Both sides are extracted from source with go/parser (the house idiom: see
// internal/httpclient/gate_test.go and docs/pg_rebind_gate_test.go), so the
// lists cannot drift from the code they describe. When this test fails, add a
// hydration case — do not relax the test, and only allowlist a key when its
// consumer genuinely reads the settings table itself.

// settingsGatePersistHelpers maps a write-side helper to the argument
// positions that carry the settings key.
var settingsGatePersistHelpers = map[string][]int{
	"upsertSettingDB":         {1},
	"upsertSettingTx":         {2},
	"applyStringSettingDB":    {3},
	"applyBoolSettingDB":      {3},
	"persistDualSchedule":     {1, 3},
	"saveBackupSettingString": {1},
}

// SQL-embedded key spellings, e.g. `WHERE key = 'auth_token'` and
// `WHERE key IN ('db_type', 'db_url')`.
var (
	settingsGateSQLEq  = regexp.MustCompile(`key\s*=\s*'([a-zA-Z0-9_.]+)'`)
	settingsGateSQLIn  = regexp.MustCompile(`key\s+IN\s*\(([^)]*)\)`)
	settingsGateSQLLit = regexp.MustCompile(`'([a-zA-Z0-9_.]+)'`)
)

// settingsGateDir resolves a path relative to this test file, so the gate
// works from any working directory (go test runs with cwd = package dir).
func settingsGateDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(thisFile)
}

// settingsGateStringConsts collects package-level `const name = "value"` and
// `var name = "value"` bindings so a key passed as an identifier
// (const ldohCookieSettingKey = "monitor_ldoh_cookie") still resolves.
func settingsGateStringConsts(files []*ast.File) map[string]string {
	out := map[string]string{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
				continue
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || len(vs.Names) != 1 || len(vs.Values) != 1 {
					continue
				}
				if lit, ok := vs.Values[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if v, err := strconv.Unquote(lit.Value); err == nil {
						out[vs.Names[0].Name] = v
					}
				}
			}
		}
	}
	return out
}

// settingsGateWriteKeys extracts every settings key the admin write side can
// persist, from helper-call arguments and from keys embedded in SQL literals.
func settingsGateWriteKeys(fset *token.FileSet, files []*ast.File) map[string]string {
	consts := settingsGateStringConsts(files)
	keys := map[string]string{} // key -> first file that persists it
	add := func(pos token.Position, key string) {
		if key == "" {
			return
		}
		if _, seen := keys[key]; !seen {
			keys[key] = fmt.Sprintf("%s:%d", settingsGateRel(pos.Filename), pos.Line)
		}
	}
	literal := func(expr ast.Expr) (string, bool) {
		switch node := expr.(type) {
		case *ast.BasicLit:
			if node.Kind != token.STRING {
				return "", false
			}
			v, err := strconv.Unquote(node.Value)
			return v, err == nil
		case *ast.Ident:
			v, ok := consts[node.Name]
			return v, ok
		}
		return "", false
	}

	for _, file := range files {
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ""
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				name = fun.Name
			case *ast.SelectorExpr:
				name = fun.Sel.Name
			}
			if positions, ok := settingsGatePersistHelpers[name]; ok {
				for _, idx := range positions {
					if idx < len(call.Args) {
						if key, ok := literal(call.Args[idx]); ok {
							add(fset.Position(call.Args[idx].Pos()), key)
						}
					}
				}
			}
			// Keys spelled inside SQL text (raw Exec/Get paths).
			for _, arg := range call.Args {
				lit, ok := arg.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				text, err := strconv.Unquote(lit.Value)
				if err != nil || !strings.Contains(text, "settings") {
					continue
				}
				pos := fset.Position(arg.Pos())
				for _, m := range settingsGateSQLEq.FindAllStringSubmatch(text, -1) {
					add(pos, m[1])
				}
				for _, group := range settingsGateSQLIn.FindAllStringSubmatch(text, -1) {
					for _, m := range settingsGateSQLLit.FindAllStringSubmatch(group[1], -1) {
						add(pos, m[1])
					}
				}
			}
			return true
		})
	}
	return keys
}

// settingsGateRel trims the repository prefix from an absolute source path so
// gate failures quote repo-relative file:line positions.
func settingsGateRel(path string) string {
	if idx := strings.Index(path, string(filepath.Separator)+"handler"+string(filepath.Separator)); idx >= 0 {
		return path[idx+1:]
	}
	if idx := strings.Index(path, string(filepath.Separator)+"store"+string(filepath.Separator)); idx >= 0 {
		return path[idx+1:]
	}
	return filepath.Base(path)
}

// settingsGateHydratedKeys extracts the settings keys ApplyRuntimeSettings
// handles, i.e. the string literals in the case clauses of its `switch key`.
// Nested switches (checkin_schedule_mode's "cron"/"interval"/"window") carry a
// different tag and are therefore excluded.
func settingsGateHydratedKeys(t *testing.T, path string) map[string]bool {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	keys := map[string]bool{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "ApplyRuntimeSettings" {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			tag, ok := sw.Tag.(*ast.Ident)
			if !ok || tag.Name != "key" {
				return true
			}
			for _, stmt := range sw.Body.List {
				clause, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				for _, expr := range clause.List {
					lit, ok := expr.(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					if v, err := strconv.Unquote(lit.Value); err == nil {
						keys[v] = true
					}
				}
			}
			return false
		})
	}
	return keys
}

func TestSettingsRehydrationGateCoversEveryAdminWritableKey(t *testing.T) {
	dir := settingsGateDir(t)
	adminDir := filepath.Join(dir, "..", "handler", "admin")
	entries, err := os.ReadDir(adminDir)
	if err != nil {
		t.Fatalf("read %s: %v", adminDir, err)
	}
	fset := token.NewFileSet()
	var files []*ast.File
	parsed := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(adminDir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, file)
		parsed++
	}
	if parsed == 0 {
		t.Fatalf("gate parsed no files; admin dir resolution is broken (%s)", adminDir)
	}

	writeKeys := settingsGateWriteKeys(fset, files)
	hydrated := settingsGateHydratedKeys(t, filepath.Join(dir, "settings.go"))
	if len(writeKeys) == 0 {
		t.Fatal("write-side extractor found no settings keys; the matcher is broken")
	}
	for _, probe := range []string{"system_name", "proxy_token"} {
		if _, ok := writeKeys[probe]; !ok {
			t.Fatalf("write-side extractor missed %q; the matcher is broken", probe)
		}
	}
	if len(hydrated) == 0 {
		t.Fatal("hydration-side extractor found no case keys; the matcher is broken")
	}
	for _, probe := range []string{"system_name", "admin_ip_allowlist"} {
		if !hydrated[probe] {
			t.Fatalf("hydration-side extractor missed %q; the matcher is broken", probe)
		}
	}

	var missing, both []string
	for key := range writeKeys {
		if hydrated[key] {
			if _, allowlisted := nonHydratedSettingKeys[key]; allowlisted {
				both = append(both, key)
			}
			continue
		}
		if _, allowlisted := nonHydratedSettingKeys[key]; !allowlisted {
			missing = append(missing, fmt.Sprintf("%s (persisted at %s)", key, writeKeys[key]))
		}
	}
	for key, reason := range nonHydratedSettingKeys {
		if strings.TrimSpace(reason) == "" {
			missing = append(missing, fmt.Sprintf("%s (allowlisted without a reason)", key))
		}
	}
	sort.Strings(missing)
	sort.Strings(both)

	if len(both) > 0 {
		t.Errorf("R2: keys both hydrated and allowlisted — drop the stale allowlist entry: %s",
			strings.Join(both, ", "))
	}
	if len(missing) > 0 {
		t.Fatalf("R1/R3: settings the admin API persists but startup hydration never reads back "+
			"(the operator's change is lost on the next restart):\n  %s\n"+
			"Fix: add a case to ApplyRuntimeSettings (clamped exactly like config.Load), or — only if the "+
			"consumer really reads the settings table itself — list the key in nonHydratedSettingKeys with a reason.",
			strings.Join(missing, "\n  "))
	}
}

// TestSettingsRehydrationGateMatcherSanity locks the extractor behaviour with
// counter-examples, so a refactor of the write side cannot silently empty the
// key set and turn the gate above into a no-op.
func TestSettingsRehydrationGateMatcherSanity(t *testing.T) {
	src := `package admin

const someKeyConst = "monitor_ldoh_cookie"

func examples(db *sqlx.DB, body map[string]any) {
	upsertSettingDB(db, "admin_ip_allowlist", nil)
	upsertSettingTx(db, tx, "checkin_cron", "0 8 * * *")
	applyStringSettingDB(db, body, "systemName", "system_name", nil)
	applyBoolSettingDB(db, body, "webhookEnabled", "webhook_enabled", nil)
	persistDualSchedule(db, "log_cleanup_cron", cron, "log_cleanup_schedule_v2", spec)
	saveBackupSettingString(db, someKeyConst, data)
	db.Exec(db.Rebind("UPDATE settings SET value = ? WHERE key = 'auth_token'"), v)
	db.Query("SELECT key, value FROM settings WHERE key IN ('db_type', 'db_url')")
	// Not a settings write: must not be collected.
	upsertSettingDB(db, dynamicKey, nil)
	notify("log_cleanup_retention_days is not persisted here")
}
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "examples.go", src, 0)
	if err != nil {
		t.Fatalf("parse counter-example source: %v", err)
	}
	got := settingsGateWriteKeys(fset, []*ast.File{file})
	want := []string{
		"admin_ip_allowlist", "auth_token", "checkin_cron", "db_type", "db_url",
		"log_cleanup_cron", "log_cleanup_schedule_v2", "monitor_ldoh_cookie",
		"system_name", "webhook_enabled",
	}
	if len(got) != len(want) {
		t.Fatalf("extractor returned %d keys (%v), want %d (%v)", len(got), sortedKeys(got), len(want), want)
	}
	for _, key := range want {
		if _, ok := got[key]; !ok {
			t.Fatalf("extractor missed %q (got %v)", key, sortedKeys(got))
		}
	}

	hydrateSrc := `package store

func ApplyRuntimeSettings(cfg *config.Config, rt *config.RuntimeSettings, settingsMap map[string]string) {
	for key, value := range settingsMap {
		switch key {
		case "system_name":
			rt.SystemName = value
		case "log_cleanup_retention_days", "log_cleanup.retention_days":
			rt.LogCleanupRetentionDays = 1
		case "checkin_schedule_mode":
			switch value {
			case "cron", "interval":
				rt.CheckinScheduleMode = value
			}
		}
	}
}
`
	hydratePath := filepath.Join(t.TempDir(), "settings.go")
	if err := os.WriteFile(hydratePath, []byte(hydrateSrc), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	gotHydrated := settingsGateHydratedKeys(t, hydratePath)
	wantHydrated := []string{"system_name", "log_cleanup_retention_days", "log_cleanup.retention_days", "checkin_schedule_mode"}
	if len(gotHydrated) != len(wantHydrated) {
		t.Fatalf("hydration extractor returned %v, want %v", sortedSet(gotHydrated), wantHydrated)
	}
	for _, key := range wantHydrated {
		if !gotHydrated[key] {
			t.Fatalf("hydration extractor missed %q (got %v)", key, sortedSet(gotHydrated))
		}
	}
	// The nested switch's value literals are not settings keys.
	for _, notAKey := range []string{"cron", "interval"} {
		if gotHydrated[notAKey] {
			t.Fatalf("hydration extractor collected %q from a nested switch", notAKey)
		}
	}
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedSet(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---- R4/R5: hydration must not assign a parse failure ----
//
// R4  every value parser ApplyRuntimeSettings calls is registered in
//     settingsGateHydrationParsers with the reason its failure mode cannot
//     destroy a configured value, and no registration is stale;
// R5  a json.Unmarshal inside the hydration switch inspects its error at the
//     call site.
//
// The defect these rules close: `rt.PayloadRules = config.ParseJsonValue(value)`
// assigned the parse result directly, and ParseJsonValue encodes "cannot read"
// as nil, so one unreadable row emptied a configured rule set at the next
// restart. "I cannot read this row" is not "the operator wants the zero
// value"; the branch must keep the resolved value and say so at WARN.

// settingsGateHydrationParsers registers the parsers the hydration switch may
// call. An entry means: this helper either carries the resolved value as its
// fallback or reports failure through a second return value the compiler
// forces the branch to check, so its result can never turn an unreadable row
// into an empty setting.
var settingsGateHydrationParsers = map[string]string{
	"parseInt":                      "returns the fallback when the cell is not an integer",
	"parseBoolSetting":              "blank keeps the fallback and any other text resolves to false, exactly like config.parseBoolean",
	"parseFloatSetting":             "returns the fallback on a parse error, NaN, Inf or a negative",
	"parseIntSetting":               "max(lo, trunc(n)) with n defaulting to the fallback: the clamp config.Load applies",
	"parseNumberSetting":            "returns the fallback on a parse error, NaN or Inf",
	"parseJSONSettingString":        "returns the trimmed raw cell when it is not a JSON string, never a zero value",
	"parseStringListSetting":        "reports an unusable cell through ok=false, which the branch must check",
	"parseJSONValueSetting":         "reports an unreadable cell through err, which the branch must check",
	"parseNotifyTaskTogglesSetting": "reports an unreadable cell through err, which the branch must check",
}

type settingsGateCall struct {
	name    string // last selector segment: "ParseJsonValue", "parseBoolSetting"
	qual    string // selector prefix, empty for a plain call: "config", "json"
	pos     token.Position
	checked bool // result consumed by an if/switch init or condition, or a 2-value define
}

func (c settingsGateCall) label() string {
	if c.qual == "" {
		return c.name
	}
	return c.qual + "." + c.name
}

// settingsGateInterestingCall reports whether a call is one the R4/R5 rules
// govern: a parse-named value parser, or a json.Unmarshal whose error must be
// inspected at the call site.
func settingsGateInterestingCall(name, qual string) bool {
	if strings.HasPrefix(strings.ToLower(name), "parse") {
		return true
	}
	return name == "Unmarshal" && qual == "json"
}

func settingsGateCallName(call *ast.CallExpr) (name, qual string) {
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name, ""
	case *ast.SelectorExpr:
		if pkg, ok := fn.X.(*ast.Ident); ok {
			return fn.Sel.Name, pkg.Name
		}
		return fn.Sel.Name, ""
	default:
		return "", ""
	}
}

// settingsGateMarkChecked records the call positions whose failure result the
// surrounding branch inspects: an if/switch init or condition, or the right
// hand side of a two-value short declaration (`list, ok := …`, `v, err := …`).
func settingsGateMarkChecked(checked map[token.Pos]bool, node ast.Node) {
	if node == nil {
		return
	}
	ast.Inspect(node, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			checked[call.Pos()] = true
		}
		return true
	})
}

// settingsGateHydrationCalls collects the governed calls inside the case
// bodies of ApplyRuntimeSettings' `switch key`, each flagged with whether its
// failure result is checked at the call site.
func settingsGateHydrationCalls(t *testing.T, path string) []settingsGateCall {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var calls []settingsGateCall
	collect := func(clauses []ast.Stmt) {
		checked := map[token.Pos]bool{}
		for _, stmt := range clauses {
			ast.Inspect(stmt, func(n ast.Node) bool {
				switch s := n.(type) {
				case *ast.IfStmt:
					settingsGateMarkChecked(checked, s.Init)
					settingsGateMarkChecked(checked, s.Cond)
				case *ast.SwitchStmt:
					settingsGateMarkChecked(checked, s.Init)
					settingsGateMarkChecked(checked, s.Tag)
				case *ast.AssignStmt:
					if s.Tok == token.DEFINE && len(s.Lhs) >= 2 {
						for _, rhs := range s.Rhs {
							settingsGateMarkChecked(checked, rhs)
						}
					}
				}
				return true
			})
		}
		for _, stmt := range clauses {
			ast.Inspect(stmt, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				name, qual := settingsGateCallName(call)
				if !settingsGateInterestingCall(name, qual) {
					return true
				}
				calls = append(calls, settingsGateCall{
					name:    name,
					qual:    qual,
					pos:     fset.Position(call.Pos()),
					checked: checked[call.Pos()],
				})
				return true
			})
		}
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "ApplyRuntimeSettings" {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			sw, ok := n.(*ast.SwitchStmt)
			if !ok {
				return true
			}
			tag, ok := sw.Tag.(*ast.Ident)
			if !ok || tag.Name != "key" {
				return true
			}
			for _, stmt := range sw.Body.List {
				clause, ok := stmt.(*ast.CaseClause)
				if !ok {
					continue
				}
				collect(clause.Body)
			}
			return false
		})
	}
	return calls
}

// settingsGateEvaluateCalls applies R4 and R5 to a collected call set. It is
// shared by the gate over the real source and the counter-example test below,
// so the rules themselves are falsifiable and not only the extractor.
func settingsGateEvaluateCalls(calls []settingsGateCall, parsers map[string]string) (unregistered, unchecked, stale []string) {
	seen := map[string]bool{}
	for _, call := range calls {
		if call.name == "Unmarshal" {
			// Governed by R5 only: encoding/json reports failure through its
			// error, never through the destination value.
			if !call.checked {
				unchecked = append(unchecked, fmt.Sprintf("%s at %s:%d", call.label(),
					settingsGateRel(call.pos.Filename), call.pos.Line))
			}
			continue
		}
		seen[call.name] = true
		reason, registered := parsers[call.name]
		switch {
		case !registered:
			unregistered = append(unregistered, fmt.Sprintf("%s at %s:%d", call.label(),
				settingsGateRel(call.pos.Filename), call.pos.Line))
		case strings.TrimSpace(reason) == "":
			unregistered = append(unregistered, fmt.Sprintf("%s (registered without a reason)", call.label()))
		}
	}
	for name, reason := range parsers {
		if !seen[name] {
			stale = append(stale, name)
		}
		if strings.TrimSpace(reason) == "" {
			stale = append(stale, fmt.Sprintf("%s (registered without a reason)", name))
		}
	}
	sort.Strings(unregistered)
	sort.Strings(unchecked)
	sort.Strings(stale)
	return unregistered, unchecked, stale
}

func TestApplyRuntimeSettingsNeverAssignsAnUncheckedParseFailure(t *testing.T) {
	dir := settingsGateDir(t)
	path := filepath.Join(dir, "settings.go")
	calls := settingsGateHydrationCalls(t, path)
	if len(calls) == 0 {
		t.Fatal("call extractor found nothing; the matcher is broken")
	}
	for _, probe := range []string{"parseJSONSettingString", "parseBoolSetting"} {
		found := false
		for _, call := range calls {
			found = found || call.name == probe
		}
		if !found {
			t.Fatalf("call extractor missed %q; the matcher is broken", probe)
		}
	}

	unregistered, unchecked, stale := settingsGateEvaluateCalls(calls, settingsGateHydrationParsers)
	if len(unregistered) > 0 {
		t.Errorf("R4: ApplyRuntimeSettings calls a value parser that is not registered:\n  %s\n"+
			"A parser used here must either carry the resolved value as its fallback or report failure "+
			"through a second return value the branch checks. Register it in settingsGateHydrationParsers "+
			"with the reason it cannot turn an unreadable row into an empty setting — or keep the resolved "+
			"value and log a WARN naming the key, the reason and what was kept.",
			strings.Join(unregistered, "\n  "))
	}
	if len(unchecked) > 0 {
		t.Errorf("R5: json.Unmarshal inside the hydration switch does not inspect its error:\n  %s\n"+
			"Ignoring the error and assigning the destination anyway turns an unreadable row into a "+
			"half-written or zeroed setting.",
			strings.Join(unchecked, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("R4: stale settingsGateHydrationParsers entries no longer called by ApplyRuntimeSettings: %s",
			strings.Join(stale, ", "))
	}
}

// TestSettingsHydrationCallGateMatcherSanity locks the extractor and the rules
// with counter-examples, so a refactor cannot quietly empty the call set and
// turn R4/R5 into a no-op. The payload_rules case is the defect that was
// fixed: it must be collected, must not be registrable, and the bare
// json.Unmarshal must be reported as unchecked.
func TestSettingsHydrationCallGateMatcherSanity(t *testing.T) {
	src := `package store

import (
	"encoding/json"
	"strings"
)

func ApplyRuntimeSettings(cfg *config.Config, rt *config.RuntimeSettings, settingsMap map[string]string) {
	for key, value := range settingsMap {
		switch key {
		case "payload_rules":
			rt.PayloadRules = config.ParseJsonValue(value)
		case "checkin_enabled":
			rt.CheckinDisabled = !parseBoolSetting(value, !rt.CheckinDisabled)
		case "checkin_interval_hours":
			rt.CheckinIntervalHours = config.ClampInt(parseIntSetting(value, 6, 1), 1, 24)
		case "admin_ip_allowlist":
			if list, ok := parseStringListSetting(value); ok {
				rt.AdminIpAllowlist = list
			}
		case "routing_weights":
			weights := rt.RoutingWeights
			if err := json.Unmarshal([]byte(value), &weights); err != nil {
				println("warn")
			} else {
				rt.RoutingWeights = weights
			}
		case "checkin_schedule_mode":
			mode := strings.ToLower(parseJSONSettingString(value))
			switch mode {
			case "cron":
				rt.CheckinScheduleMode = mode
			}
		case "sneaky":
			var decoded map[string]bool
			json.Unmarshal([]byte(value), &decoded)
			rt.NotifyTaskToggles = decoded
		}
	}
}
`
	path := filepath.Join(t.TempDir(), "settings.go")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	calls := settingsGateHydrationCalls(t, path)

	want := map[string]bool{ // label -> at least one call checked
		"config.ParseJsonValue":  false,
		"parseBoolSetting":       false,
		"parseIntSetting":        false,
		"parseStringListSetting": true,
		"parseJSONSettingString": false,
		"json.Unmarshal":         true, // the routing_weights one; the sneaky one stays unchecked
	}
	got := map[string][]settingsGateCall{}
	for _, call := range calls {
		got[call.label()] = append(got[call.label()], call)
	}
	if len(got) != len(want) {
		t.Fatalf("extractor returned labels %v, want %v", sortedCallLabels(got), sortedWantLabels(want))
	}
	for label, wantChecked := range want {
		found, ok := got[label]
		if !ok {
			t.Fatalf("extractor missed %q (got %v)", label, sortedCallLabels(got))
		}
		anyChecked := false
		for _, call := range found {
			anyChecked = anyChecked || call.checked
		}
		if anyChecked != wantChecked {
			t.Fatalf("%q checked = %v, want %v", label, anyChecked, wantChecked)
		}
	}
	if len(got["json.Unmarshal"]) != 2 {
		t.Fatalf("json.Unmarshal calls = %d, want 2 (one checked, one bare)", len(got["json.Unmarshal"]))
	}

	// The rules must fire on this source: the unregistered destructive parse
	// and the bare json.Unmarshal.
	unregistered, unchecked, stale := settingsGateEvaluateCalls(calls, map[string]string{
		"parseBoolSetting":       "reason",
		"parseIntSetting":        "reason",
		"parseStringListSetting": "reason",
		"parseJSONSettingString": "reason",
	})
	if len(unregistered) != 1 || !strings.Contains(unregistered[0], "config.ParseJsonValue") {
		t.Fatalf("R4 did not flag the destructive parse: %v", unregistered)
	}
	if len(unchecked) != 1 || !strings.Contains(unchecked[0], "json.Unmarshal") {
		t.Fatalf("R5 did not flag the bare json.Unmarshal: %v", unchecked)
	}
	if len(stale) != 0 {
		t.Fatalf("R4 reported stale entries for parsers the fixture does call: %v", stale)
	}
	// The defect helper must never become registrable by accident.
	if _, registered := settingsGateHydrationParsers["ParseJsonValue"]; registered {
		t.Fatal("ParseJsonValue must never be registered: it encodes an unreadable cell as nil")
	}
}

func sortedCallLabels(m map[string][]settingsGateCall) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedWantLabels(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---- R6: every production settings writer repo-wide is covered ----
//
// R6  every key production code outside handler/admin can persist must have a
//     case in ApplyRuntimeSettings or an entry in nonHydratedSettingKeys, and
//     every writer whose key cannot be resolved statically must be registered
//     in settingsGateDynamicWriteSites with the reason it is still covered.
//
// R1 reads the admin write side, which is where operators change settings — but
// it is not the only code that writes the table. service/oauth records a
// startup-migration completion marker there, and service/catalogsync persists
// an auto-sync toggle. Neither was visible to R1, so the marker row made every
// single boot log
//
//	settings: persisted keys not applied at startup hydration keys=oauth.identity_backfill_complete
//
// eighteen times in the testbed log alone. That is a false alarm on the one
// line an operator has to trust when a real setting is being lost, and it is
// what a directory-scoped gate cannot prevent: the write side it never reads is
// the write side that drifts.

// settingsGateDynamicWriteSites registers the writers whose key arrives as a
// parameter and therefore cannot be resolved at the write site, each with the
// reason the site is still covered. A new unregistered dynamic writer fails R6:
// "the key is computed" is exactly how a writer hides from a gate that reads
// source.
var settingsGateDynamicWriteSites = map[string]string{
	"store/setting_store.go:(*SettingsStore).Set":   "the KV primitive itself; the key is each caller's, and every caller's own site is resolved here",
	"service/settingsmigration/service.go:upsertTx": "the key arrives from the migration item builders: the three *_schedule_v2 keys, each allowlisted in nonHydratedSettingKeys",
}

// settingsGateWriteSQL matches the statement shapes that persist a settings
// row. Deliberately narrow: CREATE TABLE and SELECT touch the same table
// without writing a key.
var settingsGateWriteSQL = regexp.MustCompile(`(?i)^\s*(?:insert\s+(?:or\s+\w+\s+)?into\s+settings\b|update\s+settings\s+set\b)`)

// settingsGateSQLValuesKey finds a key spelled into the statement text itself,
// e.g. `INSERT INTO settings (key, value) VALUES ('theme', ?)`. The
// `WHERE key = 'x'` spelling is settingsGateSQLEq's job.
var settingsGateSQLValuesKey = regexp.MustCompile(`(?i)VALUES\s*\(\s*'([a-zA-Z0-9_.]+)'`)

// settingsGateKeyCandidate is the shape a settings key can have. It keeps an
// adjacent value argument (`"true"`, `"1"`) or an error-format string from
// being mistaken for the key of a raw write.
var settingsGateKeyCandidate = regexp.MustCompile(`^[a-z][a-z0-9_.]{1,63}$`)

// settingsGateKeyNonCandidates are literal-shaped strings that are values, not
// keys, so a raw write whose key is dynamic cannot report one of these.
var settingsGateKeyNonCandidates = map[string]bool{
	"true": true, "false": true, "null": true, "settings": true, "value": true, "key": true,
}

// settingsGateWriteSite is one statically located settings-table write. An
// empty key means the writer takes the key dynamically and owes an entry in
// settingsGateDynamicWriteSites.
type settingsGateWriteSite struct {
	site string // "relpath:Func" / "relpath:(*Type).Method" / "relpath:<package scope>"
	key  string
	pos  token.Position
}

// settingsGateWriteSiteLabel names a function the way an operator would grep
// for it, receiver included.
func settingsGateWriteSiteLabel(fn *ast.FuncDecl, fileLabel string) string {
	name := fn.Name.Name
	if fn.Recv != nil && len(fn.Recv.List) == 1 {
		switch typ := fn.Recv.List[0].Type.(type) {
		case *ast.StarExpr:
			if id, ok := typ.X.(*ast.Ident); ok {
				name = "(*" + id.Name + ")." + name
			}
		case *ast.Ident:
			name = typ.Name + "." + name
		}
	}
	return fileLabel + ":" + name
}

// settingsGateIsSettingsStoreCtor reports whether an expression constructs a
// store.SettingsStore, i.e. whether the variable it is assigned to has a Set
// method that writes the settings table.
func settingsGateIsSettingsStoreCtor(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name == "NewSettingsStore"
	case *ast.SelectorExpr:
		return fun.Sel.Name == "NewSettingsStore"
	}
	return false
}

// settingsGateKeyFromSQL extracts a key spelled into a statement literal.
func settingsGateKeyFromSQL(lit *ast.BasicLit) string {
	text, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	for _, re := range []*regexp.Regexp{settingsGateSQLEq, settingsGateSQLValuesKey, settingsGateSQLLit} {
		if m := re.FindStringSubmatch(text); len(m) == 2 && settingsGateKeyCandidate.MatchString(m[1]) {
			return m[1]
		}
	}
	return ""
}

// settingsGateRawKeyArg resolves the key argument of a raw settings write: the
// first direct argument that is not the statement itself and resolves to
// something shaped like a key.
func settingsGateRawKeyArg(call *ast.CallExpr, skip int, resolve func(ast.Expr) string) (string, token.Pos) {
	for i, arg := range call.Args {
		if i == skip {
			continue
		}
		key := resolve(arg)
		if key == "" || !settingsGateKeyCandidate.MatchString(key) || settingsGateKeyNonCandidates[key] {
			continue
		}
		return key, arg.Pos()
	}
	return "", token.NoPos
}

// settingsGatePackageWriteSites collects the settings-table writes of one
// parsed package. Files must be the package's non-test files together, so a key
// passed as a package-level const resolves the way the compiler would.
func settingsGatePackageWriteSites(fset *token.FileSet, files []*ast.File, fileLabel func(string) string) []settingsGateWriteSite {
	consts := settingsGateStringConsts(files)
	resolve := func(expr ast.Expr) string {
		switch node := expr.(type) {
		case *ast.BasicLit:
			if node.Kind != token.STRING {
				return ""
			}
			v, err := strconv.Unquote(node.Value)
			if err != nil {
				return ""
			}
			return v
		case *ast.Ident:
			return consts[node.Name]
		}
		return ""
	}

	var sites []settingsGateWriteSite
	add := func(site, key string, pos token.Pos) {
		for _, existing := range sites {
			if existing.site == site && existing.key == key {
				return
			}
		}
		sites = append(sites, settingsGateWriteSite{site: site, key: key, pos: fset.Position(pos)})
	}

	for _, file := range files {
		label := fileLabel(fset.Position(file.Pos()).Filename)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				// A package-level const/var holding the statement is a write
				// site too: it cannot hide behind the function that uses it.
				gen, isGen := decl.(*ast.GenDecl)
				if !isGen {
					continue
				}
				ast.Inspect(gen, func(n ast.Node) bool {
					lit, isLit := n.(*ast.BasicLit)
					if !isLit || lit.Kind != token.STRING {
						return true
					}
					if v, err := strconv.Unquote(lit.Value); err != nil || !settingsGateWriteSQL.MatchString(v) {
						return true
					}
					add(label+":<package scope>", settingsGateKeyFromSQL(lit), lit.Pos())
					return true
				})
				continue
			}
			if fn.Body == nil {
				continue
			}
			fnLabel := settingsGateWriteSiteLabel(fn, label)
			storeVars := map[string]bool{}
			var calls []*ast.CallExpr
			sqlLits := map[token.Pos]*ast.BasicLit{}
			consumed := map[token.Pos]bool{}
			pending := map[*ast.CallExpr]bool{}

			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if lit, isLit := n.(*ast.BasicLit); isLit && lit.Kind == token.STRING {
					if v, err := strconv.Unquote(lit.Value); err == nil && settingsGateWriteSQL.MatchString(v) {
						sqlLits[lit.Pos()] = lit
					}
				}
				switch node := n.(type) {
				case *ast.AssignStmt:
					if len(node.Lhs) == 1 && len(node.Rhs) == 1 {
						if id, isIdent := node.Lhs[0].(*ast.Ident); isIdent && settingsGateIsSettingsStoreCtor(node.Rhs[0]) {
							storeVars[id.Name] = true
						}
					}
				case *ast.DeclStmt:
					gen, isGen := node.Decl.(*ast.GenDecl)
					if !isGen {
						break
					}
					for _, spec := range gen.Specs {
						vs, isValue := spec.(*ast.ValueSpec)
						if !isValue || len(vs.Names) != 1 || len(vs.Values) != 1 {
							continue
						}
						if settingsGateIsSettingsStoreCtor(vs.Values[0]) {
							storeVars[vs.Names[0].Name] = true
						}
					}
				case *ast.CallExpr:
					calls = append(calls, node)
				}
				return true
			})

			// Classification runs over the calls in reverse pre-order, i.e.
			// children first: `db.Exec(ctx, db.Rebind(stmt), key, value)` nests
			// the statement one level below the argument that carries the key,
			// so the inner call must already be pending when the outer one is
			// asked to resolve it.
			for i := len(calls) - 1; i >= 0; i-- {
				call := calls[i]
				if sel, isSel := call.Fun.(*ast.SelectorExpr); isSel && sel.Sel.Name == "Set" && len(call.Args) > 0 {
					if id, isIdent := sel.X.(*ast.Ident); isIdent && storeVars[id.Name] {
						add(fnLabel, resolve(call.Args[0]), call.Args[0].Pos())
						continue
					}
				}
				sqlIdx := -1
				for j, arg := range call.Args {
					lit, isLit := arg.(*ast.BasicLit)
					if !isLit || lit.Kind != token.STRING {
						continue
					}
					if _, isSQL := sqlLits[lit.Pos()]; !isSQL {
						continue
					}
					sqlIdx = j
					consumed[lit.Pos()] = true
					break
				}
				if sqlIdx >= 0 {
					lit := call.Args[sqlIdx].(*ast.BasicLit)
					if key := settingsGateKeyFromSQL(lit); key != "" {
						add(fnLabel, key, lit.Pos())
						continue
					}
					if key, pos := settingsGateRawKeyArg(call, sqlIdx, resolve); key != "" {
						add(fnLabel, key, pos)
						continue
					}
					pending[call] = true
					continue
				}
				// The statement is one level down; this call supplies the key.
				for _, arg := range call.Args {
					inner, isCall := arg.(*ast.CallExpr)
					if !isCall || !pending[inner] {
						continue
					}
					if key, pos := settingsGateRawKeyArg(call, -1, resolve); key != "" {
						add(fnLabel, key, pos)
						delete(pending, inner)
					}
					break
				}
			}

			for call := range pending {
				add(fnLabel, "", call.Pos())
			}
			// A statement no call consumed is built here and executed through a
			// variable, so the key is whatever the caller passes.
			for pos, lit := range sqlLits {
				if !consumed[pos] {
					add(fnLabel, settingsGateKeyFromSQL(lit), pos)
				}
			}
		}
	}

	sort.Slice(sites, func(i, j int) bool {
		if sites[i].site != sites[j].site {
			return sites[i].site < sites[j].site
		}
		return sites[i].key < sites[j].key
	})
	return sites
}

// settingsGateRepoWriteSites walks the repository and collects the settings
// writes of every production package except handler/admin, which R1 already
// owns through settingsGatePersistHelpers: its helpers take the key as a
// parameter by design and R1 resolves it at their call sites.
func settingsGateRepoWriteSites(t *testing.T, root string) (sites []settingsGateWriteSite, parsedFiles, parsedPkgs int, saw map[string]bool) {
	t.Helper()
	skip := map[string]bool{"web": true, "node_modules": true, "vendor": true, "dist": true}
	repoRel := func(path string) string {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return filepath.Base(path)
		}
		return filepath.ToSlash(rel)
	}

	byDir := map[string][]string{}
	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := entry.Name()
		if entry.IsDir() {
			if path == root {
				return nil
			}
			if strings.HasPrefix(name, ".") || skip[name] || repoRel(path) == "handler/admin" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		dir := filepath.Dir(path)
		if _, seen := byDir[dir]; !seen {
			dirs = append(dirs, dir)
		}
		byDir[dir] = append(byDir[dir], path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(dirs)

	fset := token.NewFileSet()
	saw = map[string]bool{}
	for _, dir := range dirs {
		var files []*ast.File
		for _, path := range byDir[dir] {
			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				t.Fatalf("parse %s: %v", repoRel(path), parseErr)
			}
			files = append(files, file)
			parsedFiles++
			saw[repoRel(path)] = true
		}
		parsedPkgs++
		sites = append(sites, settingsGatePackageWriteSites(fset, files, repoRel)...)
	}
	return sites, parsedFiles, parsedPkgs, saw
}

func TestSettingsRehydrationGateCoversEveryRepoWriteKey(t *testing.T) {
	dir := settingsGateDir(t)
	root := filepath.Join(dir, "..")
	sites, parsedFiles, parsedPkgs, saw := settingsGateRepoWriteSites(t, root)

	// The walk is the part most likely to break quietly — a renamed directory,
	// a new ignore rule — so it is asserted before anything reads its output.
	if parsedFiles < 100 || parsedPkgs < 10 {
		t.Fatalf("repo walk parsed %d files in %d packages; the walk is broken", parsedFiles, parsedPkgs)
	}
	for _, probe := range []string{"service/oauth/connection.go", "service/catalogsync/store.go", "store/setting_store.go"} {
		if !saw[probe] {
			t.Fatalf("repo walk never parsed %s; the walk is broken", probe)
		}
	}
	if len(sites) == 0 {
		t.Fatal("R6 extractor found no settings writers outside handler/admin; the matcher is broken")
	}

	byKey := map[string]string{}
	dynamic := map[string]bool{}
	for _, site := range sites {
		if site.key == "" {
			dynamic[site.site] = true
			continue
		}
		if _, seen := byKey[site.key]; !seen {
			byKey[site.key] = fmt.Sprintf("%s:%d", site.site, site.pos.Line)
		}
	}
	// The two keys R1 could never see, named explicitly: the marker is what
	// logged a hydration warning on every boot of every deployment.
	for _, probe := range []string{"oauth.identity_backfill_complete", "catalog_auto_sync_enabled"} {
		if _, ok := byKey[probe]; !ok {
			t.Fatalf("R6 extractor missed %q (found %v); the matcher is broken", probe, sortedKeySites(byKey))
		}
	}

	hydrated := settingsGateHydratedKeys(t, filepath.Join(dir, "settings.go"))
	var missing []string
	for key, where := range byKey {
		if hydrated[key] {
			continue
		}
		if _, allowlisted := nonHydratedSettingKeys[key]; !allowlisted {
			missing = append(missing, fmt.Sprintf("%s (written at %s)", key, where))
		}
	}

	var unregistered, noReason []string
	for site := range dynamic {
		reason, ok := settingsGateDynamicWriteSites[site]
		if !ok {
			unregistered = append(unregistered, site)
			continue
		}
		if strings.TrimSpace(reason) == "" {
			noReason = append(noReason, site)
		}
	}
	var stale []string
	for site := range settingsGateDynamicWriteSites {
		if !dynamic[site] {
			stale = append(stale, site)
		}
	}
	sort.Strings(missing)
	sort.Strings(unregistered)
	sort.Strings(noReason)
	sort.Strings(stale)

	if len(missing) > 0 {
		t.Errorf("R6: settings keys written outside handler/admin that startup hydration never reads back "+
			"(every boot logs them as an unapplied setting, drowning the warnings that are real):\n  %s\n"+
			"Fix: add a case to ApplyRuntimeSettings, or — only when the consumer really reads the settings "+
			"table itself — list the key in nonHydratedSettingKeys with a reason.",
			strings.Join(missing, "\n  "))
	}
	if len(unregistered) > 0 || len(noReason) > 0 {
		t.Errorf("R6: settings writers whose key is not statically resolvable must be registered in "+
			"settingsGateDynamicWriteSites with the reason they are still covered:\n  unregistered: %s\n  without a reason: %s",
			strings.Join(unregistered, ", "), strings.Join(noReason, ", "))
	}
	if len(stale) > 0 {
		t.Errorf("R6: stale settingsGateDynamicWriteSites entries no longer written anywhere in the repo: %s",
			strings.Join(stale, ", "))
	}
}

func sortedKeySites(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for key, where := range m {
		out = append(out, key+"@"+where)
	}
	sort.Strings(out)
	return out
}

// TestSettingsRepoWriteSiteMatcherSanity locks the R6 extractor with one
// counter-example per write shape, so a refactor cannot quietly empty the site
// set and turn the rule into a no-op. The read-only query and the value/error
// literals sitting next to a key are the false positives it must not produce.
func TestSettingsRepoWriteSiteMatcherSanity(t *testing.T) {
	// @BT@ stands in for a backtick: the fixture is itself a raw string.
	fixture := strings.ReplaceAll(`package svc

const markerKey = "svc.marker_done"

var ToggleKey = "svc_toggle"

func (s *Store) SetToggle(ctx context.Context, on bool) error {
	value := "true"
	if !on {
		value = "false"
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(@BT@INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT (key) DO UPDATE SET value = excluded.value@BT@),
		ToggleKey, value)
	if err != nil {
		return fmt.Errorf("svc: write toggle: %w", err)
	}
	return nil
}

func MarkDone(db *store.DB) {
	settings := store.NewSettingsStore(db)
	if done, _ := settings.Get(markerKey); done == "1" {
		return
	}
	if err := settings.Set(markerKey, "1"); err != nil {
		fmt.Println("svc: marker not set")
	}
}

func rewriteLegacy(db *sql.DB) {
	db.Exec("UPDATE settings SET value = ? WHERE key = 'legacy_key'", "x")
}

func upsert(db *sql.DB, tx *sql.Tx, key string, value any) error {
	query := db.Rebind(@BT@INSERT INTO settings (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value@BT@)
	_, err := tx.Exec(query, key, value)
	return err
}

func primitive(db *sql.DB, key, value string) error {
	query := ""
	query = @BT@INSERT INTO settings (key, value) VALUES (?, ?)@BT@
	_, err := db.Exec(query, key, value)
	return err
}

const bulkUpsert = "INSERT INTO settings (key, value) VALUES ('pkg_scope_key', ?)"

func readOnly(db *sql.DB) (string, error) {
	var v string
	err := db.QueryRow(@BT@SELECT value FROM settings WHERE key = 'not_a_write'@BT@).Scan(&v)
	return v, err
}

func chatty(db *sql.DB) error {
	_, err := db.Exec("DELETE FROM accounts")
	if err != nil {
		return fmt.Errorf("failed to update settings: %w", err)
	}
	return nil
}
`, "@BT@", "`")

	path := filepath.Join(t.TempDir(), "svc.go")
	if err := os.WriteFile(path, []byte(fixture), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, fixture, 0)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	sites := settingsGatePackageWriteSites(fset, []*ast.File{file}, func(string) string { return "svc.go" })

	got := map[string]string{} // site -> key ("" = dynamic writer)
	for _, site := range sites {
		if existing, ok := got[site.site]; ok && existing != site.key {
			t.Fatalf("%s reported two keys (%q and %q); one writer must resolve to one key", site.site, existing, site.key)
		}
		got[site.site] = site.key
	}
	want := map[string]string{
		"svc.go:(*Store).SetToggle": "svc_toggle",
		"svc.go:MarkDone":           "svc.marker_done",
		"svc.go:rewriteLegacy":      "legacy_key",
		"svc.go:upsert":             "",
		"svc.go:primitive":          "",
		"svc.go:<package scope>":    "pkg_scope_key",
	}
	if len(got) != len(want) {
		t.Fatalf("extractor returned %d sites (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for site, key := range want {
		gotKey, ok := got[site]
		if !ok {
			t.Fatalf("extractor missed %q (got %v)", site, got)
		}
		if gotKey != key {
			t.Fatalf("%q resolved to %q, want %q", site, gotKey, key)
		}
	}
	for _, notASite := range []string{"svc.go:readOnly", "svc.go:chatty", "svc.go:SetToggle"} {
		if _, ok := got[notASite]; ok {
			t.Fatalf("extractor collected %q, which persists no settings key", notASite)
		}
	}
	for _, notAKey := range []string{"true", "false", "svc: write toggle: %w", "not_a_write", "svc: marker not set", "failed to update settings: %w", "x"} {
		for _, site := range sites {
			if site.key == notAKey {
				t.Fatalf("extractor reported %q as a settings key at %s", notAKey, site.site)
			}
		}
	}
}
