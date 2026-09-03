package store

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// The canonical table registry.
//
// Every table list the runtime needs derives from this one slice:
//
//   - AutoMigrate's CREATE TABLE order (store/migrate.go)
//   - the dialect-copy table set cmd/migrate transfers (AllTableNames)
//   - the FK-safe wipe order used before a re-insert (ClearTableNames)
//   - the factory-reset set (FactoryResetTableNames)
//   - the per-table column spec that drives the generic copy builder and the
//     insert-builder drift guard (schemaColumns)
//
// Adding a table here is therefore the only step needed for it to be created,
// copied, checksum-verified, cleared on --overwrite and wiped by a factory
// reset. The bug this file replaces was three hand-maintained lists that had
// drifted to 20/28 entries against a 37-table schema, so 17 tables were
// silently dropped by cmd/migrate and 9 survived a factory reset (including
// admin_sessions, which left pre-reset admin cookies valid).

// schemaTable binds a table name to its dual-dialect DDL builder.
type schemaTable struct {
	name  string
	build func(dialect string) string
}

// schemaTables lists every application table in creation order. Foreign keys
// point "upward" in this list (parents before children); the migration and
// clear orders are derived topologically rather than trusting this order.
var schemaTables = []schemaTable{
	{"sites", buildSitesDDL},
	{"site_api_endpoints", buildSiteAPIEndpointsDDL},
	{"site_disabled_models", buildSiteDisabledModelsDDL},
	{"accounts", buildAccountsDDL},
	{"account_tokens", buildAccountTokensDDL},
	{"checkin_logs", buildCheckinLogsDDL},
	{"model_availability", buildModelAvailabilityDDL},
	{"token_model_availability", buildTokenModelAvailabilityDDL},
	{"token_routes", buildTokenRoutesDDL},
	{"route_group_sources", buildRouteGroupSourcesDDL},
	{"oauth_route_units", buildOAuthRouteUnitsDDL},
	{"oauth_route_unit_members", buildOAuthRouteUnitMembersDDL},
	{"route_channels", buildRouteChannelsDDL},
	{"proxy_logs", buildProxyLogsDDL},
	{"proxy_video_tasks", buildProxyVideoTasksDDL},
	{"settings", buildSettingsDDL},
	{"analytics_projection_checkpoints", buildAnalyticsProjectionCheckpointsDDL},
	{"site_day_usage", buildSiteDayUsageDDL},
	{"site_hour_usage", buildSiteHourUsageDDL},
	{"model_day_usage", buildModelDayUsageDDL},
	{"downstream_api_keys", buildDownstreamAPIKeysDDL},
	{"site_announcements", buildSiteAnnouncementsDDL},
	{"events", buildEventsDDL},
	{"admin_background_tasks", buildAdminBackgroundTasksDDL},
	{"balance_history", buildBalanceHistoryDDL},
	{"model_verify_history", buildModelVerifyHistoryDDL},
	{"product_announcements", buildProductAnnouncementsDDL},
	{"announcement_dismissals", buildAnnouncementDismissalsDDL},
	{"model_name_redirects", buildModelNameRedirectsDDL},
	{"admin_audit_logs", buildAdminAuditLogsDDL},
	{"model_probe_results", buildModelProbeResultsDDL},
	{"catalog_sources", buildCatalogSourcesDDL},
	{"admin_sessions", buildAdminSessionsDDL},
}

// migrationExcludedTables names schema tables deliberately NOT copied across
// dialects by cmd/migrate, with the reason. A table is excluded by an entry
// here — never by being absent from a hand-written list, which is how 17
// tables went missing. Empty today: every application table crosses the
// SQLite<->PostgreSQL boundary.
var migrationExcludedTables = map[string]string{}

// factoryResetExcludedTables names tables a factory reset must NOT wipe, with
// the reason. schema_migrations (the additive-migration journal created by
// ensureSchemaMigrationsTable) is not part of schemaTables at all, so it can
// never be swept; it is spelled out here because wiping it would replay every
// additive step against an already-converged schema.
var factoryResetExcludedTables = map[string]string{
	"schema_migrations": "additive-migration journal: bookkeeping, not business data; wiping it replays every step",
}

