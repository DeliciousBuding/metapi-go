// metapi-go/features/oauth — TanStack Query hooks wrapping `lib/api.ts`.
//
// `useOAuthConnections` fetches ONE server-side page at a time
// (GET /api/oauth/connections?limit=&offset= returns items + total), so the
// table runs in manualPagination mode and the page count reflects the real
// total. The previous implementation fetched `limit:1000` and silently
// truncated larger fleets. The backend has no server-side q/status/sort
// params, so the toolbar search + status filter + column sorting stay
// client-side over the fetched page only (documented backend gap).
// `useOAuthProviders` fetches the available providers for the
// start-authorization dialog. Mutations invalidate the connections key
// prefix on success so the current page refreshes without a manual refetch.
// `useStartOAuth` and `useRebindOAuthConnection` return an
// `OAuthStartResponse` whose `authorizationUrl` the caller opens in a new
// tab — they do not optimistically patch the list (the connection is created
// after the OAuth callback completes).

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationOptions,
  type UseQueryOptions,
} from '@tanstack/react-query'

import { api, type OAuthSessionInfo } from '@/lib/api'

import {
  oauthKeys,
  type OAuthClient,
  type OAuthProvider,
  type OAuthStartPayload,
} from './types'

/**
 * Fetch available OAuth providers. Used by the start-authorization dialog.
 */
export function useOAuthProviders(
  options?: Omit<UseQueryOptions<OAuthProvider[]>, 'queryKey' | 'queryFn'>
) {
  return useQuery<OAuthProvider[]>({
    queryKey: oauthKeys.providers(),
    queryFn: async () => {
      const response = await api.getOAuthProviders()
      return (response.providers ?? []) as OAuthProvider[]
    },
    ...options,
  })
}

/** One server-side page of OAuth connections plus the fleet total. */
export type OAuthConnectionsPage = {
  items: OAuthClient[]
  total: number
}

/** Query key for one server-side connections page. */
export function oauthConnectionsPageQueryKey(params: {
  page: number
  pageSize: number
}) {
  return [
    ...oauthKeys.connections(),
    { page: params.page, pageSize: params.pageSize },
  ]
}

/**
 * Fetch + shape one server-side connections page. Shared by the hook and
 * the route loader so the prefetched page reuses the hook's cache key and
 * payload shape exactly. A missing / malformed `total` degrades to the
 * returned page length (the pager then shows one page — never an invented
 * total).
 */
export async function fetchOAuthConnectionsPage(params: {
  page: number
  pageSize: number
}): Promise<OAuthConnectionsPage> {
  const response = await api.getOAuthConnections({
    limit: params.pageSize,
    offset: params.page * params.pageSize,
  })
  const items = (response.items ?? []) as OAuthClient[]
  const total =
    typeof response.total === 'number' && Number.isFinite(response.total)
      ? response.total
      : items.length
  return { items, total }
}

/**
 * Fetch a single server-side page of OAuth connections. The backend returns
 * `{ items, total, limit, offset }`; `total` drives the table's page count
 * (manualPagination). `placeholderData` keeps the previous page on screen
 * while the next one loads, matching the checkin-logs pattern.
 */
export function useOAuthConnections(
  params: { page: number; pageSize: number },
  options?: Omit<
    UseQueryOptions<OAuthConnectionsPage>,
    'queryKey' | 'queryFn'
  >
) {
  return useQuery<OAuthConnectionsPage>({
    queryKey: oauthConnectionsPageQueryKey(params),
    queryFn: () => fetchOAuthConnectionsPage(params),
    placeholderData: (previous) => previous,
    staleTime: 10 * 1000,
    ...options,
  })
}

/**
 * Start an OAuth authorization flow. Returns the `authorizationUrl` to open
 * in a new tab. Invalidates the connections list on success (the connection
 * is created after the OAuth callback completes on the backend).
 */
export function useStartOAuth(
  options?: UseMutationOptions<
    Awaited<ReturnType<typeof api.startOAuthProvider>>,
    Error,
    OAuthStartPayload
  >
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (payload: OAuthStartPayload) => {
      const { provider, ...rest } = payload
      return api.startOAuthProvider(provider, rest)
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: oauthKeys.connections() })
    },
    ...options,
  })
}

