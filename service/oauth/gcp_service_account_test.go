package oauth

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/store"
)

// GCP service-account credential contract (route-decision F batch): material
// validation, JWT bearer assertion shape, token exchange, caching, and the
// redaction boundary (material and tokens never leave management surfaces).

func gcpTestKey(t *testing.T) (*rsa.PrivateKey, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	return key, pemText
}

func gcpTestMaterial(pemText, tokenURI string) string {
	doc := map[string]string{
		"type":           "service_account",
		"client_email":   "fixture@example.iam.gserviceaccount.com",
		"private_key":    pemText,
		"private_key_id": "key-1",
		"token_uri":      tokenURI,
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func TestGCPServiceAccountValidation(t *testing.T) {
	_, pemText := gcpTestKey(t)
	valid := gcpTestMaterial(pemText, "https://oauth2.googleapis.com/token")
	if _, _, err := parseGCPServiceAccount(valid); err != nil {
		t.Fatalf("valid material rejected: %v", err)
	}

	cases := map[string]string{
		"empty":          "",
		"not json":       "sk-not-json",
		"missing email":  `{"private_key":"x","token_uri":"https://oauth2.googleapis.com/token"}`,
		"http token uri": gcpTestMaterial(pemText, "http://oauth2.googleapis.com/token"),
		"query in uri":   gcpTestMaterial(pemText, "https://oauth2.googleapis.com/token?x=1"),
		"bad pem":        gcpTestMaterial("not-a-pem", "https://oauth2.googleapis.com/token"),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, err := parseGCPServiceAccount(raw); !errors.Is(err, ErrGCPServiceAccountInvalid) {
				t.Fatalf("got %v, want ErrGCPServiceAccountInvalid", err)
			}
		})
	}
}

func TestGCPAssertionSignatureAndClaims(t *testing.T) {
	key, pemText := gcpTestKey(t)
	doc, parsedKey, err := parseGCPServiceAccount(gcpTestMaterial(pemText, "https://oauth2.googleapis.com/token"))
	if err != nil {
		t.Fatal(err)
	}
	assertion, err := buildGCPAssertion(doc, parsedKey, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		t.Fatalf("assertion segments: %d", len(parts))
	}
	var header, claims map[string]any
	if json.Unmarshal(mustBase64URL(t, parts[0]), &header) != nil || json.Unmarshal(mustBase64URL(t, parts[1]), &claims) != nil {
		t.Fatal("assertion segments must be base64url JSON")
	}
	if header["alg"] != "RS256" || header["kid"] != "key-1" {
		t.Fatalf("header: %#v", header)
	}
	if claims["iss"] != "fixture@example.iam.gserviceaccount.com" || claims["aud"] != "https://oauth2.googleapis.com/token" || claims["scope"] != gcpTokenScope {
		t.Fatalf("claims: %#v", claims)
	}
	if claims["exp"].(float64)-claims["iat"].(float64) != 3600 {
		t.Fatalf("lifetime: %#v", claims)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	signature := mustBase64URL(t, parts[2])
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], signature); err != nil {
		t.Fatalf("signature does not verify: %v", err)
	}
}

func mustBase64URL(t *testing.T, segment string) []byte {
	t.Helper()
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		t.Fatalf("base64url: %v", err)
	}
	return raw
}

