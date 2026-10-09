package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

const octopusV5RealShapeFixture = `{
  "version":5,"exported_at":"2026-10-05T00:00:00Z",
  "channels":[
    {"id":11,"name":"Primary","dialect":"generic","enabled":true,"base_url":"https://upstream.example/v1","openai_chat_completion_path":"/v1/chat/completions","openai_response_path":"/v1/responses","anthropic_message_path":"/v1/messages","proxy":false,"channel_proxy":"","custom_header":[],"param_override":"","match_regex":""},
    {"id":12,"name":"Primary backup","dialect":"generic","enabled":true,"base_url":"https://upstream.example/v1","openai_chat_completion_path":"/v1/chat/completions","openai_response_path":"/v1/responses","anthropic_message_path":"/v1/messages","proxy":false,"channel_proxy":"","custom_header":[],"param_override":"","match_regex":""}
  ],
  "groups":[{"id":21,"name":"client-model","mode":"failover","active_item_id":31,"relay_config":{}}],
  "channel_keys":[{"id":13,"channel_id":11,"name":"prod","key":"fixture-key-octopus-1","enabled":true},{"id":14,"channel_id":12,"name":"backup","key":"fixture-key-octopus-2","enabled":true}],
  "channel_models":[{"id":15,"channel_id":11,"name":"provider-model"},{"id":16,"channel_id":12,"name":"provider-model"}],
  "channel_grants":[{"id":17,"channel_model_id":15,"channel_key_id":13,"protocols":2},{"id":18,"channel_model_id":16,"channel_key_id":14,"protocols":2}],
  "group_items":[{"id":31,"group_id":21,"channel_grant_id":17,"priority":0,"weight":1},{"id":32,"group_id":21,"channel_grant_id":18,"priority":1,"weight":1}],
  "api_keys":[],"llm_infos":[],"settings":[],"stats_total":[],"stats_daily":[],"stats_hourly":[],"stats_api_key":[]
}`

const octopusV5AllProtocolsFixture = `{"version":5,"exported_at":"2026-10-06T00:00:00Z","channels":[{"id":1,"name":"all protocols","dialect":"generic","enabled":true,"base_url":"https://upstream.example","openai_chat_completion_path":"/v1/route-chat","openai_response_path":"/v1/route-responses","anthropic_message_path":"/v1/route-messages","proxy":false,"channel_proxy":"","custom_header":[],"param_override":"","match_regex":""}],"channel_keys":[{"id":2,"channel_id":1,"name":"primary","key":"sk-all-protocols","enabled":true}],"channel_models":[{"id":3,"channel_id":1,"name":"provider-model"}],"channel_grants":[{"id":4,"channel_model_id":3,"channel_key_id":2,"protocols":14}],"groups":[{"id":5,"name":"all-protocol-model","mode":"failover","active_item_id":6,"relay_config":{}}],"group_items":[{"id":6,"group_id":5,"channel_grant_id":4,"priority":0,"weight":1}],"api_keys":[],"llm_infos":[],"settings":[],"stats_total":[],"stats_daily":[],"stats_hourly":[],"stats_api_key":[]}`

