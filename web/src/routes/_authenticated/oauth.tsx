// metapi-go/routes — oauth connections list.
//
// `validateSearch` uses `oauthSearchSchema` (the URL-state contract for the
// oauth page). The `sort` field transforms the comma-separated URL string
// into a SortingState array for the validated `search` object; TanStack
// Router (non-strict default) does not rewrite the URL on transforms, so the
// page's own `window.location.search` reads stay consistent.
//
// `loader` prefetches the ONE server-paginated connections page the deep
// link points at (page/pageSize parsed from the router location via
// `oauthSearchSchema`) plus the providers list. It reuses the hook's
// query-key factory + fetcher so the prefetched page is served from cache
// on mount instead of re-fetching — mirroring the checkin-logs pattern.

import { createFileRoute } from '@tanstack/react-router'

import {
  fetchOAuthConnectionsPage,
  oauthConnectionsPageQueryKey,
  oauthKeys,
  oauthSearchSchema,
} from '@/features/oauth'
import { OAuthPage } from '@/features/oauth/components/oauth-page'
import { api } from '@/lib/api'

export const Route = createFileRoute('/_authenticated/oauth')({
  validateSearch: oauthSearchSchema,
  staticData: { title: 'oauth.page.title' },
  loader: async ({ context, location }) => {
    const params = new URLSearchParams(location.searchStr)
    const parsed = oauthSearchSchema.safeParse({
      page: params.get('page') ?? undefined,
      pageSize: params.get('pageSize') ?? undefined,
    })
    const page = parsed.success ? (parsed.data.page ?? 0) : 0
    const pageSize = parsed.success ? (parsed.data.pageSize ?? 20) : 20

    await Promise.all([
      context.queryClient.prefetchQuery({
        queryKey: oauthConnectionsPageQueryKey({ page, pageSize }),
        queryFn: () => fetchOAuthConnectionsPage({ page, pageSize }),
      }),
      context.queryClient.prefetchQuery({
        queryKey: oauthKeys.providers(),
        queryFn: async () => {
          const response = await api.getOAuthProviders()
          return response.providers ?? []
        },
      }),
    ])
  },
  component: OAuthPage,
})
