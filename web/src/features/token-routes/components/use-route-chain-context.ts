import { useMemo } from 'react'

import {
  useAccounts,
  type Account,
  type AccountsSnapshot,
} from '@/features/accounts'
import { useSites } from '@/features/sites'

import type { RouteAccountOption } from './route-form-dialog'

export function useRouteChainContext(filters: {
  accountId: string
  tokenId: string
  siteId: string
}) {
  // Chain context (account/site deep-link) is read from the URL-owned filters;
  // it is preserved across every navigation but never modified by this page.
  const accountId = filters.accountId ? Number(filters.accountId) : undefined
  const tokenId = filters.tokenId ? Number(filters.tokenId) : undefined
  const siteId = filters.siteId ? Number(filters.siteId) : undefined

  const chainContext = useMemo(
    () => ({
      accountId,
      tokenId,
      siteId,
    }),
    [accountId, tokenId, siteId]
  )

  // The loader already prefetches both the accounts snapshot and the sites
  // list into the query cache, so these are cache hits (no extra round-trip).
  // Resolve chain-context IDs to human-readable names with `#ID` fallback.
  const { data: accountsSnapshot } = useAccounts()
  const { data: sitesList } = useSites()
  const accountOptions = useMemo(
    () =>
      buildAccountOptions(accountsSnapshot, tokenId ? accountId : undefined),
    [accountsSnapshot, accountId, tokenId]
  )
  const chainAccountName = useMemo(() => {
    if (!accountId) return undefined
    const match = accountsSnapshot?.accounts.find((a) => a.id === accountId)
    return match?.username ?? `#${accountId}`
  }, [accountId, accountsSnapshot])
  const chainSiteName = useMemo(() => {
    if (!siteId) return undefined
    const match = sitesList?.find((s) => s.id === siteId)
    return match?.name ?? `#${siteId}`
  }, [siteId, sitesList])

  return { chainContext, chainAccountName, chainSiteName, accountOptions }
}

// The account snapshot, not model discovery, owns manual account bindings.
// OAuth metadata is also present on the public account snapshot; inspect only
// masked credential presence, matching account-scoped relay credential choice.
type ManualRouteAccount = Account & { oauthProvider?: string | null }

function buildAccountOptions(
  snapshot: AccountsSnapshot | undefined,
  keyAccountId?: number
): RouteAccountOption[] {
  const siteMap = new Map(
    (snapshot?.sites ?? []).map((site) => [site.id, site])
  )
  const accounts: ManualRouteAccount[] = snapshot?.accounts ?? []
  return accounts
    .flatMap<RouteAccountOption>((account) => {
      const site = siteMap.get(account.siteId) ?? account.site
      if (account.status !== 'active' || site?.status === 'disabled') {
        return []
      }
      const relayCredential = account.oauthProvider
        ? account.accessTokenMasked
        : account.apiTokenMasked
      // An explicit existing key does not need the account's default relay
      // credential. The write boundary checks that the key belongs to it.
      if (!relayCredential?.trim() && account.id !== keyAccountId) return []

      const username = account.username?.trim() || `account-${account.id}`
      const siteName = site?.name?.trim() || ''
      return [
        {
          id: account.id,
          label: siteName ? `${username} @ ${siteName}` : username,
        },
      ]
    })
    .sort((left, right) =>
      left.label.localeCompare(right.label, undefined, { sensitivity: 'base' })
    )
}
