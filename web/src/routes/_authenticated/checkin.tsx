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
// `loader` prefetches the checkin log window the page's `useCheckinLogs`
// will request, using the same key shape (`[...logs(), { accountId, limit,
// offset }]`) so the prefetched page is reused rather than re-fetched on
// mount. The loader reads the accountId from `window.location.search` via
// `readCheckinSearchFromUrl()` (the feature's own URL reader, the same
// helper the page uses) because TanStack Router's loader context does not
// expose the validated `search` object in this version. The queryFn mirrors
// the hook (build the query string, call `api.getCheckinLogs`, parse each
// row with `checkinLogRowSchema`) so the cached payload matches the hook's
// output type exactly.

import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

import { api } from '@/lib/api'
import {
  checkinLogRowSchema,
  checkinQueryKeys,
  checkinSearchSchema,
  readCheckinSearchFromUrl,
} from '@/features/checkin'

const CHECKIN_PREFETCH_LIMIT = 500

export const Route = createFileRoute('/_authenticated/checkin')({
  validateSearch: checkinSearchSchema,
  loader: async ({ context }) => {
    const { accountId } = readCheckinSearchFromUrl()
    await context.queryClient.prefetchQuery({
      queryKey: [
        ...checkinQueryKeys.logs(),
        { accountId, limit: CHECKIN_PREFETCH_LIMIT, offset: undefined },
      ],
      queryFn: async () => {
        const params = new URLSearchParams()
        params.set('limit', String(CHECKIN_PREFETCH_LIMIT))
        if (accountId) params.set('accountId', String(accountId))
        const raw = await api.getCheckinLogs(params.toString())
        if (!Array.isArray(raw)) return []
        return raw.map((row) => checkinLogRowSchema.parse(row))
      },
    })
  },
  component: lazyRouteComponent(
    () => import('@/features/checkin/components/checkin-page'),
    'CheckinPage',
  ),
})
