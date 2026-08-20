// metapi-go/features/sites — display-balance resolution shared by the list
// column and the detail sheet, so both surfaces apply the same fallback
// ladder and the same em-dash policy for missing data.

import type { Site } from '../types'

/**
 * Resolve the USD balance to display for a site. The site-level aggregated
 * `totalBalance` wins; when the backend has not computed it, fall back to
 * the subscription summary's remaining USD. Returns `null` when neither is
 * available so callers render an em dash instead of a fabricated zero.
 */
export function resolveSiteBalanceUsd(site: Site): number | null {
  if (
    typeof site.totalBalance === 'number' &&
    Number.isFinite(site.totalBalance)
  ) {
    return site.totalBalance
  }
  const remaining = site.subscriptionSummary?.totalRemainingUsd
  if (typeof remaining === 'number' && Number.isFinite(remaining)) {
    return remaining
  }
  return null
}
