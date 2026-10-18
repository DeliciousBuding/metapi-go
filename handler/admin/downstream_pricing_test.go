package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
)

type downstreamPricingFixture struct {
	siteOneID, siteTwoID       int64
	accountOneID, accountTwoID int64
}

func setupDownstreamPricingTest(t *testing.T) (*store.DB, chi.Router, downstreamPricingFixture) {
	t.Helper()
	db := sqlitetest.Open(t)
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		RegisterDownstreamPricingRoutes(r, db.DB)
	})
	return db, r, seedDownstreamPricing(t, db)
}

func seedDownstreamPricing(t *testing.T, db *store.DB) downstreamPricingFixture {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	fixture := downstreamPricingFixture{}

	if _, err := db.Exec(`INSERT INTO sites (name,url,platform,status,created_at,updated_at)
		VALUES ('site-one','https://one.example.test','openai','active',?,?)`, now, now); err != nil {
		t.Fatalf("insert site one: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sites (name,url,platform,status,created_at,updated_at)
		VALUES ('site-two','https://two.example.test','anthropic','active',?,?)`, now, now); err != nil {
		t.Fatalf("insert site two: %v", err)
	}
	if err := db.Get(&fixture.siteOneID, `SELECT id FROM sites WHERE name='site-one'`); err != nil {
		t.Fatalf("site one id: %v", err)
	}
	if err := db.Get(&fixture.siteTwoID, `SELECT id FROM sites WHERE name='site-two'`); err != nil {
		t.Fatalf("site two id: %v", err)
	}

	if _, err := db.Exec(`INSERT INTO accounts (site_id,username,access_token,status,balance,unit_cost,checkin_enabled,created_at,updated_at)
		VALUES (?, 'hidden-user-one','sk-synthetic-one','active',10.0,0.66,FALSE,?,?)`, fixture.siteOneID, now, now); err != nil {
		t.Fatalf("insert account one: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO accounts (site_id,username,access_token,status,balance,checkin_enabled,created_at,updated_at)
		VALUES (?, 'hidden-user-two','sk-synthetic-two','active',10.0,FALSE,?,?)`, fixture.siteTwoID, now, now); err != nil {
		t.Fatalf("insert account two: %v", err)
	}
	if err := db.Get(&fixture.accountOneID, `SELECT id FROM accounts WHERE username='hidden-user-one'`); err != nil {
		t.Fatalf("account one id: %v", err)
	}
	if err := db.Get(&fixture.accountTwoID, `SELECT id FROM accounts WHERE username='hidden-user-two'`); err != nil {
		t.Fatalf("account two id: %v", err)
	}

	for _, row := range []struct {
		accountID int64
		model     string
	}{
		{fixture.accountOneID, "model-a"},
		{fixture.accountTwoID, "model-b"},
	} {
		if _, err := db.Exec(`INSERT INTO model_availability (account_id,model_name,available,is_manual,checked_at)
			VALUES (?,?,TRUE,FALSE,?)`, row.accountID, row.model, now); err != nil {
			t.Fatalf("insert availability %s: %v", row.model, err)
		}
	}

	billing := `{"breakdown":{"inputPerMillion":1.5,"outputPerMillion":3.0}}`
	if _, err := db.Exec(`INSERT INTO proxy_logs (account_id,model_requested,model_actual,status,total_tokens,estimated_cost,billing_details,created_at)
		VALUES (?,?,?,'success',2000,0.2,?,?)`, fixture.accountOneID, "model-a", "model-a", billing, now); err != nil {
		t.Fatalf("insert model-a log: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO proxy_logs (account_id,model_requested,model_actual,status,total_tokens,estimated_cost,billing_details,created_at)
		VALUES (?,?,?,'success',2000,0.3,?,?)`, fixture.accountTwoID, "model-b", "model-b", billing, now); err != nil {
		t.Fatalf("insert model-b log: %v", err)
	}

	for _, row := range []struct {
		accountID int64
		model     string
	}{
		{fixture.accountOneID, "model-a"},
		{fixture.accountTwoID, "model-b"},
	} {
		payload := `{"models":[{"model":"` + row.model + `","quotaType":0,"modelRatio":1,"completionRatio":2,"groups":["default"],"billingMode":"","billingExpression":""}],"usableGroups":{"default":"Standard"},"groupRatios":{"default":1}}`
		if err := store.SaveAccountPricing(context.Background(), db.DB, store.AccountPricingSnapshot{
			AccountID: row.accountID,
			Payload:   payload,
			FetchedAt: "2026-10-18T02:00:00Z",
		}); err != nil {
			t.Fatalf("save pricing for %s: %v", row.model, err)
		}
	}
	return fixture
}

func doDownstreamPricingGet(t *testing.T, r chi.Router, path string, policy auth.DownstreamRoutingPolicy) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req = auth.ProxyAuthFromRequest(req, &auth.ProxyAuthContext{Source: "managed", Policy: policy})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodeDownstreamPricing(t *testing.T, rec *httptest.ResponseRecorder) (map[string]any, string) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	raw := rec.Body.String()
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return body, raw
}

func downstreamItemNames(body map[string]any) []string {
	items, _ := body["items"].([]any)
	names := make([]string, 0, len(items))
	for _, raw := range items {
		item, _ := raw.(map[string]any)
		if name, ok := item["siteName"].(string); ok {
			names = append(names, name)
		}
	}
	return names
}

func assertNoInternalFields(t *testing.T, raw string) {
	t.Helper()
	for _, field := range []string{`"username"`, `"accountId"`, `"configuredUnitCost"`} {
		if strings.Contains(raw, field) {
			t.Fatalf("downstream response leaks %s: %s", field, raw)
		}
	}
}

func TestDownstreamPricing_SiteAllowListHidesOtherSites(t *testing.T) {
	_, r, fixture := setupDownstreamPricingTest(t)
	policy := auth.DownstreamRoutingPolicy{
		SupportedModels:  []string{"model-a", "model-b"},
		AllowedSiteIDs:   []int64{fixture.siteTwoID},
		DenyAllWhenEmpty: true,
	}

	// Allowed model, but only on an unauthorized site: empty, no internal error.
	body, raw := decodeDownstreamPricing(t, doDownstreamPricingGet(t, r, "/v1/pricing?model=model-a", policy))
	if items, _ := body["items"].([]any); len(items) != 0 {
		t.Fatalf("model-a on site-one leaked: %s", raw)
	}
	if accounts, _ := body["pricingAccounts"].([]any); len(accounts) != 0 {
		for _, rawAccount := range accounts {
			account, _ := rawAccount.(map[string]any)
			if account["siteName"] == "site-one" || account["siteId"].(float64) == float64(fixture.siteOneID) {
				t.Fatalf("pricingAccounts leaked hidden site: %s", raw)
			}
		}
	}
	if current, _ := body["currentPrices"].([]any); len(current) != 0 {
		for _, rawPrice := range current {
			price, _ := rawPrice.(map[string]any)
			if price["siteName"] == "site-one" {
				t.Fatalf("currentPrices leaked hidden site: %s", raw)
			}
		}
	}
	assertNoInternalFields(t, raw)

	// Authorized site still returns the product fields.
	body, raw = decodeDownstreamPricing(t, doDownstreamPricingGet(t, r, "/v1/pricing?model=model-b", policy))
	if names := downstreamItemNames(body); len(names) == 0 || names[0] != "site-two" {
		t.Fatalf("site-two rows missing: %s", raw)
	}
	assertNoInternalFields(t, raw)
}

func TestDownstreamPricing_EmptyModelUsesPolicyScopedTopModels(t *testing.T) {
	_, r, fixture := setupDownstreamPricingTest(t)
	policy := auth.DownstreamRoutingPolicy{
		SupportedModels:  []string{"model-a", "model-b"},
		AllowedSiteIDs:   []int64{fixture.siteTwoID},
		DenyAllWhenEmpty: true,
	}

	body, raw := decodeDownstreamPricing(t, doDownstreamPricingGet(t, r, "/v1/pricing", policy))
	for _, name := range downstreamItemNames(body) {
		if name != "site-two" {
			t.Fatalf("top-model rows leaked %q: %s", name, raw)
		}
	}
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected only model-b from site-two, got %s", raw)
	}
	if model := items[0].(map[string]any)["model"]; model != "model-b" {
		t.Fatalf("model=%v want model-b", model)
	}
	assertNoInternalFields(t, raw)
}

func TestDownstreamPricing_UnauthorizedModelReturnsEmpty(t *testing.T) {
	db, r, fixture := setupDownstreamPricingTest(t)
	policy := auth.DownstreamRoutingPolicy{
		SupportedModels:  []string{"model-b"},
		AllowedSiteIDs:   []int64{fixture.siteTwoID},
		DenyAllWhenEmpty: true,
	}

	body, raw := decodeDownstreamPricing(t, doDownstreamPricingGet(t, r, "/v1/pricing?model=model-a", policy))
	if items, _ := body["items"].([]any); len(items) != 0 {
		t.Fatalf("unauthorized model returned items: %s", raw)
	}
	if official, _ := body["officialPrices"].(map[string]any); len(official) != 0 {
		t.Fatalf("unauthorized model returned officialPrices: %s", raw)
	}

	body, raw = decodeDownstreamPricing(t, doDownstreamPricingGet(t, r, "/v1/pricing?model=model-b", policy))
	items, _ := body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["model"] != "model-b" {
		t.Fatalf("authorized exact model missing: %s", raw)
	}

	// Matching against stored model names stays case-insensitive; the row keeps
	// the stored casing.
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO model_availability (account_id,model_name,available,is_manual,checked_at)
		VALUES (?,?,TRUE,FALSE,?)`, fixture.accountTwoID, "Case-Model", now); err != nil {
		t.Fatalf("insert case-model availability: %v", err)
	}
	casePolicy := auth.DownstreamRoutingPolicy{
		SupportedModels:  []string{"case-model"},
		AllowedSiteIDs:   []int64{fixture.siteTwoID},
		DenyAllWhenEmpty: true,
	}
	body, raw = decodeDownstreamPricing(t, doDownstreamPricingGet(t, r, "/v1/pricing?model=case-model", casePolicy))
	items, _ = body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["model"] != "Case-Model" {
		t.Fatalf("case-insensitive exact lookup failed: %s", raw)
	}
}

func TestDownstreamPricing_ExcludedSitesAndCredentialRefs(t *testing.T) {
	_, r, fixture := setupDownstreamPricingTest(t)
	base := auth.DownstreamRoutingPolicy{
		SupportedModels:  []string{"model-a", "model-b"},
		DenyAllWhenEmpty: true,
	}

	excludedSite := base
	excludedSite.ExcludedSiteIDs = []int64{fixture.siteOneID}
	body, raw := decodeDownstreamPricing(t, doDownstreamPricingGet(t, r, "/v1/pricing?model=model-a", excludedSite))
	if items, _ := body["items"].([]any); len(items) != 0 {
		t.Fatalf("excluded site leaked: %s", raw)
	}
	if strings.Contains(raw, "site-one") {
		t.Fatalf("excluded site name leaked: %s", raw)
	}

	allowedCredential := base
	allowedCredential.AllowedCredentialRefs = []auth.ExcludedCredentialRef{{
		Kind:      auth.CredentialRefDefaultApiKey,
		SiteID:    fixture.siteTwoID,
		AccountID: fixture.accountTwoID,
	}}
	body, raw = decodeDownstreamPricing(t, doDownstreamPricingGet(t, r, "/v1/pricing?model=model-a", allowedCredential))
	if items, _ := body["items"].([]any); len(items) != 0 {
		t.Fatalf("credential allow-list leaked another account: %s", raw)
	}
	if strings.Contains(raw, "site-one") {
		t.Fatalf("credential allow-list leaked an unlisted account's site: %s", raw)
	}
	body, raw = decodeDownstreamPricing(t, doDownstreamPricingGet(t, r, "/v1/pricing?model=model-b", allowedCredential))
	if items, _ := body["items"].([]any); len(items) != 1 {
		t.Fatalf("credential allow-list hid the authorized account: %s", raw)
	}

	// A direct-grant allow-list cannot be mapped to account pricing rows.
	directGrantOnly := base
	directGrantOnly.AllowedCredentialRefs = []auth.ExcludedCredentialRef{{Kind: auth.CredentialRefDirectGrant, GrantID: 9}}
	body, raw = decodeDownstreamPricing(t, doDownstreamPricingGet(t, r, "/v1/pricing", directGrantOnly))
	if items, _ := body["items"].([]any); len(items) != 0 {
		t.Fatalf("unmappable direct grant returned account pricing: %s", raw)
	}

	excludedCredential := auth.DownstreamRoutingPolicy{
		SupportedModels:  []string{"model-b"},
		DenyAllWhenEmpty: true,
		ExcludedCredentialRefs: []auth.ExcludedCredentialRef{{
			Kind:      auth.CredentialRefDefaultApiKey,
			SiteID:    fixture.siteTwoID,
			AccountID: fixture.accountTwoID,
		}},
	}
	body, raw = decodeDownstreamPricing(t, doDownstreamPricingGet(t, r, "/v1/pricing?model=model-b", excludedCredential))
	if items, _ := body["items"].([]any); len(items) != 0 {
		t.Fatalf("excluded credential leaked pricing: %s", raw)
	}
}

func TestDownstreamPricing_ClampsQueryWindowAndRowCounts(t *testing.T) {
	db, r, fixture := setupDownstreamPricingTest(t)
	now := time.Now().UTC().Format(time.RFC3339)
	for i := 0; i < 20; i++ {
		model := "bulk-model-" + strings.Repeat("x", i%3) + string(rune('a'+i))
		if _, err := db.Exec(`INSERT INTO model_availability (account_id,model_name,available,is_manual,checked_at)
			VALUES (?,?,TRUE,FALSE,?)`, fixture.accountTwoID, model, now); err != nil {
			t.Fatalf("insert bulk availability: %v", err)
		}
	}

	policy := auth.DownstreamRoutingPolicy{
		SupportedModels:  []string{"*"},
		AllowedSiteIDs:   []int64{fixture.siteTwoID},
		DenyAllWhenEmpty: true,
	}
	// With no traffic the top-model list comes from availability; the hard cap
	// must still bound how many models one request explores.
	if _, err := db.Exec(`DELETE FROM proxy_logs`); err != nil {
		t.Fatalf("clear proxy logs: %v", err)
	}
	body, raw := decodeDownstreamPricing(t, doDownstreamPricingGet(t, r, "/v1/pricing?days=9999&limit=9999&topModels=9999&pricePageSize=999", policy))
	if got := body["days"].(float64); got != downstreamPricingMaxDays {
		t.Fatalf("days=%v want %d", got, downstreamPricingMaxDays)
	}
	if got := body["limit"].(float64); got != downstreamPricingMaxLimit {
		t.Fatalf("limit=%v want %d", got, downstreamPricingMaxLimit)
	}
	if got := body["meta"].(map[string]any)["modelsConsidered"].(float64); got != downstreamPricingMaxTopModels {
		t.Fatalf("modelsConsidered=%v want %d: %s", got, downstreamPricingMaxTopModels, raw)
	}
	if got := body["currentPage"].(map[string]any)["pageSize"].(float64); got != downstreamPricingMaxPricePage {
		t.Fatalf("pageSize=%v want %d: %s", got, downstreamPricingMaxPricePage, raw)
	}
}

func TestDownstreamPricing_CollapsesAccountsPerSite(t *testing.T) {
	db, r, fixture := setupDownstreamPricingTest(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO accounts (site_id,username,access_token,status,balance,checkin_enabled,created_at,updated_at)
		VALUES (?, 'hidden-user-two-b','sk-synthetic-two-b','active',10.0,FALSE,?,?)`, fixture.siteTwoID, now, now); err != nil {
		t.Fatalf("insert second account on site-two: %v", err)
	}
	var secondAccountID int64
	if err := db.Get(&secondAccountID, `SELECT id FROM accounts WHERE username='hidden-user-two-b'`); err != nil {
		t.Fatalf("second account id: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO model_availability (account_id,model_name,available,is_manual,checked_at)
		VALUES (?,?,TRUE,FALSE,?)`, secondAccountID, "model-b", now); err != nil {
		t.Fatalf("insert second availability: %v", err)
	}
	// More expensive observed price with no billing breakdown.
	if _, err := db.Exec(`INSERT INTO proxy_logs (account_id,model_requested,model_actual,status,total_tokens,estimated_cost,created_at)
		VALUES (?,?,?,'success',2000,0.5,?)`, secondAccountID, "model-b", "model-b", now); err != nil {
		t.Fatalf("insert second log: %v", err)
	}

	policy := auth.DownstreamRoutingPolicy{
		SupportedModels:  []string{"model-b"},
		AllowedSiteIDs:   []int64{fixture.siteTwoID},
		DenyAllWhenEmpty: true,
	}
	body, raw := decodeDownstreamPricing(t, doDownstreamPricingGet(t, r, "/v1/pricing?model=model-b", policy))
	items, _ := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("accounts on one site must collapse to one row: %s", raw)
	}
	item := items[0].(map[string]any)
	if item["siteId"].(float64) != float64(fixture.siteTwoID) || item["source"] != "billing_details" {
		t.Fatalf("expected the cheaper billing-derived row, got %v: %s", item, raw)
	}
	accounts, _ := body["pricingAccounts"].([]any)
	if len(accounts) != 1 || accounts[0].(map[string]any)["siteId"].(float64) != float64(fixture.siteTwoID) {
		t.Fatalf("pricingAccounts must be site-level: %s", raw)
	}
}

func TestDownstreamPricing_RequiresProxyAuth(t *testing.T) {
	_, r, _ := setupDownstreamPricingTest(t)
	req := httptest.NewRequest(http.MethodGet, "/v1/pricing", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401", rec.Code)
	}
}

// The admin surface keeps account identities and configured unit cost; only the
// downstream handler strips them. This pins the boundary so a future refactor
// cannot quietly move the stripping into the shared admin handler.
func TestAdminPricing_KeepsInternalFields(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	seedDownstreamPricing(t, db)

	rec := doGet(t, r, "/api/models/price-compare?model=model-a")
	body, raw := decodeDownstreamPricing(t, rec)
	items, _ := body["items"].([]any)
	if len(items) == 0 {
		t.Fatalf("admin items missing: %s", raw)
	}
	item := items[0].(map[string]any)
	for _, field := range []string{"accountId", "username", "configuredUnitCost"} {
		if _, ok := item[field]; !ok {
			t.Fatalf("admin item lost %s: %s", field, raw)
		}
	}
}
