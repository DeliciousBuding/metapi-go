// metapi-go features/checkin/api — TanStack Query hooks for the checkin domain.
// i18n: fallback strings use i18n.t().

import { useMutation, useQuery, useQueryClient, type UseQueryOptions } from '@tanstack/react-query'

import { api } from '@/lib/api'
import i18n from '@/i18n/config'

import { type CheckinLogRow, checkinLogRowSchema, triggerCheckinAllResultSchema, triggerCheckinResultSchema } from './types'

export const checkinQueryKeys = {
  all: ['checkin'] as const,
  logs: () => [...checkinQueryKeys.all, 'logs'] as const,
}

function assertBusinessOk(result: unknown, fallback: string): unknown {
  const envelope = result as { success?: unknown; message?: unknown }
  if (envelope && typeof envelope.success === 'boolean' && !envelope.success) {
    throw new Error(typeof envelope.message === 'string' ? envelope.message : i18n.t(fallback))
  }
  return result
}

export interface UseCheckinLogsParams { accountId?: number; limit?: number; offset?: number }

export function useCheckinLogs(params: UseCheckinLogsParams = {}, options?: Omit<UseQueryOptions<CheckinLogRow[]>, 'queryKey' | 'queryFn'>) {
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

export function useManualCheckin() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const result = await api.triggerCheckinAll()
      return triggerCheckinAllResultSchema.parse(assertBusinessOk(result, 'checkin.toast.triggerFailed'))
    },
    onSuccess: (result) => { void queryClient.invalidateQueries({ queryKey: checkinQueryKeys.logs() }); return result },
  })
}

export function useCheckinAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (accountId: number) => {
      const result = await api.triggerCheckin(accountId)
      return triggerCheckinResultSchema.parse(assertBusinessOk(result, 'checkin.toast.triggerFailed'))
    },
    onSuccess: (result) => { void queryClient.invalidateQueries({ queryKey: checkinQueryKeys.logs() }); return result },
  })
}
