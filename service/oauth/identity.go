package oauth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// AccountIdentity is the subset of token-derived identity fields that the
// connection refresh path actually carries into TokenSet. metapi-go has no
// consumer for user_id or subscription expiry today (Simplicity first), so
// this struct intentionally omits them.
type AccountIdentity struct {
	Email             string
	ChatGPTAccountID string
	PlanType          string
}

// codexIDTokenClaims mirrors the OpenAI id_token payload. The auth namespace
// claim carries the ChatGPT account identity produced by the OAuth flow.
type codexIDTokenClaims struct {
	Email string `json:"email"`
	Auth  *struct {
		ChatGPTAccountID string `json:"chatgpt_account_id"`
		ChatGPTPlanType  string `json:"chatgpt_plan_type"`
	} `json:"https://api.openai.com/auth"`
}

// codexAccessTokenClaims mirrors the OpenAI access_token payload. The AT
// stores email under a different namespace (https://api.openai.com/profile)
// than the id_token's top-level email, so a dedicated parser is needed when
// the id_token lacks fields.
type codexAccessTokenClaims struct {
	Auth    *struct {
		ChatGPTAccountID string `json:"chatgpt_account_id"`
		ChatGPTPlanType  string `json:"chatgpt_plan_type"`
	} `json:"https://api.openai.com/auth"`
	Profile *struct {
		Email string `json:"email"`
	} `json:"https://api.openai.com/profile"`
}

// ParseCodexIDToken extracts the id_token identity fields without verifying
// the signature. Returns a non-nil identity with empty fields when the token
// is malformed so callers can safely chain MergeCodexIdentity.
func ParseCodexIDToken(idToken string) *AccountIdentity {
	identity := &AccountIdentity{}
	if idToken == "" {
		return identity
	}
	payload, err := decodeJWTPayload(idToken)
	if err != nil {
		return identity
	}
	var claims codexIDTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return identity
	}
	identity.Email = strings.TrimSpace(claims.Email)
	if claims.Auth != nil {
		identity.ChatGPTAccountID = strings.TrimSpace(claims.Auth.ChatGPTAccountID)
		identity.PlanType = strings.TrimSpace(claims.Auth.ChatGPTPlanType)
	}
	return identity
}

// ParseCodexAccessToken extracts identity from the access_token JWT payload.
// The AT surfaces email under https://api.openai.com/profile rather than the
// id_token's top-level email, so this parser is the only way to recover the
// email when the id_token omitted it. Returns nil for malformed tokens so
// callers can distinguish "no AT available" from "empty identity".
func ParseCodexAccessToken(accessToken string) *AccountIdentity {
	if accessToken == "" {
		return nil
	}
	payload, err := decodeJWTPayload(accessToken)
	if err != nil {
		return nil
	}
	var claims codexAccessTokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}
	identity := &AccountIdentity{}
	if claims.Profile != nil {
		identity.Email = strings.TrimSpace(claims.Profile.Email)
	}
	if claims.Auth != nil {
		identity.ChatGPTAccountID = strings.TrimSpace(claims.Auth.ChatGPTAccountID)
		identity.PlanType = strings.TrimSpace(claims.Auth.ChatGPTPlanType)
	}
	return identity
}

// MergeCodexIdentity returns a copy of primary with any empty field filled
// from fallback. fallback may be nil (no-op). This realizes the codex2api
// behavior where the access_token recovers fields the id_token omitted.
func MergeCodexIdentity(primary, fallback *AccountIdentity) *AccountIdentity {
	merged := &AccountIdentity{}
	if primary != nil {
		*merged = *primary
	}
	if fallback == nil {
		return merged
	}
	if strings.TrimSpace(merged.Email) == "" {
		merged.Email = fallback.Email
	}
	if strings.TrimSpace(merged.ChatGPTAccountID) == "" {
		merged.ChatGPTAccountID = fallback.ChatGPTAccountID
	}
	if strings.TrimSpace(merged.PlanType) == "" {
		merged.PlanType = fallback.PlanType
	}
	return merged
}

// identityIncomplete reports whether any field a TokenSet consumer relies on
// is missing from identity, signaling that an access_token fallback is worth
// attempting.
func identityIncomplete(identity *AccountIdentity) bool {
	if identity == nil {
		return true
	}
	return strings.TrimSpace(identity.Email) == "" ||
		strings.TrimSpace(identity.ChatGPTAccountID) == "" ||
		strings.TrimSpace(identity.PlanType) == ""
}

// decodeJWTPayload base64-decodes the payload segment of a JWT without
// verifying the signature. Tolerates URL-safe and standard base64 plus
// missing padding.
func decodeJWTPayload(token string) ([]byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT: expected 3 segments")
	}
	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return nil, fmt.Errorf("invalid JWT payload: %w", err)
		}
	}
	return decoded, nil
}
