package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/service/upstream"
	"github.com/deliciousbuding/metapi-go/store"
)

// Query-contract tests for the marketplace: search, filters, sort, totals,
// and facets are computed over the whole marketplace before pagination, and
// the exact-name detail endpoint resolves models that are not on the loaded
// page. Pure filter/sort/facet semantics are covered without a database; the
// HTTP tests pin the wire contract on SQLite and PostgreSQL.

func marketplaceQueryForTest(raw string) marketplaceQuery {
	req := httptest.NewRequest(http.MethodGet, "/api/models/marketplace?"+raw, nil)
	return parseMarketplaceQuery(req)
}

func TestParseMarketplaceSortWhitelist(t *testing.T) {
	cases := []struct {
		raw    string
		echo   string
		column string
		desc   bool
	}{
		{"", "name.asc", "name", false},
		{"name:desc", "name.desc", "name", true},
		{"supplyCount.desc", "supplyCount.desc", "supplyCount", true},
		{"avgLatency:asc", "avgLatency.asc", "avgLatency", false},
		{"priceInput:desc", "priceInput.desc", "priceInput", true},
		// Unknown columns and malformed values degrade to the default instead
		// of failing the request.
		{"brand:asc", "name.asc", "name", false},
		{"name:sideways", "name.asc", "name", false},
		{"name", "name.asc", "name", false},
		{"; DROP TABLE", "name.asc", "name", false},
		// Only the first term is honoured.
		{"successRate:desc,name:asc", "successRate.desc", "successRate", true},
	}
	for _, tc := range cases {
		echo, column, desc := parseMarketplaceSort(tc.raw)
		if echo != tc.echo || column != tc.column || desc != tc.desc {
			t.Errorf("parseMarketplaceSort(%q) = %q/%q/%v, want %q/%q/%v",
				tc.raw, echo, column, desc, tc.echo, tc.column, tc.desc)
		}
	}
}

func TestParseMarketplaceQueryNormalizesLists(t *testing.T) {
	q := marketplaceQueryForTest("q=+Vision+&capability=Vision,REASONING,vision&endpointType=openai&sourceKind=Direct")
	if q.Q != "Vision" {
		t.Errorf("q = %q, want trimmed raw", q.Q)
	}
	if !reflect.DeepEqual(q.Capabilities, []string{"vision", "reasoning"}) {
		t.Errorf("capabilities = %v, want lowercased dedup", q.Capabilities)
	}
	if !reflect.DeepEqual(q.EndpointTypes, []string{"openai"}) {
		t.Errorf("endpointTypes = %v", q.EndpointTypes)
	}
	if !reflect.DeepEqual(q.SourceKinds, []string{"direct"}) {
		t.Errorf("sourceKinds = %v", q.SourceKinds)
	}
}

func marketplaceIndexRowForTest(name string, kinds []string, tags []string, endpoints []string) marketplaceIndexRow {
	row := marketplaceIndexRow{
		name:          name,
		sourceKinds:   map[string]struct{}{},
		tags:          tags,
		endpointTypes: endpoints,
	}
	for _, kind := range kinds {
		row.sourceKinds[kind] = struct{}{}
	}
	return row
}

