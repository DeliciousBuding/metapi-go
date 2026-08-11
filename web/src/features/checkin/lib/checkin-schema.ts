// metapi-go features/checkin/lib — Zod schema for the checkin page URL search
// state. The checkin page is URL-state-driven: page / pageSize / accountId /
// status / reason / date-range / text query are all encoded in the query
// string so a bookmarked or shared URL restores the exact view.
//
// This mirrors the sites feature's `sitesSearchSchema` safe-parse pattern:
// the route file for /checkin is not registered yet, so the page parses
// `window.location.search` directly via this schema (gracefully falling
// back to defaults on malformed input) instead of relying on TanStack
// Router's generated search-param validation.

import { z } from 'zod'

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
 * Parse `window.location.search` into a validated CheckinSearch. Returns
 * defaults on any validation failure so the page always boots into a known
 * state — the route layer will replace this with typed search params once
 * /checkin is registered.
 */
export function readCheckinSearchFromUrl(): CheckinSearch {
  if (typeof window === 'undefined') return getCheckinSearchDefaultValues()
  const entries = Object.fromEntries(
    new URLSearchParams(window.location.search).entries()
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