func TestOctopusV5ParseAndImportRemapsIDsAndRepeatsSafely(t *testing.T) {
	preview, err := PreviewOctopusV5([]byte(octopusV5RealShapeFixture), "source-a")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Sections["channelKeys"] != 2 || preview.Sections["channelGrants"] != 2 {
		t.Fatalf("wrong preview: %+v", preview)
	}
	if strings.Contains(string(mustJSON(t, preview)), "fixture-key-octopus") {
		t.Fatal("preview leaked credential material")
	}

	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	first, err := ImportOctopusV5(db, []byte(octopusV5RealShapeFixture), "source-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := ImportOctopusV5(db, []byte(octopusV5RealShapeFixture), "source-a")
	if err != nil {
		t.Fatal(err)
	}
	if first["channels"] != 2 || first["channelKeys"] != 2 || first["channelGrants"] != 2 {
		t.Fatalf("wrong imported counts: %+v", first)
	}
	if second["channels"] != 2 {
		t.Fatalf("reimport did not resolve source rows: %+v", second)
	}
	var channels, keys, grants, mappings int
	if err := db.Get(&channels, `SELECT COUNT(*) FROM upstream_channels`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&keys, `SELECT COUNT(*) FROM upstream_credentials`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&grants, `SELECT COUNT(*) FROM upstream_grants`); err != nil {
		t.Fatal(err)
	}
	if err := db.Get(&mappings, `SELECT COUNT(*) FROM external_source_ids`); err != nil {
		t.Fatal(err)
	}
	if channels != 2 || keys != 2 || grants != 2 || mappings != 12 {
		t.Fatalf("target graph counts channels=%d keys=%d grants=%d mappings=%d", channels, keys, grants, mappings)
	}
	var sameURLChannels int
	if err := db.Get(&sameURLChannels, `SELECT COUNT(*) FROM upstream_channels WHERE base_url = ?`, "https://upstream.example/v1"); err != nil {
		t.Fatal(err)
	}
	if sameURLChannels != 2 {
		t.Fatalf("same-url source channels merged: %d", sameURLChannels)
	}
}

func TestOctopusV5RejectsActiveItemFromAnotherGroup(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal([]byte(octopusV5RealShapeFixture), &payload); err != nil {
		t.Fatal(err)
	}
	groups := payload["groups"].([]any)
	groups = append(groups, map[string]any{"id": 22, "name": "other-model", "mode": "manual", "active_item_id": 31, "relay_config": map[string]any{}})
	payload["groups"] = groups
	if _, err := ParseOctopusV5(mustJSON(t, payload)); err == nil {
		t.Fatal("accepted an active item belonging to a different group")
	}
}

// The current official v5 export has no group-item weight; its priority is
// authoritative. Keep this distinct from the older synthetic weighted fixture.
func TestOctopusV5CurrentExportWithoutGroupItemWeight(t *testing.T) {
	payload := strings.ReplaceAll(octopusV5RealShapeFixture, `,"weight":1`, "")
	preview, err := PreviewOctopusV5([]byte(payload), "current-official-v5")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Sections["groupItems"] != 2 || len(preview.Blocking) != 0 {
		t.Fatalf("unexpected current v5 preview: %+v", preview)
	}
}

func TestOctopusV5RejectsUnsupportedAndBrokenRelationships(t *testing.T) {
	for name, payload := range map[string]string{
		"unsupported dialect":   strings.Replace(octopusV5RealShapeFixture, `"dialect":"generic"`, `"dialect":"vendor-x"`, 1),
		"unknown channel field": strings.Replace(octopusV5RealShapeFixture, `"proxy":false`, `"proxy":false,"unmapped_field":true`, 1),
		"bad grant reference":   strings.Replace(octopusV5RealShapeFixture, `"channel_key_id":13`, `"channel_key_id":999`, 1),
		"unknown protocol bit":  strings.Replace(octopusV5RealShapeFixture, `"protocols":2`, `"protocols":32`, 1),
		"invalid origin key":    octopusV5RealShapeFixture,
	} {
		t.Run(name, func(t *testing.T) {
			if name == "client header template" && !channelHasClientHeaderTemplate(json.RawMessage(`[{"header_key":"X-Trace","header_value":"{client_header:X-Trace-ID}"}]`)) {
				t.Fatal("client header template recognizer returned false")
			}
			if name == "client header template" {
				d, err := ParseOctopusV5([]byte(payload))
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("custom header: %s", d.Channels[0].CustomHeader)
			}
			origin := "source-a"
			if name == "invalid origin key" {
				origin = "../source"
			}
			if name == "invalid origin key" {
				if _, err := PreviewOctopusV5([]byte(payload), origin); err == nil {
					t.Fatal("invalid origin key accepted")
				}
				return
			}
			if _, err := ParseOctopusV5([]byte(payload)); err == nil {
				t.Fatal("invalid source accepted")
			}
		})
	}
}

func TestOctopusV5StatisticsArePreviewedAndPreserved(t *testing.T) {
	payload := strings.Replace(octopusV5RealShapeFixture, `"dialect":"generic"`, `"dialect":"generic","input_token":7,"request_success":2`, 1)
	payload = strings.Replace(payload, `"stats_total":[]`, `"stats_total":[{"model":"client-model","input_token":7}]`, 1)
	preview, err := PreviewOctopusV5([]byte(payload), "stats-source")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Sections["channelStats"] != 1 || preview.Sections["statsTotal"] != 1 {
		t.Fatalf("preview omitted stats counts: %+v", preview.Sections)
	}
	if preview.NotImported["channelStats"] != 0 || preview.NotImported["statsTotal"] != 0 {
		t.Fatalf("preserved stats are incorrectly reported as dropped: %+v", preview.NotImported)
	}
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportOctopusV5(db, []byte(payload), "stats-source"); err != nil {
		t.Fatalf("import stats: %v", err)
	}
	var count int
	if err := db.Get(&count, `SELECT COUNT(*) FROM upstream_import_stats`); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("stored stats rows=%d; want one inline and one aggregate record", count)
	}
	if _, err := ImportOctopusV5(db, []byte(payload), "stats-source"); err != nil {
		t.Fatalf("repeat import stats: %v", err)
	}
	if err := db.Get(&count, `SELECT COUNT(*) FROM upstream_import_stats`); err != nil || count != 2 {
		t.Fatalf("reimport duplicated imported statistics: count=%d err=%v", count, err)
	}
	var inline, aggregate string
	if err := db.Get(&inline, `SELECT data_json FROM upstream_import_stats WHERE section='channel' AND record_key='11'`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(inline, `"input_token":7`) || !strings.Contains(inline, `"request_success":2`) {
		t.Fatalf("inline stats changed semantics: %s", inline)
	}
	if err := db.Get(&aggregate, `SELECT data_json FROM upstream_import_stats WHERE section='stats_total' AND record_key='row:0'`); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(aggregate, `"model":"client-model"`) || !strings.Contains(aggregate, `"input_token":7`) {
		t.Fatalf("aggregate stats changed semantics: %s", aggregate)
	}
	withoutAggregate := strings.Replace(payload, `"stats_total":[{"model":"client-model","input_token":7}]`, `"stats_total":[]`, 1)
	if _, err := ImportOctopusV5(db, []byte(withoutAggregate), "stats-source"); err != nil {
		t.Fatalf("replace imported stats: %v", err)
	}
	if err := db.Get(&count, `SELECT COUNT(*) FROM upstream_import_stats`); err != nil || count != 1 {
		t.Fatalf("removed source stats were retained: count=%d err=%v", count, err)
	}
}

func TestOctopusV5CommitRefusesUnsupportedNonEmptySections(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	payload := strings.Replace(octopusV5RealShapeFixture, `"api_keys":[]`, `"api_keys":[{"id":99}]`, 1)
	if _, err := ImportOctopusV5(db, []byte(payload), "source-a"); err == nil || !strings.Contains(err.Error(), "unsupported non-empty sections") {
		t.Fatalf("unsupported API keys were not refused: %v", err)
	}
	var channels int
	if err := db.Get(&channels, `SELECT COUNT(*) FROM upstream_channels`); err != nil || channels != 0 {
		t.Fatalf("partial rows remain after rejected import: %d err=%v", channels, err)
	}
}

func TestOctopusV5SettingsAndChannelProxyPreviewSemantics(t *testing.T) {
	payload := strings.Replace(octopusV5RealShapeFixture, `"settings":[]`, `"settings":[{"key":"proxy_url","value":"http://proxy.example:8080"},{"key":"stats_save_interval","value":"10"},{"key":"model_filter","value":""}]`, 1)
	payload = strings.Replace(payload, `"proxy":false`, `"proxy":true`, 1)
	preview, err := PreviewOctopusV5([]byte(payload), "proxy-source")
	if err != nil {
		t.Fatal(err)
	}
	if preview.NotImported["settings"] != 2 || len(preview.Blocking) != 0 {
		t.Fatalf("preview doesn't distinguish mappable proxy settings from operational omissions: %+v", preview)
	}
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportOctopusV5WithUnsupportedMode(db, []byte(payload), "proxy-source", true); err != nil {
		t.Fatalf("explicit channels-only import: %v", err)
	}
	var proxySetting, channelProxy string
	if err := db.Get(&proxySetting, `SELECT value FROM settings WHERE key='proxy_url'`); err == nil {
		t.Fatalf("Octopus proxy_url unexpectedly changed global settings to %q", proxySetting)
	}
	if err := db.Get(&channelProxy, `SELECT channel_proxy FROM upstream_channels WHERE name='Primary'`); err != nil || channelProxy != "http://proxy.example:8080" {
		t.Fatalf("mapped channel proxy=%q err=%v", channelProxy, err)
	}
}

func TestOctopusV5OfficialGroupItemVirtualFieldsAreAcceptedOnlyWhenZero(t *testing.T) {
	virtual := `,"channel_id":0,"channel_name":"","model_name":"","key_name":"","protocols":0,"available":false`
	payload := strings.Replace(octopusV5RealShapeFixture, `"weight":1}`, `"weight":1`+virtual+`}`, 1)
	preview, err := PreviewOctopusV5([]byte(payload), "official-export")
	if err != nil {
		t.Fatalf("preview official v5 virtual zero fields: %v", err)
	}
	if preview.Sections["groupItems"] != 2 {
		t.Fatalf("groupItems preview count=%d, want 2", preview.Sections["groupItems"])
	}
	bad := strings.Replace(payload, `"channel_id":0`, `"channel_id":11`, 1)
	if _, err := PreviewOctopusV5([]byte(bad), "official-export"); err == nil {
		t.Fatal("non-zero joined channel_id virtual field was accepted as authoritative")
	}
}

func TestOctopusV5ProxyFlagControlsEffectiveChannelProxy(t *testing.T) {
	payload := strings.Replace(octopusV5RealShapeFixture, `"channel_proxy":""`, `"channel_proxy":"http://proxy.invalid:8080"`, 1)
	preview, err := PreviewOctopusV5([]byte(payload), "proxy-disabled")
	if err != nil || len(preview.Blocking) != 0 {
		t.Fatalf("proxy=false residual channel_proxy preview=%+v err=%v", preview, err)
	}
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportOctopusV5(db, []byte(payload), "proxy-disabled"); err != nil {
		t.Fatal(err)
	}
	var proxy string
	if err := db.Get(&proxy, `SELECT channel_proxy FROM upstream_channels WHERE name='Primary'`); err != nil || proxy != "" {
		t.Fatalf("proxy=false persisted channel proxy=%q err=%v", proxy, err)
	}

	missing := strings.Replace(octopusV5RealShapeFixture, `"proxy":false`, `"proxy":true`, 1)
	preview, err = PreviewOctopusV5([]byte(missing), "proxy-missing")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blocking) == 0 || preview.Blocking[0] != "channelProxyUnavailable" {
		t.Fatalf("proxy=true without source proxy not blocking: %+v", preview)
	}
	if _, err := ImportOctopusV5(db, []byte(missing), "proxy-missing"); err == nil {
		t.Fatal("proxy=true without source proxy imported with a direct-fallback risk")
	}
}

