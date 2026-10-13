package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/store"
	"github.com/go-chi/chi/v5"
	"github.com/jmoiron/sqlx"
)

// Key-context route-decision tests: the admission chain is evaluated
// read-only for one downstream key, the decision runs under that key's
// routing policy, and nothing is counted, consumed, leased, or written.

type keyContextEnvelope struct {
	Success    bool `json:"success"`
	KeyContext struct {
		KeyID     int64  `json:"keyId"`
		Name      string `json:"name"`
		Admission struct {
			Verdict  string `json:"verdict"`
			Blockers []struct {
				Dimension string `json:"dimension"`
				Code      string `json:"code"`
				Message   string `json:"message"`
			} `json:"blockers"`
			Notes []string `json:"notes"`
			IP    struct {
				Checked bool   `json:"checked"`
				Allowed bool   `json:"allowed"`
				Reason  string `json:"reason"`
			} `json:"ip"`
			Cost *struct {
				Used float64  `json:"used"`
				Max  *float64 `json:"max"`
			} `json:"cost"`
			Rate *struct {
				RPM struct {
					Used float64  `json:"used"`
					Max  *float64 `json:"max"`
				} `json:"rpm"`
				Scope string `json:"scope"`
			} `json:"rate"`
			Quota *struct {
				RequestsUsed int64  `json:"requestsUsed"`
				RequestsMax  *int64 `json:"requestsMax"`
				Blocker      string `json:"blocker"`
			} `json:"quota"`
		} `json:"admission"`
	} `json:"keyContext"`
	Decision struct {
		NotEvaluated []string `json:"notEvaluated"`
	} `json:"decision"`
}

func decodeKeyContext(t *testing.T, resp *httptest.ResponseRecorder) keyContextEnvelope {
	t.Helper()
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", resp.Code, resp.Body.String())
	}
	var out keyContextEnvelope
	if err := json.Unmarshal(resp.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v; body=%s", err, resp.Body.String())
	}
	return out
}

func blockerCodes(env keyContextEnvelope) []string {
	codes := make([]string, 0, len(env.KeyContext.Admission.Blockers))
	for _, blocker := range env.KeyContext.Admission.Blockers {
		codes = append(codes, blocker.Code)
	}
	return codes
}

