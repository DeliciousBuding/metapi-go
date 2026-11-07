package channelplugins

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"github.com/deliciousbuding/metapi-go/store"
)

func TestHostAuthenticationIsFinalAndAbsentDeclarationsAreNoOp(t *testing.T) {
	const source = `export const meta={apiVersion:2,kind:"provider",key:"auth",name:"Auth",version:"1.0.0",protocols:["chat"],credentialKinds:["api_key"],hooks:["request","response"],authentication:{type:"api_key",header:"X-Vendor-Key",prefix:"Key "}};export function request(i){return {method:"POST",body:i.body};}export function response(i){return {body:i.body};}`
	p, err := channelplugin.Compile(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	i := DriverInvocation{program: p, credentialKind: store.DirectCredentialAPIKey}
	headers := http.Header{}
	for _, name := range []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key", "X-Vendor-Key"} {
		headers.Set(name, "fixture-spoof")
	}
	// Bypass Header's canonical-key convention deliberately: the host must
	// remove duplicate raw-map keys too, not only normally built headers.
	rawHeaders := map[string][]string(headers)
	rawHeaders["authorization"] = []string{"fixture-lowercase-spoof"}
	rawHeaders["x-vendor-key"] = []string{"fixture-lowercase-spoof"}
	headers.Set("X-Request-Id", "fixture-trace")
	if err := i.ApplyAuthentication(headers, "fixture-current-key"); err != nil {
		t.Fatal(err)
	}
	if headers.Get("X-Vendor-Key") != "Key fixture-current-key" || headers.Get("Authorization") != "" || headers.Get("X-Api-Key") != "" || headers.Get("X-Goog-Api-Key") != "" || headers.Get("X-Request-Id") != "fixture-trace" {
		t.Fatal("host authentication was shadowed or sent multiple credentials")
	}
	if _, exists := rawHeaders["authorization"]; exists {
		t.Fatal("duplicate lowercase authentication survived")
	}
	if _, exists := rawHeaders["x-vendor-key"]; exists {
		t.Fatal("duplicate lowercase target authentication survived")
	}
	i.credentialKind = store.DirectCredentialOAuth
	if err := i.ApplyAuthentication(headers, "fixture-oauth"); !errors.Is(err, ErrBinding) {
		t.Fatal("provider signer accepted OAuth material")
	}
	p, err = channelplugin.Compile(t.Context(), strings.Replace(source, `,authentication:{type:"api_key",header:"X-Vendor-Key",prefix:"Key "}`, "", 1))
	if err != nil {
		t.Fatal(err)
	}
	i = DriverInvocation{program: p, credentialKind: store.DirectCredentialAPIKey}
	if err := i.ApplyAuthentication(headers, "unused-fixture-key"); err != nil || headers.Get("X-Vendor-Key") != "Key fixture-current-key" {
		t.Fatal("undeclared authentication changed existing headers")
	}
}

func TestHostAuthenticationSignsKlingAndZhipuShapesWithoutExposingMaterial(t *testing.T) {
	now := time.Unix(1700000000, 123000000)
	for _, tc := range []struct {
		name, separator, claim, unit, issued, signType string
		ttl, skew                                      int64
	}{
		{"kling", "|", "iss", "seconds", "", "", 1800, 5},
		{"zhipu", ".", "api_key", "milliseconds", "timestamp", "SIGN", 86400, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := contract.Authentication{Type: "jwt_hs256", Header: "Authorization", Prefix: "Bearer ", JWT: &contract.JWTAuthentication{KeySeparator: tc.separator, IssuerClaim: tc.claim, TTLSeconds: tc.ttl, TimeUnit: tc.unit, IssuedAtClaim: tc.issued, NotBeforeSkewSeconds: tc.skew, SignType: tc.signType}}
			secret := "fixture-host-signing-material"
			value, err := renderAuthentication(a, "fixture-key-id"+tc.separator+secret, now)
			if err != nil || strings.Contains(value, secret) {
				t.Fatal("host signer failed or exposed material")
			}
			parts := strings.Split(value, ".")
			if len(parts) != 3 {
				t.Fatal("invalid JWT framing")
			}
			mac := hmac.New(sha256.New, []byte(secret))
			_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
			sig, err := base64.RawURLEncoding.DecodeString(parts[2])
			if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
				t.Fatal("signature mismatch")
			}
			rawHead, _ := base64.RawURLEncoding.DecodeString(parts[0])
			rawBody, _ := base64.RawURLEncoding.DecodeString(parts[1])
			var head map[string]string
			var claims map[string]json.RawMessage
			if json.Unmarshal(rawHead, &head) != nil || json.Unmarshal(rawBody, &claims) != nil || head["alg"] != "HS256" || string(claims[tc.claim]) != `"fixture-key-id"` {
				t.Fatal("wrong algorithm/issuer")
			}
			var expiry int64
			_ = json.Unmarshal(claims["exp"], &expiry)
			want := now.Add(time.Duration(tc.ttl) * time.Second).Unix()
			if tc.unit == "milliseconds" {
				want = now.Add(time.Duration(tc.ttl) * time.Second).UnixMilli()
			}
			if expiry != want || head["sign_type"] != tc.signType {
				t.Fatal("wrong clock/TTL/header")
			}
			if tc.skew > 0 && string(claims["nbf"]) != "1699999995" {
				t.Fatal("wrong not-before skew")
			}
			if tc.issued != "" && string(claims[tc.issued]) != "1700000000123" {
				t.Fatal("wrong issued-at unit")
			}
		})
	}
}

