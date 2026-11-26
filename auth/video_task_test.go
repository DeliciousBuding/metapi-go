package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/sqlitetest"
	"github.com/deliciousbuding/metapi-go/store"
)

type videoTaskBudgetAdmitter struct {
	*httptest.ResponseRecorder
}

func (w *videoTaskBudgetAdmitter) AdmitProxyUserBudget(r *http.Request, _ *ProxyAuthContext) (*http.Request, int, string) {
	return r, 0, ""
}

func TestOwnsDirectVideoTaskExemptsV3AndUserOwnedKeysFromQuota(t *testing.T) {
	db := sqlitetest.Open(t)
	prevDB := store.GetDB()
	store.OverrideDB(db)
	t.Cleanup(func() { store.OverrideDB(prevDB) })

	now := time.Now().UTC().Format(time.RFC3339)

	// Seed a system managed key with max_requests = 1, already exhausted (used_requests = 1).
	res, err := db.Exec(`INSERT INTO downstream_api_keys
		(name, key, enabled, max_requests, used_requests, supported_models, allowed_route_ids,
		 site_weight_multipliers, excluded_site_ids, excluded_credential_refs, key_owner_type, created_at, updated_at)
		VALUES ('sys-v3', 'sk-video-sys-v3', TRUE, 1, 1, '["*"]', '[]', '{}', '[]', '[]', 'system', ?, ?)`, now, now)
	if err != nil {
		t.Fatal(err)
	}
	sysKeyID, _ := res.LastInsertId()
	sysOwner := DirectVideoTaskOwner(&ProxyAuthContext{Source: "managed", Token: "sk-video-sys-v3", KeyID: &sysKeyID})

	// Seed a gateway user + user-owned key with max_requests = 1, already exhausted.
	users := store.NewGatewayUserStore(db)
	user, err := users.CreateUser(context.Background(), "u-video", "", 0, true)
	if err != nil {
		t.Fatal(err)
	}
	userID := user.ID
	res, err = db.Exec(`INSERT INTO downstream_api_keys
		(name, key, enabled, max_requests, used_requests, supported_models, allowed_route_ids,
		 site_weight_multipliers, excluded_site_ids, excluded_credential_refs, key_owner_type, owner_user_id, created_at, updated_at)
		VALUES ('user-v3', 'sk-video-user-v3', TRUE, 1, 1, '["*"]', '[]', '{}', '[]', '[]', 'user', ?, ?, ?)`, userID, now, now)
	if err != nil {
		t.Fatal(err)
	}
	userKeyID, _ := res.LastInsertId()
	userOwner := DirectVideoTaskOwner(&ProxyAuthContext{Source: "managed", Token: "sk-video-user-v3", KeyID: &userKeyID, OwnerUserID: &userID})

	// Insert v3 provider-driver tasks for both keys.
	for _, row := range []struct {
		publicID string
		owner    string
	}{
		{"video_direct_v3_sys", sysOwner},
		{"video_direct_v3_user", userOwner},
	} {
		identity := `{"version":3,"owner":"` + row.owner + `"}`
		if _, err := db.Exec(`INSERT INTO proxy_video_tasks (public_id, upstream_video_id, site_url, token_value, requested_model, direct_identity, created_at, last_upstream_status)
			VALUES (?, 'up-1', 'https://example.invalid', '', 'm', ?, ?, 'queued')`, row.publicID, identity, now); err != nil {
			t.Fatal(err)
		}
	}

	prevRT := config.RuntimeSafe()
	config.SetRuntime(&config.RuntimeSettings{})
	t.Cleanup(func() { config.SetRuntime(prevRT) })
	handler := ProxyAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, tc := range []struct {
		name     string
		token    string
		path     string
		wantCode int
	}{
		{name: "system key owns v3 video task read when exhausted", token: "sk-video-sys-v3", path: "/v1/videos/video_direct_v3_sys", wantCode: http.StatusOK},
		{name: "user key owns v3 video task read when exhausted", token: "sk-video-user-v3", path: "/v1/videos/video_direct_v3_user", wantCode: http.StatusOK},
		{name: "user key cannot read system key task", token: "sk-video-user-v3", path: "/v1/videos/video_direct_v3_sys", wantCode: http.StatusTooManyRequests},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			rec := &videoTaskBudgetAdmitter{ResponseRecorder: httptest.NewRecorder()}
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d body=%s", rec.Code, tc.wantCode, rec.Body.String())
			}
		})
	}
}
