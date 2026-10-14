// metapi-go/features/downstream-keys/components — restriction badge row for
// the downstream key list (#1430).
//
// One compact badge per configured restriction dimension (the derivation is
// owned by ../lib/key-restrictions so the badges, the toolbar filter and the
// count line can never disagree). The badge carries the human-readable detail
// in its tooltip; clicking it opens the edit sheet located at the dimension's
// form section when one exists (routes/ip/rate have no form surface — they are
// managed via import or the API — so those badges open the sheet without a
// locate). This is a display of configured restrictions, not an authorization
// verdict — evaluation stays with the read-only key-context check.
//
// Site/account/token name maps come from the section-level
// sites/accounts/tokens queries; an ID that no longer resolves renders as
// `#id (unresolved)` — the contract's honest fallback (refs are only
// validated at write time; deletes do not cascade).

import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { cn } from '@/lib/utils'

import {
  parseCredentialRefs,
  parseIdArray,
  type CredentialRef,
} from '../lib/credential-refs'
import {
  keyRestrictionDetails,
  restrictionAnchorField,
  type KeyRestrictionDimension,
} from '../lib/key-restrictions'
import type { DownstreamApiKeyItem } from '../types'
import { normalizeModelRules, parseAccessPolicy } from './key-form-shared'

export type ScopeNameMaps = {
  sites: Map<number, string>
  accounts: Map<number, string>
  tokens: Map<number, string>
}

type KeyScopeCellProps = {
  item: Partial<DownstreamApiKeyItem>
  names: ScopeNameMaps
  /** Badge click-through: open the edit sheet, located at the returned
   *  `data-key-scope` anchor when the dimension has a form section. */
  onOpenDetails?: (anchorField: string | null) => void
}

