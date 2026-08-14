// metapi-go/routes — checkin logs list.
//
// `validateSearch` uses `checkinSearchSchema` (the URL-state contract for the
// checkin page). The schema carries `.default()` values for page / pageSize;
// with TanStack Router's default (non-strict) search handling these defaults
// populate the validated `search` object passed to the loader / `useSearch()`
// but are NOT written back to the URL, so the page's own
// `window.location.search` reads (via `readCheckinSearchFromUrl`) stay
// consistent.
//
// `loader` prefetches the first server-paginated checkin-logs page the page's
// `useCheckinLogs` hook will request. It builds the exact same
// `CheckinLogsQuery` the page derives from URL state (`buildInitialCheckinLogsQuery`)
// and reuses the hook's query key + fetcher (`fetchCheckinLogs`), so the
// prefetched page is served from cache on mount instead of re-fetching.

import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

import {
  buildInitialCheckinLogsQuery,
  checkinQueryKeys,
  checkinSearchSchema,
  fetchCheckinLogs,
} from '@/features/checkin'

export const Route = createFileRoute('/_authenticated/checkin')({
  validateSearch: checkinSearchSchema,
  loader: async ({ context }) => {
    const initialQuery = buildInitialCheckinLogsQuery()
    await context.queryClient.prefetchQuery({
      queryKey: checkinQueryKeys.logsList(initialQuery),
      queryFn: () => fetchCheckinLogs(initialQuery),
    })
  },
  component: lazyRouteComponent(
    () => import('@/features/checkin/components/checkin-page'),
    'CheckinPage'
  ),
})
