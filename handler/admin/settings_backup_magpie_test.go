package admin

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMagpieBackupPreviewAndCommitRequireBodyPassphrase(t *testing.T) {
	db := setupBackupTestDB(t)
	h := &backupHandler{db: db.DB}
	passphrase := "fixture-magpie-passphrase"
	envelope := adminSealMagpie(t, map[string]any{
		"version": 2, "created": "2026-10-20T00:00:00Z", "app": "fixture", "keys": true,
		"providers": []any{map[string]any{
			"id": "relay", "name": "Fixture relay", "key": "fixture-provider",
			"chat": "https://relay.example/v1", "models": []string{"fixture-model"},
		}},
		"groups": []any{map[string]any{
			"id": "fixture", "name": "Fixture group", "members": []string{"relay/fixture-model"},
		}},
		"gatewayKeys": []any{map[string]any{
			"id": "gateway-1", "name": "Fixture gateway", "secret": "fixture-gateway",
			"models": []string{"group/fixture"},
		}},
	}, passphrase)
	body := adminMagpieBody(t, envelope, passphrase)

	previewReq := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import/preview", strings.NewReader(body))
	previewReq.Header.Set("X-External-Origin-Key", "magpie-handler")
	previewRec := httptest.NewRecorder()
	h.previewBackupImport(previewRec, previewReq)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", previewRec.Code, previewRec.Body.String())
	}
	for _, leaked := range []string{"fixture-provider", "fixture-gateway", "relay.example", "Fixture relay"} {
		if strings.Contains(previewRec.Body.String(), leaked) {
			t.Fatalf("preview leaked %q: %s", leaked, previewRec.Body.String())
		}
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM upstream_channels`); err != nil || count != 0 {
		t.Fatalf("preview wrote channels: count=%d err=%v", count, err)
	}

	commitReq := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import", strings.NewReader(body))
	commitReq.Header.Set("X-External-Origin-Key", "magpie-handler")
	commitRec := httptest.NewRecorder()
	h.importBackup(commitRec, commitReq)
	if commitRec.Code != http.StatusOK || strings.Contains(commitRec.Body.String(), "fixture-provider") || strings.Contains(commitRec.Body.String(), "fixture-gateway") {
		t.Fatalf("commit status/body=%d %s", commitRec.Code, commitRec.Body.String())
	}
	if err := db.Get(&count, `SELECT COUNT(*) FROM upstream_channels`); err != nil || count != 1 {
		t.Fatalf("imported channels=%d err=%v", count, err)
	}

	badReq := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import/preview", strings.NewReader(adminMagpieBody(t, envelope, "wrong-passphrase")))
	badRec := httptest.NewRecorder()
	h.previewBackupImport(badRec, badReq)
	if badRec.Code != http.StatusBadRequest || strings.Contains(badRec.Body.String(), "wrong-passphrase") {
		t.Fatalf("wrong passphrase status/body=%d %s", badRec.Code, badRec.Body.String())
	}

	rawReq := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import/preview", strings.NewReader(string(envelope)))
	rawRec := httptest.NewRecorder()
	h.previewBackupImport(rawRec, rawReq)
	if rawRec.Code != http.StatusBadRequest || !strings.Contains(rawRec.Body.String(), "passphrase is required") {
		t.Fatalf("raw envelope without passphrase status/body=%d %s", rawRec.Code, rawRec.Body.String())
	}

	duplicate := strings.Replace(body, `"passphrase":`, `"passphrase":"duplicate","passphrase":`, 1)
	dupReq := httptest.NewRequest(http.MethodPost, "/api/settings/backup/import/preview", strings.NewReader(duplicate))
	dupRec := httptest.NewRecorder()
	h.previewBackupImport(dupRec, dupReq)
	if dupRec.Code != http.StatusBadRequest || !strings.Contains(dupRec.Body.String(), "duplicate JSON object key") {
		t.Fatalf("duplicate wrapper key status/body=%d %s", dupRec.Code, dupRec.Body.String())
	}
}

func adminMagpieBody(t *testing.T, envelope []byte, passphrase string) string {
	t.Helper()
	var data any
	if err := json.Unmarshal(envelope, &data); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"data": data, "passphrase": passphrase})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func adminSealMagpie(t *testing.T, bundle map[string]any, passphrase string) []byte {
	t.Helper()
	plain, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	salt := []byte("0123456789abcdef")
	nonce := []byte("012345678901")
	const iterations = 100_000
	key, err := pbkdf2.Key(sha256.New, passphrase, salt, iterations, 32)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	aad := []byte("magpie-backup/1/pbkdf2-sha256/100000/30313233343536373839616263646566")
	sealed := gcm.Seal(nil, nonce, plain, aad)
	raw, err := json.Marshal(map[string]any{
		"format": "magpie-backup", "version": 1, "kdf": "pbkdf2-sha256",
		"iterations": iterations, "salt": salt, "nonce": nonce, "data": sealed,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
