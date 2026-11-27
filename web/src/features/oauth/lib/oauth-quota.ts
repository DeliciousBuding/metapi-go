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
 * Usage ratio in [0, 1] when both `used` and positive `limit` are reported.
 * Returns `null` when either value is missing or `limit <= 0` so bars and
 * tone helpers never fabricate percentages from absent numbers.
 */
export function resolveOAuthQuotaUsageRatio(
  window: OAuthQuotaWindow | null | undefined
): number | null {
  if (!window || !window.supported) return null
  if (
    typeof window.used !== 'number' ||
    !Number.isFinite(window.used) ||
    typeof window.limit !== 'number' ||
    !Number.isFinite(window.limit) ||
    window.limit <= 0
  ) {
    return null
  }
  return Math.max(0, Math.min(1, window.used / window.limit))
}

export type OAuthQuotaSeverity = 'ok' | 'high' | 'exhausted'

/**
 * Classify a usage ratio into a semantic utilization band.
 * - `>= 1`: exhausted
 * - `>= 0.8`: high
 * - `< 0.8`: ok
 */
export function classifyOAuthQuotaSeverity(ratio: number): OAuthQuotaSeverity {
  if (ratio >= 1) return 'exhausted'
  if (ratio >= 0.8) return 'high'
  return 'ok'
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

/**
 * Classify a subscription's `activeUntil` timestamp relative to `nowMs`.
 * Returns `expired` when in the past, `expiringSoon` within 7 days, `active`
 * otherwise, or `unknown` when absent/unparseable.
 */
export type OAuthSubscriptionExpiryState =
  | 'expired'
  | 'expiringSoon'
  | 'active'
  | 'unknown'

const EXPIRING_SOON_WINDOW_MS = 7 * 24 * 60 * 60 * 1000

export function resolveOAuthSubscriptionExpiryState(
  activeUntil: string | null | undefined,
  nowMs: number
): OAuthSubscriptionExpiryState {
  if (!activeUntil?.trim()) return 'unknown'
  const ts = Date.parse(activeUntil)
  if (!Number.isFinite(ts)) return 'unknown'
  const remainingMs = ts - nowMs
  if (remainingMs <= 0) return 'expired'
  if (remainingMs <= EXPIRING_SOON_WINDOW_MS) return 'expiringSoon'
  return 'active'
}