// backupExcludedTables names schema tables a backup export must NOT carry,
// with the operator-facing reason. Everything else in the registry ships in a
// type=all backup and is replayed by the import endpoints, so a table is left
// out of backups by an entry here — never by being absent from a hand-written
// list, which is how service/backup.AllTables drifted to 28 of 37 tables and
// silently dropped product_announcements, model_name_redirects and friends.
//
// The reasons are emitted verbatim into the export payload
// (metadata.excluded_tables) so a backup file states its own gaps; keep each
// one to a single sentence and put the long form in docs/api/settings.md.
var backupExcludedTables = map[string]string{
	// Session credential material. A backup is semi-trusted input (see
	// backupsvc.RuntimeLocalSettingKeys for the same threat model applied to
	// settings rows): importing token hashes would let a cookie issued on the
	// source deployment authenticate against this one. A restored deployment
	// must require a fresh login instead.
	"admin_sessions": "session credential material: an import must never plant admin session token hashes, so a restored deployment requires a fresh login",
	// One row per authenticated admin write, appended forever (no retention job
	// prunes it). Beyond volume, the rows describe operations performed against
	// the source deployment; replaying them into another database makes that
	// database's audit trail assert events that never happened there, which
	// destroys the only property an audit log has.
	"admin_audit_logs": "append-only audit trail of the source deployment's admin writes; replaying it into another database forges that database's audit record (and no retention job bounds its size)",
	// Appended by the background prober on every pass, and read back by
	// service/route_rebuild.go (latest row per account_id+model_name) to steer
	// routing. Imported probe rows from another deployment would therefore
	// drive this deployment's route rebuild on stale evidence; the prober
	// re-populates the table within one interval after a restore.
	"model_probe_results": "high-frequency background probe telemetry that route rebuild reads as its latest-per-model signal; stale rows from another deployment would steer routing, and the prober regenerates them after a restore",
	// Every row's url is fetched server-side by the catalog sync
	// (service/pricingcatalog provider on httpclient.SharedTransport, which
	// carries no dial-level SSRF guard), while the import URL guard
	// (service/import_url_guard.go importURLColumns) covers only sites and
	// site_api_endpoints today. Admitting these rows through a semi-trusted
	// backup would let a crafted payload plant a cloud-metadata / link-local
	// fetch target. Extend that guard, then delete this entry.
	"catalog_sources": "each row's url is fetched server-side by the catalog sync and the import URL guard does not cover this table yet, so a crafted backup could plant an SSRF fetch target",
}

// Column kinds reported by schemaColumns. They drive value coercion in the
// generic copy builder and the boolean-default parity assertions.
const (
	kindInt   = "int"
	kindFloat = "float"
	kindBool  = "bool"
	kindText  = "text"
)

// schemaColumn is one column of a table as declared by the DDL registry.
type schemaColumn struct {
	name string
	kind string
}

var fkParentRE = regexp.MustCompile(`(?i)\bREFERENCES\s+"?([A-Za-z_][A-Za-z0-9_]*)"?\s*\(`)

var schemaFactsOnce sync.Once
var schemaFactsVal *schemaFacts

type schemaFacts struct {
	// names in registry (creation) order
	registry []string
	// table -> tables it references
	parents map[string][]string
	// table -> ordered column spec parsed from the PostgreSQL DDL
	columns map[string][]schemaColumn
	// parents-first topological order of registry names
	order []string
	err   error
}

func facts() *schemaFacts {
	schemaFactsOnce.Do(func() { schemaFactsVal = buildSchemaFacts() })
	return schemaFactsVal
}

func buildSchemaFacts() *schemaFacts {
	f := &schemaFacts{
		parents: make(map[string][]string, len(schemaTables)),
		columns: make(map[string][]schemaColumn, len(schemaTables)),
	}
	seen := make(map[string]bool, len(schemaTables))
	for _, t := range schemaTables {
		if t.name == "" || t.build == nil {
			f.err = fmt.Errorf("store: schema registry entry %q is incomplete", t.name)
			return f
		}
		if seen[t.name] {
			f.err = fmt.Errorf("store: schema registry lists table %q twice", t.name)
			return f
		}
		seen[t.name] = true
		f.registry = append(f.registry, t.name)

		// The PostgreSQL spelling is the parse source: it names BOOLEAN and
		// DOUBLE PRECISION explicitly, where SQLite folds booleans into
		// INTEGER. Column names must be identical in both dialects; a test
		// pins that so the parser can never describe half the schema.
		pgCols, pgParents, err := parseCreateTable(t.build(DialectPostgres))
		if err != nil {
			f.err = fmt.Errorf("store: parse %s postgres DDL: %w", t.name, err)
			return f
		}
		sqliteCols, sqliteParents, err := parseCreateTable(t.build(DialectSQLite))
		if err != nil {
			f.err = fmt.Errorf("store: parse %s sqlite DDL: %w", t.name, err)
			return f
		}
		if !sameNames(pgCols, sqliteCols) {
			f.err = fmt.Errorf("store: %s declares different columns per dialect (pg=%v sqlite=%v)",
				t.name, columnNames(pgCols), columnNames(sqliteCols))
			return f
		}
		f.columns[t.name] = pgCols
		f.parents[t.name] = uniqueParents(t.name, append(append([]string{}, pgParents...), sqliteParents...))
	}
	f.order = topoParentsFirst(f.registry, f.parents)
	return f
}

