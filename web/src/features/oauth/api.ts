// metapi-go/features/oauth — TanStack Query hooks wrapping `lib/api.ts`.
//
// `useOAuthConnections` fetches the full connection list with a large limit
// (the backend paginates server-side, but the table does client-side
// pagination/sorting/filtering via `useDataTable`). `useOAuthProviders`
// fetches the available providers for the start-authorization dialog.
// Mutations invalidate the connections key on success so the table refreshes
// without a manual refetch. `useStartOAuth` and `useRebindOAuthConnection`
// return an `OAuthStartResponse` whose `authorizationUrl` the caller opens
// in a new tab — they do not optimistically patch the list (the connection
// is created after the OAuth callback completes).

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

/**
 * Fetch all OAuth connections. The backend paginates server-side; we fetch
 * with a large limit so the table can do client-side pagination/sorting.
 */
export function useOAuthConnections(
  options?: Omit<UseQueryOptions<OAuthClient[]>, 'queryKey' | 'queryFn'>
) {
  return useQuery<OAuthClient[]>({
    queryKey: oauthKeys.connections(),
    queryFn: async () => {
      const response = await api.getOAuthConnections({ limit: 1000, offset: 0 })
      return (response.items ?? []) as OAuthClient[]
    },
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

type DeleteOAuthConnectionContext = { previous: OAuthClient[] | undefined }

/**
 * Delete an OAuth connection by account id. Removes the row optimistically.
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
      const previous = queryClient.getQueryData<OAuthClient[]>(
        oauthKeys.connections()
      )
      queryClient.setQueryData<OAuthClient[]>(
        oauthKeys.connections(),
        (current) =>
          (current ?? []).filter((client) => client.accountId !== accountId)
      )
      return { previous }
    },
    onError: (_error, _accountId, context) => {
      if (context?.previous) {
        queryClient.setQueryData(oauthKeys.connections(), context.previous)
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
