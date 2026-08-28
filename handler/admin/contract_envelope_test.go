package admin

// Wave 4 contract audit — envelope consistency.
//
// The admin API has two response families and both are pinned here:
//
//  1. Success responses: 2xx + application/json + camelCase keys. Three
//     list shapes coexist by design (the status quo the frontend consumes):
//     bare array (/api/sites, /api/routes, /api/account-tokens),
//     {items,total,page,pageSize} (/api/channels),
//     and {success,...} envelopes (/api/downstream-keys).
//  2. Error responses: non-2xx with the unified camelCase {"error":"..."}
//     body (shared.WriteError). A few TS-era paths still answer non-2xx with
//     a legacy {"message":"..."} body; the frontend http-client reads both
//     (resolveResponseMessage), so that shape is pinned as-is rather than
//     changed.
//
// snake_case keys must never leak into success payloads. The one documented
// exception is the additive "request_id" correlation field on unified error
// bodies (handler/shared/errors.go).

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// snakeCaseKeyAllowlist lists JSON keys that are intentionally snake_case.
// request_id is the documented correlation field on unified error bodies.
var snakeCaseKeyAllowlist = map[string]bool{
	"request_id": true,
}

// assertNoSnakeCaseKeys walks decoded JSON and fails on any object key that
// still carries an underscore (raw DB column leak), except the allowlist.
func assertNoSnakeCaseKeys(t *testing.T, value any, path string) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if strings.Contains(key, "_") && !snakeCaseKeyAllowlist[key] {
				t.Errorf("snake_case key %q leaked into response (at %q)", key, childPath)
			}
			assertNoSnakeCaseKeys(t, child, childPath)
		}
	case []any:
		for index, child := range typed {
			assertNoSnakeCaseKeys(t, child, path+"["+itoa(int64(index))+"]")
		}
	}
}

// assertSuccessEnvelope verifies a success response: 2xx status, JSON
// content type, decodable body, and no snake_case key leaks. Returns the
// decoded value for shape assertions.
func assertSuccessEnvelope(t *testing.T, resp *httptest.ResponseRecorder) any {
	t.Helper()
	if resp.Code < 200 || resp.Code > 299 {
		t.Fatalf("status = %d, want 2xx; body=%s", resp.Code, resp.Body.String())
	}
	if contentType := resp.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("Content-Type = %q, want application/json", contentType)
	}
	var decoded any
	if err := json.Unmarshal(resp.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("success body is not valid JSON: %v; body=%s", err, resp.Body.String())
	}
	assertNoSnakeCaseKeys(t, decoded, "")
	return decoded
}

// assertUnifiedErrorEnvelope verifies the unified admin error contract:
// non-2xx status + {"error": non-empty string}, never success=true.
func assertUnifiedErrorEnvelope(t *testing.T, resp *httptest.ResponseRecorder) {
	t.Helper()
	if resp.Code < 400 {
		t.Fatalf("status = %d, want non-2xx for failure; body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("error body is not valid JSON: %v; raw=%s", err, resp.Body.String())
	}
	message, ok := body["error"].(string)
	if !ok || strings.TrimSpace(message) == "" {
		t.Fatalf("expected non-empty \"error\" string in unified error body, got %#v", body)
	}
	if success, has := body["success"]; has && success == true {
		t.Fatalf("error body must not claim success=true: %#v", body)
	}
}

// ---- Success envelopes: bare-array list endpoints ----

func TestEnvelope_SitesList_BareArrayCamelCase(t *testing.T) {
	_, r := setupSitesTest(t)
	newSiteFixture(t, r, "EnvelopeSite", "https://api.openai.com")

	decoded := assertSuccessEnvelope(t, doGet(t, r, "/api/sites"))
	sites, ok := decoded.([]any)
	if !ok {
		t.Fatalf("GET /api/sites = %T, want bare JSON array", decoded)
	}
	if len(sites) != 1 {
		t.Fatalf("sites = %d, want 1", len(sites))
	}
}

func TestEnvelope_RoutesList_BareArrayCamelCase(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	seedRouteChannelRefs(t, db)

	decoded := assertSuccessEnvelope(t, doGet(t, r, "/api/routes"))
	routes, ok := decoded.([]any)
	if !ok {
		t.Fatalf("GET /api/routes (no page) = %T, want bare JSON array", decoded)
	}
	if len(routes) != 1 {
		t.Fatalf("routes = %d, want 1", len(routes))
	}
}

func TestEnvelope_AccountTokensList_BareArrayCamelCase(t *testing.T) {
	db, r := setupTokensTest(t)
	_, accountID := tokenFixture(t, db, r)
	createTokenFixture(t, db, accountID, "envelope-token", "sk-env-123456", "", true, true)

	decoded := assertSuccessEnvelope(t, doGet(t, r, "/api/account-tokens"))
	tokens, ok := decoded.([]any)
	if !ok {
		t.Fatalf("GET /api/account-tokens = %T, want bare JSON array", decoded)
	}
	if len(tokens) != 1 {
		t.Fatalf("tokens = %d, want 1", len(tokens))
	}
}

// ---- Success envelopes: paged-envelope list endpoints ----

func TestEnvelope_ChannelsList_PagedEnvelope(t *testing.T) {
	db, r := setupTokenRoutesTest(t)
	routeID, accountID, tokenID := seedRouteChannelRefs(t, db)
	if _, err := db.Exec(
		`INSERT INTO route_channels
			(route_id, account_id, token_id, source_model, priority, weight, enabled, manual_override)
		 VALUES (?, ?, ?, 'gpt-4o', 0, 10, TRUE, FALSE)`,
		routeID, accountID, tokenID,
	); err != nil {
		t.Fatalf("insert channel: %v", err)
	}

	decoded := assertSuccessEnvelope(t, doGet(t, r, "/api/channels"))
	envelope, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("GET /api/channels = %T, want object envelope", decoded)
	}
	for _, key := range []string{"items", "total", "page", "pageSize"} {
		if _, present := envelope[key]; !present {
			t.Fatalf("channels envelope missing key %q; got %#v", key, envelope)
		}
	}
}

