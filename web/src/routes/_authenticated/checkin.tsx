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
// `CheckinLogsQuery` the page derives from URL state, parsing the router's
// `location.searchStr` (rather than the global `window.location.search`, so
// the loader is SSR-safe and follows the router's location) via
// `parseCheckinSearch` + `buildInitialCheckinLogsQuery`, and reuses the
// hook's query key + fetcher (`fetchCheckinLogs`) so the prefetched page is
// served from cache on mount instead of re-fetching.

import { createFileRoute } from '@tanstack/react-router'

import {
  buildInitialCheckinLogsQuery,
  checkinQueryKeys,
  checkinSearchSchema,
  fetchCheckinLogs,
  parseCheckinSearch,
} from '@/features/checkin'
import { CheckinPage } from '@/features/checkin/components/checkin-page'

export const Route = createFileRoute('/_authenticated/checkin')({
  validateSearch: checkinSearchSchema,
  loader: async ({ context, location }) => {
    const initialQuery = buildInitialCheckinLogsQuery(
      parseCheckinSearch(location.searchStr)
    )
    await context.queryClient.prefetchQuery({
      queryKey: checkinQueryKeys.logsList(initialQuery),
      queryFn: () => fetchCheckinLogs(initialQuery),
    })
  },
  component: CheckinPage,
})
