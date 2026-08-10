// metapi-go/features/site-announcements — domain types for the
// announcements management feature.
//
// The backend `/api/announcements` surface (H1: product risk banners) has
// full CRUD: list, create, update, delete, dismiss. The `Announcement`
// type from `lib/api.ts` is the canonical contract; this feature aliases it
// as `SiteAnnouncement` (the legacy feature name) and adds the form-payload
// type consumed by the add/edit dialog. The optional `siteName` field is
// reserved for future per-site scoping — the current API returns global
// announcements, so the column shows an em-dash when absent.

import type { Announcement } from '@/lib/api'

/** Row type for the announcements table (alias of the API `Announcement`). */
export type SiteAnnouncement = Announcement

/** Announcement severity — drives badge colour and filter facets. */
export type AnnouncementSeverity = Announcement['severity']

/**
 * Wire payload for `api.createAnnouncement` / `api.updateAnnouncement`.
 * Matches the backend body shape so the call is unchanged.
 */
export type AnnouncementFormPayload = {
  title: string
  message: string
  severity: AnnouncementSeverity
  link?: string | null
  enabled?: boolean
}

/**
 * TanStack Query key factory. Centralised so invalidation is grep-able and
 * the keys stay stable across hooks.
 */
export const announcementsKeys = {
  all: ['site-announcements'] as const,
  list: () => [...announcementsKeys.all, 'list'] as const,
}
