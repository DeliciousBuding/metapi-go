import { useState } from 'react'

import type { Account, AccountToken } from '@/features/accounts'
import type { Site } from '@/features/sites'

import { accountDisplayName, tokenDisplayName } from '../lib/credential-display'
import { credentialRefKey, type CredentialRef } from '../lib/credential-refs'

export function matchingAccount(
  site: Site,
  account: Account,
  tokens: AccountToken[],
  value: CredentialRef[],
  selectedKeys: Set<string>,
  onlySelected: boolean,
  query: string
) {
  const matches = (text: string) => text.toLocaleLowerCase().includes(query)
  const accountName = accountDisplayName(account)
  const accountMatches = matches(`${site.name} ${accountName} ${account.id}`)
  const accountTokens = tokens.filter(
    (token) =>
      (!onlySelected ||
        selectedKeys.has(
          credentialRefKey({
            kind: 'account_token',
            siteId: site.id,
            accountId: account.id,
            tokenId: token.id,
          })
        )) &&
      (accountMatches || matches(`${tokenDisplayName(token)} ${token.id}`))
  )
  // The snapshot redacts apiToken to apiTokenMasked, so the masked field
  // is the honest "account has a default API key" indicator.
  const hasDefaultKey = Boolean(account.apiTokenMasked)
  const defaultRef: CredentialRef = {
    kind: 'default_api_key',
    siteId: site.id,
    accountId: account.id,
  }
  const showDefault =
    (!onlySelected || selectedKeys.has(credentialRefKey(defaultRef))) &&
    accountMatches
  const accountSelectedCount = value.filter(
    (ref) => ref.siteId === site.id && ref.accountId === account.id
  ).length
  if (!showDefault && accountTokens.length === 0) return null
  return {
    accountId: account.id,
    accountName,
    defaultRef,
    showDefault,
    hasDefaultKey,
    accountTokens,
    accountSelectedCount,
  }
}

function toggleExpanded(previous: Set<number>, id: number) {
  const next = new Set(previous)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  return next
}

export function useCredentialRefExpansion(value: CredentialRef[]) {
  // Sites (and token lists) holding a selection start expanded so edit mode
  // reveals the checked rows immediately. Mount-only state: the hosting form
  // remounts per sheet open, so a stale expansion set never leaks across
  // keys.
  const [expandedSiteIds, setExpandedSiteIds] = useState<Set<number>>(
    () => new Set(value.map((ref) => ref.siteId))
  )
  const [expandedTokenAccounts, setExpandedTokenAccounts] = useState<
    Set<number>
  >(
    () =>
      new Set(
        value
          .filter((ref) => ref.kind === 'account_token')
          .map((ref) => ref.accountId)
      )
  )

  return {
    expandedSiteIds,
    expandedTokenAccounts,
    toggleSite: (siteId: number) =>
      setExpandedSiteIds((previous) => toggleExpanded(previous, siteId)),
    toggleTokenList: (accountId: number) =>
      setExpandedTokenAccounts((previous) =>
        toggleExpanded(previous, accountId)
      ),
  }
}