func TestHostAuthenticationRejectsInvalidMaterialWithoutEcho(t *testing.T) {
	a := contract.Authentication{Type: "jwt_hs256", Header: "Authorization", JWT: &contract.JWTAuthentication{KeySeparator: "|", IssuerClaim: "iss", TTLSeconds: 1800, TimeUnit: "seconds"}}
	for _, material := range []string{"", "private-fixture", "|private-fixture", "id|", "id\n|private-fixture", strings.Repeat("x", 33<<10)} {
		value, err := renderAuthentication(a, material, time.Now())
		if value != "" || !errors.Is(err, ErrBinding) || strings.Contains(err.Error(), "private-fixture") {
			t.Fatal("invalid material accepted or leaked")
		}
	}
	api := contract.Authentication{Type: "api_key", Header: "X-Vendor-Key", Prefix: "Key "}
	if value, err := renderAuthentication(api, "fixture-key", time.Now()); err != nil || value != "fixture-key" {
		t.Fatal("custom API Key header lost value")
	}
}

func TestHostAuthenticationWSLeaseExpiresAndFollowsCurrentKeyAndProgram(t *testing.T) {
	const source = `export const meta={apiVersion:2,kind:"provider",key:"auth",name:"Auth",version:"1.0.0",protocols:["responsesWebsocket"],credentialKinds:["api_key"],hooks:["request","event"],authentication:{type:"jwt_hs256",header:"Authorization",prefix:"Bearer ",jwt:{keySeparator:"|",issuerClaim:"iss",ttlSeconds:60,timeUnit:"seconds"}}};export function request(i){return {body:i.body};}export function event(i){return {body:i.body};}`
	p, err := channelplugin.Compile(t.Context(), source)
	if err != nil {
		t.Fatal(err)
	}
	i := DriverInvocation{program: p, credentialKind: store.DirectCredentialAPIKey}
	now := time.Unix(1700000000, 0)
	first := http.Header{}
	lease, err := i.applySessionAuthentication(first, "id|fixture-secret", nil, now)
	if err != nil || lease == nil {
		t.Fatal("missing WS lease")
	}
	second := http.Header{}
	kept, err := i.applySessionAuthentication(second, "id|fixture-secret", lease, now.Add(time.Second))
	if err != nil || kept != lease || first.Get("Authorization") != second.Get("Authorization") {
		t.Fatal("signing clock discarded WS identity")
	}
	for _, tc := range []struct {
		key   string
		clock time.Time
	}{
		{"id|fixture-secret", now.Add(55 * time.Second)},
		{"id|rotated-fixture-secret", now.Add(time.Second)},
	} {
		header := http.Header{}
		changed, err := i.applySessionAuthentication(header, tc.key, lease, tc.clock)
		if err != nil || changed == lease || first.Get("Authorization") == header.Get("Authorization") {
			t.Fatal("expired/rotated identity reused WS authentication")
		}
	}
	p, err = channelplugin.Compile(t.Context(), strings.Replace(source, `ttlSeconds:60`, `ttlSeconds:120`, 1))
	if err != nil {
		t.Fatal(err)
	}
	i.program = p
	header := http.Header{}
	changed, err := i.applySessionAuthentication(header, "id|fixture-secret", lease, now.Add(time.Second))
	if err != nil || changed == lease || first.Get("Authorization") == header.Get("Authorization") {
		t.Fatal("upgraded authentication reused old WS identity")
	}
	i.credentialKind = store.DirectCredentialOAuth
	if changed, err := i.applySessionAuthentication(http.Header{}, "id|fixture-secret", lease, now); !errors.Is(err, ErrBinding) || changed != nil {
		t.Fatal("WS lease bypassed current credential kind")
	}
}