func TestOctopusV5GroupNamesCannotBecomePatterns(t *testing.T) {
	for _, name := range []string{"gpt-*", "gpt-?", "re:.*"} {
		payload := strings.Replace(octopusV5RealShapeFixture, `"name":"client-model"`, `"name":"`+name+`"`, 1)
		if _, err := ParseOctopusV5([]byte(payload)); err == nil {
			t.Errorf("pattern-like group name %q accepted", name)
		}
	}
	if _, err := ParseOctopusV5([]byte(strings.Replace(octopusV5RealShapeFixture, `"name":"client-model"`, `"name":"gpt-4o"`, 1))); err != nil {
		t.Fatalf("literal route name rejected: %v", err)
	}
}

func TestOctopusV5RejectsMetadataBaseAndProxyTargets(t *testing.T) {
	for name, payload := range map[string]string{
		"base URL":      strings.Replace(octopusV5RealShapeFixture, `https://upstream.example/v1`, `http://169.254.169.254/latest`, 1),
		"channel proxy": strings.Replace(strings.Replace(octopusV5RealShapeFixture, `"proxy":false`, `"proxy":true`, 1), `"channel_proxy":""`, `"channel_proxy":"http://169.254.169.254:8080"`, 1),
		"global proxy":  strings.Replace(strings.Replace(octopusV5RealShapeFixture, `"proxy":false`, `"proxy":true`, 1), `"settings":[]`, `"settings":[{"key":"proxy_url","value":"http://169.254.169.254:8080"}]`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := PreviewOctopusV5([]byte(payload), "ssrf-source"); err == nil {
				t.Fatal("metadata destination accepted by preview parser")
			}
		})
	}
}