func TestRouteDecisionKeyContext_Postgres(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("PG_TEST_DSN"))
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL integration test")
	}
	db, err := store.Open(store.DialectPostgres, dsn, false)
	if err != nil {
		t.Fatalf("failed to open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	pgtest.Reset(t, db.DB)
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	fake := &fakeDecisionRouter{}
	r := chi.NewRouter()
	RegisterTokenRoutesWithDeps(r, db.DB, TokenRoutesDeps{Router: fake})

	keyID := seedKeyContextKey(t, db.DB, "kc-pg", map[string]any{
		"max_cost":  1.0,
		"used_cost": 2.5,
	})
	env := decodeKeyContext(t, doGet(t, r, keyContextURL(keyID, "&model=gpt-4o")))
	if !containsString(blockerCodes(env), "over_cost") {
		t.Fatalf("pg over_cost = %v", blockerCodes(env))
	}
	if len(fake.policies) != 1 || !fake.policies[0].DenyAllWhenEmpty {
		t.Fatalf("pg policy passthrough = %+v", fake.policies)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func seedKeyContextKey(t *testing.T, db *sqlx.DB, name string, mutate map[string]any) int64 {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	id, err := execInsertID(db, `INSERT INTO downstream_api_keys
		(name, key, enabled, expires_at, max_cost, used_cost, max_requests, used_requests,
		 max_rpm, max_tpm, supported_models, allowed_route_ids, ip_allowlist, ip_blocklist,
		 access_policy, created_at, updated_at)
		VALUES (?, ?, TRUE, NULL, NULL, 0, NULL, 0, NULL, NULL, NULL, NULL, NULL, NULL, NULL, ?, ?)`,
		name, "sk-"+name, now, now)
	if err != nil {
		t.Fatalf("insert key %s: %v", name, err)
	}
	for column, value := range mutate {
		if _, err := db.Exec(db.Rebind("UPDATE downstream_api_keys SET "+column+" = ? WHERE id = ?"), value, id); err != nil {
			t.Fatalf("mutate %s: %v", column, err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(db.Rebind("DELETE FROM downstream_quota_usage WHERE key_id = ?"), id)
		_, _ = db.Exec(db.Rebind("DELETE FROM downstream_api_keys WHERE id = ?"), id)
	})
	return id
}

func keyContextURL(keyID int64, extra string) string {
	return "/api/routes/decision/key-context?keyId=" + strconv.FormatInt(keyID, 10) + extra
}

func TestRouteDecisionKeyContext_Validation(t *testing.T) {
	_, r := setupTokenRoutesDecisionTest(t, TokenRoutesDeps{Router: &fakeDecisionRouter{}})
	if resp := doGet(t, r, "/api/routes/decision/key-context?model=gpt-4o"); resp.Code != http.StatusBadRequest {
		t.Fatalf("missing keyId status = %d, want 400", resp.Code)
	}
	if resp := doGet(t, r, keyContextURL(999999, "&model=gpt-4o")); resp.Code != http.StatusNotFound {
		t.Fatalf("unknown keyId status = %d, want 404; body=%s", resp.Code, resp.Body.String())
	}
	if resp := doGet(t, r, "/api/routes/decision/key-context?keyId=1"); resp.Code != http.StatusBadRequest {
		t.Fatalf("missing model status = %d, want 400", resp.Code)
	}
}

func TestRouteDecisionKeyContext_AllowAndPolicyPassthrough(t *testing.T) {
	fake := &fakeDecisionRouter{decision: routing.RouteDecisionExplanation{
		NotEvaluated: []string{"downstreamAdmission", "strategySelection"},
	}}
	db, r := setupTokenRoutesDecisionTest(t, TokenRoutesDeps{Router: fake})
	keyID := seedKeyContextKey(t, db.DB, "kc-allow", map[string]any{
		"supported_models":  `["gpt-4o"]`,
		"allowed_route_ids": "[7]",
	})

	env := decodeKeyContext(t, doGet(t, r, keyContextURL(keyID, "&model=gpt-4o&endpoint="+url.QueryEscape("/v1/responses"))))
	if !env.Success || env.KeyContext.KeyID != keyID || env.KeyContext.Name != "kc-allow" {
		t.Fatalf("keyContext identity = %+v", env.KeyContext)
	}
	if env.KeyContext.Admission.Verdict != "allow" || len(env.KeyContext.Admission.Blockers) != 0 {
		t.Fatalf("admission = %v %v", env.KeyContext.Admission.Verdict, blockerCodes(env))
	}
	// The decision ran under the key's routing policy, plus the endpoint's
	// protocol context.
	if len(fake.policies) != 1 {
		t.Fatalf("explain calls = %d", len(fake.policies))
	}
	policy := fake.policies[0]
	if !policy.DenyAllWhenEmpty || len(policy.SupportedModels) != 1 || policy.SupportedModels[0] != "gpt-4o" ||
		len(policy.AllowedRouteIDs) != 1 || policy.AllowedRouteIDs[0] != 7 {
		t.Fatalf("policy passthrough = %+v", policy)
	}
	if policy.RequiredUpstreamProtocol != store.DirectProtocolResponses {
		t.Fatalf("protocol context = %v", policy.RequiredUpstreamProtocol)
	}
	// downstreamAdmission was answered, so it leaves the not-evaluated list.
	for _, dim := range env.Decision.NotEvaluated {
		if dim == "downstreamAdmission" {
			t.Fatalf("downstreamAdmission still unevaluated: %v", env.Decision.NotEvaluated)
		}
	}
	if env.KeyContext.Admission.IP.Checked {
		t.Fatal("IP dimension must report not-checked without clientIp")
	}
}

func TestRouteDecisionKeyContext_Blockers(t *testing.T) {
	fake := &fakeDecisionRouter{}
	db, r := setupTokenRoutesDecisionTest(t, TokenRoutesDeps{Router: fake})

	disabledID := seedKeyContextKey(t, db.DB, "kc-disabled", map[string]any{"enabled": false})
	expiredID := seedKeyContextKey(t, db.DB, "kc-expired", map[string]any{"expires_at": "2020-01-01T00:00:00Z"})
	costID := seedKeyContextKey(t, db.DB, "kc-cost", map[string]any{"max_cost": 1.0, "used_cost": 2.5})
	ipID := seedKeyContextKey(t, db.DB, "kc-ip", map[string]any{"ip_blocklist": "10.9.8.7"})

	env := decodeKeyContext(t, doGet(t, r, keyContextURL(disabledID, "&model=gpt-4o")))
	if env.KeyContext.Admission.Verdict != "deny" || !containsString(blockerCodes(env), "disabled") {
		t.Fatalf("disabled = %v %v", env.KeyContext.Admission.Verdict, blockerCodes(env))
	}

	env = decodeKeyContext(t, doGet(t, r, keyContextURL(expiredID, "&model=gpt-4o")))
	if !containsString(blockerCodes(env), "expired") {
		t.Fatalf("expired = %v", blockerCodes(env))
	}

	env = decodeKeyContext(t, doGet(t, r, keyContextURL(costID, "&model=gpt-4o")))
	if !containsString(blockerCodes(env), "over_cost") {
		t.Fatalf("over_cost = %v", blockerCodes(env))
	}
	if env.KeyContext.Admission.Cost == nil || env.KeyContext.Admission.Cost.Used != 2.5 ||
		env.KeyContext.Admission.Cost.Max == nil || *env.KeyContext.Admission.Cost.Max != 1.0 {
		t.Fatalf("cost window = %+v", env.KeyContext.Admission.Cost)
	}

	// IP dimension: blocked for the listed client, not checked without one.
	env = decodeKeyContext(t, doGet(t, r, keyContextURL(ipID, "&model=gpt-4o&clientIp=10.9.8.7")))
	if !env.KeyContext.Admission.IP.Checked || env.KeyContext.Admission.IP.Allowed ||
		!containsString(blockerCodes(env), "ip_blocked") {
		t.Fatalf("ip blocked = %+v %v", env.KeyContext.Admission.IP, blockerCodes(env))
	}
	env = decodeKeyContext(t, doGet(t, r, keyContextURL(ipID, "&model=gpt-4o")))
	if env.KeyContext.Admission.IP.Checked {
		t.Fatal("IP must stay not-checked without clientIp")
	}
	if !containsString(env.KeyContext.Admission.Notes, "ipRulesExistButUnchecked") {
		t.Fatalf("notes = %v, want ipRulesExistButUnchecked", env.KeyContext.Admission.Notes)
	}
}

func TestRouteDecisionKeyContext_QuotaWindowAndReadOnly(t *testing.T) {
	fake := &fakeDecisionRouter{}
	db, r := setupTokenRoutesDecisionTest(t, TokenRoutesDeps{Router: fake})

	quota, err := json.Marshal(map[string]any{
		"quota": map[string]any{
			"requests": 2,
			"period":   map[string]any{"type": "all_time"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	keyID := seedKeyContextKey(t, db.DB, "kc-quota", map[string]any{"access_policy": string(quota)})
	nowMs := time.Now().UnixMilli()
	for i := 0; i < 2; i++ {
		if _, err := db.Exec(db.Rebind(`INSERT INTO downstream_quota_usage (key_id, event_key, occurred_at, requests) VALUES (?, ?, ?, 1)`), keyID, "fixture-"+strconv.Itoa(i), nowMs); err != nil {
			t.Fatalf("seed quota usage: %v", err)
		}
	}

	env := decodeKeyContext(t, doGet(t, r, keyContextURL(keyID, "&model=gpt-4o")))
	if !containsString(blockerCodes(env), "over_quota") {
		t.Fatalf("over_quota = %v", blockerCodes(env))
	}
	if env.KeyContext.Admission.Quota == nil || env.KeyContext.Admission.Quota.RequestsUsed != 2 ||
		env.KeyContext.Admission.Quota.RequestsMax == nil || *env.KeyContext.Admission.Quota.RequestsMax != 2 {
		t.Fatalf("quota window = %+v", env.KeyContext.Admission.Quota)
	}

	// Read-only contract: repeated evaluations change nothing.
	for i := 0; i < 2; i++ {
		decodeKeyContext(t, doGet(t, r, keyContextURL(keyID, "&model=gpt-4o")))
	}
	var usageRows int
	if err := db.Get(&usageRows, db.Rebind("SELECT COUNT(*) FROM downstream_quota_usage WHERE key_id = ?"), keyID); err != nil {
		t.Fatal(err)
	}
	if usageRows != 2 {
		t.Fatalf("quota usage rows = %d, want 2 (no consumption)", usageRows)
	}
	var usedRequests int64
	if err := db.Get(&usedRequests, db.Rebind("SELECT COALESCE(used_requests,0) FROM downstream_api_keys WHERE id = ?"), keyID); err != nil {
		t.Fatal(err)
	}
	if usedRequests != 0 {
		t.Fatalf("used_requests = %d, want 0 (no counting)", usedRequests)
	}
	if !containsString(env.KeyContext.Admission.Notes, "evaluationIsReadOnly") {
		t.Fatalf("notes = %v", env.KeyContext.Admission.Notes)
	}
}

func TestRouteDecisionKeyContext_RateWindowSnapshot(t *testing.T) {
	fake := &fakeDecisionRouter{}
	db, r := setupTokenRoutesDecisionTest(t, TokenRoutesDeps{Router: fake})
	keyID := seedKeyContextKey(t, db.DB, "kc-rate", map[string]any{"max_rpm": 60})

	env := decodeKeyContext(t, doGet(t, r, keyContextURL(keyID, "&model=gpt-4o")))
	rate := env.KeyContext.Admission.Rate
	if rate == nil || rate.Scope != "processLocal" || rate.RPM.Max == nil || *rate.RPM.Max != 60 {
		t.Fatalf("rate window = %+v", rate)
	}
	if rate.RPM.Used != 0 {
		t.Fatalf("rpm used = %v, want 0 (nothing recorded in this test)", rate.RPM.Used)
	}
	if !containsString(env.KeyContext.Admission.Notes, "rateWindowProcessLocal") {
		t.Fatalf("notes = %v", env.KeyContext.Admission.Notes)
	}
}
