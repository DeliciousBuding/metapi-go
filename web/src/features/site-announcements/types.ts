// metapi-go/features/site-announcements — domain types for upstream site
// announcements (notices scraped from upstream sites, e.g. NewAPI-style
// /api/notice). Re-uses the backend contract declared in
// `lib/api/site-announcements.ts` so the feature speaks the same vocabulary
// as the transport layer.
//
// This is a SEPARATE product domain from the operator risk banners served by
// /api/announcements: distinct types, distinct query keys
// (`['site-announcements']`), distinct UI. The two are never merged.

import type {
  SiteAnnouncementListParams,
  SiteAnnouncementStatusFilter,
  SiteAnnouncementSyncResult,
} from '@/lib/api/site-announcements'

export type {
  SiteAnnouncement,
  SiteAnnouncementListParams,
} from '@/lib/api/site-announcements'

// --- View-level filter state ------------------------------------------------

/**
 * Client-side view of the active filters. `null` / empty / 'all' mean the
 * corresponding wire param is omitted entirely.
 */
export type SiteAnnouncementFilters = {
  siteId: number | null
  platform: string
  read: 'all' | 'true' | 'false'
  status: 'all' | SiteAnnouncementStatusFilter
}

export const DEFAULT_SITE_ANNOUNCEMENTS_FILTERS: SiteAnnouncementFilters = {
  siteId: null,
  platform: '',
  read: 'all',
  status: 'all',
}

export const SITE_ANNOUNCEMENTS_PAGE_SIZE = 20

/**
 * The list endpoint returns a bare array with NO total count, so the page
 * fetches one extra row to learn whether a next page exists.
 */
const SITE_ANNOUNCEMENTS_FETCH_LIMIT = SITE_ANNOUNCEMENTS_PAGE_SIZE + 1

/**
 * Build the wire params for one page of announcements. Pure and shared by
 * the page and the route loader prefetch so the prefetched cache key matches
 * the page's first fetch exactly.
 */
export function buildSiteAnnouncementsParams(
  page: number,
  filters: SiteAnnouncementFilters
): SiteAnnouncementListParams {
  return {
    limit: SITE_ANNOUNCEMENTS_FETCH_LIMIT,
    offset: page * SITE_ANNOUNCEMENTS_PAGE_SIZE,
    siteId: filters.siteId ?? undefined,
    platform: filters.platform || undefined,
    read: filters.read === 'all' ? undefined : filters.read === 'true',
    status: filters.status === 'all' ? undefined : filters.status,
  }
}

// --- Background sync task -----------------------------------------------------

export type SiteAnnouncementSyncTaskStatus =
  | 'pending'
  | 'running'
  | 'succeeded'
  | 'failed'

/**
 * Task envelope returned by GET /api/tasks/{id} for a site-announcements-sync
 * task. Mirrors handler/admin.BackgroundTask (camelCase JSON) — the same
 * shape the database-migration section observes through `api.getTask`.
 */
export type SiteAnnouncementSyncTask = {
  id: string
  type: string
  title: string
  status: SiteAnnouncementSyncTaskStatus
  message: string
  error?: string | null
  result?: SiteAnnouncementSyncResult | null
  createdAt: string
  updatedAt: string
  startedAt?: string | null
  finishedAt?: string | null
}

export const TERMINAL_SYNC_TASK_STATUSES =
  new Set<SiteAnnouncementSyncTaskStatus>(['succeeded', 'failed'])

/**
 * TanStack Query key factory. Deliberately its own namespace — never share a
 * key with the operator-banner `['announcements']` domain.
 */
export const siteAnnouncementsKeys = {
  all: ['site-announcements'] as const,
  list: (params: SiteAnnouncementListParams) =>
    [...siteAnnouncementsKeys.all, 'list', params] as const,
  syncTask: (taskId: string) =>
    [...siteAnnouncementsKeys.all, 'sync-task', taskId] as const,
}