func TestOctopusV5BlockingClientHeaderAndRelayConfig(t *testing.T) {
	cases := []struct {
		name, payload string
		blocking      bool
	}{
		{"supported request id template", strings.Replace(octopusV5RealShapeFixture, `"custom_header":[]`, `"custom_header":[{"header_key":"X-Trace","header_value":"{client_header:X-Request-ID}"}]`, 1), false},
		{"authorization template", strings.Replace(octopusV5RealShapeFixture, `"custom_header":[]`, `"custom_header":[{"header_key":"Authorization","header_value":"{client_header:Authorization}"}]`, 1), true},
		{"relay config", strings.Replace(octopusV5RealShapeFixture, `"relay_config":{}`, `"relay_config":{"member_max_attempts":5}`, 1), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			preview, err := PreviewOctopusV5([]byte(tc.payload), "blocked-source")
			if err != nil {
				t.Fatal(err)
			}
			if got := len(preview.Blocking) > 0; got != tc.blocking {
				t.Fatalf("blocking=%v, want %v: %+v", got, tc.blocking, preview)
			}
			if _, err := ParseOctopusV5([]byte(tc.payload)); err != nil {
				t.Fatalf("source should be structurally parseable: %v", err)
			}
		})
	}
}

func TestOctopusV5DefaultRelayConfigRequiresAcknowledgedAdaptation(t *testing.T) {
	const defaultRelay = `{"member_max_attempts":2,"member_retry_interval_seconds":3,"member_non_stream_response_timeout_seconds":120,"member_stream_first_event_timeout_seconds":30,"member_cooldown_seconds":60,"member_affinity_seconds":300}`
	payload := strings.Replace(octopusV5RealShapeFixture, `"relay_config":{}`, `"relay_config":`+defaultRelay, 1)
	preview, err := PreviewOctopusV5([]byte(payload), "relay-default-source")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blocking) != 0 || len(preview.Adaptations) != 1 || preview.Adaptations[0] != "groupRelayConfigDefaults" {
		t.Fatalf("default relay policy preview = %+v, want explicit adaptation and no hard blocker", preview)
	}
	if _, err := ImportOctopusV5WithUnsupportedMode(nil, []byte(payload), "relay-default-source", false); err == nil || !strings.Contains(err.Error(), "acknowledgement") {
		t.Fatalf("default relay policy imported without acknowledgement: %v", err)
	}

	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportOctopusV5WithUnsupportedMode(db, []byte(payload), "relay-default-source", true); err != nil {
		t.Fatalf("acknowledged default relay policy import: %v", err)
	}
	var groups, items int
	if err := db.Get(&groups, `SELECT COUNT(*) FROM upstream_groups`); err != nil || groups != 1 {
		t.Fatalf("imported groups=%d err=%v", groups, err)
	}
	if err := db.Get(&items, `SELECT COUNT(*) FROM upstream_group_items`); err != nil || items != 2 {
		t.Fatalf("imported group items=%d err=%v", items, err)
	}
	var relay string
	if err := db.Get(&relay, `SELECT relay_config FROM upstream_groups`); err != nil || relay != "{}" {
		t.Fatalf("adapted relay config = %q err=%v; want normalized empty runtime policy", relay, err)
	}
}

