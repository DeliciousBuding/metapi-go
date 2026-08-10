// metapi-go/features/sites — TanStack Query hooks wrapping `lib/api.ts`.
//
// `api.getSites` returns the full list (no server-side pagination today), so
// `useSites` is a single query and the table does client-side
// pagination/sorting/filtering. Mutations invalidate the list key on success
// so the table refreshes without a manual refetch. The created-site payload
// is returned by `useCreateSite` so the dialog can hand it to the
// `SiteCreatedModal` guided next-step flow.

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationOptions,
  type UseQueryOptions,
} from '@tanstack/react-query'

import { api } from '@/lib/api'

import { sitesKeys, type Site, type SiteBatchAction, type SiteBatchResult, type SiteFormPayload } from './types'

/**
 * Fetch all sites. The backend returns the full array; the table owns
 * client-side pagination/sorting/filtering.
 */
export function useSites(
  options?: Omit<UseQueryOptions<Site[]>, 'queryKey' | 'queryFn'>,
) {
  return useQuery<Site[]>({
    queryKey: sitesKeys.list(),
    queryFn: async () => {
      const result = await api.getSites()
      return (Array.isArray(result) ? result : []) as Site[]
    },
    ...options,
  })
}

type CreateSiteContext = { previous: Site[] | undefined }

/**
 * Create a site. Returns the created `Site` (with id) so the caller can
 * open the guided "next step: add an account" modal with the new id.
 * Optimistically rolls back the list on error.
 */
export function useCreateSite(
  options?: UseMutationOptions<Site, Error, SiteFormPayload, CreateSiteContext>,
) {
  const queryClient = useQueryClient()
  return useMutation<Site, Error, SiteFormPayload, CreateSiteContext>({
    mutationFn: async (payload) => {
      const created = (await api.addSite(payload)) as Site
      return created
    },
    onMutate: async (payload) => {
      await queryClient.cancelQueries({ queryKey: sitesKeys.list() })
      const previous = queryClient.getQueryData<Site[]>(sitesKeys.list())
      const optimistic: Site = {
        id: Math.floor(Math.random() * -1_000_000),
        name: payload.name,
        url: payload.url,
        externalCheckinUrl: payload.externalCheckinUrl || null,
        platform: payload.platform,
        status: 'active',
        proxyUrl: payload.proxyUrl || null,
        useSystemProxy: payload.useSystemProxy,
        globalWeight: payload.globalWeight,
        maxConcurrency: payload.maxConcurrency,
        postRefreshProbeEnabled: payload.postRefreshProbeEnabled,
        postRefreshProbeModel: payload.postRefreshProbeModel || null,
        postRefreshProbeScope: payload.postRefreshProbeScope ?? null,
        postRefreshProbeLatencyThresholdMs:
          payload.postRefreshProbeLatencyThresholdMs ?? null,
        accountCount: 0,
      }
      queryClient.setQueryData<Site[]>(sitesKeys.list(), (current) => [
        optimistic,
        ...(current ?? []),
      ])
      return { previous }
    },
    onError: (_error, _payload, context) => {
      if (context?.previous) {
        queryClient.setQueryData(sitesKeys.list(), context.previous)
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: sitesKeys.list() })
    },
    ...options,
  })
}

type UpdateSiteContext = { previous: Site[] | undefined }

/**
 * Update a site by id. The payload is `Partial<Site>` (not the form payload)
 * because the page sends partial field updates — e.g. toggling `status` or
 * `isPinned` — that aren't part of `SiteFormPayload`. The full form save also
 * flows through here (`buildPayload` returns a `SiteFormPayload` which is
 * assignable to `Partial<Site>`). Optimistically patches the matching row in
 * the list cache so toggles/edits feel instant.
 */
export function useUpdateSite(
  options?: UseMutationOptions<
    Site,
    Error,
    { id: number; payload: Partial<Site> },
    UpdateSiteContext
  >,
) {
  const queryClient = useQueryClient()
  return useMutation<
    Site,
    Error,
    { id: number; payload: Partial<Site> },
    UpdateSiteContext
  >({
    mutationFn: async ({ id, payload }) => {
      const updated = (await api.updateSite(id, payload)) as Site
      return updated
    },
    onMutate: async ({ id, payload }) => {
      await queryClient.cancelQueries({ queryKey: sitesKeys.list() })
      const previous = queryClient.getQueryData<Site[]>(sitesKeys.list())
      queryClient.setQueryData<Site[]>(sitesKeys.list(), (current) =>
        (current ?? []).map((site) =>
          site.id === id ? { ...site, ...payload } : site,
        ),
      )
      return { previous }
    },
    onError: (_error, _args, context) => {
      if (context?.previous) {
        queryClient.setQueryData(sitesKeys.list(), context.previous)
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: sitesKeys.list() })
    },
    ...options,
  })
}

/**
 * Delete a site. Removes the row from the list cache optimistically.
 */
export function useDeleteSite(
  options?: UseMutationOptions<void, Error, number, { previous: Site[] | undefined }>,
) {
  const queryClient = useQueryClient()
  return useMutation<void, Error, number, { previous: Site[] | undefined }>({
    mutationFn: async (id) => {
      await api.deleteSite(id)
    },
    onMutate: async (id) => {
      await queryClient.cancelQueries({ queryKey: sitesKeys.list() })
      const previous = queryClient.getQueryData<Site[]>(sitesKeys.list())
      queryClient.setQueryData<Site[]>(sitesKeys.list(), (current) =>
        (current ?? []).filter((site) => site.id !== id),
      )
      return { previous }
    },
    onError: (_error, _id, context) => {
      if (context?.previous) {
        queryClient.setQueryData(sitesKeys.list(), context.previous)
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: sitesKeys.list() })
    },
    ...options,
  })
}

/**
 * Batch enable/disable/delete/toggle-system-proxy across many sites. Returns
 * the per-id success/failure breakdown so the bulk-action bar can report
 * partial failures.
 */
export function useBatchUpdateSites(
  options?: UseMutationOptions<
    SiteBatchResult,
    Error,
    { ids: number[]; action: SiteBatchAction }
  >,
) {
  const queryClient = useQueryClient()
  return useMutation<
    SiteBatchResult,
    Error,
    { ids: number[]; action: SiteBatchAction }
  >({
    mutationFn: async ({ ids, action }) => {
      const result = (await api.batchUpdateSites({ ids, action })) as SiteBatchResult
      return result ?? { successIds: [], failedItems: [] }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: sitesKeys.list() })
    },
    ...options,
  })
}

/**
 * Probe a site's URL to pre-fill the form (platform / endpoints). Used by
 * the "detect" button in the add-site dialog.
 */
export function useDetectSite(
  options?: UseMutationOptions<Partial<Site>, Error, string>,
) {
  return useMutation<Partial<Site>, Error, string>({
    mutationFn: async (url) => {
      const detected = (await api.detectSite(url)) as Partial<Site>
      return detected ?? {}
    },
    ...options,
  })
}
