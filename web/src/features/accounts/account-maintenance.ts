import { useMutation, useQueryClient } from '@tanstack/react-query'

import i18n from '@/i18n/config'
import { api } from '@/lib/api'
import { assertBusinessOk } from '@/lib/assert-business-ok'
import { toast } from '@/lib/toast'

import { accountQueryKeys } from './account-queries'
import type { BatchAccountResult } from './api'

export function useRefreshAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const result = await api.refreshBalance(id)
      return assertBusinessOk(result, 'accounts.toast.refreshFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      toast.success(i18n.t('accounts.toast.balanceRefreshed'))
    },
  })
}

export type BatchAccountAction =
  | 'enable'
  | 'disable'
  | 'delete'
  | 'refreshBalance'

export function useBatchUpdateAccounts() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      ids,
      action,
    }: {
      ids: number[]
      action: BatchAccountAction
    }) => {
      const result = await api.batchUpdateAccounts({ ids, action })
      return assertBusinessOk<BatchAccountResult>(
        result,
        'accounts.toast.batchFailed'
      )
    },
    onSuccess: (result) => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      const failedItems = result?.failedItems ?? []
      if (failedItems.length > 0) {
        const failedList = failedItems.map((item) => `#${item.id}`).join(', ')
        toast.warning(
          i18n.t('accounts.toast.bulkPartial', {
            success: result?.successIds?.length ?? 0,
            failed: failedItems.length,
            items: failedList,
          })
        )
      }
    },
  })
}