type DimensionDetails = Partial<Record<KeyRestrictionDimension, string>>

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

  // Per-dimension tooltip text. Each bucket lists exactly what is configured,
  // preserving the honesty markers of the pre-badge rows (denied channel
  // allow-list, unreadable policy, unmapped future fields, incomplete quota
  // history).
  const details: DimensionDetails = {}
  // policyAttention escalates the badge to the warning variant when the
  // stored policy blocks access, cannot be read, or carries fields this UI
  // version does not know.
  let policyAttention = false

  const policy = parseAccessPolicy(item.accessPolicy)
  const countLabel = (count: number) =>
    t('settings.downstream.keys.scope.count', { count })
  const policyLines: string[] = []
  if (policy === undefined) {
    if (
      typeof item.accessPolicy === 'string' &&
      item.accessPolicy.trim() !== ''
    ) {
      policyLines.push(t('settings.downstream.keys.scope.unknown'))
      policyAttention = true
    }
  } else if (policy) {
    if (policy.allowedUpstreamChannelIds !== undefined) {
      policyLines.push(
        `${t('settings.downstream.keys.scope.channels')}: ${
          policy.allowedUpstreamChannelIds.length === 0
            ? t('settings.downstream.keys.scope.channelsDenied')
            : `${countLabel(policy.allowedUpstreamChannelIds.length)} · ${policy.allowedUpstreamChannelIds.map((id) => `#${id}`).join(', ')}`
        }`
      )
      if (policy.allowedUpstreamChannelIds.length === 0) {
        policyAttention = true
      }
    }
    if (policy.modelIds?.length) {
      policyLines.push(
        `${t('settings.downstream.keys.scope.models')}: ${countLabel(policy.modelIds.length)} · ${policy.modelIds.join(', ')}`
      )
    }
    if (policy.modelMappings?.length) {
      policyLines.push(
        `${t('settings.downstream.keys.scope.mappings')}: ${countLabel(policy.modelMappings.length)}`
      )
    }
    if (policy.blockReason) {
      policyLines.push(t('settings.downstream.keys.scope.blocked'))
      policyAttention = true
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
      policyLines.push(
        `${t('settings.downstream.keys.scope.quota')}: ${t(
          `settings.downstream.keys.access.periods.${
            policy.quota.period.type === 'calendar_duration'
              ? policy.quota.period.calendarDuration?.unit
              : policy.quota.period.type
          }`
        )} · ${limitText}`
      )
      if (policy.quota.historyMissingBefore !== undefined) {
        policyLines.push(t('settings.downstream.keys.scope.historyMissing'))
        policyAttention = true
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
      policyLines.push(t('settings.downstream.keys.scope.additional'))
      policyAttention = true
    }
  }
  if (policyLines.length > 0) {
    details.policy = policyLines.join('\n')
  }

  const modelRules = normalizeModelRules(item.supportedModels).filter(
    (rule) => rule !== '*'
  )
  if (modelRules.length > 0) {
    details.models = modelRules.join(', ')
  }

  const routes = parseIdArray(item.allowedRouteIds)
  if (routes.length > 0) {
    details.routes = routes.map((id) => `#${id}`).join(', ')
  }

  const siteLines: string[] = []
  const allowedSites = parseIdArray(item.allowedSiteIds)
  if (allowedSites.length > 0) {
    siteLines.push(
      `${t('settings.downstream.keys.scope.sitesAllow')} (${allowedSites.length}): ${allowedSites.map((id) => resolveName(names.sites, id)).join(', ')}`
    )
  }
  const excludedSites = parseIdArray(item.excludedSiteIds)
  if (excludedSites.length > 0) {
    siteLines.push(
      `${t('settings.downstream.keys.scope.sitesExclude')} (${excludedSites.length}): ${excludedSites.map((id) => resolveName(names.sites, id)).join(', ')}`
    )
  }
  if (siteLines.length > 0) {
    details.sites = siteLines.join('\n')
  }

  const credentialLines: string[] = []
  const allowedRefs = parseCredentialRefs(item.allowedCredentialRefs)
  if (allowedRefs.length > 0) {
    credentialLines.push(
      `${t('settings.downstream.keys.scope.credsAllow')} (${allowedRefs.length}): ${allowedRefs.map(describeRef).join('; ')}`
    )
  }
  const excludedRefs = parseCredentialRefs(item.excludedCredentialRefs)
  if (excludedRefs.length > 0) {
    credentialLines.push(
      `${t('settings.downstream.keys.scope.credsExclude')} (${excludedRefs.length}): ${excludedRefs.map(describeRef).join('; ')}`
    )
  }
  if (credentialLines.length > 0) {
    details.credentials = credentialLines.join('\n')
  }

  const ipParts: string[] = []
  const ipAllow =
    typeof item.ipAllowlist === 'string' ? item.ipAllowlist.trim() : ''
  const ipBlock =
    typeof item.ipBlocklist === 'string' ? item.ipBlocklist.trim() : ''
  if (ipAllow) {
    ipParts.push(`${t('settings.downstream.keys.scope.ipAllow')}: ${ipAllow}`)
  }
  if (ipBlock) {
    ipParts.push(`${t('settings.downstream.keys.scope.ipBlock')}: ${ipBlock}`)
  }
  if (ipParts.length > 0) {
    details.ip = ipParts.join('\n')
  }

  const rateParts: string[] = []
  if (item.maxRpm != null) {
    rateParts.push(`${item.maxRpm} RPM`)
  }
  if (item.maxTpm != null) {
    rateParts.push(`${item.maxTpm} TPM`)
  }
  if (rateParts.length > 0) {
    details.rate = rateParts.join(' · ')
  }

  const budgetParts: string[] = []
  if (item.maxRequests != null) {
    budgetParts.push(
      `${t('settings.downstream.keys.fields.maxRequests')}: ${item.maxRequests}`
    )
  }
  if (item.maxCost != null) {
    budgetParts.push(
      `${t('settings.downstream.keys.fields.maxCost')}: ${item.maxCost}`
    )
  }
  if (budgetParts.length > 0) {
    details.budget = budgetParts.join(' · ')
  }

  const infos = keyRestrictionDetails(item)
  if (infos.length === 0) {
    return (
      <Badge variant='outline' className='text-muted-foreground'>
        {t('settings.downstream.keys.restriction.none')}
      </Badge>
    )
  }

  return (
    <div className='flex max-w-80 flex-wrap gap-1' data-testid='key-scope-cell'>
      {infos.map(({ dimension, count }) => (
        <Badge
          key={dimension}
          variant={
            dimension === 'policy' && policyAttention ? 'warning' : 'secondary'
          }
          render={props.onOpenDetails ? <button type='button' /> : undefined}
          title={details[dimension]}
          onClick={
            props.onOpenDetails
              ? () => props.onOpenDetails?.(restrictionAnchorField(dimension))
              : undefined
          }
          className={cn(props.onOpenDetails && 'cursor-pointer')}
        >
          {t(`settings.downstream.keys.restriction.dim.${dimension}`)} ·{' '}
          {count ?? t('settings.downstream.keys.picker.unknown')}
        </Badge>
      ))}
    </div>
  )
}
