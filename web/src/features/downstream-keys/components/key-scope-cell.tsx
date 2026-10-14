// metapi-go/features/downstream-keys/components — human-readable
// routing-scope cell for the downstream key list (#1026 UI follow-up).
//
// Shows additional routing scope, source policy and period quota. Model grants
// and lifetime limits retain their own columns; this is not an authorization verdict.
// Site/account/token name maps come
// from the section-level sites/accounts/tokens queries; an ID that no longer
// resolves renders as `#id (unresolved)` — the contract's honest fallback
// (refs are only validated at write time; deletes do not cascade).

import { useTranslation } from 'react-i18next'

import {
  parseCredentialRefs,
  parseIdArray,
  type CredentialRef,
} from '../lib/credential-refs'
import type { DownstreamApiKeyItem } from '../types'
import { parseAccessPolicy } from './key-form-shared'

export type ScopeNameMaps = {
  sites: Map<number, string>
  accounts: Map<number, string>
  tokens: Map<number, string>
}

type KeyScopeCellProps = {
  item: Partial<DownstreamApiKeyItem>
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
  const policy = parseAccessPolicy(item.accessPolicy)
  const countLabel = (count: number) =>
    t('settings.downstream.keys.scope.count', { count })
  if (policy === undefined) {
    rows.push({ label: t('settings.downstream.keys.scope.unknown'), text: '' })
  } else if (policy) {
    if (policy.allowedUpstreamChannelIds !== undefined) {
      rows.push({
        label: t('settings.downstream.keys.scope.channels'),
        text:
          policy.allowedUpstreamChannelIds.length === 0
            ? t('settings.downstream.keys.scope.channelsDenied')
            : `${countLabel(policy.allowedUpstreamChannelIds.length)} · ${policy.allowedUpstreamChannelIds.map((id) => `#${id}`).join(', ')}`,
      })
    }
    if (policy.modelIds?.length) {
      rows.push({
        label: t('settings.downstream.keys.scope.models'),
        text: `${countLabel(policy.modelIds.length)} · ${policy.modelIds.join(', ')}`,
      })
    }
    if (policy.modelMappings?.length) {
      rows.push({
        label: t('settings.downstream.keys.scope.mappings'),
        text: countLabel(policy.modelMappings.length),
      })
    }
    if (policy.blockReason) {
      rows.push({
        label: t('settings.downstream.keys.scope.blocked'),
        text: '',
      })
    }
    if (policy.quota) {
      const limits = [
        ['requests', policy.quota.requests],
        ['totalTokens', policy.quota.totalTokens],
        ['cost', policy.quota.cost],
      ] as const
      const limitText = limits
        .filter(([, value]) => value !== undefined)
        .map(
          ([key, value]) =>
            `${t(`settings.downstream.keys.access.${key}`)}: ${value}`
        )
        .join(' · ')
      rows.push({
        label: t('settings.downstream.keys.scope.quota'),
        text: `${t(
          `settings.downstream.keys.access.periods.${policy.quota.period.type === 'calendar_duration' ? policy.quota.period.calendarDuration?.unit : policy.quota.period.type}`
        )} · ${limitText}`,
      })
      if (policy.quota.historyMissingBefore !== undefined) {
        rows.push({
          label: t('settings.downstream.keys.scope.historyMissing'),
          text: '',
        })
      }
    }
    if (
      Object.keys(policy).some(
        (key) =>
          ![
            'allowedUpstreamChannelIds',
            'modelIds',
            'modelMappings',
            'quota',
            'blockReason',
          ].includes(key)
      )
    ) {
      rows.push({
        label: t('settings.downstream.keys.scope.additional'),
        text: '',
      })
    }
  }

  const allowedSites = parseIdArray(item.allowedSiteIds)
  if (allowedSites.length > 0) {
    rows.push({
      label: `${t('settings.downstream.keys.scope.sitesAllow')} (${allowedSites.length})`,
      text: allowedSites.map((id) => resolveName(names.sites, id)).join(', '),
    })
  }
  const excludedSites = parseIdArray(item.excludedSiteIds)
  if (excludedSites.length > 0) {
    rows.push({
      label: `${t('settings.downstream.keys.scope.sitesExclude')} (${excludedSites.length})`,
      text: excludedSites.map((id) => resolveName(names.sites, id)).join(', '),
    })
  }

  const ipAllow =
    typeof item.ipAllowlist === 'string' ? item.ipAllowlist.trim() : ''
  const ipBlock =
    typeof item.ipBlocklist === 'string' ? item.ipBlocklist.trim() : ''
  if (ipAllow || ipBlock) {
    const parts: string[] = []
    if (ipAllow) {
      parts.push(`${t('settings.downstream.keys.scope.ipAllow')}: ${ipAllow}`)
    }
    if (ipBlock) {
      parts.push(`${t('settings.downstream.keys.scope.ipBlock')}: ${ipBlock}`)
    }
    rows.push({
      label: t('settings.downstream.keys.scope.ip'),
      text: parts.join(' · '),
    })
  }
  const rateParts: string[] = []
  if (item.maxRpm != null) {
    rateParts.push(`${item.maxRpm} RPM`)
  }
  if (item.maxTpm != null) {
    rateParts.push(`${item.maxTpm} TPM`)
  }
  if (rateParts.length > 0) {
    rows.push({
      label: t('settings.downstream.keys.scope.rate'),
      text: rateParts.join(' · '),
    })
  }

  const allowedRefs = parseCredentialRefs(item.allowedCredentialRefs)
  if (allowedRefs.length > 0) {
    rows.push({
      label: `${t('settings.downstream.keys.scope.credsAllow')} (${allowedRefs.length})`,
      text: allowedRefs.map(describeRef).join('; '),
    })
  }
  const excludedRefs = parseCredentialRefs(item.excludedCredentialRefs)
  if (excludedRefs.length > 0) {
    rows.push({
      label: `${t('settings.downstream.keys.scope.credsExclude')} (${excludedRefs.length})`,
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
          <span className='text-muted-foreground'>
            {row.label}
            {row.text ? ': ' : ''}
          </span>
          {row.text}
        </div>
      ))}
    </div>
  )
}
