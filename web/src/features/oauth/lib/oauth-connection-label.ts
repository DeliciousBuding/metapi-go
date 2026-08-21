// metapi-go/features/oauth/lib — display label for an OAuth connection.
//
// The wire payload has no single "name" field: a connection is identified by
// whichever of username / email / accountKey the provider returned, and some
// providers return none of them. Shared by the list page's failure toasts
// (so the operator can tell WHICH account failed instead of reading a bare
// numeric id) and by the detail sheet's title.

import type { OAuthClient } from '../types'

export function resolveOAuthConnectionLabel(connection: OAuthClient): string {
  return (
    connection.username ??
    connection.email ??
    connection.accountKey ??
    String(connection.accountId)
  )
}
