// metapi-go/features/accounts/lib — pure resolver for the one-shot
// site → account deep link (the sites guided-flow CTA writes
// `/accounts?siteId=…&create=1`).

import type { Site } from '../types'

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
