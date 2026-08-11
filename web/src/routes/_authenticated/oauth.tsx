// metapi-go/routes — oauth connections list.
//
// `validateSearch` uses `oauthSearchSchema` (the URL-state contract for the
// oauth page). The `sort` field transforms the comma-separated URL string
// into a SortingState array for the validated `search` object; TanStack
// Router (non-strict default) does not rewrite the URL on transforms, so the
// page's own `window.location.search` reads stay consistent.
//
// `loader` prefetches the connections list (`oauthKeys.connections()`) and
// the providers list (`oauthKeys.providers()`) the page's
// `useOAuthConnections` / `useOAuthProviders` will request. The queryFns
// mirror the hooks (unwrap `.items` / `.providers` from the backend
// envelopes) so the cached payloads match the hooks' output types exactly.

import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

import { oauthKeys, oauthSearchSchema } from '@/features/oauth'
import { api } from '@/lib/api'

export const Route = createFileRoute('/_authenticated/oauth')({
  validateSearch: oauthSearchSchema,
  loader: async ({ context }) => {
    await Promise.all([
      context.queryClient.prefetchQuery({
        queryKey: oauthKeys.connections(),
        queryFn: async () => {
          const response = await api.getOAuthConnections({
            limit: 1000,
            offset: 0,
          })
          return response.items ?? []
        },
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
  component: lazyRouteComponent(
    () => import('@/features/oauth/components/oauth-page'),
    'OAuthPage'
  ),
})
