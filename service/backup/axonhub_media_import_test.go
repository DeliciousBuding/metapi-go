package backup

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

const axonHubMediaAssociationsFixture = `{"version":"1.4","timestamp":"2026-10-10T00:00:00Z","channels":[
 {"id":1,"type":"jina","name":"jina media","base_url":"https://jina.invalid/base","credentials":{"apiKeys":["fixture-active","fixture-disabled"]},"disabled_api_keys":[{"key":"fixture-disabled"}],"supported_models":["actual-vector","actual-rerank","unassociated"],"endpoints":[{"api_format":"jina/embeddings","path":"/native-vector"},{"api_format":"openai/embeddings","base_url":"https://generic.invalid","path":"/generic-vector"}],"settings":{"modelMappings":[{"from":"mapped-vector","to":"actual-vector"}],"modelProtocols":[{"model":"vector-public","apiFormats":["openai/embeddings","jina/embeddings"]},{"model":"vector-native","apiFormats":["jina/embeddings"]},{"model":"rerank-public","apiFormats":["jina/rerank"]}]}},
 {"id":2,"type":"modelscope","name":"image media","base_url":"https://images.invalid/api","credentials":{"apiKey":"fixture-images"},"supported_models":["actual-image","unassociated-image"],"endpoints":[{"api_format":"openai/image_generation","path":"/generic-image"}],"settings":{"modelProtocols":[{"model":"image-public","apiFormats":["modelscope/image_generation","openai/image_generation"]}]}},
 {"id":3,"type":"qiniu","name":"chat only","base_url":"https://chat.invalid","credentials":{"apiKey":"fixture-chat"},"supported_models":["not-an-embedding"]}
],"models":[
 {"id":1,"model_id":"vector-public","type":"embedding","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"mapped-vector"}}]}},
 {"id":2,"model_id":"vector-native","type":"embedding","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"actual-vector"}}]}},
 {"id":3,"model_id":"rerank-public","type":"rerank","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":1,"modelId":"actual-rerank"}}]}},
 {"id":4,"model_id":"image-public","type":"image_generation","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":2,"modelId":"actual-image"}}]}},
 {"id":5,"model_id":"unavailable-vector","type":"embedding","settings":{"associations":[{"type":"channel_model","channelModel":{"channelId":3,"modelId":"not-an-embedding"}}]}}
]}`

