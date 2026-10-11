package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDashboardIndependentUpstreamCount(t *testing.T) {
	for _, dialect := range []string{"sqlite", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			db, r := setupStatsSQLiteTest(t)
			if dialect == "postgres" {
				db, r = setupStatsPostgresTest(t)
			}
			if _, err := db.Exec(`INSERT INTO upstream_channels (origin_key, source_id, name, dialect, enabled, base_url,
				openai_chat_completion_path, openai_response_path, anthropic_message_path,
				proxy, channel_proxy, custom_header, param_override, match_regex)
				VALUES ('journey', 1, 'Independent', 'generic', TRUE, 'https://upstream.example',
				'/v1/chat/completions', '/v1/responses', '/v1/messages', FALSE, '', '[]', '', '')`); err != nil {
				t.Fatal(err)
			}
			resp := doGet(t, r, "/api/stats/dashboard?view=summary&force=1")
			var body map[string]any
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if resp.Code != http.StatusOK || body["upstreamChannelCount"] != float64(1) || body["siteCount"] != float64(0) || body["totalAccounts"] != float64(0) {
				t.Fatalf("independent upstream must be counted without synthetic accounts: status=%d body=%v", resp.Code, body)
			}
		})
	}
}

func TestDashboardIndependentUpstreamCountFailureIsUnknown(t *testing.T) {
	db, r := setupStatsSQLiteTest(t)
	if _, err := db.Exec(`DROP TABLE upstream_channels`); err != nil {
		t.Fatal(err)
	}
	resp := doGet(t, r, "/api/stats/dashboard?view=summary&force=1")
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, present := body["upstreamChannelCount"]; !present || body["upstreamChannelCount"] != nil || body["siteCount"] != float64(0) {
		t.Fatalf("a failed count must be unknown while other counts survive: %v", body)
	}
}

func TestOAuthUnsupportedWritesAreNotSuccess(t *testing.T) {
	_, r := setupOAuthRoutesTest(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/api/oauth/import"},
		{http.MethodPost, "/api/oauth/route-units"},
		{http.MethodPatch, "/api/oauth/route-units/1"},
		{http.MethodDelete, "/api/oauth/route-units/1"},
	} {
		t.Run(tc.method+tc.path, func(t *testing.T) {
			resp := httptest.NewRecorder()
			r.ServeHTTP(resp, httptest.NewRequest(tc.method, tc.path, nil))
			var body map[string]any
			if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if resp.Code != http.StatusNotImplemented || body["success"] == true || body["code"] != "oauth_operation_not_supported" {
				t.Fatalf("unsupported write must not claim success: status=%d body=%v", resp.Code, body)
			}
		})
	}
}
