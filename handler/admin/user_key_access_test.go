package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestUserKeyAdminOwnershipAndPortableBackup(t *testing.T) {
	for _, dialect := range []string{store.DialectSQLite, store.DialectPostgres} {
		t.Run(dialect, func(t *testing.T) {
			db, r := setupDownstreamKeysTest(t)
			if dialect == store.DialectPostgres {
				db, r = setupDownstreamKeysPostgresTest(t)
			}
			users := store.NewGatewayUserStore(db)
			user, err := users.CreateUser(context.Background(), "person", "", 0, true)
			if err != nil {
				t.Fatal(err)
			}
			call := func(method, path, body string, want int) *httptest.ResponseRecorder {
				t.Helper()
				req := httptest.NewRequest(method, path, strings.NewReader(body))
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				if w.Code != want {
					t.Fatalf("%s %s: %d %s want %d", method, path, w.Code, w.Body.String(), want)
				}
				return w
			}
			for _, id := range []int64{0, -1, 999999} {
				call("POST", "/api/downstream-keys", fmt.Sprintf(`{"name":"bad","key":"sk-bad-fixture","ownerUserId":%d}`, id), 400)
			}
			w := call("POST", "/api/downstream-keys", fmt.Sprintf(`{"name":"personal","key":"sk-personal-fix","ownerUserId":%d,"supportedModels":["*"]}`, user.ID), 200)
			var created struct {
				Item struct {
					ID           int64  `json:"id"`
					OwnerUserID  int64  `json:"ownerUserId"`
					KeyOwnerType string `json:"keyOwnerType"`
				} `json:"item"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
				t.Fatal(err)
			}
			if created.Item.OwnerUserID != user.ID || created.Item.KeyOwnerType != "user" {
				t.Fatalf("owner response: %s", w.Body.String())
			}
			path := fmt.Sprintf("/api/downstream-keys/%d", created.Item.ID)
			for _, body := range []string{`{"ownerUserId":null}`, `{"ownerUserId":999999}`, `{"keyOwnerType":"system"}`} {
				call("PUT", path, body, 400)
			}
			call("PUT", path, `{"name":"renamed","enabled":false}`, 200)
			call("POST", "/api/downstream-keys", `{"name":"system","key":"sk-system-fix","supportedModels":["*"]}`, 200)
			if _, err := db.Exec(`INSERT INTO proxy_logs (owner_user_id,user_group_id,created_at,status) VALUES (?,?,'2026-10-21T00:00:00Z','success')`, user.ID, user.GroupID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO proxy_file_objects (file_id,owner) VALUES ('file-local-user','user-key:fixture'),('file-system','fixture-system')`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(`INSERT INTO proxy_video_tasks (public_id,upstream_video_id,site_url,token_value,direct_identity) VALUES ('video-local-user','fixture','','','{"owner":"user-key:fixture"}')`); err != nil {
				t.Fatal(err)
			}
			for _, exportType := range []string{"all", "accounts"} {
				payload, err := buildBackupPayload(db.DB, exportType)
				if err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(payload)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(encoded), "sk-personal-fix") || !strings.Contains(string(encoded), "sk-system-fix") || !strings.Contains(string(encoded), "excluded_rows") {
					t.Fatalf("unsafe portable Key export: %s", encoded)
				}
				if strings.Contains(string(encoded), "file-local-user") || strings.Contains(string(encoded), "video-local-user") || !strings.Contains(string(encoded), "file-system") {
					t.Fatalf("unsafe user resource export: %s", encoded)
				}
			}
			for table, raw := range map[string]string{
				"proxy_file_objects": `[{"file_id":"planted-file","owner":"user-key:fixture"}]`,
				"proxy_video_tasks":  `[{"public_id":"planted-video","upstream_video_id":"fixture","site_url":"","token_value":"","direct_identity":"{\"owner\": \"user-key:fixture\"}"}]`,
			} {
				if _, err := importBackupTables(db.DB, map[string]json.RawMessage{table: json.RawMessage(raw)}, false); err == nil {
					t.Fatalf("user media import accepted: %s", table)
				}
			}
			for _, row := range []string{
				`{"name":"planted","key":"sk-planted-fixture","key_owner_type":"user","owner_user_id":1}`,
				`{"name":"planted","key":"sk-planted-fixture","key_owner_type":"user"}`,
				`{"name":"planted","key":"sk-planted-fixture","owner_user_id":1}`,
			} {
				if _, err := importBackupTables(db.DB, map[string]json.RawMessage{"downstream_api_keys": json.RawMessage("[" + row + "]")}, false); err == nil {
					t.Fatalf("user Key portable import accepted: %s", row)
				}
			}
			if err := users.UpdateUser(context.Background(), user.ID, "", user.GroupID, false); err != nil {
				t.Fatal(err)
			}
			call("POST", "/api/downstream-keys", fmt.Sprintf(`{"name":"disabled","key":"sk-disabled-fix","ownerUserId":%d}`, user.ID), 400)
		})
	}
}
