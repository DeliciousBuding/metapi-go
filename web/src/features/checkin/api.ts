// metapi-go features/checkin/api — TanStack Query hooks for the checkin domain.

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'
import type { z } from 'zod'

import i18n from '@/i18n/config'
import { api } from '@/lib/api'
import { assertBusinessOk } from '@/lib/assert-business-ok'

import {
  type CheckinLogsQuery,
  type CheckinLogsResponse,
  checkinLogRowSchema,
  triggerCheckinAllResultSchema,
  triggerCheckinResultSchema,
} from './types'

export const checkinQueryKeys = {
  all: ['checkin'] as const,
  logs: () => [...checkinQueryKeys.all, 'logs'] as const,
  logsList: (params: CheckinLogsQuery) =>
    [...checkinQueryKeys.logs(), params] as const,
}

export type UseCheckinLogsParams = CheckinLogsQuery

/**
 * Build the flat query-param record the backend expects (arrays joined with
 * commas) from the structured `CheckinLogsQuery`. Shared by the hook and the
 * route loader so the prefetched page reuses the hook's cache key exactly.
 */
function buildCheckinLogsParams(
  params: CheckinLogsQuery
): Record<string, string | number | boolean | null | undefined> {
  return {
    limit: params.limit,
    offset: params.offset,
    accountId: params.accountId,
    status: params.status,
    reason: params.reason?.length ? params.reason.join(',') : undefined,
    site: params.site?.length ? params.site.join(',') : undefined,
    from: params.from,
    to: params.to,
    search: params.search,
  }
}

/**
 * Fetch + parse a server-side paginated checkin-logs page. The backend returns
 * `{ items, total, page, pageSize }` (mirroring /api/stats/proxy-logs); each
 * row is parsed defensively with `checkinLogRowSchema` before feature code
 * touches it. Used by the hook and the route loader so the cached payload
 * shape matches exactly.
 */
export async function fetchCheckinLogs(
  params: CheckinLogsQuery
): Promise<CheckinLogsResponse> {
  const raw = await api.getCheckinLogs(buildCheckinLogsParams(params))
  const payload = (raw ?? {}) as Partial<CheckinLogsResponse>
  const rawItems = Array.isArray(payload.items) ? payload.items : []
  const items = rawItems.map((row) => checkinLogRowSchema.parse(row))
  const total = typeof payload.total === 'number' ? payload.total : 0
  const page = typeof payload.page === 'number' ? payload.page : 1
  const pageSize =
    typeof payload.pageSize === 'number'
      ? payload.pageSize
      : (params.limit ?? 0)
  return { items, total, page, pageSize }
}

/**
 * Fetch a server-side paginated + filtered page of checkin logs. The backend
 * returns `{ items, total, page, pageSize }` (mirroring /api/stats/proxy-logs),
 * so the data table runs in manualPagination/manualFiltering mode and the
 * page count reflects the real total — not the legacy 500-row client cap that
 * silently dropped older records from date-range filters.
 */
export function useCheckinLogs(
  params: UseCheckinLogsParams = {},
  options?: Omit<UseQueryOptions<CheckinLogsResponse>, 'queryKey' | 'queryFn'>
) {
  return useQuery({
    queryKey: checkinQueryKeys.logsList(params),
    queryFn: () => fetchCheckinLogs(params),
    placeholderData: (previous) => previous,
    staleTime: 15 * 1000,
    ...options,
  })
}

export type TriggerCheckinAllResult = z.infer<
  typeof triggerCheckinAllResultSchema
>

/**
 * Parse the POST /api/checkin/trigger envelope. The backend answers 200 with
 * `success: (failed == 0)` plus a real per-account summary. A partial
 * completion is a genuine outcome the caller must present (some succeeded,
 * some failed), so it is returned — not thrown — and only an envelope failure
 * that carries no summary at all counts as a hard error. Throwing on every
 * `success:false` here used to swallow the breakdown before any UI could
 * show it.
 */
export function parseTriggerCheckinAllResult(
  raw: unknown
): TriggerCheckinAllResult {
  const parsed = triggerCheckinAllResultSchema.parse(raw)
  if (!parsed.success && !parsed.summary) {
    throw new Error(parsed.message || i18n.t('checkin.toast.triggerFailed'))
  }
  return parsed
}

export function useManualCheckin() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const result = await api.triggerCheckinAll()
      return parseTriggerCheckinAllResult(result)
    },
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: checkinQueryKeys.logs() })
      // Check-in rewrites account balance / last-check-in time; refresh the
      // accounts snapshot so the account page does not keep stale totals.
      void queryClient.invalidateQueries({ queryKey: ['accounts'] })
      return result
    },
  })
}

export function useCheckinAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (accountId: number) => {
      const result = await api.triggerCheckin(accountId)
      return triggerCheckinResultSchema.parse(
        assertBusinessOk(result, 'checkin.toast.triggerFailed')
      )
    },
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: checkinQueryKeys.logs() })
      // Check-in rewrites account balance / last-check-in time; refresh the
      // accounts snapshot so the account page does not keep stale totals.
      void queryClient.invalidateQueries({ queryKey: ['accounts'] })
      return result
    },
  })
}
