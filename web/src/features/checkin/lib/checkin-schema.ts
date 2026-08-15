// metapi-go features/checkin/lib — Zod schema for the checkin page URL search
// state. The checkin page is URL-state-driven: page / pageSize / accountId /
// status / reason / date-range / text query are all encoded in the query
// string so a bookmarked or shared URL restores the exact view.
//
// The `/checkin` route file registers this schema via `validateSearch`, and
// its loader parses the router's `location.searchStr` with
// `parseCheckinSearch`. The page reads the validated search via `useSearch`
// and writes changes back via `navigate({ search, replace: true })` — the
// router owns URL state end to end.

import { z } from 'zod'

import { asStringParam, stringSearchParam } from '@/lib/helpers/searchParams'

import type { CheckinLogsQuery } from '../types'
import { localDatetimeInputToUtcRfc3339 } from './checkin-time'

export const DEFAULT_CHECKIN_PAGE_SIZE = 20

// ---------------------------------------------------------------------------
// URL search schema
// ---------------------------------------------------------------------------

export const checkinSearchSchema = z.object({
  page: z.coerce.number().int().min(1).catch(1).default(1),
  pageSize: z.coerce
    .number()
    .int()
    .min(1)
    .max(200)
    .catch(DEFAULT_CHECKIN_PAGE_SIZE)
    .default(DEFAULT_CHECKIN_PAGE_SIZE),
  accountId: z.coerce.number().int().positive().optional().catch(undefined),
  // Comma-separated multi-select filter values (status / reason category /
  // site name).
  status: z.string().optional(),
  reason: z.string().optional(),
  site: z.string().optional(),
  // datetime-local input values (YYYY-MM-DDTHH:mm, local tz). Router JSON
  // parsing may hand over numeric/boolean primitives for these too, so they
  // use the tolerant string param (normalized via `asStringParam`).
  from: stringSearchParam,
  to: stringSearchParam,
  q: stringSearchParam,
})
export type CheckinSearch = z.infer<typeof checkinSearchSchema>

export function getCheckinSearchDefaultValues(): CheckinSearch {
  return checkinSearchSchema.parse({})
}

/**
 * Parse a raw search string (with or without the leading `?`) into a
 * validated CheckinSearch. Pure — no `window` access — so both the page
 * (client, `window.location.search`) and the route loader (router
 * `location.searchStr`) share the same parser. Returns defaults on any
 * validation failure so callers always boot into a known state.
 */
export function parseCheckinSearch(searchStr: string): CheckinSearch {
  const entries = Object.fromEntries(
    new URLSearchParams(
      searchStr.startsWith('?') ? searchStr.slice(1) : searchStr
    ).entries()
  )
  const parsed = checkinSearchSchema.safeParse(entries)
  return parsed.success ? parsed.data : getCheckinSearchDefaultValues()
}

/**
 * Split a comma-separated filter string into a trimmed string array.
 */
export function parseFilterValues(value: string | undefined): string[] {
  if (!value) return []
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

/**
 * Build the server-side `CheckinLogsQuery` from an already-parsed
 * `CheckinSearch`. Pure (no `window` access) so the route loader can pass the
 * result of `parseCheckinSearch(location.searchStr)` — the prefetch cache key
 * then exactly matches the hook's first fetch (no double-fetch on mount).
 * `from`/`to` are converted to UTC RFC3339 (no milliseconds) so the
 * lexicographic `created_at` bound is correct.
 */
export function buildInitialCheckinLogsQuery(
  search: CheckinSearch
): CheckinLogsQuery {
  const statusValues = parseFilterValues(search.status)
  const reasonValues = parseFilterValues(search.reason)
  const siteValues = parseFilterValues(search.site)
  return {
    limit: search.pageSize,
    offset: (search.page - 1) * search.pageSize,
    accountId: search.accountId,
    status: statusValues[0],
    reason: reasonValues,
    site: siteValues,
    from: localDatetimeInputToUtcRfc3339(asStringParam(search.from), false),
    to: localDatetimeInputToUtcRfc3339(asStringParam(search.to), true),
    search: asStringParam(search.q) || undefined,
  }
}
