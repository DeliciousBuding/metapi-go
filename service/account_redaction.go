package service

import (
	"encoding/json"
	"strings"
)

// RedactAccountSecrets removes plaintext credentials from an admin account
// response map, setting accessTokenMasked / apiTokenMasked when a value was
// present, and stripping every credential-shaped field from nested config.
//
// This is the single credential-policy SSOT for account surfaces. Both the
// list/overview rows (buildAccountMap), search rows, and the write responses
// (PUT /api/accounts/{id}, POST /api/accounts/{id}/rebind-session) answer
// through it, so a no-op write can never harvest what the list surface
// deliberately withholds. Revealing a secret stays an explicit, separately
// routed act (GET /api/account-tokens/{id}/value, downstream-key export), and
// the credential-provisioning responses that hand back a secret the caller did
// not supply (password login, verify-token discovery) build their own maps.
//
// extraConfig carries credentials in several shapes: oauth.refreshToken /
// idToken / sessionToken / accessToken, sub2apiAuth.refreshToken, and
// autoRelogin.passwordCipher. Instead of enumerating today's field names, the
// nested walker drops every key whose normalized name contains token, secret,
// cipher, password, or apikey. Display metadata (oauth provider/email/plan,
// tokenExpiresAt, account names, proxyUrl, ...) survives because numeric and
// boolean values are kept. Masked projections are kept by their suffix.
//
// Values arrive either scanned from the DB (string / []byte / nil) or built by
// a handler from a typed struct row (*string), so secretValue() normalizes both
// instead of letting fmt.Sprint render a pointer address as the "secret".
func RedactAccountSecrets(account map[string]any) {
	if account == nil {
		return
	}
	if v := secretValue(account["accessToken"]); v != "" {
		account["accessTokenMasked"] = maskSecretTail(v)
	}
	delete(account, "accessToken")
	if v := secretValue(account["apiToken"]); v != "" {
		account["apiTokenMasked"] = maskSecretTail(v)
	}
	delete(account, "apiToken")
	// DB scans may hand extraConfig back as []byte; normalize it so the
	// nested walker can parse and redact the JSON object.
	if raw, ok := account["extraConfig"].([]byte); ok {
		account["extraConfig"] = string(raw)
	}
	redactNestedCredentials(account)
}

// redactNestedCredentials recursively strips credential-shaped fields from a
// decoded response object. It reports whether anything was removed so a JSON
// config string is only re-encoded when redaction actually changed it
// (preserving the stored formatting otherwise).
func redactNestedCredentials(m map[string]any) bool {
	changed := false
	for key, value := range m {
		if isCredentialFieldName(key) && !isMetadataScalar(value) {
			delete(m, key)
			changed = true
			continue
		}
		// extraConfig is persisted as a JSON string but typed rows answer with
		// *string and DB scans may hand back []byte. Normalize all three and
		// redact the nested credential keys in place, re-encoding only when
		// something was removed (preserving the stored formatting otherwise).
		if isExtraConfigFieldName(key) {
			switch typed := value.(type) {
			case string:
				if redacted, ok := redactCredentialJSONString(typed); ok {
					m[key] = redacted
					changed = true
				}
				continue
			case *string:
				if typed != nil {
					if redacted, ok := redactCredentialJSONString(*typed); ok {
						m[key] = redacted
						changed = true
					}
				}
				continue
			case []byte:
				if redacted, ok := redactCredentialJSONString(string(typed)); ok {
					m[key] = redacted
					changed = true
				}
				continue
			}
		}
		switch typed := value.(type) {
		case map[string]any:
			if redactNestedCredentials(typed) {
				changed = true
			}
		case []any:
			if redactNestedCredentialSlice(typed) {
				changed = true
			}
		}
	}
	return changed
}

func redactNestedCredentialSlice(values []any) bool {
	changed := false
	for _, value := range values {
		switch typed := value.(type) {
		case map[string]any:
			if redactNestedCredentials(typed) {
				changed = true
			}
		case []any:
			if redactNestedCredentialSlice(typed) {
				changed = true
			}
		}
	}
	return changed
}

// redactCredentialJSONString redacts a JSON object/array string and returns the
// re-encoded value plus true when at least one credential key was removed.
func redactCredentialJSONString(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || (trimmed[0] != '{' && trimmed[0] != '[') {
		return raw, false
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return raw, false
	}
	var changed bool
	switch typed := decoded.(type) {
	case map[string]any:
		changed = redactNestedCredentials(typed)
	case []any:
		changed = redactNestedCredentialSlice(typed)
	}
	if !changed {
		return raw, false
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return raw, false
	}
	return string(encoded), true
}

// isCredentialFieldName matches the status quo credential vocabulary across
// snake_case and camelCase. Masked projections (accessTokenMasked) are not
// credentials and must remain visible.
func isCredentialFieldName(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
	if normalized == "" || strings.HasSuffix(normalized, "masked") {
		return false
	}
	for _, fragment := range []string{"token", "secret", "cipher", "password", "apikey"} {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

// isMetadataScalar keeps non-string scalars (tokenExpiresAt, tokenCount,
// hasApiKey, ...) so stripping credentials does not erase display metadata.
func isMetadataScalar(value any) bool {
	switch value.(type) {
	case bool, float64, float32,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		json.Number:
		return true
	}
	return false
}

func isExtraConfigFieldName(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", " ", "").Replace(key))
	return normalized == "extraconfig"
}

// secretValue reads a credential-bearing response value that may be a string,
// a []byte from a DB scan, a *string from a typed struct row, or nil. Empty
// (including a nil pointer) normalizes to "" so callers mask nothing.
func secretValue(v any) string {
	switch typed := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case *string:
		if typed == nil {
			return ""
		}
		return strings.TrimSpace(*typed)
	case []byte:
		return strings.TrimSpace(string(typed))
	default:
		return strings.TrimSpace(asString(v))
	}
}

func maskSecretTail(secret string) string {
	s := strings.TrimSpace(secret)
	if s == "" {
		return ""
	}
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}