func sameNames(a, b []schemaColumn) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].name != b[i].name {
			return false
		}
	}
	return true
}

func columnNames(cols []schemaColumn) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.name
	}
	return out
}

func uniqueParents(self string, parents []string) []string {
	seen := make(map[string]bool, len(parents))
	var out []string
	for _, p := range parents {
		if p == self || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

// parseCreateTable splits a CREATE TABLE statement into its column spec and
// the tables it references. Table-level clauses (CONSTRAINT / FOREIGN KEY /
// UNIQUE / CHECK / PRIMARY KEY) are not columns but still yield FK parents.
func parseCreateTable(ddl string) ([]schemaColumn, []string, error) {
	open := strings.Index(ddl, "(")
	closed := strings.LastIndex(ddl, ")")
	if open < 0 || closed <= open {
		return nil, nil, fmt.Errorf("not a CREATE TABLE statement")
	}
	body := ddl[open+1 : closed]

	var cols []schemaColumn
	for _, part := range splitTopLevel(body) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		upper := strings.ToUpper(part)
		if strings.HasPrefix(upper, "CONSTRAINT") ||
			strings.HasPrefix(upper, "FOREIGN KEY") ||
			strings.HasPrefix(upper, "PRIMARY KEY") ||
			strings.HasPrefix(upper, "UNIQUE") ||
			strings.HasPrefix(upper, "CHECK") {
			continue
		}
		fields := strings.Fields(part)
		name := strings.Trim(fields[0], `"`)
		if !isPlainIdent(name) {
			return nil, nil, fmt.Errorf("unexpected column declaration %q", part)
		}
		cols = append(cols, schemaColumn{name: name, kind: columnKind(strings.Join(fields[1:], " "))})
	}
	if len(cols) == 0 {
		return nil, nil, fmt.Errorf("no columns parsed")
	}

	var parents []string
	for _, m := range fkParentRE.FindAllStringSubmatch(ddl, -1) {
		parents = append(parents, m[1])
	}
	return cols, parents, nil
}

// splitTopLevel splits on commas that are not nested inside parentheses.
func splitTopLevel(body string) []string {
	var out []string
	depth := 0
	start := 0
	inQuote := false
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '\'':
			inQuote = !inQuote
		case '(':
			if !inQuote {
				depth++
			}
		case ')':
			if !inQuote {
				depth--
			}
		case ',':
			if depth == 0 && !inQuote {
				out = append(out, body[start:i])
				start = i + 1
			}
		}
	}
	out = append(out, body[start:])
	return out
}

func isPlainIdent(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return false
	}
	return true
}

// columnKind classifies a column by the type tokens that follow its name.
func columnKind(rest string) string {
	upper := strings.ToUpper(strings.TrimSpace(rest))
	switch {
	case strings.HasPrefix(upper, "BOOLEAN"), strings.HasPrefix(upper, "BOOL "):
		return kindBool
	case strings.HasPrefix(upper, "DOUBLE"), strings.HasPrefix(upper, "REAL"),
		strings.HasPrefix(upper, "NUMERIC"), strings.HasPrefix(upper, "FLOAT"),
		strings.HasPrefix(upper, "DECIMAL"):
		return kindFloat
	case strings.HasPrefix(upper, "BIGSERIAL"), strings.HasPrefix(upper, "SERIAL"),
		strings.HasPrefix(upper, "BIGINT"), strings.HasPrefix(upper, "SMALLINT"),
		strings.HasPrefix(upper, "INTEGER"), strings.HasPrefix(upper, "INT"):
		return kindInt
	default:
		return kindText
	}
}

