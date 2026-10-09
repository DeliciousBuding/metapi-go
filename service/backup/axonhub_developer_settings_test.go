package backup

import (
	"encoding/json"
	"testing"
)

func TestAxonHubImportsInheritedDeveloperAssociations(t *testing.T) {
	// Developer rules bind the requested model at inheritance time; a model
	// can add local rules or explicitly opt out without changing its siblings.
	raw := []byte(`{"version":"1.4","timestamp":"2026-10-09T00:00:00Z","channels":[
		{"id":1,"type":"openai","name":"fixture-one","base_url":"https://one.invalid","credentials":{"apiKey":"fixture-one"},"supported_models":["gpt-6","gpt-6-mini","standalone"],"tags":["developer"]},
		{"id":2,"type":"openai","name":"fixture-two","base_url":"https://two.invalid","credentials":{"apiKey":"fixture-two"},"supported_models":["gpt-6","gpt-6-mini","standalone"]}
	],"models":[
		{"id":1,"developer":"openai","model_id":"gpt-6","settings":{"associations":[{"type":"channel_model","priority":1,"channelModel":{"channelId":1,"modelId":"gpt-6"}}]}},
		{"id":2,"developer":"openai","model_id":"gpt-6-mini"},
		{"id":3,"developer":"openai","model_id":"standalone","settings":{"disableDeveloperSettingsInheritance":true,"associations":[{"type":"channel_model","channelModel":{"channelId":2,"modelId":"standalone"}}]}}
	],"system_configs":[]}`)
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	payload["system_configs"] = []any{map[string]any{
		"key":   "system_model_settings",
		"value": `{"query_all_channel_models":false,"developer_settings":[{"developer":"openai","associations":[{"type":"channel_tags_model","priority":5,"channelTagsModel":{"channelTags":["developer"]}},{"type":"channel_model","priority":3,"channelModel":{"channelId":2}}]}]}`,
	}}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	db := openAxonHubTestDB(t)
	counts, err := ImportAxonHubV14(db, raw, "inherited", false)
	if err != nil {
		t.Fatal(err)
	}
	if counts["routes"] != 3 || counts["groupItems"] != 5 {
		t.Fatalf("inherited graph = %#v, want 3 routes / 5 candidates", counts)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_group_items i JOIN upstream_route_groups g ON g.group_id=i.group_id JOIN token_routes r ON r.id=g.route_id WHERE r.model_pattern='gpt-6' AND i.priority=1`); got != 1 {
		t.Fatal("local rule did not retain higher priority over the inherited candidate")
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_group_items i JOIN upstream_route_groups g ON g.group_id=i.group_id JOIN token_routes r ON r.id=g.route_id WHERE r.model_pattern='standalone'`); got != 1 {
		t.Fatal("model that opted out inherited extra candidates")
	}
}
