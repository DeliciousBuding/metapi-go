package store

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDirectVideoIdentityLegacyUpgrade(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	legacy := strings.ReplaceAll(buildProxyVideoTasksDDL(DialectSQLite), "direct_identity TEXT,", "")
	if _, err := db.Exec(legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO proxy_video_tasks (public_id,upstream_video_id,site_url,token_value) VALUES ('legacy-video','legacy-upstream','https://example.com','legacy-value')`); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	var identity *string
	var token string
	if err := db.QueryRow(`SELECT direct_identity,token_value FROM proxy_video_tasks WHERE public_id='legacy-video'`).Scan(&identity, &token); err != nil {
		t.Fatal(err)
	}
	if identity != nil || token != "legacy-value" {
		t.Fatal("upgrade changed legacy video mapping")
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
}

func TestDirectVideoIdentityMigrationRoundTrip(t *testing.T) {
	identity := map[string]any{"version": float64(1), "owner": "opaque-hash", "grantId": float64(7), "credentialId": float64(8)}
	rows := buildProxyVideoTasks([]map[string]any{{"id": float64(2), "public_id": "video_direct_fixture", "upstream_video_id": "upstream-video", "site_url": "", "token_value": "", "direct_identity": identity}})
	for _, dialect := range []string{DialectSQLite, DialectPostgres} {
		var query string
		var values []any
		if dialect == DialectSQLite {
			query, values = buildInsertSQLite(rows[0])
		} else {
			query, values = buildInsertPG(rows[0])
		}
		if !strings.Contains(query, "direct_identity") {
			t.Fatal("migration lost video task identity")
		}
		var decoded map[string]any
		index := -1
		for i, column := range rows[0].columns {
			if column == "direct_identity" {
				index = i
			}
		}
		if index < 0 {
			t.Fatal("identity column missing")
		}
		if err := json.Unmarshal([]byte(values[index].(string)), &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded["owner"] != identity["owner"] || decoded["credentialId"] != identity["credentialId"] {
			t.Fatal("migration changed identity")
		}
	}
}
