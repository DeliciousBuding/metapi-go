package contract

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// Authentication describes host-owned credential presentation, never an
// executable hook. Secret material is supplied by the current resolver only.
type Authentication struct {
	Type   string             `json:"type"`
	Header string             `json:"header"`
	Prefix string             `json:"prefix,omitempty"`
	JWT    *JWTAuthentication `json:"jwt,omitempty"`
}

type JWTAuthentication struct {
	KeySeparator         string `json:"keySeparator"`
	IssuerClaim          string `json:"issuerClaim"`
	TTLSeconds           int64  `json:"ttlSeconds"`
	TimeUnit             string `json:"timeUnit"`
	IssuedAtClaim        string `json:"issuedAtClaim,omitempty"`
	NotBeforeSkewSeconds int64  `json:"notBeforeSkewSeconds,omitempty"`
	SignType             string `json:"signType,omitempty"`
}

var authenticationHeader = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")
var authenticationClaim = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]{0,63}$`)

func (a Authentication) Validate() error {
	invalid := fmt.Errorf("invalid_provider_authentication")
	name := strings.ToLower(a.Header)
	if len(a.Header) == 0 || len(a.Header) > 128 || !authenticationHeader.MatchString(a.Header) || len(a.Prefix) > 256 || strings.IndexFunc(a.Prefix, unicode.IsControl) >= 0 {
		return invalid
	}
	if strings.HasPrefix(name, "sec-") || strings.HasPrefix(name, "proxy-") || strings.HasPrefix(name, "x-forwarded-") || strings.HasPrefix(name, "x-codex-") || strings.HasPrefix(name, "x-claude-code-") {
		return invalid
	}
	switch name {
	case "host", "cookie", "set-cookie", "connection", "transfer-encoding", "content-type", "content-length", "content-encoding", "accept-encoding", "accept", "te", "trailer", "upgrade", "forwarded", "x-real-ip":
		return invalid
	case "session-id", "conversation_id", "conversation-id", "thread-id", "chatgpt-account-id", "anthropic-version", "anthropic-beta", "openai-beta", "openai-organization", "openai-project", "x-request-id", "x-client-request-id", "request-id", "traceparent", "tracestate", "baggage":
		return invalid
	}
	if a.Type == "api_key" {
		if a.JWT != nil {
			return invalid
		}
		return nil
	}
	if a.Type != "jwt_hs256" || a.JWT == nil {
		return invalid
	}
	j := a.JWT
	if j.KeySeparator != "." && j.KeySeparator != "|" || j.TTLSeconds < 60 || j.TTLSeconds > 86400 || j.TimeUnit != "seconds" && j.TimeUnit != "milliseconds" || j.NotBeforeSkewSeconds < 0 || j.NotBeforeSkewSeconds > 300 || j.SignType != "" && j.SignType != "SIGN" {
		return invalid
	}
	if !authenticationClaim.MatchString(j.IssuerClaim) || reservedAuthenticationClaim(j.IssuerClaim) {
		return invalid
	}
	if j.IssuedAtClaim != "" && (!authenticationClaim.MatchString(j.IssuedAtClaim) || j.IssuedAtClaim == j.IssuerClaim || j.IssuedAtClaim == "exp" || j.IssuedAtClaim == "nbf") {
		return invalid
	}
	return nil
}

func reservedAuthenticationClaim(name string) bool {
	return name == "exp" || name == "iat" || name == "nbf"
}
