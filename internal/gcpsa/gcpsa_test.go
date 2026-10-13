package gcpsa

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
	"strings"
	"testing"
	"time"
)

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
	if _, _, err := Parse(valid); err != nil {
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
			if _, _, err := Parse(raw); !errors.Is(err, ErrInvalidMaterial) {
				t.Fatalf("got %v, want ErrInvalidMaterial", err)
			}
		})
	}
}

func TestGCPAssertionSignatureAndClaims(t *testing.T) {
	key, pemText := gcpTestKey(t)
	doc, parsedKey, err := Parse(gcpTestMaterial(pemText, "https://oauth2.googleapis.com/token"))
	if err != nil {
		t.Fatal(err)
	}
	assertion, err := buildAssertion(doc, parsedKey, time.Unix(1700000000, 0))
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
	if claims["iss"] != "fixture@example.iam.gserviceaccount.com" || claims["aud"] != "https://oauth2.googleapis.com/token" || claims["scope"] != tokenScope {
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
