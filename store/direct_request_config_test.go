package store

import (
	"strings"
	"testing"
)

func TestDirectRequestConfigMigrationKeepsLegacyBehavior(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ddl := strings.Replace(buildUpstreamChannelsDDL(DialectSQLite), "request_config TEXT NOT NULL DEFAULT '{}', ", "", 1)
	if _, err := db.Exec(ddl); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO upstream_channels(origin_key,source_id,name,dialect,enabled,base_url,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex) VALUES('legacy',1,'legacy','generic',1,'https://fixture.invalid','/chat','/responses','/messages',0,'','[]','{}','')`); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	var config DirectRequestConfig
	if err := db.Get(&config, `SELECT request_config FROM upstream_channels WHERE name='legacy'`); err != nil || config.Active() {
		t.Fatalf("legacy request behavior changed: %#v %v", config, err)
	}
	for _, raw := range []string{`{"future":true}`, `{"body":[{"op":"set","path":"stream","value":"true"}]}`, `{"headers":[{"op":"copy","from":"Authorization","to":"X-Leak"}]}`} {
		if config.Scan(raw) == nil {
			t.Fatalf("persisted invalid config accepted: %s", raw)
		}
	}
}
