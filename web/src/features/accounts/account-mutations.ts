import { useMutation, useQueryClient } from '@tanstack/react-query'

import i18n from '@/i18n/config'
import { api } from '@/lib/api'
import { assertBusinessOk } from '@/lib/assert-business-ok'
import { toast } from '@/lib/toast'

import { accountQueryKeys } from './account-queries'
import type { CreateAccountResult, LoginAccountResult } from './api'
import type { AccountPayload, LoginAccountPayload } from './types'

export function resolveCreatedAccountId(
  result: CreateAccountResult | undefined
): number | undefined {
  if (result?.id && result.id > 0) return result.id

  const createdItem = result?.items?.find(
    (item) => item.status === 'created' && item.id && item.id > 0
  )
  if (createdItem?.id) return createdItem.id

  return undefined
}

type CreateAccountVariables = {
  payload: AccountPayload
  /** Present when replaying after the master-token reauth prompt. */
  confirmToken?: string
}

export function useCreateAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ payload, confirmToken }: CreateAccountVariables) => {
      const result = await api.addAccount(payload, confirmToken)
      return assertBusinessOk<CreateAccountResult>(
        result,
        'accounts.toast.createFailed'
      )
    },
    onSuccess: (data) => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      return data
    },
  })
}

// ---------------------------------------------------------------------------
// useLoginAccount — POST /api/accounts/login (password-mode binding)
// ---------------------------------------------------------------------------

export function useLoginAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (payload: LoginAccountPayload) => {
      const result = await api.loginAccount(payload)
      return assertBusinessOk<LoginAccountResult>(
        result,
        'accounts.toast.loginFailed'
      )
    },
    onSuccess: (data) => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      return data
    },
  })
}

type UpdateAccountVariables = {
  id: number
  payload: AccountPayload
  /** Present when replaying after the master-token reauth prompt. */
  confirmToken?: string
}

export function useUpdateAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      id,
      payload,
      confirmToken,
    }: UpdateAccountVariables) => {
      const result = await api.updateAccount(id, payload, confirmToken)
      return assertBusinessOk(result, 'accounts.toast.updateFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      toast.success(i18n.t('accounts.toast.updated'))
    },
  })
}

export function useDeleteAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const result = await api.deleteAccount(id)
      return assertBusinessOk(result, 'accounts.toast.deleteFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      toast.success(i18n.t('accounts.toast.deleted'))
    },
  })
}
