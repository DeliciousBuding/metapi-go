package store

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
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

func TestProviderBindingEndpointSlotsAndPortableJSON(t *testing.T) {
	entries := (DirectEndpoints{}).Entries()
	if len(entries) != contract.MaxProviderProtocols {
		t.Fatalf("driver/endpoint vocabularies diverged: %d", len(entries))
	}
	for _, entry := range entries {
		if !contract.ValidProviderProtocol(entry.Key) {
			t.Fatalf("endpoint slot missing from provider contract: %s", entry.Key)
		}
	}
	var config DirectRequestConfig
	if err := config.Scan(`{"provider":{"key":"driver","protocols":["chat"],"options":{"mode":"safe"}}}`); err != nil || !config.Active() {
		t.Fatalf("provider round-trip failed: %+v %v", config, err)
	}
	endpoints := DirectEndpoints{Chat: &DirectEndpoint{URL: "https://fixture.invalid/chat", Auth: DirectAuthBearer}}
	if err := config.ValidateEndpoints(endpoints); err != nil {
		t.Fatal(err)
	}
	value, err := config.Value()
	var restored DirectRequestConfig
	if err != nil || restored.Scan(value) != nil || restored.Provider == nil || string(restored.Provider.Options["mode"]) != `"safe"` {
		t.Fatalf("provider JSON lost: %v %v", value, err)
	}
	config.Provider.Protocols = []string{"video"}
	if config.ValidateEndpoints(endpoints) == nil {
		t.Fatal("provider created an absent transport slot")
	}
	for _, raw := range []string{
		`{"provider":{"key":"driver","protocols":[]}}`,
		`{"provider":{"key":"driver","protocols":["chat","chat"]}}`,
		`{"provider":{"key":"driver","protocols":["unknown"]}}`,
		`{"provider":{"key":"driver","protocols":["chat"],"unknown":true}}`,
		`{"provider":{"key":"driver","protocols":["chat"]},"plugins":[{"key":"driver","protocols":["chat"]}]}`,
	} {
		if err := config.Scan(raw); err == nil {
			t.Fatalf("invalid provider configuration accepted: %s", raw)
		}
	}
	if err := config.Scan(`{"provider":null}`); err != nil || config.Active() {
		t.Fatalf("null provider changed legacy defaults: %+v %v", config, err)
	}
	encoded, err := json.Marshal(config)
	if err != nil || string(encoded) != `{"options":{}}` {
		t.Fatalf("legacy config encoding changed: %s %v", encoded, err)
	}
}
