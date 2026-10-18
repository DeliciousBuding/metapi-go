package admin

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestRootKeySettingRetired(t *testing.T) {
	db, router, _ := setupEdgeTest(t)
	response := doPutJSON(t, router, "/api/settings/runtime", map[string]any{"proxyToken": "sk-synthetic-retired"})
	if response.Code != http.StatusGone {
		t.Fatalf("retired setting: %d %s", response.Code, response.Body.String())
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM settings WHERE key='proxy_token'`); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("retired setting was persisted")
	}
}

func TestBackupImportCannotRestoreRetiredRootSetting(t *testing.T) {
	db := setupBackupTestDB(t)
	rt := &config.RuntimeSettings{}
	if err := store.MigrateLegacyProxyToken(db, rt, 0); err != nil {
		t.Fatal(err)
	}
	result, err := importBackupTables(db.DB, map[string]json.RawMessage{
		"settings": json.RawMessage(`[{"key":"proxy_token","value":"\"sk-synthetic-backup-root\""}]`),
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.skippedSettings != 1 {
		t.Fatal("retired setting was not reported as skipped")
	}
	var legacyCount, keyCount int
	if err := db.Get(&legacyCount, `SELECT COUNT(*) FROM settings WHERE key='proxy_token'`); err != nil {
		t.Fatal(err)
	}
	if err := store.MigrateLegacyProxyToken(db, &config.RuntimeSettings{ProxyToken: "sk-synthetic-backup-root"}, 0); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&keyCount, `SELECT COUNT(*) FROM downstream_api_keys`); err != nil {
		t.Fatal(err)
	}
	if legacyCount != 0 || keyCount != 0 {
		t.Fatal("backup or restart restored the retired root credential")
	}
}