func TestFilterMarketplaceIndexSemantics(t *testing.T) {
	rows := []marketplaceIndexRow{
		func() marketplaceIndexRow {
			r := marketplaceIndexRowForTest("gpt-4o", []string{"account"}, []string{"vision"}, []string{"openai"})
			r.searchHaystack = "gpt-4o\x00vision\x00relay-a"
			return r
		}(),
		func() marketplaceIndexRow {
			r := marketplaceIndexRowForTest("claude-opus", []string{"oauth"}, []string{"reasoning"}, []string{"anthropic"})
			r.searchHaystack = "claude-opus\x00reasoning\x00relay-b"
			return r
		}(),
		func() marketplaceIndexRow {
			r := marketplaceIndexRowForTest("deepseek-v4", []string{"direct"}, []string{"reasoning", "tool_call"}, []string{"openai"})
			r.searchHaystack = "deepseek-v4\x00direct-conn"
			return r
		}(),
	}

	filterNames := func(raw string) []string {
		q := marketplaceQueryForTest(raw)
		var names []string
		for _, row := range filterMarketplaceIndex(rows, q) {
			names = append(names, row.name)
		}
		return names
	}

	// q is a case-insensitive substring over the joined haystack.
	if got := filterNames("q=RELAY-B"); !reflect.DeepEqual(got, []string{"claude-opus"}) {
		t.Errorf("q case-insensitive = %v", got)
	}
	// OR within one dimension.
	if got := filterNames("capability=vision,tool_call"); len(got) != 2 {
		t.Errorf("capability OR = %v, want 2 rows", got)
	}
	// AND across dimensions.
	if got := filterNames("capability=reasoning&endpointType=openai"); !reflect.DeepEqual(got, []string{"deepseek-v4"}) {
		t.Errorf("AND across dimensions = %v", got)
	}
	if got := filterNames("endpointType=anthropic&sourceKind=oauth"); !reflect.DeepEqual(got, []string{"claude-opus"}) {
		t.Errorf("endpoint+sourceKind = %v", got)
	}
	// A model matches sourceKind when ANY of its sources has the kind.
	if got := filterNames("sourceKind=direct,account"); len(got) != 2 {
		t.Errorf("sourceKind OR = %v, want 2 rows", got)
	}
	// Unknown source kinds match nothing rather than disabling the filter.
	if got := filterNames("sourceKind=bogus"); len(got) != 0 {
		t.Errorf("unknown sourceKind = %v, want empty", got)
	}
	// Capability/tag matching is case-insensitive against stored values.
	if got := filterNames("capability=TOOL_CALL"); !reflect.DeepEqual(got, []string{"deepseek-v4"}) {
		t.Errorf("capability case = %v", got)
	}
}

func TestSortMarketplaceIndexNullsLastAndStable(t *testing.T) {
	lat := func(v float64) *float64 { return &v }
	rows := []marketplaceIndexRow{
		{name: "b-model", avgLatency: lat(200)},
		{name: "z-model", avgLatency: nil},
		{name: "a-model", avgLatency: lat(200)},
		{name: "c-model", avgLatency: lat(90)},
	}
	names := func() []string {
		out := make([]string, 0, len(rows))
		for _, row := range rows {
			out = append(out, row.name)
		}
		return out
	}

	sortMarketplaceIndex(rows, marketplaceQuery{Sort: "avgLatency.asc", sortColumn: "avgLatency"})
	if got := names(); !reflect.DeepEqual(got, []string{"c-model", "a-model", "b-model", "z-model"}) {
		t.Fatalf("asc = %v, want measured first (ties by name), unmeasured last", got)
	}

	sortMarketplaceIndex(rows, marketplaceQuery{Sort: "avgLatency.desc", sortColumn: "avgLatency", sortDesc: true})
	if got := names(); !reflect.DeepEqual(got, []string{"a-model", "b-model", "c-model", "z-model"}) {
		t.Fatalf("desc = %v, want unmeasured still last", got)
	}

	sortMarketplaceIndex(rows, marketplaceQuery{Sort: "name.desc", sortColumn: "name", sortDesc: true})
	if got := names(); !reflect.DeepEqual(got, []string{"z-model", "c-model", "b-model", "a-model"}) {
		t.Fatalf("name desc = %v", got)
	}
}

