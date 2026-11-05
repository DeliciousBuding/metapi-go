package store

import (
	"strings"
	"testing"
)

func TestDirectRateLimitMigrationAndValidation(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	legacy := strings.Replace(buildUpstreamChannelsDDL(DialectSQLite), ", rate_limit TEXT NOT NULL DEFAULT '{}'", "", 1)
	if _, err := db.Exec(legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO upstream_channels(origin_key,source_id,name,dialect,enabled,base_url,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex) VALUES('legacy',1,'legacy','generic',1,'https://provider.invalid','/v1/chat/completions','','',0,'','[]','','')`); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	var limits DirectRateLimit
	if err := db.Get(&limits, `SELECT rate_limit FROM upstream_channels`); err != nil || limits.Enabled() {
		t.Fatalf("legacy row acquired active limits: %+v %v", limits, err)
	}
	for _, raw := range []string{`null`, `[]`, `{"rpm":-1}`, `{"rpm":1.5}`, `{"future":0}`, `{"queueTimeoutMs":9223372036854775807}`, `{} {}`} {
		if err := limits.Scan(raw); err == nil {
			t.Fatalf("invalid rate configuration accepted: %s", raw)
		}
	}
	if err := limits.Scan(`{"rpm":2,"tpm":100,"maxConcurrent":1,"queueSize":2,"queueTimeoutMs":5}`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE upstream_channels SET rate_limit=?`, limits); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	var loaded DirectRateLimit
	if err := db.Get(&loaded, `SELECT rate_limit FROM upstream_channels`); err != nil || loaded != limits {
		t.Fatalf("migration reset persisted limits: %+v %v", loaded, err)
	}
}
