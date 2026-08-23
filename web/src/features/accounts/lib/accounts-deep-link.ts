// metapi-go/features/accounts/lib — pure resolvers for the one-shot
// site → account deep links (the sites guided-flow CTAs write
// `/accounts?siteId=…&create=1` and `/accounts?siteId=…&create=1&segment=apikey`).

import type { CredentialMode, Site } from '../types'

/**
 * Resolve the one-shot deep-link intent from the accounts route search state.
 * Returns the referenced site to preselect when the `create` flag is set AND
 * the referenced `siteId` is present in the loaded snapshot; otherwise null.
 * Stale, malformed, or absent deep-link values never preselect and never open
 * the dialog (the page falls back to the normal empty create form).
 */
export function resolveDeepLinkPreselect(
  createFlag: boolean | undefined,
  siteId: number | undefined,
  sites: Site[]
): number | null {
  if (createFlag !== true) return null
  if (!siteId || siteId <= 0) return null
  if (!sites.some((site) => site.id === siteId)) return null
  return siteId
}

/**
 * Resolve the credential mode hinted by the deep-link `segment` param
 * (`segment=apikey` from the sites guided-flow "添加 API Key" CTA). Unknown
 * or absent segments resolve to null so the account form keeps its session
 * default — the segment only ever narrows the default, never forces an
 * operator out of another mode.
 */
export function resolveDeepLinkCredentialMode(
  segment: string | undefined
): CredentialMode | null {
  if (segment === 'apikey') return 'apikey'
  return null
}

