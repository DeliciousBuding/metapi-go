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
	legacy = strings.ReplaceAll(legacy, "accounting_state TEXT,", "")
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
	var accounting *string
	var token string
	if err := db.QueryRow(`SELECT direct_identity,accounting_state,token_value FROM proxy_video_tasks WHERE public_id='legacy-video'`).Scan(&identity, &accounting, &token); err != nil {
		t.Fatal(err)
	}
	if identity != nil || accounting != nil || token != "legacy-value" {
		t.Fatal("upgrade changed legacy video mapping")
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
}

func TestDirectVideoIdentityMigrationRoundTrip(t *testing.T) {
	identity := map[string]any{"version": float64(1), "owner": "opaque-hash", "grantId": float64(7), "credentialId": float64(8)}
	accounting := map[string]any{"version": float64(1), "initialized": true, "keyId": float64(9), "usage": map[string]any{"found": true, "totalTokens": float64(75)}, "cost": float64(0.15)}
	rows := buildProxyVideoTasks([]map[string]any{{"id": float64(2), "public_id": "video_direct_fixture", "upstream_video_id": "upstream-video", "site_url": "", "token_value": "", "direct_identity": identity, "accounting_state": accounting}})
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
			if column == "accounting_state" {
				var state map[string]any
				if json.Unmarshal([]byte(values[i].(string)), &state) != nil || state["cost"] != accounting["cost"] || state["keyId"] != accounting["keyId"] {
					t.Fatal("migration changed video accounting baseline")
				}
			}
		}
		if index < 0 {
			t.Fatal("identity column missing")
		}
		if !strings.Contains(query, "accounting_state") {
			t.Fatal("migration dropped video accounting")
		}
		if err := json.Unmarshal([]byte(values[index].(string)), &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded["owner"] != identity["owner"] || decoded["credentialId"] != identity["credentialId"] {
			t.Fatal("migration changed identity")
		}
	}
}
