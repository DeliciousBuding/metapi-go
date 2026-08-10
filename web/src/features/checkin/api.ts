// metapi-go features/checkin/api — TanStack Query hooks for the checkin
// domain. Establishes the query-key conventions:
//   ['checkin']                  — root
//   ['checkin', 'logs']          — log list (GET /api/checkin/logs)
//
// The shared axios layer in @/lib/http-client already toasts business
// errors ({success:false}) and HTTP failures, so these hooks keep their
// error handling minimal. The manual-trigger mutations do NOT toast here —
// the trigger-all response carries a summary object and the trigger-one
// response carries a per-account `status` (`success`/`failed`/`skipped`),
// so the owning components decide the surface (success vs. partial-failure
// toast) by inspecting the result. Both mutations invalidate the logs cache
// so the list refreshes after a run.

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'

import { api } from '@/lib/api'

import {
  type CheckinLogRow,
  checkinLogRowSchema,
  triggerCheckinAllResultSchema,
  triggerCheckinResultSchema,
} from './types'

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const checkinQueryKeys = {
  all: ['checkin'] as const,
  logs: () => [...checkinQueryKeys.all, 'logs'] as const,
}

// ---------------------------------------------------------------------------
// Envelope helper — backend returns {success, message, data} for writes.
// http-client already toasted the failure; we only throw to flip the
// mutation to its error state and reject mutateAsync.
// ---------------------------------------------------------------------------

function assertBusinessOk(result: unknown, fallback: string): unknown {
  const envelope = result as { success?: unknown; message?: unknown }
  if (
    envelope &&
    typeof envelope.success === 'boolean' &&
    !envelope.success
  ) {
    throw new Error(
      typeof envelope.message === 'string' ? envelope.message : fallback,
    )
  }
  return result
}

// ---------------------------------------------------------------------------
// useCheckinLogs — GET /api/checkin/logs?limit=&offset=&accountId=
//
// The backend returns a bare JSON array (no pagination envelope) of nested
// row objects. Server-side params are limited to `limit` (1–500, default
// 50), `offset`, and `accountId`; all other filtering (date range, status,
// failure-reason category, text search) is client-side over the fetched
// window. We fetch a large window (default 500) to keep the client-side
// filters meaningful, matching the legacy page's `limit=100` approach but
// wider.
// ---------------------------------------------------------------------------

export interface UseCheckinLogsParams {
  accountId?: number
  limit?: number
  offset?: number
}

export function useCheckinLogs(
  params: UseCheckinLogsParams = {},
  options?: Omit<
    UseQueryOptions<CheckinLogRow[]>,
    'queryKey' | 'queryFn'
  >,
) {
  const { accountId, limit = 500, offset } = params
  return useQuery({
    queryKey: [...checkinQueryKeys.logs(), { accountId, limit, offset }],
    queryFn: async () => {
      const search = new URLSearchParams()
      search.set('limit', String(limit))
      if (offset !== undefined) search.set('offset', String(offset))
      if (accountId) search.set('accountId', String(accountId))
      const raw = await api.getCheckinLogs(search.toString())
      if (!Array.isArray(raw)) return [] as CheckinLogRow[]
      return raw.map((row) => checkinLogRowSchema.parse(row))
    },
    staleTime: 15 * 1000,
    ...options,
  })
}

// ---------------------------------------------------------------------------
// useManualCheckin — POST /api/checkin/trigger (run all eligible accounts)
//
// Returns the full trigger summary so the page can render a result toast
// (total / success / failed / skipped). The mutation does not toast itself.
// ---------------------------------------------------------------------------

export function useManualCheckin() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const result = await api.triggerCheckinAll()
      return triggerCheckinAllResultSchema.parse(
        assertBusinessOk(result, '签到执行失败'),
      )
    },
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: checkinQueryKeys.logs() })
      return result
    },
  })
}

// ---------------------------------------------------------------------------
// useCheckinAccount — POST /api/checkin/trigger/:id (trigger one account)
//
// The HTTP 200 response carries the per-account outcome in `status`
// (`success` / `failed` / `skipped`), so a "successful" mutation may still
// represent a failed checkin. The page inspects `result.status` to choose
// the right toast.
// ---------------------------------------------------------------------------

export function useCheckinAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (accountId: number) => {
      const result = await api.triggerCheckin(accountId)
      return triggerCheckinResultSchema.parse(
        assertBusinessOk(result, '签到执行失败'),
      )
    },
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: checkinQueryKeys.logs() })
      return result
    },
  })
}