func TestAxonHubMediaImportPreservesAssociationsAndSnapshot(t *testing.T) {
	db := openAxonHubTestDB(t)
	raw := []byte(axonHubMediaAssociationsFixture)
	preview, err := PreviewAxonHubV14(db, raw, "media-associations")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Blocking) != 0 || len(preview.SkippedChannels) != 0 || preview.Routable["channels"] != 3 || preview.Routable["models"] != 3 || preview.Routable["grants"] != 3 || preview.Routable["routes"] != 4 {
		t.Fatalf("unexpected media plan: %+v", preview)
	}
	if !residualContains(preview.Residuals, "model_has_no_importable_channel:unavailable-vector") {
		t.Fatalf("chat-only embedding lacks explicit residual: %v", preview.Residuals)
	}
	if countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`) != 0 {
		t.Fatal("preview wrote channels")
	}
	counts, err := ImportAxonHubV14(db, raw, "media-associations", false)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range preview.Routable {
		if counts[key] != int64(want) {
			t.Fatalf("preview/import differ for %s: %d / %d", key, want, counts[key])
		}
	}
	if countRows(t, db, `SELECT COUNT(*) FROM upstream_grants g JOIN upstream_credentials c ON c.id=g.credential_id WHERE c.enabled=?`, false) != 0 ||
		countRows(t, db, `SELECT COUNT(*) FROM upstream_models WHERE name LIKE 'unassociated%'`) != 0 ||
		countRows(t, db, `SELECT COUNT(*) FROM token_routes WHERE model_pattern='unavailable-vector'`) != 0 {
		t.Fatal("import manufactured unassociated or disabled-key grants")
	}
	var endpoints store.DirectEndpoints
	if err := db.Get(&endpoints, db.Rebind(`SELECT endpoint_config FROM upstream_channels WHERE name=?`), "jina media"); err != nil {
		t.Fatal(err)
	}
	if endpoints.JinaEmbeddings == nil || endpoints.Embeddings == nil || endpoints.JinaEmbeddings.URL != "https://jina.invalid/base/native-vector" || endpoints.JinaEmbeddings.Profile != "jina-embeddings" || endpoints.Embeddings.URL != "https://generic.invalid/generic-vector" || endpoints.Embeddings.Profile != "" {
		t.Fatalf("embedding formats lost their own contracts: %+v", endpoints)
	}
	if err := db.Get(&endpoints, db.Rebind(`SELECT endpoint_config FROM upstream_channels WHERE name=?`), "image media"); err != nil {
		t.Fatal(err)
	}
	if endpoints.ModelScopeImageGeneration == nil || endpoints.ImageGeneration == nil || endpoints.ModelScopeImageGeneration.URL != "https://images.invalid/api/images/generations" || endpoints.ModelScopeImageGeneration.Profile != "modelscope-image" || endpoints.ImageGeneration.URL != "https://images.invalid/api/generic-image" || endpoints.ImageGeneration.Profile != "" {
		t.Fatalf("image formats lost their own contracts: %+v", endpoints)
	}
	var routeIDs []int64
	if err := db.Select(&routeIDs, `SELECT id FROM token_routes ORDER BY id`); err != nil {
		t.Fatal(err)
	}
	candidates, err := service.NewProxyRoutingStore(db).LoadRouteChannels(context.Background(), routeIDs)
	if err != nil {
		t.Fatal(err)
	}
	wantOrder := []store.DirectProtocolOrder{{protoEmbeddings, protoJinaEmbeddings}, {protoJinaEmbeddings}, {protoRerank}, {protoModelScopeImage, protoImageGeneration}}
	if len(candidates) != len(wantOrder) {
		t.Fatalf("loaded %d route candidates, want %d", len(candidates), len(wantOrder))
	}
	for i, candidate := range candidates {
		if candidate.Channel.Direct == nil || !reflect.DeepEqual(candidate.Channel.Direct.ProtocolOrder, wantOrder[i]) {
			t.Fatalf("candidate %d lost ordered source formats: %+v", i, candidate.Channel.Direct)
		}
	}
	if candidates[0].Channel.Direct.GrantID != candidates[1].Channel.Direct.GrantID {
		t.Fatal("two aliases should share one actual-model grant")
	}
	before, err := axonHubMappedSources(db, db, "media-associations")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE upstream_grants SET success_count=9`); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportAxonHubV14(db, raw, "media-associations", false); err != nil {
		t.Fatal(err)
	}
	after, err := axonHubMappedSources(db, db, "media-associations")
	if err != nil || !reflect.DeepEqual(before, after) || countRows(t, db, `SELECT COUNT(*) FROM upstream_grants WHERE success_count=9`) != 3 {
		t.Fatalf("reimport lost identity/runtime state: %v", err)
	}
	// A colliding origin fails after beginning its own graph import. Its rows
	// must roll back without touching this origin's endpoints or grant identity.
	if _, err := ImportAxonHubV14(db, raw, "media-conflict", false); err == nil {
		t.Fatal("a second origin stole the media routes")
	}
	if countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`) != 3 || countRows(t, db, `SELECT COUNT(*) FROM external_source_ids WHERE origin_key=?`, "media-conflict") != 0 {
		t.Fatal("failed import left partial media graph")
	}
	reduced := removeAxonHubChannelAndItsModels(t, raw, 2)
	removal, err := PreviewAxonHubV14(db, reduced, "media-associations")
	if err != nil || removal.Removals["upstream_channels"] != 1 || removal.Removals["upstream_grants"] != 1 || removal.Removals["token_routes"] != 1 {
		t.Fatalf("media removal preview: %+v err=%v", removal, err)
	}
	if _, err := ImportAxonHubV14(db, reduced, "media-associations", false); !errors.Is(err, ErrAxonHubReplacementRequired) {
		t.Fatalf("media removal skipped replacement gate: %v", err)
	}
	if countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`) != 3 {
		t.Fatal("refused replacement mutated media graph")
	}
	if _, err := ImportAxonHubV14(db, reduced, "media-associations", true); err != nil {
		t.Fatal(err)
	}
	if countRows(t, db, `SELECT COUNT(*) FROM upstream_grants`) != 2 || countRows(t, db, `SELECT COUNT(*) FROM token_routes`) != 3 || countRows(t, db, `SELECT COUNT(*) FROM upstream_channels WHERE name='image media'`) != 0 {
		t.Fatal("confirmed media replacement left stale grants or routes")
	}
}
