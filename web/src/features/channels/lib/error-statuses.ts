// metapi-go/features/channels — failing-status vocabulary shared by the
// error banner, the page filter wiring and their tests (kept out of the
// component file so fast-refresh stays component-only).

import type { ChannelStatus } from '../types'

/**
 * Runtime-failing statuses: cooldown (temporarily pulled from rotation) and
 * breaker_open (circuit tripped). `manually_disabled` is deliberate operator
 * intent rather than a failure, so it stays out of the error count and the
 * one-click filter.
 */
export const CHANNELS_ERROR_STATUSES: readonly ChannelStatus[] = [
  'cooldown',
  'breaker_open',
]

/** The comma-separated `?status=` value the filter action writes. */
export const CHANNELS_ERROR_STATUS_FILTER = CHANNELS_ERROR_STATUSES.join(',')

/**
 * True when the URL status filter is scoped to failing statuses only (any
 * non-empty subset of CHANNELS_ERROR_STATUSES). Mixed or healthy selections
 * keep the banner in its normal count mode.
 */
export function isErrorOnlyStatusFilter(statusFilter: string): boolean {
  const values = statusFilter.split(',').filter(Boolean)
  return (
    values.length > 0 &&
    values.every((value) =>
      (CHANNELS_ERROR_STATUSES as readonly string[]).includes(value)
    )
  )
}
