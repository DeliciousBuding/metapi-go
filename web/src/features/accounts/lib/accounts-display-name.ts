// metapi-go/features/accounts/lib — display-name resolution for accounts.
// Shared between the table columns (row label) and the page-level toggle
// feedback toasts so both surfaces present the exact same identity.

import type { Account } from '../types'

export function resolveAccountDisplayName(
  account: Account,
  fallbackApiKey: string,
  fallbackUnnamed: string
): string {
  if (account.username && account.username.trim()) return account.username
  return account.credentialMode === 'apikey' ? fallbackApiKey : fallbackUnnamed
}
