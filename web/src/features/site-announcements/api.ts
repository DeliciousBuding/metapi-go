// metapi-go/features/site-announcements — TanStack Query hooks wrapping the
// `siteAnnouncementsApi` methods merged into `@/lib/api`.
//
// Read-path queries live here; the write-path mutations (mark read / read-all
// / clear / sync) are composed in the page component so their toast wording
// stays with the i18n-aware UI, mirroring the program-logs section pattern.

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

import { api } from '@/lib/api'

import {
  siteAnnouncementsKeys,
  TERMINAL_SYNC_TASK_STATUSES,
  type SiteAnnouncement,
  type SiteAnnouncementListParams,
  type SiteAnnouncementSyncTask,
} from './types'

const SYNC_TASK_POLL_INTERVAL_MS = 2000

/**
 * Fetch one page of upstream site announcements (bare-array contract). The
 * query key embeds the full params object so cache reuse is exact; the page
 * keeps showing the previous rows while a filtered page loads.
 */
export function useSiteAnnouncements(
  params: SiteAnnouncementListParams,
  options?: Omit<UseQueryOptions<SiteAnnouncement[]>, 'queryKey' | 'queryFn'>
) {
  return useQuery<SiteAnnouncement[]>({
    queryKey: siteAnnouncementsKeys.list(params),
    queryFn: async () => {
      const result = await api.getSiteAnnouncements(params)
      return Array.isArray(result) ? result : []
    },
    placeholderData: (previous) => previous,
    ...options,
  })
}

type GetTaskResponse = { success: boolean; task: SiteAnnouncementSyncTask }

/**
 * Poll one queued site-announcements-sync background task through the shared
 * `api.getTask` endpoint (no second task-status API). Polling stops once the
 * task reaches a terminal status; the page owns the terminal toast +
 * list invalidation.
 */
export function useSiteAnnouncementSyncTask(
  taskId: string | null,
  options?: Omit<
    UseQueryOptions<SiteAnnouncementSyncTask>,
    'queryKey' | 'queryFn'
  >
) {
  return useQuery<SiteAnnouncementSyncTask>({
    queryKey:
      taskId === null
        ? [...siteAnnouncementsKeys.all, 'sync-task', 'none']
        : siteAnnouncementsKeys.syncTask(taskId),
    queryFn: async () => {
      // `enabled` guarantees a taskId; the guard keeps the type checker happy.
      const response = (await api.getTask(taskId as string)) as GetTaskResponse
      return response.task
    },
    enabled: taskId !== null,
    refetchInterval: (query) =>
      query.state.data &&
      TERMINAL_SYNC_TASK_STATUSES.has(query.state.data.status)
        ? false
        : SYNC_TASK_POLL_INTERVAL_MS,
    ...options,
  })
}
