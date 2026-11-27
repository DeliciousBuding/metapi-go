package store

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

// CredentialFragmentsSelect projects only the first/last four characters and length.
// Both identifiers must come from the query author. No full credential crosses
// the database boundary, and both dialects expose the same snake_case aliases.
func CredentialFragmentsSelect(db *sqlx.DB, column, aliasBase string) string {
	if db != nil && db.DriverName() == "pgx" {
		return fmt.Sprintf(
			"LEFT(%s, 4) AS %s_prefix, RIGHT(%s, 4) AS %s_suffix, LENGTH(%s) AS %s_len",
			column, aliasBase, column, aliasBase, column, aliasBase)
	}
	return fmt.Sprintf(
		"SUBSTR(%s, 1, 4) AS %s_prefix, SUBSTR(%s, -4) AS %s_suffix, LENGTH(%s) AS %s_len",
		column, aliasBase, column, aliasBase, column, aliasBase)
}
