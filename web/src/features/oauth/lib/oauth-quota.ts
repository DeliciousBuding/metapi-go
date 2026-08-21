// metapi-go/features/oauth/lib — quota-payload classification helpers.
//
// The backend's `OAuthQuotaInfo` is a three-state contract: the snapshot as a
// whole can be `supported` / `unsupported` / `error`, and EACH window
// (5-hour / 7-day) independently carries a `supported` flag plus nullable
// numbers. Reading those states inline inside JSX produced the "fabricated
// zero" class of bug the list column already guards against: a missing
// `used` rendered as `0` reads as "nothing consumed", and a missing `limit`
// as `0` reads as "quota exhausted". Both are lies about data the provider
// never sent.
//
// These pure functions name the states once so the detail panel can render
// an explicit sentence ("not reported by upstream" / "no data yet") instead
// of a number, and so the policy is unit-testable without a DOM.

import type { OAuthClient } from '../types'

/** The quota snapshot attached to a connection (non-null form). */
export type OAuthQuotaSnapshot = NonNullable<OAuthClient['quota']>

/** One quota window (5-hour or 7-day) of a snapshot. */
export type OAuthQuotaWindow = OAuthQuotaSnapshot['windows']['fiveHour']

/**
 * Availability of the whole quota snapshot.
 *
 * - `missing`: the connection carries no `quota` object at all (provider was
 *   never polled, or the list projection omitted it).
 * - `unsupported`: the provider does not expose quota data.
 * - `error`: the last quota sync failed; any numbers present are stale.
 * - `reported`: the snapshot is usable (individual windows may still be
 *   unsupported — see {@link resolveOAuthQuotaWindowState}).
 */
export type OAuthQuotaAvailability =
  | 'missing'
  | 'unsupported'
  | 'error'
  | 'reported'

/**
 * How much of a single quota window the upstream actually reported.
 *
 * - `unsupported`: the provider does not report this window.
 * - `noData`: the window is supported but every number is null/absent.
 * - `reported`: at least one of used/limit/remaining carries a number.
 */
export type OAuthQuotaWindowState = 'unsupported' | 'noData' | 'reported'

export function resolveOAuthQuotaAvailability(
  quota: OAuthQuotaSnapshot | null | undefined
): OAuthQuotaAvailability {
  if (!quota) return 'missing'
  if (quota.status === 'unsupported') return 'unsupported'
  if (quota.status === 'error') return 'error'
  if (!quota.windows) return 'missing'
  return 'reported'
}

export function resolveOAuthQuotaWindowState(
  window: OAuthQuotaWindow | null | undefined
): OAuthQuotaWindowState {
  if (!window || !window.supported) return 'unsupported'
  const hasReportedNumber =
    window.used != null || window.limit != null || window.remaining != null
  return hasReportedNumber ? 'reported' : 'noData'
}

/**
 * True when the subscription block carries at least one non-empty field.
 * An all-null `subscription` object must hide the block entirely rather than
 * render three em dashes (the accounts/sites detail-sheet empty-value
 * convention: either an em dash or a hidden block, never a fake value).
 */
export function hasOAuthSubscriptionDetails(
  quota: OAuthQuotaSnapshot | null | undefined
): boolean {
  const subscription = quota?.subscription
  if (!subscription) return false
  return Boolean(
    subscription.planType?.trim() ||
    subscription.activeStart?.trim() ||
    subscription.activeUntil?.trim()
  )
}
