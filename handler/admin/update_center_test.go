package admin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
)

func setupUpdateCenterTest(t *testing.T) chi.Router {
	t.Helper()
	r := chi.NewRouter()
	RegisterUpdateCenterRoutes(r)
	return r
}

func TestUpdateCenterStatus_LocalOnly(t *testing.T) {
	r := setupUpdateCenterTest(t)

	resp := doGet(t, r, "/api/update-center/status")
	if resp.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s, want 200", resp.Code, resp.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, resp.Body.String())
	}
	if body["updateAvailable"] != false {
		t.Fatalf("updateAvailable=%v, must stay false (no invented remote update)", body["updateAvailable"])
	}
	if body["currentVersion"] != "0.0.0" || body["latestVersion"] != "0.0.0" {
		t.Fatalf("versions=%v/%v, want local 0.0.0 placeholders", body["currentVersion"], body["latestVersion"])
	}
	if body["lastCheckedAt"] != nil {
		t.Fatalf("lastCheckedAt=%v, want nil (no fake poll timestamp)", body["lastCheckedAt"])
	}
	residual, _ := body["residual"].(string)
	if residual == "" {
		t.Fatalf("expected residual field for local stub honesty: %v", body)
	}
	if mode, _ := body["mode"].(string); mode != "external" {
		t.Fatalf("mode=%v, want external (UC-1)", body["mode"])
	}
}
