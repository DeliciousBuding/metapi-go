package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/go-chi/chi/v5"
)

func putRuntimeSettings(t *testing.T, r chi.Router, body map[string]any, confirm string) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/settings/runtime", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if confirm != "" {
		req.Header.Set(auth.ReauthConfirmHeader, confirm)
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// Egress destinations and the admin ACL are credential-bearing: a session
// alone must not be able to redirect traffic or widen the management surface.
func TestSettingsRuntime_SensitiveFieldsRequireReauth(t *testing.T) {
	_, r, _ := setupEdgeTest(t)
	const token = "admin-edge-test-token"

	for _, body := range []map[string]any{
		{"systemProxyUrl": "http://proxy.example:3128"},
		{"telegramApiBaseUrl": "http://127.0.0.1:18098"},
		{"telegramUseSystemProxy": true},
		{"telegramBotToken": "synthetic-bot-token"},
		{"adminIpAllowlist": []any{"127.0.0.1"}},
		{"smtpHost": "127.0.0.1", "smtpPort": 18100},
		{"feishuWebhook": "http://127.0.0.1:18099/hook"},
	} {
		resp := putRuntimeSettings(t, r, body, "")
		if resp.Code != http.StatusForbidden {
			t.Errorf("body %v without confirmation = %d, want 403: %s", body, resp.Code, resp.Body.String())
			continue
		}
		if !bytes.Contains(resp.Body.Bytes(), []byte(`"reauthRequired":true`)) {
			t.Errorf("body %v rejection missing reauthRequired: %s", body, resp.Body.String())
		}
		if resp := putRuntimeSettings(t, r, body, token); resp.Code != http.StatusOK {
			t.Errorf("body %v with confirmation = %d, want 200: %s", body, resp.Code, resp.Body.String())
		}
	}

	// Ordinary settings still save without re-confirmation.
	resp := putRuntimeSettings(t, r, map[string]any{"systemName": "Synthetic Gateway"}, "")
	if resp.Code != http.StatusOK {
		t.Fatalf("benign settings update = %d, want 200: %s", resp.Code, resp.Body.String())
	}
}

// pathID must reject every form strconv.ParseInt would otherwise accept
// (`+1`, surrounding whitespace, NBSP) so the handler and any path-based
// middleware agree on what an id is.
func TestPathIDRejectsNonDigitForms(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/api/downstream-keys/{id}", func(w http.ResponseWriter, req *http.Request) {
		if _, ok := pathID(w, req); !ok {
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	cases := map[string]int{
		"/api/downstream-keys/1":            http.StatusOK,
		"/api/downstream-keys/+1":           http.StatusBadRequest,
		"/api/downstream-keys/%201%20":      http.StatusBadRequest,
		"/api/downstream-keys/%091":         http.StatusBadRequest,
		"/api/downstream-keys/%C2%A01":      http.StatusBadRequest,
		"/api/downstream-keys/%E3%80%801":   http.StatusBadRequest,
		"/api/downstream-keys/1.0":          http.StatusBadRequest,
		"/api/downstream-keys/0":            http.StatusBadRequest,
		"/api/downstream-keys/-1":           http.StatusBadRequest,
		"/api/downstream-keys/999999999999": http.StatusOK,
	}
	for path, want := range cases {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("%s = %d, want %d", path, rec.Code, want)
		}
	}
}
