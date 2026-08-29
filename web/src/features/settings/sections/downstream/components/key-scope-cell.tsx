// metapi-go/features/settings/sections/downstream/components — human-readable
// routing-scope cell for the downstream key list (#1026 UI follow-up).
//
// Renders every configured policy dimension (allowed/excluded sites and
// credential refs) with resolved names instead of raw IDs. Name maps come
// from the section-level sites/accounts/tokens queries; an ID that no longer
// resolves renders as `#id (unresolved)` — the contract's honest fallback
// (refs are only validated at write time; deletes do not cascade).

import { useTranslation } from 'react-i18next'

import {
  parseCredentialRefs,
  parseIdArray,
  type CredentialRef,
} from '../lib/credential-refs'

export type ScopeNameMaps = {
  sites: Map<number, string>
  accounts: Map<number, string>
  tokens: Map<number, string>
}

type KeyScopeCellProps = {
  item: {
    allowedSiteIds?: number[] | string | null
    excludedSiteIds?: number[] | string | null
    allowedCredentialRefs?: string | unknown[] | null
    excludedCredentialRefs?: string | unknown[] | null
  }
  names: ScopeNameMaps
}

export function KeyScopeCell(props: KeyScopeCellProps) {
  const { t } = useTranslation()
  const { item, names } = props

  function resolveName(
    map: Map<number, string>,
    id: number | undefined
  ): string {
    if (id === undefined) {
      return t('settings.downstream.keys.scope.unknownId', { id: 0 })
    }
    return map.get(id) ?? t('settings.downstream.keys.scope.unknownId', { id })
  }

  function describeRef(ref: CredentialRef): string {
    const site = resolveName(names.sites, ref.siteId)
    const account = resolveName(names.accounts, ref.accountId)
    if (ref.kind === 'account_token') {
      const token = resolveName(names.tokens, ref.tokenId)
      return `${site} / ${account} / ${token}`
    }
    return `${site} / ${account} / ${t(
      'settings.downstream.keys.scope.defaultKey'
    )}`
  }

  const rows: Array<{ label: string; text: string }> = []

  const allowedSites = parseIdArray(item.allowedSiteIds)
  if (allowedSites.length > 0) {
    rows.push({
      label: t('settings.downstream.keys.scope.sitesAllow'),
      text: allowedSites.map((id) => resolveName(names.sites, id)).join(', '),
    })
  }
  const excludedSites = parseIdArray(item.excludedSiteIds)
  if (excludedSites.length > 0) {
    rows.push({
      label: t('settings.downstream.keys.scope.sitesExclude'),
      text: excludedSites.map((id) => resolveName(names.sites, id)).join(', '),
    })
  }

  const allowedRefs = parseCredentialRefs(item.allowedCredentialRefs)
  if (allowedRefs.length > 0) {
    rows.push({
      label: t('settings.downstream.keys.scope.credsAllow'),
      text: allowedRefs.map(describeRef).join('; '),
    })
  }
  const excludedRefs = parseCredentialRefs(item.excludedCredentialRefs)
  if (excludedRefs.length > 0) {
    rows.push({
      label: t('settings.downstream.keys.scope.credsExclude'),
      text: excludedRefs.map(describeRef).join('; '),
    })
  }

  if (rows.length === 0) {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('settings.downstream.keys.scope.unrestricted')}
      </span>
    )
  }

  return (
    <div className='space-y-0.5 text-xs' data-testid='key-scope-cell'>
      {rows.map((row) => (
        <div key={row.label}>
          <span className='text-muted-foreground'>{row.label}: </span>
          {row.text}
        </div>
      ))}
    </div>
  )
}
