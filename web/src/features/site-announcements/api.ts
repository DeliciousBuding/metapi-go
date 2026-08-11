// metapi-go/features/site-announcements — TanStack Query hooks wrapping
// `lib/api.ts`.
//
// `useAnnouncements` fetches the full admin list (no server-side
// pagination); the table does client-side pagination/sorting/filtering.
// Mutations optimistically patch the list cache and invalidate on success
// so the table refreshes without a manual refetch. `useCreateAnnouncement`
// returns the updated `AnnouncementsResponse` so the hook can patch the cache
// with the server's authoritative list.

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationOptions,
  type UseQueryOptions,
} from '@tanstack/react-query'

import { api, type AnnouncementsResponse } from '@/lib/api'

import {
  announcementsKeys,
  type AnnouncementFormPayload,
  type SiteAnnouncement,
} from './types'

/**
 * Fetch all announcements (admin list — includes disabled/dismissed). The
 * backend returns `{ items: Announcement[] }`; the hook flattens to the
 * array for the table.
 */
export function useAnnouncements(
  options?: Omit<UseQueryOptions<SiteAnnouncement[]>, 'queryKey' | 'queryFn'>
) {
  return useQuery<SiteAnnouncement[]>({
    queryKey: announcementsKeys.list(),
    queryFn: async () => {
      const response = await api.getAnnouncements()
      return (response.items ?? []) as SiteAnnouncement[]
    },
    ...options,
  })
}

type CreateAnnouncementContext = { previous: SiteAnnouncement[] | undefined }

/**
 * Create an announcement. The backend returns the full updated list; the hook
 * patches the cache with it. Falls back to optimistic prepend on error
 * rollback.
 */
export function useCreateAnnouncement(
  options?: UseMutationOptions<
    SiteAnnouncement[],
    Error,
    AnnouncementFormPayload,
    CreateAnnouncementContext
  >
) {
  const queryClient = useQueryClient()
  return useMutation<
    SiteAnnouncement[],
    Error,
    AnnouncementFormPayload,
    CreateAnnouncementContext
  >({
    mutationFn: async (payload) => {
      const response: AnnouncementsResponse =
        await api.createAnnouncement(payload)
      return (response.items ?? []) as SiteAnnouncement[]
    },
    onMutate: async (payload) => {
      await queryClient.cancelQueries({ queryKey: announcementsKeys.list() })
      const previous = queryClient.getQueryData<SiteAnnouncement[]>(
        announcementsKeys.list()
      )
      const optimistic: SiteAnnouncement = {
        id: Math.floor(Math.random() * -1_000_000),
        title: payload.title,
        message: payload.message,
        severity: payload.severity,
        link: payload.link ?? null,
        enabled: payload.enabled ?? true,
        dismissed: false,
        dismissedAt: null,
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
      }
      queryClient.setQueryData<SiteAnnouncement[]>(
        announcementsKeys.list(),
        (current) => [optimistic, ...(current ?? [])]
      )
      return { previous }
    },
    onError: (_error, _payload, context) => {
      if (context?.previous) {
        queryClient.setQueryData(announcementsKeys.list(), context.previous)
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: announcementsKeys.list() })
    },
    ...options,
  })
}

type UpdateAnnouncementContext = { previous: SiteAnnouncement[] | undefined }

/**
 * Update an announcement by id. Optimistically patches the matching row in
 * the list cache so edits feel instant. The backend returns `{ success,
 * revision }` (not the updated entity), so the list is invalidated on
 * settled to fetch the authoritative state.
 */
export function useUpdateAnnouncement(
  options?: UseMutationOptions<
    void,
    Error,
    { id: number; payload: AnnouncementFormPayload },
    UpdateAnnouncementContext
  >
) {
  const queryClient = useQueryClient()
  return useMutation<
    void,
    Error,
    { id: number; payload: AnnouncementFormPayload },
    UpdateAnnouncementContext
  >({
    mutationFn: async ({ id, payload }) => {
      await api.updateAnnouncement(id, payload)
    },
    onMutate: async ({ id, payload }) => {
      await queryClient.cancelQueries({ queryKey: announcementsKeys.list() })
      const previous = queryClient.getQueryData<SiteAnnouncement[]>(
        announcementsKeys.list()
      )
      queryClient.setQueryData<SiteAnnouncement[]>(
        announcementsKeys.list(),
        (current) =>
          (current ?? []).map((item) =>
            item.id === id
              ? {
                  ...item,
                  title: payload.title,
                  message: payload.message,
                  severity: payload.severity,
                  link: payload.link ?? null,
                  enabled: payload.enabled ?? item.enabled,
                  updatedAt: new Date().toISOString(),
                }
              : item
          )
      )
      return { previous }
    },
    onError: (_error, _args, context) => {
      if (context?.previous) {
        queryClient.setQueryData(announcementsKeys.list(), context.previous)
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: announcementsKeys.list() })
    },
    ...options,
  })
}

type DeleteAnnouncementContext = { previous: SiteAnnouncement[] | undefined }

/**
 * Delete an announcement by id. Removes the row optimistically.
 */
export function useDeleteAnnouncement(
  options?: UseMutationOptions<void, Error, number, DeleteAnnouncementContext>
) {
  const queryClient = useQueryClient()
  return useMutation<void, Error, number, DeleteAnnouncementContext>({
    mutationFn: async (id) => {
      await api.deleteAnnouncement(id)
    },
    onMutate: async (id) => {
      await queryClient.cancelQueries({ queryKey: announcementsKeys.list() })
      const previous = queryClient.getQueryData<SiteAnnouncement[]>(
        announcementsKeys.list()
      )
      queryClient.setQueryData<SiteAnnouncement[]>(
        announcementsKeys.list(),
        (current) => (current ?? []).filter((item) => item.id !== id)
      )
      return { previous }
    },
    onError: (_error, _id, context) => {
      if (context?.previous) {
        queryClient.setQueryData(announcementsKeys.list(), context.previous)
      }
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: announcementsKeys.list() })
    },
    ...options,
  })
}
