package backup

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/pgtest"
	"github.com/deliciousbuding/metapi-go/routing"
	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func accessRuntimeDB(t *testing.T) *store.DB {
	t.Helper()
	_ = store.CloseDatabase()
	rt := &config.RuntimeSettings{DbType: store.DialectSQLite, DbUrl: ":memory:", ProxyToken: "fixture-global"}
	if dsn := os.Getenv("PG_TEST_DSN"); dsn != "" {
		rt.DbType = store.DialectPostgres
		rt.DbUrl = dsn
	}
	old := config.RuntimeSafe()
	config.SetRuntime(rt)
	if err := store.EnsureRuntimeDatabase(&config.Config{DataDir: t.TempDir()}, rt); err != nil {
		t.Fatal(err)
	}
	db := store.GetDB()
	if db.Dialect == store.DialectPostgres {
		pgtest.Reset(t, db.DB)
	}
	t.Cleanup(func() { _ = store.CloseDatabase(); config.SetRuntime(old) })
	return db
}

func accessPayload(t *testing.T) map[string]any {
	t.Helper()
	var p map[string]any
	if err := json.Unmarshal(readAxonHubFixture(t, "axonhub-basic.json"), &p); err != nil {
		t.Fatal(err)
	}
	channels := p["channels"].([]any)
	ch := channels[0].(map[string]any)
	ch["tags"] = []string{"prod", "east"}
	for _, id := range []int{5, 6} {
		b, _ := json.Marshal(ch)
		var copy map[string]any
		_ = json.Unmarshal(b, &copy)
		copy["id"] = id
		copy["name"] = fmt.Sprint("fixture-", id)
		if id == 5 {
			copy["tags"] = []string{"prod", "west"}
		}
		channels = append(channels, copy)
	}
	p["channels"] = channels
	model := p["models"].([]any)[0].(map[string]any)
	model["settings"] = map[string]any{"associations": []any{map[string]any{"type": "model", "modelId": map[string]any{"modelId": "fixture-model"}}}}
	p["projects"] = []any{map[string]any{"id": 1, "name": "restricted project", "status": "active", "profiles": map[string]any{"activeProfile": "active", "profiles": []any{map[string]any{"name": "active", "channelIDs": []int{4, 5}, "channelTags": []string{"prod", "east"}, "channelTagsMatchMode": "all"}}}}}
	key := p["api_keys"].([]any)[0].(map[string]any)
	key["key"] = "sk-fixture-access"
	key["type"] = "personal"
	key["profiles"] = map[string]any{"activeProfile": "active", "profiles": []any{map[string]any{"name": "active", "channelTags": []string{"blocked"}, "channelTagsMatchMode": "none", "modelIDs": []string{"fixture-model"}, "modelMappings": []any{map[string]any{"from": "client-.*", "to": "fixture-model"}}}}}
	delete(p, "channel_model_prices")
	return p
}

func encodeAccessPayload(t *testing.T, p map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func accessHTTPHandler(db *store.DB) http.Handler {
	router := routing.NewTokenRouter(service.NewProxyRoutingStore(db), &config.Config{}, nil, nil)
	return auth.ProxyAuth()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pac := auth.GetProxyAuth(r.Context())
		selected, err := router.SelectChannel(r.Context(), r.URL.Query().Get("model"), routing.DownstreamRoutingPolicy{RequiredUpstreamProtocol: routing.UpstreamProtocolChat, AccessPolicy: pac.Policy.AccessPolicy, SupportedModels: pac.Policy.SupportedModels, DenyAllWhenEmpty: pac.Policy.DenyAllWhenEmpty})
		if err != nil {
			http.Error(w, "route error", 500)
			return
		}
		if selected == nil {
			http.Error(w, "model not available", 403)
			return
		}
		_, _ = fmt.Fprintf(w, "%d:%s", selected.Direct.ChannelID, selected.ActualModel)
	}))
}

func accessRequest(h http.Handler, key, ip, model string) *httptest.ResponseRecorder {
	r := httptest.NewRequest("POST", "/v1/chat/completions?model="+model, strings.NewReader(`{}`))
	r.RemoteAddr = ip
	r.Header.Set("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAxonHubAccessHTTPIntersectsProjectKeyAndModelMapping(t *testing.T) {
	db := accessRuntimeDB(t)
	p := accessPayload(t)
	if _, err := ImportAxonHubV14(db, encodeAccessPayload(t, p), "access-origin", false); err != nil {
		t.Fatal(err)
	}
	h := accessHTTPHandler(db)
	var expected int64
	if err := db.Get(&expected, db.Rebind(`SELECT target_id FROM external_source_ids WHERE origin_key=? AND entity_type='upstream_channels' AND source_id=4`), "access-origin"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, key, ip, model string
		status               int
	}{
		{"mapped", "sk-fixture-access", "192.0.2.1:44", "client-alias", 200},
		{"exact", "sk-fixture-access", "192.0.2.1:44", "fixture-model", 200},
		{"forbidden model", "sk-fixture-access", "192.0.2.1:44", "unrelated-model", 403},
		{"IP denied", "sk-fixture-access", "198.51.100.1:44", "fixture-model", 403},
		{"unknown key", "sk-other", "192.0.2.1:44", "fixture-model", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := accessRequest(h, tc.key, tc.ip, tc.model)
			if w.Code != tc.status {
				t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
			}
			if tc.status == 200 && w.Body.String() != fmt.Sprintf("%d:fixture-model", expected) {
				t.Fatalf("selection escaped source boundary: %s", w.Body.String())
			}
		})
	}
	key := p["api_keys"].([]any)[0].(map[string]any)
	key["scopes"] = []string{"read_api_keys", "write_api_keys"}
	if _, err := ImportAxonHubV14(db, encodeAccessPayload(t, p), "access-origin", false); err != nil {
		t.Fatal(err)
	}
	if w := accessRequest(h, "sk-fixture-access", "192.0.2.1:44", "client-alias"); w.Code != 403 {
		t.Fatalf("management key became proxy access: %d", w.Code)
	}
}