func TestBuildMarketplaceFacetsDisjunctive(t *testing.T) {
	rows := []marketplaceIndexRow{}
	for _, row := range []marketplaceIndexRow{
		marketplaceIndexRowForTest("gpt-4o", []string{"account"}, []string{"vision"}, []string{"openai"}),
		marketplaceIndexRowForTest("claude-opus", []string{"oauth"}, []string{"reasoning"}, []string{"anthropic"}),
		marketplaceIndexRowForTest("deepseek-v4", []string{"direct"}, []string{"reasoning"}, []string{"openai"}),
	} {
		// The query layer searches haystacks; facets count rows.
		row.searchHaystack = row.name
		rows = append(rows, row)
	}
	facetCount := func(facets []marketplaceFacetValue, value string) int {
		for _, facet := range facets {
			if facet.Value == value {
				return facet.Count
			}
		}
		return -1
	}

	// Selecting endpointType=anthropic must not collapse the endpoint-type
	// facet: alternatives stay visible with counts from the disjunctive
	// context (q + the other dimensions still apply).
	q := marketplaceQueryForTest("endpointType=anthropic")
	facets := buildMarketplaceFacets(rows, q)
	if got := facetCount(facets.EndpointTypes, "openai"); got != 2 {
		t.Errorf("endpointTypes[openai] = %d, want 2 (self-dimension excluded)", got)
	}
	if got := facetCount(facets.EndpointTypes, "anthropic"); got != 1 {
		t.Errorf("endpointTypes[anthropic] = %d, want 1", got)
	}
	// Other dimensions still constrain the facet context.
	if got := facetCount(facets.Capabilities, "reasoning"); got != 1 {
		t.Errorf("capabilities[reasoning] = %d, want 1 (anthropic rows only)", got)
	}
	if got := facetCount(facets.SourceKinds, "oauth"); got != 1 {
		t.Errorf("sourceKinds[oauth] = %d, want 1", got)
	}
	if got := facetCount(facets.SourceKinds, "account"); got != -1 {
		t.Errorf("sourceKinds[account] = %d, want absent", got)
	}

	// q narrows every facet context.
	q = marketplaceQueryForTest("q=gpt")
	facets = buildMarketplaceFacets(rows, q)
	if got := facetCount(facets.Capabilities, "vision"); got != 1 {
		t.Errorf("q=gpt capabilities[vision] = %d, want 1", got)
	}
	if got := facetCount(facets.Capabilities, "reasoning"); got != -1 {
		t.Errorf("q=gpt capabilities[reasoning] = %d, want absent", got)
	}
}

func TestLowestCatalogInputRate(t *testing.T) {
	if got := lowestCatalogInputRate([]any{}); got != nil {
		t.Errorf("empty = %v, want nil", got)
	}
	sources := []any{
		map[string]any{"groupPricing": map[string]any{
			"default": map[string]any{"inputPerMillion": 2.5},
			"vip":     map[string]any{"inputPerMillion": 1.25},
		}},
		map[string]any{"groupPricing": map[string]any{
			"catalog": map[string]any{"inputPerMillion": 3.0},
		}},
	}
	if got := lowestCatalogInputRate(sources); got == nil || *got != 1.25 {
		t.Errorf("lowest = %v, want 1.25", got)
	}
	// Unusable shapes are skipped, not mistaken for a zero price.
	if got := lowestCatalogInputRate([]any{map[string]any{"groupPricing": "nope"}}); got != nil {
		t.Errorf("malformed = %v, want nil", got)
	}
}

// ---- HTTP wire contract ----

type marketplaceQueryEnvelope struct {
	Items      []map[string]any `json:"items"`
	Total      int              `json:"total"`
	Page       int              `json:"page"`
	PageSize   int              `json:"pageSize"`
	FacetScope string           `json:"facetScope"`
	Facets     struct {
		EndpointTypes []marketplaceFacetValue `json:"endpointTypes"`
		Capabilities  []marketplaceFacetValue `json:"capabilities"`
		SourceKinds   []marketplaceFacetValue `json:"sourceKinds"`
	} `json:"facets"`
	Meta struct {
		Query struct {
			Q             string   `json:"q"`
			Capabilities  []string `json:"capabilities"`
			EndpointTypes []string `json:"endpointTypes"`
			SourceKinds   []string `json:"sourceKinds"`
			Sort          string   `json:"sort"`
		} `json:"query"`
	} `json:"meta"`
}

