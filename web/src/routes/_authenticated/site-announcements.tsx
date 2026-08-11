// metapi-go/routes — site announcements list.
//
// `validateSearch` uses `announcementsSearchSchema` (the URL-state contract
// for the announcements page). The `sort` field transforms the
// comma-separated URL string into a SortingState array for the validated
// `search` object; TanStack Router (non-strict default) does not rewrite the
// URL on transforms, so the page's own `window.location.search` reads stay
// consistent.
//
// `loader` prefetches the admin announcements list
// (`announcementsKeys.list()`) the page's `useAnnouncements` will request.
// The queryFn mirrors the hook (unwrap `.items` from the
// `AnnouncementsResponse` envelope) so the cached payload matches the hook's
// output type exactly.

import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

import {
  announcementsKeys,
  announcementsSearchSchema,
} from '@/features/site-announcements'
import { api } from '@/lib/api'

export const Route = createFileRoute('/_authenticated/site-announcements')({
  validateSearch: announcementsSearchSchema,
  loader: async ({ context }) => {
    await context.queryClient.prefetchQuery({
      queryKey: announcementsKeys.list(),
      queryFn: async () => {
        const response = await api.getAnnouncements()
        return response.items ?? []
      },
    })
  },
  component: lazyRouteComponent(
    () => import('@/features/site-announcements/components/announcements-page'),
    'AnnouncementsPage'
  ),
})
