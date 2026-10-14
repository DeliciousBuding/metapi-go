import {
  normalizeModelRules,
  parseAccessPolicy,
} from '../components/key-form-shared'
import type { DownstreamApiKeyItem } from '../types'
// Restriction visibility for the downstream key list (#1430).
//
// The list response carries every restriction column already; this module is
// the single owner of "which dimensions count as restricted" so the filter,
// the row indicator and the scope cell never drift apart. It reports
// configured restrictions only — it never evaluates permissions (that is the
// read-only key-context check's job).
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

function hasText(value: string | null | undefined): boolean {
  return typeof value === 'string' && value.trim() !== ''
}

// keyRestrictionDimensions lists every configured restriction dimension of
// one key. An empty result means unrestricted. The model wildcard ('*') is
// the explicit all-models grant, not a restriction.
export function keyRestrictionDimensions(
  item: Partial<DownstreamApiKeyItem>
): KeyRestrictionDimension[] {
  const dimensions: KeyRestrictionDimension[] = []
  const modelRules = normalizeModelRules(item.supportedModels).filter(
    (rule) => rule !== '*'
  )
  if (modelRules.length > 0) dimensions.push('models')
  if (parseIdArray(item.allowedRouteIds).length > 0) dimensions.push('routes')
  if (
    parseIdArray(item.allowedSiteIds).length > 0 ||
    parseIdArray(item.excludedSiteIds).length > 0
  ) {
    dimensions.push('sites')
  }
  if (
    parseCredentialRefs(item.allowedCredentialRefs).length > 0 ||
    parseCredentialRefs(item.excludedCredentialRefs).length > 0
  ) {
    dimensions.push('credentials')
  }
  if (hasText(item.ipAllowlist) || hasText(item.ipBlocklist)) {
    dimensions.push('ip')
  }
  if (item.maxRpm != null || item.maxTpm != null) {
    dimensions.push('rate')
  }
  if (item.maxCost != null || item.maxRequests != null) {
    dimensions.push('budget')
  }
  const policy = parseAccessPolicy(item.accessPolicy)
  if (policy != null && Object.keys(policy).length > 0) {
    dimensions.push('policy')
  }
  return dimensions
}

export function isKeyRestricted(item: Partial<DownstreamApiKeyItem>): boolean {
  return keyRestrictionDimensions(item).length > 0
}
