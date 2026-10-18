package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

// ---------------------------------------------------------------------------
// S1 — client-controlled Upgrade headers must not skip admission unless the
// request is a real GET handshake on a Responses WebSocket route.
// ---------------------------------------------------------------------------

func serveProxyAuth(t *testing.T, handler http.Handler, method, path string, headers map[string]string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, path, reader)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func newAdmissionTestMiddleware(t *testing.T) http.Handler {
	t.Helper()
	config.SetRuntime(proxyCfg("synthetic-global-token"))
	t.Cleanup(func() { config.SetRuntime(nil) })
	return ProxyAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func forgedUpgradeHeaders(key string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + key,
		"Upgrade":       "websocket",
		"Connection":    "Upgrade",
	}
}

func TestProxyAuthMiddleware_ForgedUpgradeOnPOSTIsAdmittedAndCounted(t *testing.T) {
	setupTestDB(t)
	ResetKeyAdmissionForTest()
	t.Cleanup(ResetKeyAdmissionForTest)

	maxRPM := int64(1)
	maxRequests := int64(100)
	id := insertTestKeyWithRPM(t, "sk-forged-post", &maxRequests, 0, &maxRPM, nil)
	handler := newAdmissionTestMiddleware(t)

	first := serveProxyAuth(t, handler, http.MethodPost, "/v1/chat/completions",
		forgedUpgradeHeaders("sk-forged-post"), `{"model":"m","messages":[]}`)
	if first.Code != http.StatusOK {
		t.Fatalf("forged upgrade POST status = %d, want 200: %s", first.Code, first.Body.String())
	}
	if got := readUsedRequests(t, id); got != 1 {
		t.Fatalf("used_requests after forged upgrade POST = %d, want 1", got)
	}

	second := serveProxyAuth(t, handler, http.MethodPost, "/v1/chat/completions",
		forgedUpgradeHeaders("sk-forged-post"), `{"model":"m","messages":[]}`)
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second forged upgrade POST status = %d, want 429 (RPM must apply)", second.Code)
	}
	if got := readUsedRequests(t, id); got != 1 {
		t.Fatalf("used_requests after denied POST = %d, want 1 (admission deny must not burn)", got)
	}
}

