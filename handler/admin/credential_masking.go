package admin

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

// credentialFragmentsSelect returns SQL column expressions that expose just
// enough of a stored credential — the first 4 characters, the last 4
// characters, and the total length — to rebuild its masked form, WITHOUT
// materializing the full plaintext secret across the DB→Go boundary. This
// hardens against logging/metrics side-channels: a credential that is never
// scanned into Go memory cannot be leaked by a stray slog or metrics call.
//
// Aliases are emitted in snake_case so queryRows()' camelCase key mapping
// yields {aliasBase}Prefix / {aliasBase}Suffix / {aliasBase}Len in the row.
//
// SQLite (modernc.org/sqlite) has no LEFT()/RIGHT() built-ins — confirmed by
// query at runtime — so the SQLite branch uses SUBSTR(s, -4) for the suffix
// (negative start = count from the right). Postgres + any future driver that
// exposes LEFT()/RIGHT() uses those for readability.
func credentialFragmentsSelect(db *sqlx.DB, column, aliasBase string) string {
	if db != nil && db.DriverName() == "pgx" {
		return fmt.Sprintf(
			"LEFT(%s, 4) AS %s_prefix, RIGHT(%s, 4) AS %s_suffix, LENGTH(%s) AS %s_len",
			column, aliasBase, column, aliasBase, column, aliasBase)
	}
	return fmt.Sprintf(
		"SUBSTR(%s, 1, 4) AS %s_prefix, SUBSTR(%s, -4) AS %s_suffix, LENGTH(%s) AS %s_len",
		column, aliasBase, column, aliasBase, column, aliasBase)
}

// maskSecretFromFragments rebuilds the exact maskSecret() output (first4 +
// "****" + last4, collapsing to "****" for secrets <= 8 chars, "" for empty)
// from the prefix/suffix/length fragments selected by credentialFragmentsSelect.
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
