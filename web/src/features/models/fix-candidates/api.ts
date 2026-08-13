// metapi-go/features/models/fix-candidates — TanStack Query hooks.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import i18n from '@/i18n/config'
import { api } from '@/lib/api'

import {
  fixCandidatesQueryKeys,
  redirectFixApplyResponseSchema,
  redirectFixCandidatesResponseSchema,
} from './types'

export function useRedirectFixCandidates() {
  return useQuery({
    queryKey: fixCandidatesQueryKeys.list(),
    queryFn: async () => {
      const raw = await api.getRedirectFixCandidates()
      return redirectFixCandidatesResponseSchema.parse(raw)
    },
    staleTime: 15 * 1000,
  })
}

export function useApplyRedirectFixCandidates() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const raw = await api.applyRedirectFixCandidates()
      return redirectFixApplyResponseSchema.parse(raw)
    },
    onSuccess: (res) => {
      void queryClient.invalidateQueries({
        queryKey: fixCandidatesQueryKeys.all,
      })
      toast.success(
        i18n.t('fixCandidates.toast.applySucceeded', {
          count: res.removed ?? res.count ?? 0,
        })
      )
    },
    onError: (err) => {
      toast.error(
        i18n.t('fixCandidates.toast.applyFailed', {
          message: (err as Error).message,
        })
      )
    },
  })
}
