// metapi-go/features/settings/sections/downstream/lib — display helpers for
// upstream account/token rows used by the credential-ref tree picker and the
// key scope cell. Kept outside component files so React fast refresh only
// sees components in .tsx modules.

import type { Account, AccountToken } from '@/features/accounts/types'

export function accountDisplayName(account: Account): string {
  return account.username?.trim() || `#${account.id}`
}

export function tokenDisplayName(token: AccountToken): string {
  return token.name?.trim() || token.tokenMasked?.trim() || `#${token.id}`
}
