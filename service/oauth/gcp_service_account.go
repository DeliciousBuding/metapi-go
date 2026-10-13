package oauth

import (
	"context"
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
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deliciousbuding/metapi-go/internal/httpclient"
	"github.com/deliciousbuding/metapi-go/internal/ssrf"
)

// GCP service-account credential kind (route-decision F batch).
//
// Durable material is the Google service-account JSON key document
// (client_email / private_key / token_uri), stored in
// upstream_credentials.gcp_service_account. The exchanged access token is
// cached in upstream_credentials.secret with its expiry in oauth_state — the
// single-owner token contract is unchanged. The private key is used in memory
// for JWT signing only: never logged, never returned by management
// projections, never written into route caches.

const (
	gcpTokenExchangeTimeout  = 15 * time.Second
	gcpTokenExchangeMaxBytes = 1 << 20
	gcpTokenScope            = "https://www.googleapis.com/auth/cloud-platform"
	gcpTokenSkew             = 3 * time.Minute
	gcpAssertionLifetime     = time.Hour
)

var (
	ErrGCPServiceAccountInvalid = errors.New("invalid GCP service account material")
	ErrGCPTokenExchange         = errors.New("GCP token exchange failed")
	ErrGCPTokenRejected         = errors.New("GCP token exchange rejected the service account material")
)

// gcpServiceAccountMaterial is the subset of the Google service-account key
// document this credential kind uses.
type gcpServiceAccountMaterial struct {
	ClientEmail  string `json:"client_email"`
	PrivateKey   string `json:"private_key"`
	PrivateKeyID string `json:"private_key_id"`
	TokenURI     string `json:"token_uri"`
}

// parseGCPServiceAccount validates the SA JSON document: required fields, a
// well-formed token URI, and a parseable PKCS8 RSA private key. Returns the
// material plus the parsed key so callers validate exactly what they sign
// with. The input is never included in errors.
func parseGCPServiceAccount(raw string) (*gcpServiceAccountMaterial, *rsa.PrivateKey, error) {
	var doc gcpServiceAccountMaterial
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") || json.Unmarshal([]byte(trimmed), &doc) != nil {
		return nil, nil, ErrGCPServiceAccountInvalid
	}
	doc.ClientEmail = strings.TrimSpace(doc.ClientEmail)
	doc.TokenURI = strings.TrimSpace(doc.TokenURI)
	if doc.ClientEmail == "" || doc.TokenURI == "" || doc.PrivateKey == "" {
		return nil, nil, ErrGCPServiceAccountInvalid
	}
	u, err := url.Parse(doc.TokenURI)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || ssrf.IsForbiddenSiteHostname(u.Hostname()) {
		return nil, nil, ErrGCPServiceAccountInvalid
	}
	// Google-issued key files always carry https. Plain http stays possible
	// only for loopback so tests can point the document at httptest servers.
	if u.Scheme != "https" {
		host := u.Hostname()
		loopback := host == "localhost"
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			loopback = true
		}
		if u.Scheme != "http" || !loopback {
			return nil, nil, ErrGCPServiceAccountInvalid
		}
	}
	key, err := parseGCPRSAPrivateKey(doc.PrivateKey)
	if err != nil {
		return nil, nil, ErrGCPServiceAccountInvalid
	}
	return &doc, key, nil
}

func parseGCPRSAPrivateKey(pemText string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, fmt.Errorf("no PEM block")
	}
	if block.Type == "RSA PRIVATE KEY" { // PKCS#1
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes) // Google key files are PKCS#8
	if err != nil {
		return nil, err
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("not an RSA key")
	}
	return rsaKey, rsaKey.Validate()
}

// buildGCPAssertion signs a JWT bearer assertion (RFC 7523, Google flavor).
func buildGCPAssertion(doc *gcpServiceAccountMaterial, key *rsa.PrivateKey, now time.Time) (string, error) {
	header := map[string]string{"alg": "RS256", "typ": "JWT"}
	if doc.PrivateKeyID != "" {
		header["kid"] = doc.PrivateKeyID
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claims := map[string]any{
		"iss":   doc.ClientEmail,
		"scope": gcpTokenScope,
		"aud":   doc.TokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(gcpAssertionLifetime).Unix(),
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	encode := base64.RawURLEncoding.EncodeToString
	signingInput := encode(headerJSON) + "." + encode(claimsJSON)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + encode(signature), nil
}

type gcpTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// exchangeGCPServiceAccount trades the SA assertion for an access token at the
// document's token_uri. Returns the token and its absolute expiry (Unix ms).
func exchangeGCPServiceAccount(ctx context.Context, rawMaterial string, proxyURL *string) (string, int64, error) {
	doc, key, err := parseGCPServiceAccount(rawMaterial)
	if err != nil {
		return "", 0, err
	}
	assertion, err := buildGCPAssertion(doc, key, time.Now())
	if err != nil {
		return "", 0, ErrGCPTokenExchange
	}

	proxy := httpclient.NoProxy
	if proxyURL != nil && *proxyURL != "" {
		parsed, parseErr := url.Parse(*proxyURL)
		if parseErr != nil || parsed.Host == "" || parsed.RawQuery != "" || parsed.Fragment != "" || ssrf.IsForbiddenSiteHostname(parsed.Hostname()) || (parsed.Scheme != "http" && parsed.Scheme != "https" && parsed.Scheme != "socks5" && parsed.Scheme != "socks5h") {
			return "", 0, ErrGCPTokenExchange
		}
		proxy = http.ProxyURL(parsed)
	}
	transport := httpclient.NewTransport(httpclient.Options{Proxy: proxy, SiteDialGuard: true, DialTimeout: 5 * time.Second, ResponseHeaderTimeout: gcpTokenExchangeTimeout})
	defer transport.CloseIdleConnections()

	callCtx, cancel := context.WithTimeout(ctx, gcpTokenExchangeTimeout)
	defer cancel()
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	// The token endpoint is one exact URL from the credential document; never
	// follow redirects with a signed assertion in hand.
	client := httpclient.NewClient(transport, gcpTokenExchangeTimeout, func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse })
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, doc.TokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, ErrGCPTokenExchange
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", 0, ErrGCPTokenExchange
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, gcpTokenExchangeMaxBytes))
	if err != nil {
		return "", 0, ErrGCPTokenExchange
	}
	if resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		// The material itself was rejected (invalid_grant / deleted key). The
		// response body can echo assertion details; never surface it.
		return "", 0, ErrGCPTokenRejected
	}
	if resp.StatusCode != http.StatusOK {
		return "", 0, ErrGCPTokenExchange
	}
	var token gcpTokenResponse
	if json.Unmarshal(body, &token) != nil || strings.TrimSpace(token.AccessToken) == "" || token.ExpiresIn <= 0 || !strings.EqualFold(token.TokenType, "Bearer") {
		return "", 0, ErrGCPTokenExchange
	}
	return token.AccessToken, time.Now().Add(time.Duration(token.ExpiresIn) * time.Second).UnixMilli(), nil
}