func TestAxonHubAccessEmptyIntersectionAndOriginCollisionRollback(t *testing.T) {
	db := accessRuntimeDB(t)
	p := accessPayload(t)
	profile := p["api_keys"].([]any)[0].(map[string]any)["profiles"].(map[string]any)["profiles"].([]any)[0].(map[string]any)
	profile["channelIDs"] = []int{6}
	if _, err := ImportAxonHubV14(db, encodeAccessPayload(t, p), "origin-one", false); err != nil {
		t.Fatal(err)
	}
	if w := accessRequest(accessHTTPHandler(db), "sk-fixture-access", "192.0.2.1:44", "client-alias"); w.Code != 403 {
		t.Fatalf("empty intersection allowed traffic: %d", w.Code)
	}
	before := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`)
	// Avoid the earlier route-name collision so this actually reaches the key
	// ownership check after staging the second origin's channels and grants.
	p["models"].([]any)[0].(map[string]any)["model_id"] = "other-origin-model"
	_, err := ImportAxonHubV14(db, encodeAccessPayload(t, p), "origin-two", false)
	if err == nil || !strings.Contains(err.Error(), "another origin") {
		t.Fatalf("expected key collision, got %v", err)
	}
	if strings.Contains(err.Error(), "sk-fixture-access") {
		t.Fatal("collision error exposed secret")
	}
	if after := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`); before != after {
		t.Fatalf("transaction leaked channels: %d -> %d", before, after)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM external_source_ids WHERE origin_key='origin-two'`); n != 0 {
		t.Fatal("failed import leaked ownership mappings")
	}
}

func TestAxonHubQuotaImportsHistoryAndKeepsLocalUsageOnReimport(t *testing.T) {
	db := accessRuntimeDB(t)
	p := accessPayload(t)
	profile := p["api_keys"].([]any)[0].(map[string]any)["profiles"].(map[string]any)["profiles"].([]any)[0].(map[string]any)
	profile["quota"] = map[string]any{"requests": 3, "totalTokens": 100, "cost": "0.5", "period": map[string]any{"type": "all_time"}}
	p["usage_logs"] = []any{map[string]any{"id": 21, "api_key_key": "sk-fixture-access", "created_at": time.Now().Add(-time.Minute).Format(time.RFC3339Nano), "total_tokens": 40, "total_cost": 0.2}}
	raw := encodeAccessPayload(t, p)
	if _, err := ImportAxonHubV14(db, raw, "quota-origin", false); err != nil {
		t.Fatal(err)
	}
	result := auth.AuthorizeDownstreamToken("sk-fixture-access", config.Runtime())
	if !result.OK {
		t.Fatalf("under-limit history rejected: %+v", result)
	}
	var id int64
	_ = db.Get(&id, `SELECT id FROM downstream_api_keys WHERE key='sk-fixture-access'`)
	tokens := int64(60)
	cost := 0.1
	auth.RecordManagedKeyQuotaUsage(id, &tokens, &cost)
	if result = auth.AuthorizeDownstreamToken("sk-fixture-access", config.Runtime()); result.OK || result.StatusCode != 429 {
		t.Fatalf("token quota ignored: %+v", result)
	}
	if _, err := ImportAxonHubV14(db, raw, "quota-origin", false); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, db, `SELECT COUNT(*) FROM downstream_quota_usage WHERE key_id=?`, id); n != 2 {
		t.Fatalf("history duplicated or local usage reset: %d", n)
	}
	if result = auth.AuthorizeDownstreamToken("sk-fixture-access", config.Runtime()); result.OK {
		t.Fatal("reimport reset budget")
	}
	delete(p, "usage_logs")
	if _, err := ImportAxonHubV14(db, encodeAccessPayload(t, p), "quota-origin", false); err != nil {
		t.Fatal(err)
	}
	if result = auth.AuthorizeDownstreamToken("sk-fixture-access", config.Runtime()); result.OK || result.Reason != "quota_history_missing" {
		t.Fatalf("missing history widened quota: %+v", result)
	}
}

func TestAxonHubQuotaNativeBackupIncludesPolicyAndUsage(t *testing.T) {
	db := accessRuntimeDB(t)
	p := accessPayload(t)
	if _, err := ImportAxonHubV14(db, encodeAccessPayload(t, p), "roundtrip", false); err != nil {
		t.Fatal(err)
	}
	var id int64
	_ = db.Get(&id, `SELECT id FROM downstream_api_keys WHERE key='sk-fixture-access'`)
	tokens := int64(9)
	cost := 0.2
	auth.RecordManagedKeyQuotaUsage(id, &tokens, &cost)
	payload, err := BuildPayload(db.DB, "all")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "downstream_quota_usage") || !strings.Contains(string(raw), "access_policy") {
		t.Fatal("native backup omitted policy or usage")
	}
}
