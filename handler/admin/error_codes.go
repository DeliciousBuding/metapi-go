package admin

// Machine-readable errorCode values carried on unified admin error bodies
// ({"error", "errorCode", ...}). Contract + registry table: docs/api.md.
//
// Convention: errorCode values are stable camelCase identifiers. The field
// is OPTIONAL and additive — endpoints without a registered code omit it
// entirely, and the human-readable "error" message stays the source of
// truth for display. Clients must never substring-match on "error" when a
// code exists for their case.
//
// New codes are added only for real call sites (no speculative taxonomy);
// register every new constant in the docs/api.md registry table.
const (
	// ErrorCodeInvalidID rejects a URL path ID that is missing, non-numeric
	// or non-positive (pathID helper; every /api route with an {id} segment).
	ErrorCodeInvalidID = "invalidId"

	// ErrorCodeInvalidDatabaseType rejects a runtime-database dialect that is
	// neither sqlite nor postgres (/api/settings/database/* endpoints).
	ErrorCodeInvalidDatabaseType = "invalidDatabaseType"

	// ErrorCodeEmptyMigrationTarget rejects a migration request whose target
	// connection string is blank (POST /api/settings/database/migrate).
	ErrorCodeEmptyMigrationTarget = "emptyMigrationTarget"

	// ErrorCodeSameMigrationTarget rejects a migration whose target resolves
	// to the currently-running database (POST /api/settings/database/migrate).
	ErrorCodeSameMigrationTarget = "sameMigrationTarget"

	// ErrorCodeAccountNotFound rejects a request whose `{id}` path segment or
	// body references an account that does not exist (accounts / account
	// tokens / accounts-health route families).
	ErrorCodeAccountNotFound = "accountNotFound"

	// ErrorCodeOperationNotImplemented marks an honest 501 residual surface
	// (a known unimplemented feature — never a fake stub success). Emitted on
	// the residual 501 bodies via writeNotImplementedResidual.
	ErrorCodeOperationNotImplemented = "operationNotImplemented"
	// ErrorCodeTokenNotFound rejects an account-token operation whose
	// referenced token does not exist (account-tokens route family).
	ErrorCodeTokenNotFound = "tokenNotFound"

	// ErrorCodeRouteNotFound rejects a token-route operation whose
	// referenced route does not exist (routes family).
	ErrorCodeRouteNotFound = "routeNotFound"

	// ErrorCodeChannelNotFound rejects a route-channels operation whose
	// referenced channel does not exist (routes/{id}/channels family).
	ErrorCodeChannelNotFound = "channelNotFound"

	// ErrorCodeSiteNotFound rejects a request whose `{id}` path segment or
	// body references a site that does not exist (sites / accounts / site
	// announcements route families).
	ErrorCodeSiteNotFound = "siteNotFound"

	// ErrorCodeOperationNotSupported rejects an operation that the account
	// connection type does not support (e.g. API-key connections managing
	// account tokens).
	ErrorCodeOperationNotSupported = "operationNotSupported"

	// ErrorCodeResourceDisabled marks a failure caused by a capability being
	// currently unavailable — engine not configured, probe scheduler not
	// running, or the like.
	ErrorCodeResourceDisabled = "resourceDisabled"

	// ErrorCodeInvalidSettingsValue rejects a runtime-settings apply whose body
	// carries an invalid field value (the settings_apply validation funnel);
	// 5xx apply failures intentionally carry no code.
	ErrorCodeInvalidSettingsValue = "invalidSettingsValue"

	// ErrorCodeResourceLoadFailed marks a 5xx read-path failure where entity
	// data could not be loaded (accounts / announcements / events / channels /
	// site announcements / stats / token routes). One code for the whole
	// load-failure family; the per-entity English message stays the display
	// fallback.
	ErrorCodeResourceLoadFailed = "resourceLoadFailed"
)
