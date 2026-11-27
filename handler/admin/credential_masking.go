package admin

// maskSecretFromFragments rebuilds the exact maskSecret() output (first4 +
// "****" + last4, collapsing to "****" for secrets <= 8 chars, "" for empty)
// from the prefix/suffix/length fragments selected by store.CredentialFragmentsSelect.
// The plaintext secret never enters Go memory.
func maskSecretFromFragments(prefix, suffix any, length int64) string {
	if length <= 0 {
		return ""
	}
	if length <= 8 {
		return "****"
	}
	return coerceString(prefix) + "****" + coerceString(suffix)
}
