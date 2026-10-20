package backup

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/deliciousbuding/metapi-go/service"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestMagpieBackupImportsDirectGraphAndGatewayScope(t *testing.T) {
	db := openAxonHubTestDB(t)
	passphrase := "fixture-passphrase"
	raw := sealMagpieForTest(t, magpieFixtureBundle(), passphrase)
	preview, err := PreviewMagpieBackup(db, raw, passphrase, "magpie-test")
	if err != nil {
		t.Fatal(err)
	}
	if preview.Source != MagpieBackupOrigin || preview.Sections["providers"] != 1 {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	if preview.Routable["channels"] != 1 || preview.Routable["routes"] != 3 {
		t.Fatalf("unexpected routable counts: %#v", preview.Routable)
	}
	encodedPreview, _ := json.Marshal(preview)
	for _, secret := range []string{"fixture-provider-primary", "fixture-provider-backup", "fixture-gateway-secret"} {
		if strings.Contains(string(encodedPreview), secret) {
			t.Fatalf("preview leaked credential material")
		}
	}
	if strings.Contains(string(encodedPreview), "relay.example") {
		t.Fatalf("preview leaked a source URL")
	}

	counts, err := ImportMagpieBackup(db, raw, passphrase, "magpie-test", false)
	if err != nil {
		t.Fatal(err)
	}
	if counts["channels"] != 1 || counts["credentials"] != 2 || counts["models"] != 2 ||
		counts["grants"] != 4 || counts["routes"] != 3 || counts["groups"] != 3 ||
		counts["groupItems"] != 8 || counts["gatewayKeys"] != 1 {
		t.Fatalf("unexpected import counts: %#v", counts)
	}
	var endpoints store.DirectEndpoints
	if err := db.Get(&endpoints, db.Rebind(`SELECT endpoint_config FROM upstream_channels WHERE name=?`), "Fixture Relay"); err != nil {
		t.Fatal(err)
	}
	if endpoints.Chat == nil || endpoints.Chat.URL != "https://relay.example/v1/chat/completions" || endpoints.Chat.Auth != store.DirectAuthBearer {
		t.Fatalf("chat endpoint = %#v", endpoints.Chat)
	}
	if endpoints.Messages == nil || endpoints.Messages.URL != "https://relay.example/v1/messages" || endpoints.Messages.Auth != store.DirectAuthAPIKey {
		t.Fatalf("messages endpoint = %#v", endpoints.Messages)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_group_items i JOIN upstream_groups g ON g.id=i.group_id WHERE g.name=? AND i.priority=1`, "Coding"); got != 2 {
		t.Fatalf("ordered group priority count = %d, want 2", got)
	}
	var mode string
	var active int64
	if err := db.QueryRowx(db.Rebind(`SELECT mode,active_item_id FROM upstream_groups WHERE name=?`), "Coding").Scan(&mode, &active); err != nil {
		t.Fatal(err)
	}
	if mode != "failover" || active != 0 {
		t.Fatalf("group mode=%q active=%d", mode, active)
	}
	var policyRaw string
	if err := db.Get(&policyRaw, db.Rebind(`SELECT access_policy FROM downstream_api_keys WHERE key=?`), "fixture-gateway-secret"); err != nil {
		t.Fatal(err)
	}
	policy, err := store.ParseDownstreamAccessPolicy(policyRaw)
	if err != nil {
		t.Fatal(err)
	}
	if policy == nil || len(policy.ModelIDs) != 1 || policy.ModelIDs[0] != "group/coding" {
		t.Fatalf("gateway key policy = %#v", policy)
	}

	var routeIDs []int64
	if err := db.Select(&routeIDs, db.Rebind(`SELECT id FROM token_routes`)); err != nil {
		t.Fatal(err)
	}
	candidates, err := service.NewProxyRoutingStore(db).LoadRouteChannels(context.Background(), routeIDs)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 8 {
		t.Fatalf("routing candidates = %d, want 8", len(candidates))
	}
}

func TestMagpieBackupRejectsWrongPassphraseAndCorruption(t *testing.T) {
	raw := sealMagpieForTest(t, magpieFixtureBundle(), "right-passphrase")
	withBOM := append([]byte{0xEF, 0xBB, 0xBF}, raw...)
	if !IsMagpieBackupPayload(withBOM) {
		t.Fatal("backup with a UTF-8 BOM was not recognized")
	}
	if _, err := ParseMagpieBackup(withBOM, "right-passphrase"); err != nil {
		t.Fatalf("backup with a UTF-8 BOM: %v", err)
	}
	if _, err := ParseMagpieBackup(raw, "wrong-passphrase"); !errors.Is(err, ErrMagpiePassphrase) {
		t.Fatalf("wrong passphrase error = %v", err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	envelope["version"] = 2
	unsupported, _ := json.Marshal(envelope)
	if _, err := ParseMagpieBackup(unsupported, "right-passphrase"); !errors.Is(err, ErrMagpieVersion) {
		t.Fatalf("unsupported version error = %v", err)
	}
	damaged := append([]byte(nil), raw...)
	index := bytes.Index(damaged, []byte(`"data": "`))
	if index < 0 {
		t.Fatal("fixture envelope shape changed")
	}
	damaged[index+len(`"data": "`)] ^= 0xff
	if _, err := ParseMagpieBackup(damaged, "right-passphrase"); err == nil {
		t.Fatal("damaged ciphertext was accepted")
	}
}

func TestMagpieBackupReimportPreservesMappingsAndRequiresReplacement(t *testing.T) {
	db := openAxonHubTestDB(t)
	passphrase := "fixture-passphrase"
	first := sealMagpieForTest(t, magpieFixtureBundle(), passphrase)
	if _, err := ImportMagpieBackup(db, first, passphrase, "magpie-replace", false); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportMagpieBackup(db, first, passphrase, "magpie-replace", false); err != nil {
		t.Fatalf("identical re-import: %v", err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_channels`); got != 1 {
		t.Fatalf("re-import duplicated channels: %d", got)
	}

	bundle := magpieFixtureBundle()
	bundle["providers"] = []any{bundle["providers"].([]any)[0].(map[string]any)}
	provider := bundle["providers"].([]any)[0].(map[string]any)
	provider["models"] = []string{"gpt-6"}
	group := bundle["groups"].([]any)[0].(map[string]any)
	group["members"] = []string{"relay/gpt-6"}
	second := sealMagpieForTest(t, bundle, passphrase)
	preview, err := PreviewMagpieBackup(db, second, passphrase, "magpie-replace")
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Removals) == 0 || preview.RemovalImpact == nil {
		t.Fatalf("replacement preview has no removals: %#v", preview)
	}
	if _, err := ImportMagpieBackup(db, second, passphrase, "magpie-replace", false); !errors.Is(err, ErrMagpieReplacementRequired) {
		t.Fatalf("unconfirmed replacement error = %v", err)
	}
	if _, err := ImportMagpieBackup(db, second, passphrase, "magpie-replace", true, preview.RemovalImpact.Revision); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, db, `SELECT COUNT(*) FROM upstream_models WHERE name='claude-sonnet-5'`); got != 0 {
		t.Fatalf("removed source model survived replacement: %d", got)
	}
}

func TestMagpieBackupBlocksKeysWhoseScopeCannotBeResolved(t *testing.T) {
	db := openAxonHubTestDB(t)
	bundle := magpieFixtureBundle()
	keys := bundle["gatewayKeys"].([]any)
	key := keys[0].(map[string]any)
	key["models"] = []string{"group/missing"}
	raw := sealMagpieForTest(t, bundle, "fixture-passphrase")
	if _, err := ImportMagpieBackup(db, raw, "fixture-passphrase", "magpie-blocked", false); err != nil {
		t.Fatal(err)
	}
	var policyRaw string
	if err := db.Get(&policyRaw, db.Rebind(`SELECT access_policy FROM downstream_api_keys WHERE key=?`), "fixture-gateway-secret"); err != nil {
		t.Fatal(err)
	}
	policy, err := store.ParseDownstreamAccessPolicy(policyRaw)
	if err != nil {
		t.Fatal(err)
	}
	if policy == nil || policy.BlockReason != "source_key_model_scope_unresolved" {
		t.Fatalf("gateway key was not blocked safely: %#v", policy)
	}
}

func magpieFixtureBundle() map[string]any {
	return map[string]any{
		"version": 2,
		"created": "2026-10-20T00:00:00Z",
		"app":     "fixture-magpie",
		"keys":    true,
		"providers": []any{map[string]any{
			"id": "relay", "name": "Fixture Relay", "preset": "custom",
			"key": "fixture-provider-primary", "keyName": "primary",
			"keys": []any{map[string]any{"name": "backup", "key": "fixture-provider-backup", "weight": 3}},
			"chat": "https://relay.example/v1", "anthropic": "https://relay.example",
			"models":  []string{"gpt-6", "claude-sonnet-5"},
			"headers": map[string]string{"X-Fixture": "present"},
			"routing": "order",
		}},
		"groups": []any{map[string]any{
			"id": "coding", "name": "Coding", "members": []string{"relay/gpt-6", "relay/claude-sonnet-5"}, "routing": "order",
		}},
		"gatewayKeys": []any{map[string]any{
			"id": "gateway-1", "name": "Fixture gateway", "secret": "fixture-gateway-secret",
			"models": []string{"group/coding"},
			"limit":  map[string]any{"period": "day", "tokens": 1000},
		}},
		"settings": map[string]any{"lan": false},
		"profiles": map[string]any{"fixture": map[string]any{"name": "fixture"}},
		"agents":   map[string]string{"codex": "fixture"},
		"library":  map[string]any{"skills": []string{"fixture"}},
		"searches": []any{map[string]any{"vendor": "fixture"}},
		"icons":    map[string][]byte{"fixture.png": {1, 2, 3}},
	}
}

func sealMagpieForTest(t *testing.T, bundle map[string]any, passphrase string) []byte {
	t.Helper()
	plain, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	salt := []byte("0123456789abcdef")
	nonce := []byte("012345678901")
	const iterations = 100_000
	key, err := pbkdf2.Key(sha256.New, passphrase, salt, iterations, 32)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	aad := []byte("magpie-backup/1/pbkdf2-sha256/100000/30313233343536373839616263646566")
	sealed := gcm.Seal(nil, nonce, plain, aad)
	raw, err := json.MarshalIndent(map[string]any{
		"format": "magpie-backup", "version": 1, "kdf": "pbkdf2-sha256",
		"iterations": iterations, "salt": salt, "nonce": nonce, "data": sealed,
	}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
