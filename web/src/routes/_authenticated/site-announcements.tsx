// metapi-go/routes — upstream site announcements list.
//
// Replaces the legacy redirect that aliased /site-announcements onto the
// Settings operator-announcement section (the wrong product domain): the
// route is now a real surface for upstream site notices backed by
// /api/site-announcements — kept strictly separate from the operator risk
// banners of /api/announcements.
//
// `loader` prefetches the first unfiltered page plus the sites list (site
// names join + filter facets) so the page renders with data on first paint.
// Params are built through the same helper the page uses, so the prefetched
// cache key matches the page's first fetch exactly.
//
// `validateSearch` syncs the filters + page cursor to the URL so a refresh or
// shared link restores the exact view (W19-T1 P2-l residual). The router
// JSON-parses search values, so every param uses the resilient
// stringSearchParam union and the page normalizes to its own defaults.

import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import {
  buildSiteAnnouncementsParams,
  DEFAULT_SITE_ANNOUNCEMENTS_FILTERS,
  siteAnnouncementsKeys,
} from '@/features/site-announcements'
import { SiteAnnouncementsPage } from '@/features/site-announcements/components/site-announcements-page'
import { sitesKeys } from '@/features/sites'
import { api } from '@/lib/api'
import { stringSearchParam } from '@/lib/helpers/searchParams'

const firstPageParams = buildSiteAnnouncementsParams(
  0,
  DEFAULT_SITE_ANNOUNCEMENTS_FILTERS
)

export const siteAnnouncementsSearchSchema = z.object({
  siteId: stringSearchParam,
  platform: stringSearchParam,
  read: stringSearchParam,
  status: stringSearchParam,
  page: stringSearchParam,
})

export const Route = createFileRoute('/_authenticated/site-announcements')({
  staticData: { title: 'siteAnnouncements.page.title' },
  validateSearch: siteAnnouncementsSearchSchema,
  loader: async ({ context }) => {
    await Promise.all([
      context.queryClient.prefetchQuery({
        queryKey: siteAnnouncementsKeys.list(firstPageParams),
        queryFn: () => api.getSiteAnnouncements(firstPageParams),
      }),
      context.queryClient.prefetchQuery({
        queryKey: sitesKeys.list(),
        queryFn: () => api.getSites(),
      }),
    ])
  },
  component: SiteAnnouncementsPage,
})