func TestProxyAuthMiddleware_ForgedUpgradeOnGETModelsIsCounted(t *testing.T) {
	setupTestDB(t)
	ResetKeyAdmissionForTest()
	t.Cleanup(ResetKeyAdmissionForTest)

	maxRequests := int64(100)
	id := insertTestKeyWithRPM(t, "sk-forged-models", &maxRequests, 0, nil, nil)
	handler := newAdmissionTestMiddleware(t)

	rec := serveProxyAuth(t, handler, http.MethodGet, "/v1/models", forgedUpgradeHeaders("sk-forged-models"), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("forged upgrade GET /v1/models status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := readUsedRequests(t, id); got != 1 {
		t.Fatalf("used_requests after forged upgrade GET /v1/models = %d, want 1", got)
	}
}

func TestProxyAuthMiddleware_ForgedUpgradeOnPOSTResponsesIsCounted(t *testing.T) {
	setupTestDB(t)
	ResetKeyAdmissionForTest()
	t.Cleanup(ResetKeyAdmissionForTest)

	maxRequests := int64(100)
	id := insertTestKeyWithRPM(t, "sk-forged-responses", &maxRequests, 0, nil, nil)
	handler := newAdmissionTestMiddleware(t)

	rec := serveProxyAuth(t, handler, http.MethodPost, "/v1/responses",
		forgedUpgradeHeaders("sk-forged-responses"), `{"model":"m","input":[]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("forged upgrade POST /v1/responses status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if got := readUsedRequests(t, id); got != 1 {
		t.Fatalf("used_requests after forged upgrade POST /v1/responses = %d, want 1", got)
	}
}

func TestProxyAuthMiddleware_ResponsesWSHandshakeSkipsHandshakeBilling(t *testing.T) {
	for _, path := range []string{"/v1/responses", "/responses", "/responses/compact"} {
		t.Run(path, func(t *testing.T) {
			setupTestDB(t)
			ResetKeyAdmissionForTest()
			t.Cleanup(ResetKeyAdmissionForTest)

			key := "sk-ws-handshake-" + strings.TrimPrefix(strings.ReplaceAll(path, "/", "-"), "-")
			maxRPM := int64(1)
			maxRequests := int64(100)
			id := insertTestKeyWithRPM(t, key, &maxRequests, 0, &maxRPM, nil)
			handler := newAdmissionTestMiddleware(t)

			rec := serveProxyAuth(t, handler, http.MethodGet, path, forgedUpgradeHeaders(key), "")
			if rec.Code != http.StatusOK {
				t.Fatalf("GET %s handshake status = %d, want 200: %s", path, rec.Code, rec.Body.String())
			}
			if got := readUsedRequests(t, id); got != 0 {
				t.Fatalf("used_requests after real GET %s handshake = %d, want 0 (billed per message)", path, got)
			}
			// RPM admission is also deferred to the session loop, so a second
			// handshake attempt is still authorized here.
			again := serveProxyAuth(t, handler, http.MethodGet, path, forgedUpgradeHeaders(key), "")
			if again.Code != http.StatusOK {
				t.Fatalf("second handshake on %s status = %d, want 200 (no handshake billing)", path, again.Code)
			}
			if got := readUsedRequests(t, id); got != 0 {
				t.Fatalf("used_requests after second handshake on %s = %d, want 0", path, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// M5 — chunked bodies must not bypass maxTPM.
// ---------------------------------------------------------------------------

func TestProxyAuthMiddleware_ChunkedBodyStillTripsMaxTPM(t *testing.T) {
	setupTestDB(t)
	ResetKeyAdmissionForTest()
	t.Cleanup(ResetKeyAdmissionForTest)

	maxTPM := int64(1)
	id := insertTestKeyWithRPM(t, "sk-chunked-tpm", nil, 0, nil, &maxTPM)
	handler := newAdmissionTestMiddleware(t)

	body := `{"prompt":"` + strings.Repeat("a", 400) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(body))
	req.ContentLength = -1 // chunked: no Content-Length signal
	req.Header.Set("Authorization", "Bearer sk-chunked-tpm")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("chunked body with maxTPM=1 status = %d, want 429: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "over_tpm") {
		t.Fatalf("expected over_tpm error, got %s", rec.Body.String())
	}
	if got := readUsedRequests(t, id); got != 0 {
		t.Fatalf("used_requests after TPM deny = %d, want 0", got)
	}
}

func TestEstimateAdmissionTokens_ChunkedBodyUsesReadBytes(t *testing.T) {
	body := `{"prompt":"` + strings.Repeat("b", 400) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", strings.NewReader(body))
	req.ContentLength = -1

	if got := estimateAdmissionTokens(req); got < 100 {
		t.Fatalf("chunked estimate = %d, want >= 100 (400 prompt chars / 4)", got)
	}
	// The peek must restore the body for the handler.
	restored, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(restored) != body {
		t.Fatalf("body not restored after peek: %q", string(restored))
	}
}

func TestEstimateAdmissionTokens_UnestimableBodyReservesMinimum(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/completions", nil)
	req.ContentLength = -1
	if got := estimateAdmissionTokens(req); got != 1 {
		t.Fatalf("unestimable body estimate = %d, want conservative 1", got)
	}
}

// ---------------------------------------------------------------------------
// H9/L3 — per-turn reauthorization helper.
// ---------------------------------------------------------------------------

func TestReauthorizeManagedKeyForTurn_ConsumesExactlyOnce(t *testing.T) {
	setupTestDB(t)
	ResetKeyAdmissionForTest()
	t.Cleanup(ResetKeyAdmissionForTest)

	maxRequests := int64(10)
	id := insertTestKeyWithRPM(t, "sk-turn-once", &maxRequests, 0, nil, nil)

	for turn := 1; turn <= 2; turn++ {
		result := ReauthorizeManagedKeyForTurn(context.Background(), "sk-turn-once", "127.0.0.1", 0)
		if !result.OK {
			t.Fatalf("turn %d rejected: %s (%d)", turn, result.Error, result.StatusCode)
		}
		if got := readUsedRequests(t, id); got != int64(turn) {
			t.Fatalf("used_requests after turn %d = %d, want %d", turn, got, turn)
		}
	}
}

func TestReauthorizeManagedKeyForTurn_DisabledKeyRejected(t *testing.T) {
	setupTestDB(t)
	ResetKeyAdmissionForTest()
	t.Cleanup(ResetKeyAdmissionForTest)

	id := insertTestKey(t, "sk-turn-disabled", true, nil, 0, nil, 0, nil)
	if _, err := testDB(t).Exec(`UPDATE downstream_api_keys SET enabled = FALSE WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}

	result := ReauthorizeManagedKeyForTurn(context.Background(), "sk-turn-disabled", "127.0.0.1", 0)
	if result.OK || result.StatusCode != http.StatusForbidden || result.Reason != "disabled" {
		t.Fatalf("disabled key result = %+v, want 403 disabled", result)
	}
	if got := readUsedRequests(t, id); got != 0 {
		t.Fatalf("used_requests after disabled turn = %d, want 0", got)
	}
}

func TestReauthorizeManagedKeyForTurn_ExpiredKeyRejected(t *testing.T) {
	setupTestDB(t)
	ResetKeyAdmissionForTest()
	t.Cleanup(ResetKeyAdmissionForTest)

	expiredAt := "2020-01-01T00:00:00Z"
	insertTestKey(t, "sk-turn-expired", true, nil, 0, nil, 0, &expiredAt)

	result := ReauthorizeManagedKeyForTurn(context.Background(), "sk-turn-expired", "127.0.0.1", 0)
	if result.OK || result.Reason != "expired" {
		t.Fatalf("expired key result = %+v, want expired", result)
	}
}

func TestReauthorizeManagedKeyForTurn_OverCostRejected(t *testing.T) {
	setupTestDB(t)
	ResetKeyAdmissionForTest()
	t.Cleanup(ResetKeyAdmissionForTest)

	maxCost := 1.0
	id := insertTestKey(t, "sk-turn-overcost", true, &maxCost, 0, nil, 0, nil)
	if _, err := testDB(t).Exec(`UPDATE downstream_api_keys SET used_cost = 1 WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}

	result := ReauthorizeManagedKeyForTurn(context.Background(), "sk-turn-overcost", "127.0.0.1", 0)
	if result.OK || result.StatusCode != http.StatusTooManyRequests || result.Reason != "over_cost" {
		t.Fatalf("over-cost key result = %+v, want 429 over_cost", result)
	}
	if got := readUsedRequests(t, id); got != 0 {
		t.Fatalf("used_requests after over-cost turn = %d, want 0", got)
	}
}

func TestReauthorizeManagedKeyForTurn_RPMDenyDoesNotBurnRequests(t *testing.T) {
	setupTestDB(t)
	ResetKeyAdmissionForTest()
	t.Cleanup(ResetKeyAdmissionForTest)

	maxRPM := int64(1)
	maxRequests := int64(10)
	id := insertTestKeyWithRPM(t, "sk-turn-rpm", &maxRequests, 0, &maxRPM, nil)

	if result := ReauthorizeManagedKeyForTurn(context.Background(), "sk-turn-rpm", "127.0.0.1", 0); !result.OK {
		t.Fatalf("first turn rejected: %s", result.Error)
	}
	denied := ReauthorizeManagedKeyForTurn(context.Background(), "sk-turn-rpm", "127.0.0.1", 0)
	if denied.OK || denied.Reason != "over_rpm" {
		t.Fatalf("second turn result = %+v, want over_rpm", denied)
	}
	if got := readUsedRequests(t, id); got != 1 {
		t.Fatalf("used_requests after RPM deny = %d, want 1", got)
	}
}

func TestReauthorizeManagedKeyForTurn_PolicyReloaded(t *testing.T) {
	setupTestDB(t)
	ResetKeyAdmissionForTest()
	t.Cleanup(ResetKeyAdmissionForTest)

	id := insertTestKey(t, "sk-turn-policy", true, nil, 0, nil, 0, nil)

	before := ReauthorizeManagedKeyForTurn(context.Background(), "sk-turn-policy", "127.0.0.1", 0)
	if !before.OK {
		t.Fatalf("initial turn rejected: %s", before.Error)
	}
	if len(before.Policy.SupportedModels) != 0 {
		t.Fatalf("initial policy models = %v, want empty", before.Policy.SupportedModels)
	}

	if _, err := testDB(t).Exec(`UPDATE downstream_api_keys SET supported_models = '["model-a"]' WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	after := ReauthorizeManagedKeyForTurn(context.Background(), "sk-turn-policy", "127.0.0.1", 0)
	if !after.OK {
		t.Fatalf("turn after policy update rejected: %s", after.Error)
	}
	if len(after.Policy.SupportedModels) != 1 || after.Policy.SupportedModels[0] != "model-a" {
		t.Fatalf("policy not reloaded: models = %v", after.Policy.SupportedModels)
	}
}
