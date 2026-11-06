package backup

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"github.com/deliciousbuding/metapi-go/store"
)

func TestAxonHubDecisionsReimportAndOriginIsolation(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := []byte(`{"version":"1.4","channels":[{"id":1,"type":"openai","name":"Decisions fixture","base_url":"https://fixture.invalid/api","credentials":{"apiKey":"synthetic-decisions"},"supported_models":["provider-model"],"endpoints":[{"api_format":"openai/decisions","path":"/decide"}],"settings":{"modelProtocols":[{"model":"decision-alias","apiFormats":["openai/decisions"]}]}}],"models":[{"id":1,"model_id":"decision-alias","type":"chat","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"provider-model"}}]}}]}`)
	preview, err := PreviewAxonHubV14(db, raw, "decisions-a")
	if err != nil || len(preview.Blocking) != 0 || len(preview.SkippedChannels) != 0 || preview.Routable["routes"] != 1 {
		t.Fatalf("Decisions preview failed: %v %+v", err, preview)
	}
	for _, residual := range preview.Residuals {
		if residualContains([]string{residual}, "openai/decisions") {
			t.Fatal("executable Decisions remained a protocol residual")
		}
	}
	for _, origin := range []string{"decisions-a", "decisions-a"} {
		if _, err := ImportAxonHubV14(db, raw, origin, false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ImportAxonHubV14(db, raw, "decisions-b", false); err == nil {
		t.Fatal("a different origin claimed the existing model route")
	}
	if _, err := ImportAxonHubV14(db, bytes.ReplaceAll(raw, []byte("decision-alias"), []byte("decision-b")), "decisions-b", false); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`); got != 2 {
		t.Fatalf("reimport duplicated or merged origins: %d", got)
	}
	rows, err := db.Query(`SELECT endpoint_config FROM upstream_channels ORDER BY origin_key`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var endpoints store.DirectEndpoints
		if err := rows.Scan(&endpoints); err != nil || endpoints.Decisions == nil || endpoints.Decisions.URL != "https://fixture.invalid/api/decide" {
			t.Fatalf("Decisions persistence lost URL: %v %+v", err, endpoints)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	var order store.DirectProtocolOrder
	if err := db.Get(&order, `SELECT protocol_order FROM upstream_group_items ORDER BY id LIMIT 1`); err != nil || !slices.Equal(order, store.DirectProtocolOrder{protoDecisions}) {
		t.Fatalf("model format restriction widened: %v %v", err, order)
	}
}

func TestAxonHubDecisionsCannotBorrowCustomOAuth(t *testing.T) {
	for _, provider := range []string{"codex", "fenno", "claudecode"} {
		ch := AxonHubSourceChannel{Type: provider, BaseURL: "https://fixture.invalid", Credentials: AxonHubSourceCredentials{OAuth: true, OAuthCredentials: &AxonHubSourceOAuth{AccessToken: "synthetic-access"}}, Endpoints: []AxonHubSourceEndpoint{{APIFormat: "openai/decisions"}}}
		compiled, problems, _ := compileAxonHubChannel(ch)
		if compiled != nil || !slices.Contains(problems, "oauth_custom_endpoint_unsupported") {
			raw, _ := json.Marshal(problems)
			t.Fatalf("%s Decisions acquired a generation OAuth grant: %s", provider, raw)
		}
	}
}