func TestOctopusV5ImportRollsBackWholeGraphOnWriteFailure(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_octopus_credential BEFORE INSERT ON upstream_credentials BEGIN SELECT RAISE(ABORT, 'injected credential write failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportOctopusV5(db, []byte(octopusV5RealShapeFixture), "rollback-source"); err == nil || !strings.Contains(err.Error(), "injected credential write failure") {
		t.Fatalf("import error = %v, want injected write failure", err)
	}
	for _, table := range []string{"upstream_channels", "external_source_ids", "upstream_groups", "upstream_import_stats"} {
		var count int
		if err := db.Get(&count, `SELECT COUNT(*) FROM `+table); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("partial import left %d row(s) in %s", count, table)
		}
	}
}

func TestOctopusV5PostgresImportReimportAndAtomicRollback(t *testing.T) {
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set")
	}
	db, err := store.Open(store.DialectPostgres, dsn, false)
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatalf("PostgreSQL AutoMigrate: %v", err)
	}
	origin := fmt.Sprintf("octopus-pg-integration-%d", time.Now().UnixNano())
	fixture := strings.Replace(octopusV5RealShapeFixture, `"name":"client-model"`, `"name":"client-model-`+origin+`"`, 1)
	_, _ = db.Exec(`DROP TRIGGER IF EXISTS reject_octopus_credential ON upstream_credentials`)
	_, _ = db.Exec(`DROP FUNCTION IF EXISTS reject_octopus_credential()`)
	if _, err := db.Exec(`CREATE FUNCTION reject_octopus_credential() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected credential write failure'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TRIGGER reject_octopus_credential BEFORE INSERT ON upstream_credentials FOR EACH ROW EXECUTE FUNCTION reject_octopus_credential()`); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportOctopusV5(db, []byte(fixture), origin); err == nil || !strings.Contains(err.Error(), "injected credential write failure") {
		t.Fatalf("injected import error = %v", err)
	}
	for _, table := range []string{"upstream_channels", "external_source_ids", "upstream_groups", "upstream_import_stats"} {
		var count int
		if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM `+table+` WHERE origin_key = ?`), origin); err != nil {
			t.Fatalf("count %s after rollback: %v", table, err)
		}
		if count != 0 {
			t.Errorf("PostgreSQL rollback left %d rows in %s", count, table)
		}
	}
	if _, err := db.Exec(`DROP TRIGGER reject_octopus_credential ON upstream_credentials`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP FUNCTION reject_octopus_credential()`); err != nil {
		t.Fatal(err)
	}
	first, err := ImportOctopusV5(db, []byte(fixture), origin)
	if err != nil {
		t.Fatalf("PostgreSQL import: %v", err)
	}
	second, err := ImportOctopusV5(db, []byte(fixture), origin)
	if err != nil {
		t.Fatalf("PostgreSQL re-import: %v", err)
	}
	if first["channels"] != 2 || second["channels"] != 2 {
		t.Fatalf("PostgreSQL import counts first=%v second=%v", first, second)
	}
	for table, want := range map[string]int{"upstream_channels": 2, "upstream_credentials": 2, "upstream_grants": 2, "upstream_groups": 1, "upstream_group_items": 2} {
		var count int
		if err := db.Get(&count, db.Rebind(`SELECT COUNT(*) FROM `+table+` WHERE origin_key = ?`), origin); err != nil {
			t.Fatalf("count %s after re-import: %v", table, err)
		}
		if count != want {
			t.Errorf("%s rows after re-import = %d, want %d", table, count, want)
		}
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