func decodeMarketplaceQueryEnvelope(t *testing.T, resp *httptest.ResponseRecorder) marketplaceQueryEnvelope {
	t.Helper()
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	var envelope marketplaceQueryEnvelope
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode query envelope: %v; body=%s", err, resp.Body.String())
	}
	return envelope
}

func marketplaceItemNames(envelope marketplaceQueryEnvelope) []string {
	names := make([]string, 0, len(envelope.Items))
	for _, item := range envelope.Items {
		names = append(names, fmt.Sprint(item["name"]))
	}
	return names
}

// seedMarketplaceQueryFixture builds one site with a plain account and an
// OAuth account, plus a standalone direct grant, so every supply kind is
// present:
//
//	mq-claude-alpha  account supply, anthropic dialect (latency 200)
//	mq-gpt-beta      account + oauth supply, openai dialect (latencies 100 and 120)
//	mq-gpt-direct    direct grant supply, openai dialect (no latency)
func seedMarketplaceQueryFixture(t *testing.T, db *store.DB) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)

	siteID, err := execInsertID(db.DB,
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES ('mq-site', 'https://mq.example.test', 'openai', 'active', ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	plainAccount, err := execInsertID(db.DB,
		`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
		 VALUES (?, 'mq-plain', 'token', 'active', false, ?, ?)`, siteID, now, now)
	if err != nil {
		t.Fatalf("insert plain account: %v", err)
	}
	oauthAccount, err := execInsertID(db.DB,
		`INSERT INTO accounts (site_id, username, access_token, oauth_provider, status, checkin_enabled, created_at, updated_at)
		 VALUES (?, 'mq-oauth', 'token', 'mq-oauth-provider', 'active', false, ?, ?)`, siteID, now, now)
	if err != nil {
		t.Fatalf("insert oauth account: %v", err)
	}
	availability := []struct {
		accountID int64
		model     string
		latency   int
	}{
		{plainAccount, "mq-claude-alpha", 200},
		{plainAccount, "mq-gpt-beta", 100},
		{oauthAccount, "mq-gpt-beta", 120},
	}
	for _, row := range availability {
		if _, err := db.Exec(db.Rebind(
			`INSERT INTO model_availability (account_id, model_name, available, is_manual, latency_ms, checked_at)
			 VALUES (?, ?, true, false, ?, ?)`), row.accountID, row.model, row.latency, now); err != nil {
			t.Fatalf("insert availability %s: %v", row.model, err)
		}
	}

	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	insert := func(entity string, args ...any) int64 {
		t.Helper()
		id, err := upstream.InsertNative(context.Background(), tx, entity, args...)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	channelID := insert("channels", "MQ Direct", "generic", "newapi", true, "https://mq-direct.example.test", store.DirectEndpoints{}, "", "", "", false, "", "", "", "")
	credentialID := insert("credentials", channelID, "MQ Key", "query-fixture-only", true, "api_key", "{}")
	modelID := insert("models", channelID, "mq-gpt-direct", true)
	insert("grants", modelID, credentialID, 1, true)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestModelsMarketplace_QueryContractSQLite(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedMarketplaceQueryFixture(t, db)

	// Search reaches the whole marketplace, not the loaded page: the match
	// sits beyond the first page of the unfiltered listing.
	pageOne := decodeMarketplaceQueryEnvelope(t, doGet(t, r, "/api/models/marketplace?page=1&pageSize=2"))
	if pageOne.Total != 3 || len(pageOne.Items) != 2 {
		t.Fatalf("unfiltered page 1 = total %d items %d, want 3/2", pageOne.Total, len(pageOne.Items))
	}
	search := decodeMarketplaceQueryEnvelope(t, doGet(t, r, "/api/models/marketplace?page=1&pageSize=2&q=BETA"))
	if search.Total != 1 || !reflect.DeepEqual(marketplaceItemNames(search), []string{"mq-gpt-beta"}) {
		t.Fatalf("q=BETA = total %d items %v", search.Total, marketplaceItemNames(search))
	}
	if search.Meta.Query.Q != "BETA" {
		t.Fatalf("query echo q = %q, want raw trimmed value", search.Meta.Query.Q)
	}

	// total shares the filter context; facets stay disjunctive per dimension.
	filtered := decodeMarketplaceQueryEnvelope(t, doGet(t, r, "/api/models/marketplace?page=1&pageSize=50&sourceKind=oauth"))
	if filtered.Total != 1 || !reflect.DeepEqual(marketplaceItemNames(filtered), []string{"mq-gpt-beta"}) {
		t.Fatalf("sourceKind=oauth = total %d items %v", filtered.Total, marketplaceItemNames(filtered))
	}
	if filtered.FacetScope != marketplaceFacetScope {
		t.Fatalf("facetScope = %q, want %q", filtered.FacetScope, marketplaceFacetScope)
	}
	sourceFacet := map[string]int{}
	for _, facet := range filtered.Facets.SourceKinds {
		sourceFacet[facet.Value] = facet.Count
	}
	if sourceFacet["account"] != 2 || sourceFacet["oauth"] != 1 || sourceFacet["direct"] != 1 {
		t.Fatalf("sourceKinds facet = %v, want account=2 oauth=1 direct=1 (self filter excluded)", sourceFacet)
	}

	// AND across dimensions: the direct-only model carries no oauth supply.
	combined := decodeMarketplaceQueryEnvelope(t, doGet(t, r, "/api/models/marketplace?page=1&pageSize=50&sourceKind=direct&endpointType=openai"))
	if combined.Total != 1 || !reflect.DeepEqual(marketplaceItemNames(combined), []string{"mq-gpt-direct"}) {
		t.Fatalf("direct+openai = total %d items %v", combined.Total, marketplaceItemNames(combined))
	}

	// Sort: measured rows first by latency, unmeasured last in both
	// directions; numeric ties break by name.
	asc := decodeMarketplaceQueryEnvelope(t, doGet(t, r, "/api/models/marketplace?page=1&pageSize=50&sort=avgLatency:asc"))
	if got := marketplaceItemNames(asc); !reflect.DeepEqual(got, []string{"mq-gpt-beta", "mq-claude-alpha", "mq-gpt-direct"}) {
		t.Fatalf("avgLatency asc = %v", got)
	}
	desc := decodeMarketplaceQueryEnvelope(t, doGet(t, r, "/api/models/marketplace?page=1&pageSize=50&sort=avgLatency:desc"))
	if got := marketplaceItemNames(desc); !reflect.DeepEqual(got, []string{"mq-claude-alpha", "mq-gpt-beta", "mq-gpt-direct"}) {
		t.Fatalf("avgLatency desc = %v", got)
	}
	supply := decodeMarketplaceQueryEnvelope(t, doGet(t, r, "/api/models/marketplace?page=1&pageSize=50&sort=supplyCount:desc"))
	if got := marketplaceItemNames(supply); !reflect.DeepEqual(got, []string{"mq-gpt-beta", "mq-claude-alpha", "mq-gpt-direct"}) {
		t.Fatalf("supplyCount desc = %v (beta has 2 sources, alpha 1, direct 1; tie by name)", got)
	}
	// Unknown sort columns fall back to name.asc and the echo says so.
	fallback := decodeMarketplaceQueryEnvelope(t, doGet(t, r, "/api/models/marketplace?page=1&pageSize=50&sort=brand:desc"))
	if fallback.Meta.Query.Sort != "name.asc" {
		t.Fatalf("invalid sort echo = %q, want name.asc", fallback.Meta.Query.Sort)
	}
	if got := marketplaceItemNames(fallback); !reflect.DeepEqual(got, []string{"mq-claude-alpha", "mq-gpt-beta", "mq-gpt-direct"}) {
		t.Fatalf("fallback order = %v", got)
	}

	// Legacy shape: no page keeps {models, meta}; filters still apply.
	legacy := doGet(t, r, "/api/models/marketplace?sourceKind=oauth")
	if legacy.Code != http.StatusOK {
		t.Fatalf("legacy filtered status = %d", legacy.Code)
	}
	var legacyBody map[string]any
	if err := json.Unmarshal(legacy.Body.Bytes(), &legacyBody); err != nil {
		t.Fatalf("decode legacy: %v", err)
	}
	legacyModels, ok := legacyBody["models"].([]any)
	if !ok || len(legacyModels) != 1 {
		t.Fatalf("legacy filtered models = %#v, want 1", legacyBody["models"])
	}
	if _, hasItems := legacyBody["items"]; hasItems {
		t.Fatal("legacy response must not include items")
	}
}

func TestModelsMarketplace_DetailEndpoint(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedMarketplaceQueryFixture(t, db)

	// The detail resolves a model that is not on the first page of the list.
	resp := doGet(t, r, "/api/models/marketplace/detail?name=mq-gpt-direct")
	if resp.Code != http.StatusOK {
		t.Fatalf("detail status = %d; body=%s", resp.Code, resp.Body.String())
	}
	var body struct {
		Model map[string]any `json:"model"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if body.Model["name"] != "mq-gpt-direct" {
		t.Fatalf("detail name = %#v", body.Model["name"])
	}
	if _, ok := body.Model["supplySources"]; !ok {
		t.Fatal("detail must include supplySources")
	}

	if resp := doGet(t, r, "/api/models/marketplace/detail?name=mq-absent"); resp.Code != http.StatusNotFound {
		t.Fatalf("unknown model status = %d, want 404", resp.Code)
	}
	if resp := doGet(t, r, "/api/models/marketplace/detail"); resp.Code != http.StatusBadRequest {
		t.Fatalf("empty name status = %d, want 400", resp.Code)
	}
}

// TestModelsMarketplace_QueryScale seeds a catalog-sized fleet and verifies
// the query contract stays correct end to end. The aggregation runs a fixed
// set of batch queries (availability, tokens, routes, grants, logs) — there
// is no per-model query to regress into an N+1 — so correctness at scale is
// what this pins; no synthetic latency budget is asserted.
func TestModelsMarketplace_QueryScale(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	now := time.Now().UTC().Format(time.RFC3339)

	siteID, err := execInsertID(db.DB,
		`INSERT INTO sites (name, url, platform, status, created_at, updated_at)
		 VALUES ('mq-scale-site', 'https://mq-scale.example.test', 'openai', 'active', ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert site: %v", err)
	}
	accountID, err := execInsertID(db.DB,
		`INSERT INTO accounts (site_id, username, access_token, status, checkin_enabled, created_at, updated_at)
		 VALUES (?, 'mq-scale', 'token', 'active', false, ?, ?)`, siteID, now, now)
	if err != nil {
		t.Fatalf("insert account: %v", err)
	}
	tx, err := db.Beginx()
	if err != nil {
		t.Fatal(err)
	}
	stmt := tx.Rebind(`INSERT INTO model_availability (account_id, model_name, available, is_manual, latency_ms, checked_at)
		 VALUES (?, ?, true, false, ?, ?)`)
	for i := 0; i < 1200; i++ {
		name := "mq-scale-gpt-" + strconv.Itoa(i)
		if i%7 == 0 {
			name = "mq-scale-claude-" + strconv.Itoa(i)
		}
		if _, err := tx.Exec(stmt, accountID, name, 100+i%50, now); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	all := decodeMarketplaceQueryEnvelope(t, doGet(t, r, "/api/models/marketplace?page=1&pageSize=50"))
	if all.Total != 1200 || len(all.Items) != 50 {
		t.Fatalf("scale page 1 = total %d items %d, want 1200/50", all.Total, len(all.Items))
	}
	last := decodeMarketplaceQueryEnvelope(t, doGet(t, r, "/api/models/marketplace?page=24&pageSize=50"))
	if last.Total != 1200 || len(last.Items) != 50 {
		t.Fatalf("scale page 24 = total %d items %d", last.Total, len(last.Items))
	}

	// A page-24 model is findable from a page-1 search.
	needle := "mq-scale-gpt-1199"
	search := decodeMarketplaceQueryEnvelope(t, doGet(t, r, "/api/models/marketplace?page=1&pageSize=50&q="+needle))
	if search.Total != 1 || !reflect.DeepEqual(marketplaceItemNames(search), []string{needle}) {
		t.Fatalf("scale search = total %d items %v", search.Total, marketplaceItemNames(search))
	}

	// Name-heuristic endpoint facets cover the whole inventory: the claude
	// slice is every 7th row (0..1199 step 7 = 172 models).
	claudes := decodeMarketplaceQueryEnvelope(t, doGet(t, r, "/api/models/marketplace?page=1&pageSize=50&endpointType=anthropic"))
	wantClaudes := 0
	for i := 0; i < 1200; i += 7 {
		wantClaudes++
	}
	if claudes.Total != wantClaudes {
		t.Fatalf("anthropic total = %d, want %d", claudes.Total, wantClaudes)
	}
	facetEndpoint := map[string]int{}
	for _, facet := range claudes.Facets.EndpointTypes {
		facetEndpoint[facet.Value] = facet.Count
	}
	if facetEndpoint["openai"] != 1200-wantClaudes || facetEndpoint["anthropic"] != wantClaudes {
		t.Fatalf("endpoint facet = %v, want openai=%d anthropic=%d", facetEndpoint, 1200-wantClaudes, wantClaudes)
	}
}

func TestModelsMarketplace_QueryContractPostgres(t *testing.T) {
	db, r := setupStatsPostgresTest(t)
	seedMarketplaceQueryFixture(t, db)

	search := decodeMarketplaceQueryEnvelope(t, doGet(t, r, "/api/models/marketplace?page=1&pageSize=50&q=beta&sourceKind=oauth"))
	if search.Total != 1 || !reflect.DeepEqual(marketplaceItemNames(search), []string{"mq-gpt-beta"}) {
		t.Fatalf("pg q+sourceKind = total %d items %v", search.Total, marketplaceItemNames(search))
	}
	sorted := decodeMarketplaceQueryEnvelope(t, doGet(t, r, "/api/models/marketplace?page=1&pageSize=50&sort=avgLatency:asc"))
	if got := marketplaceItemNames(sorted); !reflect.DeepEqual(got, []string{"mq-gpt-beta", "mq-claude-alpha", "mq-gpt-direct"}) {
		t.Fatalf("pg avgLatency asc = %v", got)
	}
	if resp := doGet(t, r, "/api/models/marketplace/detail?name=mq-gpt-direct"); resp.Code != http.StatusOK {
		t.Fatalf("pg detail status = %d", resp.Code)
	}
	if resp := doGet(t, r, "/api/models/marketplace/detail?name=absent"); resp.Code != http.StatusNotFound {
		t.Fatalf("pg absent detail status = %d, want 404", resp.Code)
	}
}

// Guard: the query layer must never widen the projection — detail and list
// rows stay free of credential material, as the supply-sources contract
// requires.
func TestModelsMarketplace_DetailRedactsCredentials(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedMarketplaceQueryFixture(t, db)

	resp := doGet(t, r, "/api/models/marketplace/detail?name=mq-gpt-direct")
	if resp.Code != http.StatusOK {
		t.Fatalf("detail status = %d", resp.Code)
	}
	if strings.Contains(resp.Body.String(), "query-fixture-only") {
		t.Fatal("detail leaked credential material")
	}
}
