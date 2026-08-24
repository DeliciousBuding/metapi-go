// metapi-go/api — site announcements domain module (UPSTREAM site notices).
//
// Distinct product surface from the operator risk banners served by
// /api/announcements (Settings > Content > Announcements): this module talks
// to /api/site-announcements, which stores announcements scraped from
// upstream sites (NewAPI-style /api/notice and friends) for operator review.
// Types, query keys and UI never overlap with the operator-banner domain.

import { buildQueryString, request } from './transport'

/**
 * One upstream site announcement row as returned by GET
 * /api/site-announcements (bare JSON array, camelCase contract). Content
 * fields are UNTRUSTED upstream data — render as plain text only.
 */
export type SiteAnnouncement = {
  id: number
  siteId: number
  platform: string
  title: string
  content: string
  level: string
  sourceKey: string
  sourceUrl: string | null
  startsAt: string | null
  endsAt: string | null
  firstSeenAt: string
  lastSeenAt: string
  upstreamCreatedAt: string | null
  upstreamUpdatedAt: string | null
  readAt: string | null
  dismissedAt: string | null
  rawPayload: string | null
}

export type SiteAnnouncementStatusFilter = 'active' | 'expired' | 'dismissed'

export type SiteAnnouncementListParams = {
  limit?: number
  offset?: number
  siteId?: number
  platform?: string
  read?: boolean
  status?: SiteAnnouncementStatusFilter
}

/** Terminal payload of the site-announcements-sync background task. */
export type SiteAnnouncementSyncResult = {
  scannedSites: number
  inserted: number
  updated: number
  unsupported: number
  notifications: number
  events: number
  failed: number
  failedSites: Array<{ siteId: number; siteName: string; message: string }>
}

/** POST /api/site-announcements/sync response (task accepted immediately). */
export type SiteAnnouncementSyncResponse = {
  success: boolean
  queued: boolean
  reused: boolean
  taskId: string
}

export const siteAnnouncementsApi = {
  /**
   * List upstream site announcements. The backend returns a BARE JSON array
   * (no wrapper object) ordered by firstSeenAt desc; server-side filters:
   * siteId / platform / read / status. No total count is returned, so
   * callers paginate with a limit+1 probe.
   */
  getSiteAnnouncements: (params?: SiteAnnouncementListParams) =>
    request<SiteAnnouncement[]>(
      `/api/site-announcements${buildQueryString(params)}`
    ),

  /** Mark one announcement read. The page surfaces its own failure toast. */
  markSiteAnnouncementRead: (id: number) =>
    request<{ success: boolean }>(
      `/api/site-announcements/${encodeURIComponent(String(id))}/read`,
      { method: 'POST', skipErrorHandler: true }
    ),

  /** Mark every announcement read. The page surfaces its own failure toast. */
  markAllSiteAnnouncementsRead: () =>
    request<{ success: boolean }>('/api/site-announcements/read-all', {
      method: 'POST',
      skipErrorHandler: true,
    }),

  /** Delete ALL announcements. The page guards this behind a ConfirmDialog. */
  clearSiteAnnouncements: () =>
    request<{ success: boolean }>('/api/site-announcements', {
      method: 'DELETE',
      skipErrorHandler: true,
    }),

  /**
   * Queue a background sync of upstream announcements (all active sites, or
   * one site when `siteId` is provided). Resolves immediately with a taskId;
   * progress is observed by polling `api.getTask` — no second task API.
   */
  syncSiteAnnouncements: (data?: { siteId?: number }) =>
    request<SiteAnnouncementSyncResponse>('/api/site-announcements/sync', {
      method: 'POST',
      body: JSON.stringify(data ?? {}),
      skipErrorHandler: true,
    }),
}
