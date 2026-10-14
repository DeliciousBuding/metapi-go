import {
  normalizeModelRules,
  parseAccessPolicy,
} from '../components/key-form-shared'
import type { DownstreamApiKeyItem } from '../types'
// Restriction visibility for the downstream key list (#1430).
//
// The list response carries every restriction column already; this module is
// the single owner of "which dimensions count as restricted" so the filter,
// the row badges and the count line never drift apart. It reports configured
// restrictions only — it never evaluates permissions (that is the read-only
// key-context check's job).
import { parseCredentialRefs, parseIdArray } from './credential-refs'

export type KeyRestrictionDimension =
  | 'models'
  | 'routes'
  | 'sites'
  | 'credentials'
  | 'ip'
  | 'rate'
  | 'budget'
  | 'policy'

/** Display/filter order of the dimensions — the toolbar filter and any
 *  future per-dimension surface iterate this list instead of hardcoding one. */
export const KEY_RESTRICTION_DIMENSIONS = [
  'models',
  'routes',
  'sites',
  'credentials',
  'ip',
  'rate',
  'budget',
  'policy',
] as const satisfies readonly KeyRestrictionDimension[]

export type KeyRestrictionInfo = {
  dimension: KeyRestrictionDimension
  /** Number of configured knobs in the dimension; null when the stored value
   *  cannot be read (an unparseable access policy is still fail-closed at
   *  request time, so it must surface as restricted — just uncountable). */
  count: number | null
}

function hasText(value: unknown): boolean {
  return typeof value === 'string' && value.trim() !== ''
}

/** Entry count of one IP list column. Mirrors the backend splitIPList
 *  semantics (auth/downstream.go): newline- and comma-separated, blank
 *  entries dropped. */
function countIpEntries(raw: string | null | undefined): number {
  if (typeof raw !== 'string' || raw.trim() === '') return 0
  return raw
    .split(/[\n,]/)
    .map((entry) => entry.trim())
    .filter((entry) => entry !== '').length
}

// keyRestrictionDetails lists every configured restriction dimension of one
// key, in display order. An empty result means unrestricted. The model
// wildcard ('*') is the explicit all-models grant, not a restriction.
export function keyRestrictionDetails(
  item: Partial<DownstreamApiKeyItem>
): KeyRestrictionInfo[] {
  const details: KeyRestrictionInfo[] = []
  const modelRules = normalizeModelRules(item.supportedModels).filter(
    (rule) => rule !== '*'
  )
  if (modelRules.length > 0) {
    details.push({ dimension: 'models', count: modelRules.length })
  }
  const routes = parseIdArray(item.allowedRouteIds)
  if (routes.length > 0) {
    details.push({ dimension: 'routes', count: routes.length })
  }
  const siteCount =
    parseIdArray(item.allowedSiteIds).length +
    parseIdArray(item.excludedSiteIds).length
  if (siteCount > 0) {
    details.push({ dimension: 'sites', count: siteCount })
  }
  const credentialCount =
    parseCredentialRefs(item.allowedCredentialRefs).length +
    parseCredentialRefs(item.excludedCredentialRefs).length
  if (credentialCount > 0) {
    details.push({ dimension: 'credentials', count: credentialCount })
  }
  const ipCount =
    countIpEntries(item.ipAllowlist) + countIpEntries(item.ipBlocklist)
  if (ipCount > 0) {
    details.push({ dimension: 'ip', count: ipCount })
  }
  const rateCount =
    (item.maxRpm != null ? 1 : 0) + (item.maxTpm != null ? 1 : 0)
  if (rateCount > 0) {
    details.push({ dimension: 'rate', count: rateCount })
  }
  const budgetCount =
    (item.maxCost != null ? 1 : 0) + (item.maxRequests != null ? 1 : 0)
  if (budgetCount > 0) {
    details.push({ dimension: 'budget', count: budgetCount })
  }
  const policy = parseAccessPolicy(item.accessPolicy)
  if (policy === undefined && hasText(item.accessPolicy)) {
    // Stored but unreadable: fail-closed at request time, so it counts as a
    // restriction whose shape we cannot enumerate.
    details.push({ dimension: 'policy', count: null })
  } else if (policy != null && Object.keys(policy).length > 0) {
    details.push({ dimension: 'policy', count: Object.keys(policy).length })
  }
  return details
}

export function keyRestrictionDimensions(
  item: Partial<DownstreamApiKeyItem>
): KeyRestrictionDimension[] {
  return keyRestrictionDetails(item).map((detail) => detail.dimension)
}

export function isKeyRestricted(item: Partial<DownstreamApiKeyItem>): boolean {
  return keyRestrictionDetails(item).length > 0
}

/** data-key-scope anchor of the form section that manages the dimension, or
 *  null when the dimension has no form editing surface (routes/ip/rate are
 *  managed via import or the API — their badges still open the edit sheet,
 *  just without a section to locate). */
export function restrictionAnchorField(
  dimension: KeyRestrictionDimension
): string | null {
  switch (dimension) {
    case 'models':
      return 'supportedModels'
    case 'sites':
      return 'allowedSiteIds'
    case 'credentials':
      return 'allowedCredentialRefs'
    case 'budget':
      return 'maxRequests'
    case 'policy':
      return 'accessPolicy'
    default:
      return null
  }
}
