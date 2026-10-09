package store

import (
	"strings"
	"testing"
)

func TestDirectEndpointConfigMigratesExistingChannels(t *testing.T) {
	db, err := Open(DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	legacy := strings.Replace(buildUpstreamChannelsDDL(DialectSQLite), "endpoint_config TEXT NOT NULL DEFAULT '{}', ", "", 1)
	if _, err := db.Exec(legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO upstream_channels(origin_key,source_id,name,dialect,enabled,base_url,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex) VALUES('legacy',1,'legacy','generic',1,'https://provider.invalid','/v1/chat/completions','/v1/responses','/v1/messages',0,'','[]','','')`); err != nil {
		t.Fatal(err)
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	var endpoints DirectEndpoints
	if err := db.QueryRow(`SELECT endpoint_config FROM upstream_channels WHERE name='legacy'`).Scan(&endpoints); err != nil {
		t.Fatal(err)
	}
	if endpoints.Chat != nil || endpoints.Responses != nil || endpoints.Messages != nil {
		t.Fatal("legacy path contract changed")
	}
	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	endpoints.Responses = &DirectEndpoint{URL: "https://responses.invalid/api/infer", Auth: DirectAuthBearer}
	if _, err := db.Exec(`UPDATE upstream_channels SET endpoint_config=? WHERE name='legacy'`, endpoints); err != nil {
		t.Fatal(err)
	}
	var loaded DirectEndpoints
	if err := db.QueryRow(`SELECT endpoint_config FROM upstream_channels WHERE name='legacy'`).Scan(&loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.Responses == nil || *loaded.Responses != *endpoints.Responses {
		t.Fatalf("endpoint lost: %+v", loaded)
	}
}

func TestDirectEndpointsRejectMalformedConfiguration(t *testing.T) {
	for _, raw := range []string{`broken`, `{"chat":{"url":"https://provider.invalid","auth":"cookie"}}`, `{"messages":{"url":"https://user:secret@provider.invalid","auth":"bearer"}}`, `{"responses":{"url":"file:///tmp/example","auth":"bearer"}}`} {
		var endpoints DirectEndpoints
		if endpoints.Scan(raw) == nil {
			t.Fatalf("accepted malformed configuration %s", raw)
		}
	}
}