func TestGCPCredentialResolveExchangeAndCache(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}

	key, pemText := gcpTestKey(t)
	var exchanges atomic.Int32
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exchanges.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Errorf("form: %v", err)
		}
		if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
			t.Errorf("grant_type: %q", r.Form.Get("grant_type"))
		}
		// The assertion must actually verify against the fixture public key —
		// a transport-level check that signing wired the right material.
		parts := strings.Split(r.Form.Get("assertion"), ".")
		if len(parts) != 3 {
			t.Errorf("assertion segments: %d", len(parts))
			w.WriteHeader(400)
			return
		}
		digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		sig, sigErr := base64.RawURLEncoding.DecodeString(parts[2])
		if sigErr != nil || rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, digest[:], sig) != nil {
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"access_token":"fixture-gcp-token","expires_in":3600,"token_type":"Bearer"}`)
	}))
	t.Cleanup(tokenServer.Close)

	// Channel + gcp credential through the real management owner.
	if _, err := db.Exec(`INSERT INTO upstream_channels(id,origin_key,source_id,name,dialect,provider,enabled,base_url,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex) VALUES(1,'fixture',1,'vertex-fixture','generic','gemini',1,'https://region-aiplatform.googleapis.com','/v1/chat/completions','/v1/responses','/v1/messages',0,'','[]','','')`); err != nil {
		t.Fatal(err)
	}
	created, err := CreateDirectCredential(t.Context(), db.DB, 1, DirectCredentialCreate{
		Name:              "GCP SA",
		Enabled:           true,
		Kind:              store.DirectCredentialGCPServiceAccount,
		GCPServiceAccount: ptrString(gcpTestMaterial(pemText, tokenServer.URL)),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Management projection must not leak material or any token.
	summaries, err := ListDirectCredentials(t.Context(), db.DB, 1)
	if err != nil || len(summaries) != 1 {
		t.Fatalf("list: %v %#v", err, summaries)
	}
	raw, _ := json.Marshal(summaries[0])
	if strings.Contains(string(raw), "fixture@example") || strings.Contains(string(raw), "PRIVATE KEY") {
		t.Fatalf("management projection leaks material: %s", raw)
	}

	got, err := ResolveDirectCredential(t.Context(), db.DB, created.ID, nil, false)
	if err != nil || got == nil || got.AccessToken != "fixture-gcp-token" {
		t.Fatalf("first resolve: %v %#v", err, got)
	}
	if exchanges.Load() != 1 {
		t.Fatalf("first resolve must exchange once, got %d", exchanges.Load())
	}

	// Cached token within freshness window: no second exchange.
	again, err := ResolveDirectCredential(t.Context(), db.DB, created.ID, nil, false)
	if err != nil || again.AccessToken != "fixture-gcp-token" || exchanges.Load() != 1 {
		t.Fatalf("cached resolve: %v %#v exchanges=%d", err, again, exchanges.Load())
	}

	// Force bypasses the cache.
	if _, err := ResolveDirectCredential(t.Context(), db.DB, created.ID, nil, true); err != nil || exchanges.Load() != 2 {
		t.Fatalf("force resolve: %v exchanges=%d", err, exchanges.Load())
	}
}

func TestGCPCredentialRejectedMaterial(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	_, pemText := gcpTestKey(t)
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400) // invalid_grant
		fmt.Fprint(w, `{"error":"invalid_grant"}`)
	}))
	t.Cleanup(tokenServer.Close)

	if _, err := db.Exec(`INSERT INTO upstream_channels(id,origin_key,source_id,name,dialect,provider,enabled,base_url,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex) VALUES(1,'fixture',1,'vertex-fixture','generic','gemini',1,'https://region-aiplatform.googleapis.com','/v1/chat/completions','/v1/responses','/v1/messages',0,'','[]','','')`); err != nil {
		t.Fatal(err)
	}
	created, err := CreateDirectCredential(t.Context(), db.DB, 1, DirectCredentialCreate{
		Name:              "GCP SA",
		Enabled:           true,
		Kind:              store.DirectCredentialGCPServiceAccount,
		GCPServiceAccount: ptrString(gcpTestMaterial(pemText, tokenServer.URL)),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	_, err = ResolveDirectCredential(t.Context(), db.DB, created.ID, nil, false)
	if !errors.Is(err, ErrDirectCredentialRevoked) {
		t.Fatalf("rejected material: got %v, want ErrDirectCredentialRevoked", err)
	}
}

func TestGCPCredentialKindConfusionRejected(t *testing.T) {
	db, err := store.Open(store.DialectSQLite, ":memory:", false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := store.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO upstream_channels(id,origin_key,source_id,name,dialect,provider,enabled,base_url,openai_chat_completion_path,openai_response_path,anthropic_message_path,proxy,channel_proxy,custom_header,param_override,match_regex) VALUES(1,'fixture',1,'c','generic','gemini',1,'https://x.invalid','/v1/chat/completions','/v1/responses','/v1/messages',0,'','[]','','')`); err != nil {
		t.Fatal(err)
	}
	_, pemText := gcpTestKey(t)
	material := gcpTestMaterial(pemText, "https://oauth2.googleapis.com/token")

	// gcp kind without material field, material on the wrong kind, and both
	// key+material set are all invalid.
	if _, err := CreateDirectCredential(t.Context(), db.DB, 1, DirectCredentialCreate{Name: "a", Kind: store.DirectCredentialGCPServiceAccount}); !errors.Is(err, ErrInvalidDirectCredential) {
		t.Fatalf("gcp without material: %v", err)
	}
	if _, err := CreateDirectCredential(t.Context(), db.DB, 1, DirectCredentialCreate{Name: "a", Kind: store.DirectCredentialAPIKey, GCPServiceAccount: &material}); !errors.Is(err, ErrInvalidDirectCredential) {
		t.Fatalf("material on api_key kind: %v", err)
	}
	key := "sk-other"
	if _, err := CreateDirectCredential(t.Context(), db.DB, 1, DirectCredentialCreate{Name: "a", Kind: store.DirectCredentialGCPServiceAccount, APIKey: &key, GCPServiceAccount: &material}); !errors.Is(err, ErrInvalidDirectCredential) {
		t.Fatalf("key+material both set: %v", err)
	}
}

func ptrString(s string) *string { return &s }