func TestEnvelope_AccountsSnapshot_GeneratedAtAccountsSites(t *testing.T) {
	_, r := setupAccountsPaginationTest(t)

	decoded := assertSuccessEnvelope(t, doGet(t, r, "/api/accounts?refresh=true"))
	snapshot, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("GET /api/accounts = %T, want snapshot object", decoded)
	}
	for _, key := range []string{"generatedAt", "accounts", "sites"} {
		if _, present := snapshot[key]; !present {
			t.Fatalf("accounts snapshot missing key %q; got keys %#v", key, mapKeys(snapshot))
		}
	}
}

// ---- Success envelopes: {success,...} family ----

func TestEnvelope_DownstreamKeysList_SuccessEnvelope(t *testing.T) {
	db, r := setupDownstreamKeysTest(t)
	seedDownstreamKeyFixture(t, db, "envelope-key", true)

	decoded := assertSuccessEnvelope(t, doGet(t, r, "/api/downstream-keys"))
	envelope, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("GET /api/downstream-keys = %T, want object envelope", decoded)
	}
	if envelope["success"] != true {
		t.Fatalf("downstream-keys envelope success = %v, want true", envelope["success"])
	}
	items, ok := envelope["items"].([]any)
	if !ok {
		t.Fatalf("downstream-keys envelope items = %#v, want array", envelope["items"])
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
}

// ---- Error envelopes: unified {"error"} contract ----

func TestEnvelope_SitesDeleteInvalidID_UnifiedError(t *testing.T) {
	_, r := setupSitesTest(t)
	assertUnifiedErrorEnvelope(t, doDelete(t, r, "/api/sites/not-a-number"))
}

func TestEnvelope_RouteChannelsInvalidID_UnifiedError(t *testing.T) {
	_, r := setupTokenRoutesTest(t)
	assertUnifiedErrorEnvelope(t, doGet(t, r, "/api/routes/abc/channels"))
}

func TestEnvelope_AccountTokensCreateInvalidAccount_UnifiedError(t *testing.T) {
	_, r := setupTokensTest(t)
	assertUnifiedErrorEnvelope(t, doPostJSON(t, r, "/api/account-tokens", map[string]any{
		"accountId": 0,
		"token":     "sk-anything",
	}))
}

func TestEnvelope_AccountTokensUpdateInvalidID_UnifiedError(t *testing.T) {
	_, r := setupTokensTest(t)
	assertUnifiedErrorEnvelope(t, doPutJSON(t, r, "/api/account-tokens/0", map[string]any{
		"name": "x",
	}))
}

func TestEnvelope_RoutesCreateMissingPattern_UnifiedError(t *testing.T) {
	_, r := setupTokenRoutesTest(t)
	assertUnifiedErrorEnvelope(t, doPostJSON(t, r, "/api/routes", map[string]any{}))
}

// ---- Error envelopes: legacy TS-era {"message"} shape, pinned as-is ----

// TestEnvelope_AccountsBatchInvalid_PinnedLegacyMessageEnvelope documents
// that POST /api/accounts/batch still answers validation failures with the
// TS-era {"message":"..."} body instead of the unified {"error"}. The
// frontend http-client resolveResponseMessage reads `message` before
// `error`, so both envelopes surface identically; the shape is pinned
// instead of changed to keep the wire behavior stable.
func TestEnvelope_AccountsBatchInvalid_PinnedLegacyMessageEnvelope(t *testing.T) {
	_, r, _ := setupAccountsTest(t)
	resp := doPostJSON(t, r, "/api/accounts/batch", map[string]any{})
	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("batch error body is not JSON: %v; raw=%s", err, resp.Body.String())
	}
	message, ok := body["message"].(string)
	if !ok || strings.TrimSpace(message) == "" {
		t.Fatalf("expected legacy non-empty \"message\" string, got %#v", body)
	}
	if success, has := body["success"]; has && success == true {
		t.Fatalf("error body must not claim success=true: %#v", body)
	}
}
