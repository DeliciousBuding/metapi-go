// metapi-go/features/site-announcements — URL state for the list page.
//
// Filters and the page cursor live in the URL (?siteId=&platform=&read=&
// status=&page=) so a refresh or shared link restores the exact view,
// matching the other list pages (W19-T1 P2-l residual). The route's
// validateSearch keeps the raw params resilient (stringSearchParam); the
// normalizers below map any unknown or malformed value back to the page
// default instead of throwing.
//
// Pure module (no React imports) so the contract is unit-testable and the
// page component file keeps exporting only components (fast-refresh).

import type { SiteAnnouncementFilters } from './types'

/** Raw validated search params for this page (everything optional). */
export type SiteAnnouncementsSearchParams = {
  siteId?: string | number | boolean
  platform?: string | number | boolean
  read?: string | number | boolean
  status?: string | number | boolean
  page?: string | number | boolean
}

function asParamString(value: string | number | boolean | undefined): string {
  if (typeof value === 'string') return value
  return value === undefined ? '' : String(value)
}

function siteIdFromParam(
  value: string | number | boolean | undefined
): number | null {
  const raw = asParamString(value).trim()
  if (!raw) return null
  const parsed = Number(raw)
  return Number.isInteger(parsed) && parsed > 0 ? parsed : null
}

function readFilterFromParam(
  value: string | number | boolean | undefined
): SiteAnnouncementFilters['read'] {
  const raw = asParamString(value)
  return raw === 'true' || raw === 'false' ? raw : 'all'
}

function statusFilterFromParam(
  value: string | number | boolean | undefined
): SiteAnnouncementFilters['status'] {
  const raw = asParamString(value)
  return raw === 'active' || raw === 'expired' || raw === 'dismissed'
    ? raw
    : 'all'
}

function pageFromParam(value: string | number | boolean | undefined): number {
  const raw = asParamString(value).trim()
  if (!raw) return 0
  const parsed = Number(raw)
  return Number.isInteger(parsed) && parsed > 0 ? parsed : 0
}

/** Normalize the validated URL search into page filters + a page cursor. */
export function parseSiteAnnouncementsSearch(
  search: SiteAnnouncementsSearchParams
): {
  filters: SiteAnnouncementFilters
  page: number
} {
  return {
    filters: {
      siteId: siteIdFromParam(search.siteId),
      platform: asParamString(search.platform),
      read: readFilterFromParam(search.read),
      status: statusFilterFromParam(search.status),
    },
    page: pageFromParam(search.page),
  }
}

/** Serialize filters + page back to an href, dropping default values. */
export function buildSiteAnnouncementsHref(
  filters: SiteAnnouncementFilters,
  page: number
): string {
  const params = new URLSearchParams()
  if (filters.siteId !== null) params.set('siteId', String(filters.siteId))
  if (filters.platform !== '') params.set('platform', filters.platform)
  if (filters.read !== 'all') params.set('read', filters.read)
  if (filters.status !== 'all') params.set('status', filters.status)
  if (page > 0) params.set('page', String(page))
  const query = params.toString()
  return query ? `/site-announcements?${query}` : '/site-announcements'
}
