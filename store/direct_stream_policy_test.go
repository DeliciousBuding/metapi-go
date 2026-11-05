package store

import (
	"strings"
	"testing"
)

func TestDirectStreamPolicyMigrationPreservesExistingChannel(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ddl := strings.Replace(buildUpstreamChannelsDDL(DialectSQLite), "stream_policy TEXT NOT NULL DEFAULT 'unlimited', ", "", 1)
	if _, err := db.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO upstream_channels (origin_key,source_id,name,dialect,enabled,base_url,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex) VALUES ('fixture',1,'Existing','generic',TRUE,'https://fixture.invalid','','','',FALSE,'','[]','','')`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := AutoMigrate(db); err != nil {
			t.Fatal(err)
		}
	}
	var name, policy string
	if err := db.QueryRow(`SELECT name,stream_policy FROM upstream_channels WHERE source_id=1`).Scan(&name, &policy); err != nil || name != "Existing" || policy != "unlimited" {
		t.Fatalf("migrated channel=%s policy=%s err=%v", name, policy, err)
	}
}
