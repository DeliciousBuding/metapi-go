package channelplugins

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"unicode"

	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"github.com/deliciousbuding/metapi-go/store"
)

// ApplyAuthentication is intentionally separate from Call: neither signing
// material nor the resulting private header passes through a JavaScript realm.
// Run after all other header builders; authentication overrides cannot be
// replaced by a request descriptor, wire hook, client header or private rule.
func (i DriverInvocation) ApplyAuthentication(headers http.Header, secret string) error {
	if i.program == nil {
		return ErrUnavailable
	}
	a := i.program.Manifest().Authentication
	if a == nil {
		return nil
	}
	if i.credentialKind != store.DirectCredentialAPIKey || headers == nil {
		return ErrBinding
	}
	value, err := renderAuthentication(*a, secret, time.Now())
	if err != nil {
		return err
	}
	setAuthenticationHeader(headers, *a, value)
	return nil
}

// AuthenticationLease holds at most one short-lived signed header in its
// owning downstream WS session. It is not a credential resolver or global
// token cache. Every turn must still resolve and authorize the current key.
type AuthenticationLease struct {
	identity  [32]byte
	value     string
	expiresAt time.Time
}

// ApplySessionAuthentication preserves the handshake identity across WS turns
// until expiry or a change to the current key/program. Signing anew each turn
// would change the runtime key, discard the socket and lose continuation.
func (i DriverInvocation) ApplySessionAuthentication(headers http.Header, secret string, previous *AuthenticationLease) (*AuthenticationLease, error) {
	return i.applySessionAuthentication(headers, secret, previous, time.Now())
}

func (i DriverInvocation) applySessionAuthentication(headers http.Header, secret string, previous *AuthenticationLease, now time.Time) (*AuthenticationLease, error) {
	if i.program == nil {
		return nil, ErrUnavailable
	}
	a := i.program.Manifest().Authentication
	if a == nil || a.Type != "jwt_hs256" {
		return nil, i.ApplyAuthentication(headers, secret)
	}
	if i.credentialKind != store.DirectCredentialAPIKey || headers == nil {
		return nil, ErrBinding
	}
	identity := sha256.Sum256([]byte(i.Fingerprint() + "\x00" + secret))
	if previous != nil && previous.identity == identity && now.Before(previous.expiresAt) {
		setAuthenticationHeader(headers, *a, previous.value)
		return previous, nil
	}
	value, err := renderAuthentication(*a, secret, now)
	if err != nil {
		return nil, err
	}
	// Renew before the actual JWT expiry; an idle/replaced socket may need a
	// new upgrade with this header. No expired token is reused for that dial.
	lease := &AuthenticationLease{identity: identity, value: value, expiresAt: now.Add(time.Duration(a.JWT.TTLSeconds-5) * time.Second)}
	setAuthenticationHeader(headers, *a, value)
	return lease, nil
}

func setAuthenticationHeader(headers http.Header, a contract.Authentication, value string) {
	for name := range headers {
		if strings.EqualFold(name, "Authorization") || strings.EqualFold(name, "X-Api-Key") || strings.EqualFold(name, "X-Goog-Api-Key") || strings.EqualFold(name, a.Header) {
			delete(headers, name)
		}
	}
	headers.Set(a.Header, a.Prefix+value)
}

// Query authentication is rebuilt by the host. In particular a credentialless
// query must not inherit a custom signing header from its submitting request.
func (i DriverInvocation) ClearAuthentication(headers http.Header) {
	target := ""
	if i.program != nil {
		if authentication := i.program.Manifest().Authentication; authentication != nil {
			target = authentication.Header
		}
	}
	for name := range headers {
		if strings.EqualFold(name, "Authorization") || strings.EqualFold(name, "X-Api-Key") || strings.EqualFold(name, "X-Goog-Api-Key") || target != "" && strings.EqualFold(name, target) {
			delete(headers, name)
		}
	}
}

func renderAuthentication(a contract.Authentication, material string, now time.Time) (string, error) {
	if a.Validate() != nil || material == "" || len(material) > 32<<10 {
		return "", ErrBinding
	}
	if a.Type == "api_key" {
		if strings.IndexFunc(material, unicode.IsControl) >= 0 {
			return "", ErrBinding
		}
		return material, nil
	}
	j := a.JWT
	id, secret, found := strings.Cut(material, j.KeySeparator)
	if !found || id == "" || len(id) > 256 || strings.IndexFunc(id, unicode.IsControl) >= 0 || secret == "" {
		return "", ErrBinding
	}
	clock, expires := now.Unix(), now.Add(time.Duration(j.TTLSeconds)*time.Second).Unix()
	if j.TimeUnit == "milliseconds" {
		clock, expires = now.UnixMilli(), now.Add(time.Duration(j.TTLSeconds)*time.Second).UnixMilli()
	}
	claims := map[string]any{j.IssuerClaim: id, "exp": expires}
	if j.IssuedAtClaim != "" {
		claims[j.IssuedAtClaim] = clock
	}
	if j.NotBeforeSkewSeconds != 0 {
		nbf := now.Add(-time.Duration(j.NotBeforeSkewSeconds) * time.Second)
		claims["nbf"] = nbf.Unix()
		if j.TimeUnit == "milliseconds" {
			claims["nbf"] = nbf.UnixMilli()
		}
	}
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	if j.SignType != "" {
		header["sign_type"] = j.SignType
	}
	head, err := json.Marshal(header)
	if err != nil {
		return "", ErrBinding
	}
	body, err := json.Marshal(claims)
	if err != nil {
		return "", ErrBinding
	}
	encoded := base64.RawURLEncoding.EncodeToString(head) + "." + base64.RawURLEncoding.EncodeToString(body)
	signature := hmac.New(sha256.New, []byte(secret))
	_, _ = signature.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(signature.Sum(nil)), nil
}