// topoParentsFirst orders names so every table precedes the tables that
// reference it. Ties break by registry order, so the result is deterministic.
// A cycle (no FK edge in this schema has one) degrades to registry order for
// the remaining tables instead of looping forever.
func topoParentsFirst(names []string, parents map[string][]string) []string {
	inSet := make(map[string]bool, len(names))
	for _, n := range names {
		inSet[n] = true
	}
	emitted := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for len(out) < len(names) {
		progressed := false
		for _, n := range names {
			if emitted[n] {
				continue
			}
			ready := true
			for _, p := range parents[n] {
				if inSet[p] && !emitted[p] {
					ready = false
					break
				}
			}
			if ready {
				out = append(out, n)
				emitted[n] = true
				progressed = true
				break
			}
		}
		if !progressed {
			for _, n := range names {
				if !emitted[n] {
					out = append(out, n)
					emitted[n] = true
				}
			}
			break
		}
	}
	return out
}

// SchemaTableNames returns every application table AutoMigrate creates, in
// registry order. It is the authoritative table set; the measured table count
// of a migrated database must equal len(SchemaTableNames()).
func SchemaTableNames() []string {
	f := facts()
	out := make([]string, len(f.registry))
	copy(out, f.registry)
	return out
}

// schemaRegistryErr surfaces a malformed registry (duplicate name, unparsable
// DDL, per-dialect column drift) instead of panicking later.
func schemaRegistryErr() error { return facts().err }

// AllTableNames returns the application tables transferred between dialects by
// cmd/migrate, ordered parents-before-children so inserts satisfy foreign keys.
// It is derived from the schema registry minus migrationExcludedTables; this is
// the single source of truth for the migration table set and cmd/migrate must
// not maintain its own copy.
func AllTableNames() []string {
	f := facts()
	out := make([]string, 0, len(f.order))
	for _, t := range f.order {
		if _, skip := migrationExcludedTables[t]; skip {
			continue
		}
		out = append(out, t)
	}
	return out
}

// ClearTableNames returns the same table set in FK-safe delete order (children
// before parents): the reverse of AllTableNames, which is topologically
// parents-first. Used to wipe a target database before a re-insert.
func ClearTableNames() []string {
	all := AllTableNames()
	out := make([]string, len(all))
	for i, t := range all {
		out[len(all)-1-i] = t
	}
	return out
}

// FactoryResetTableNames returns every table a factory reset must wipe, in
// FK-safe delete order, derived from the same registry that drives
// AutoMigrate and cmd/migrate minus factoryResetExcludedTables. Deriving it
// (instead of reusing the backup export list) is what makes admin_sessions,
// admin_audit_logs and the other previously-missed tables part of the reset:
// a pre-reset admin cookie must not survive "restore factory settings".
func FactoryResetTableNames() []string {
	out := make([]string, 0, len(schemaTables))
	for _, t := range ClearTableNames() {
		if _, skip := factoryResetExcludedTables[t]; skip {
			continue
		}
		out = append(out, t)
	}
	return out
}

// BackupTableNames returns the tables a backup export carries, ordered
// parents-before-children so an import can replay the payload in one pass
// without violating a foreign key. It is the schema registry minus
// backupExcludedTables; service/backup must not keep its own copy of the list.
func BackupTableNames() []string {
	out := make([]string, 0, len(schemaTables))
	for _, t := range AllTableNames() {
		if _, skip := backupExcludedTables[t]; skip {
			continue
		}
		out = append(out, t)
	}
	return out
}

// BackupExcludedTables returns a copy of the backup exclusion set: table name
// to the reason it is not carried. Callers surface it to operators
// (metadata.excluded_tables in the export payload) and the drift guard checks
// it against the registry, so an exclusion can never be silent or orphaned.
func BackupExcludedTables() map[string]string {
	out := make(map[string]string, len(backupExcludedTables))
	for t, reason := range backupExcludedTables {
		out[t] = reason
	}
	return out
}

// schemaColumns returns the parsed column spec of a table.
func schemaColumns(table string) ([]schemaColumn, error) {
	f := facts()
	if f.err != nil {
		return nil, f.err
	}
	cols, ok := f.columns[table]
	if !ok {
		return nil, fmt.Errorf("store: table %q is not in the schema registry", table)
	}
	out := make([]schemaColumn, len(cols))
	copy(out, cols)
	return out, nil
}

// tableHasSerialID reports whether a table's spec declares an "id" column, so
// sequence syncing after a copy can skip text-PK tables (settings,
// admin_sessions, analytics_projection_checkpoints) without a hardcoded list.
func tableHasSerialID(table string) bool {
	cols, err := schemaColumns(table)
	if err != nil {
		return false
	}
	for _, c := range cols {
		if c.name == "id" {
			return true
		}
	}
	return false
}