type DeleteOAuthConnectionContext = {
  previousPages: Array<
    [readonly unknown[], OAuthConnectionsPage | undefined]
  >
}

/**
 * Delete an OAuth connection by account id. Removes the row optimistically
 * from every cached connections page (the query key now carries the page
 * params) and adjusts each page's total. Rolls back all pages on error.
 */
export function useDeleteOAuthConnection(
  options?: UseMutationOptions<
    void,
    Error,
    number,
    DeleteOAuthConnectionContext
  >
) {
  const queryClient = useQueryClient()
  return useMutation<void, Error, number, DeleteOAuthConnectionContext>({
    mutationFn: async (accountId) => {
      await api.deleteOAuthConnection(accountId)
    },
    onMutate: async (accountId) => {
      await queryClient.cancelQueries({ queryKey: oauthKeys.connections() })
      const previousPages = queryClient.getQueriesData<OAuthConnectionsPage>({
        queryKey: oauthKeys.connections(),
      })
      queryClient.setQueriesData<OAuthConnectionsPage>(
        { queryKey: oauthKeys.connections() },
        (current) =>
          current
            ? {
                items: current.items.filter(
                  (client) => client.accountId !== accountId
                ),
                total: Math.max(0, current.total - 1),
              }
            : current
      )
      return { previousPages }
    },
    onError: (_error, _accountId, context) => {
      for (const [queryKey, previous] of context?.previousPages ?? []) {
        queryClient.setQueryData(queryKey, previous)
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: oauthKeys.connections() })
    },
    ...options,
  })
}

/**
 * Refresh quota for a single OAuth connection. Invalidates the list on
 * success so the refreshed quota displays.
 */
export function useRefreshOAuthQuota(
  options?: UseMutationOptions<
    Awaited<ReturnType<typeof api.refreshOAuthConnectionQuota>>,
    Error,
    number
  >
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (accountId: number) => {
      return api.refreshOAuthConnectionQuota(accountId)
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: oauthKeys.connections() })
    },
    ...options,
  })
}

/**
 * Rebind an OAuth connection (re-run the authorization flow). Returns an
 * `OAuthStartResponse` whose `authorizationUrl` the caller opens in a new
 * tab. Invalidates the connections list on success.
 */
export function useRebindOAuthConnection(
  options?: UseMutationOptions<
    Awaited<ReturnType<typeof api.rebindOAuthConnection>>,
    Error,
    number
  >
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (accountId: number) => {
      return api.rebindOAuthConnection(accountId)
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: oauthKeys.connections() })
    },
    ...options,
  })
}

/**
 * Poll a pending OAuth session by `state` until the backend reports success
 * or error. Refetches every 2s while `status === 'pending'` and stops once
 * the session settles. Pass `null` to disable (e.g. before the start-authorization
 * mutation has returned a `state`).
 */
export function useOAuthSession(
  state: string | null,
  options?: Omit<UseQueryOptions<OAuthSessionInfo>, 'queryKey' | 'queryFn'>
) {
  return useQuery<OAuthSessionInfo>({
    queryKey: oauthKeys.session(state ?? ''),
    queryFn: async () => {
      // `enabled: !!state` guarantees `state` is non-null here; the guard
      // satisfies the type checker without a non-null assertion.
      if (!state) throw new Error('OAuth session state is required')
      return api.getOAuthSession(state)
    },
    enabled: !!state,
    refetchInterval: (query) =>
      query.state.data?.status === 'pending' ? 2000 : false,
    ...options,
  })
}

/**
 * Submit a manual callback URL for a pending OAuth session. Used when the
 * provider's redirect cannot reach the local callback port directly (e.g.
 * behind a firewall); the operator pastes the full redirect URL. Invalidates
 * the session query so polling resumes immediately, plus the connections
 * list so a newly created connection shows up.
 */
export function useSubmitOAuthManualCallback(
  options?: UseMutationOptions<
    { success: true },
    Error,
    { state: string; callbackUrl: string }
  >
) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ state, callbackUrl }) =>
      api.submitOAuthManualCallback(state, callbackUrl),
    onSettled: (_data, _err, vars) => {
      void queryClient.invalidateQueries({
        queryKey: oauthKeys.session(vars.state),
      })
      void queryClient.invalidateQueries({
        queryKey: oauthKeys.connections(),
      })
    },
    ...options,
  })
}
