// metapi-go features/checkin/lib — Zod schema for the checkin page URL search
// state. The checkin page is URL-state-driven: page / pageSize / accountId /
// status / reason / date-range / text query are all encoded in the query
// string so a bookmarked or shared URL restores the exact view.
//
// The `/checkin` route file registers this schema via `validateSearch`, and
// its loader parses the router's `location.searchStr` with
// `parseCheckinSearch`. The page still parses `window.location.search`
// directly via `readCheckinSearchFromUrl` for its client-only state
// initialisation (both share `parseCheckinSearch` underneath).

import { z } from 'zod'

import type { CheckinLogsQuery } from '../types'
import { localDatetimeInputToUtcRfc3339 } from './checkin-time'

const DEFAULT_CHECKIN_PAGE_SIZE = 20

// ---------------------------------------------------------------------------
// URL search schema
// ---------------------------------------------------------------------------

export const checkinSearchSchema = z.object({
  page: z.coerce.number().int().min(1).default(1),
  pageSize: z.coerce.number().int().min(1).max(200).default(20),
  accountId: z.coerce.number().int().positive().optional(),
  // Comma-separated multi-select filter values (status / reason category /
  // site name).
  status: z.string().optional(),
  reason: z.string().optional(),
  site: z.string().optional(),
  // datetime-local input values (YYYY-MM-DDTHH:mm, local tz).
  from: z.string().optional(),
  to: z.string().optional(),
  q: z.string().optional(),
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
 * Parse `window.location.search` into a validated CheckinSearch (the page's
 * client-only entry point). Returns defaults on any validation failure so
 * the page always boots into a known state.
 */
export function readCheckinSearchFromUrl(): CheckinSearch {
  if (typeof window === 'undefined') return getCheckinSearchDefaultValues()
  return parseCheckinSearch(window.location.search)
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
 * Build the query string for the current page state (used to write state
 * back to the URL via history.replaceState).
 */
export function buildCheckinSearchString(params: {
  pageIndex: number
  pageSize: number
  accountId?: number
  statusValues: string[]
  reasonValues: string[]
  siteValues: string[]
  from?: string
  to?: string
  query?: string
}): string {
  const search = new URLSearchParams()
  if (params.pageIndex > 0) search.set('page', String(params.pageIndex + 1))
  if (params.pageSize !== DEFAULT_CHECKIN_PAGE_SIZE) {
    search.set('pageSize', String(params.pageSize))
  }
  if (params.accountId) search.set('accountId', String(params.accountId))
  if (params.statusValues.length) {
    search.set('status', params.statusValues.join(','))
  }
  if (params.reasonValues.length) {
    search.set('reason', params.reasonValues.join(','))
  }
  if (params.siteValues.length) {
    search.set('site', params.siteValues.join(','))
  }
  if (params.from) search.set('from', params.from)
  if (params.to) search.set('to', params.to)
  if (params.query) search.set('q', params.query)
  const query = search.toString()
  return query ? `?${query}` : ''
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
    from: localDatetimeInputToUtcRfc3339(search.from, false),
    to: localDatetimeInputToUtcRfc3339(search.to, true),
    search: search.q || undefined,
  }
}
