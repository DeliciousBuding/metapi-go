// metapi-go features/accounts/api — TanStack Query hooks for the accounts
// domain. Establishes the query-key conventions for the rewrite:
//   ['accounts']               — snapshot list (GET /api/accounts)
//   ['account-tokens', id]      — tokens for an account (see tokens/api.ts)
//
// Mutations wrap the flat `api` object from @/lib/api. The shared axios layer
// in @/lib/http-client already toasts business errors ({success:false}) and
// HTTP failures, so these hooks keep their own error handling minimal: the
// mutationFn throws on a `success:false` body so useMutation transitions to
// error state (and so mutateAsync rejects in the form handler), while
// success-side cache invalidation + UI toasts live in the components.

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'

import i18n from '@/i18n/config'
import { api } from '@/lib/api'
import { assertBusinessOk } from '@/lib/assert-business-ok'
import { toast } from '@/lib/toast'

import type {
  AccountPayload,
  AccountStatus,
  AccountsSnapshot,
  LoginAccountPayload,
} from './types'

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const accountQueryKeys = {
  all: ['accounts'] as const,
  snapshot: () => [...accountQueryKeys.all, 'snapshot'] as const,
  detail: (id: number) => ['accounts', 'detail', id] as const,
}

// ---------------------------------------------------------------------------
// useAccounts — snapshot list (accounts + sites + generatedAt)
// ---------------------------------------------------------------------------

export function useAccounts(
  options?: Omit<UseQueryOptions<AccountsSnapshot>, 'queryKey' | 'queryFn'>
) {
  return useQuery({
    queryKey: accountQueryKeys.snapshot(),
    queryFn: async () => {
      const snapshot = await api.getAccountsSnapshot()
      if (!snapshot || !Array.isArray(snapshot.accounts)) {
        throw new Error('Failed to load accounts snapshot')
      }
      return snapshot as AccountsSnapshot
    },
    staleTime: 10 * 1000,
    ...options,
  })
}

// ---------------------------------------------------------------------------
// useCreateAccount — POST /api/accounts
// ---------------------------------------------------------------------------

export interface CreateAccountResult {
  success?: boolean
  message?: string
  data?: { id?: number; account?: { id?: number } } & Record<string, unknown>
}

export function useCreateAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (payload: AccountPayload) => {
      const result = await api.addAccount(payload)
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

export interface LoginAccountResult {
  success?: boolean
  message?: string
  account?: { id?: number; username?: string }
  reusedAccount?: boolean
}

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

// ---------------------------------------------------------------------------
// useUpdateAccount — PUT /api/accounts/:id
// ---------------------------------------------------------------------------

export function useUpdateAccount() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      id,
      payload,
    }: {
      id: number
      payload: AccountPayload
    }) => {
      const result = await api.updateAccount(id, payload)
      return assertBusinessOk(result, 'accounts.toast.updateFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      toast.success(i18n.t('accounts.toast.updated'))
    },
  })
}

// ---------------------------------------------------------------------------
// useRefreshAccount — POST /api/accounts/:id/balance (per-row 余额刷新)
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// useDeleteAccount — DELETE /api/accounts/:id
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// useBatchUpdateAccounts — POST /api/accounts/batch { ids, action }
// ---------------------------------------------------------------------------

export type BatchAccountAction =
  | 'enable'
  | 'disable'
  | 'delete'
  | 'refreshBalance'

export interface BatchAccountResult {
  successIds?: number[]
  failedItems?: Array<{ id: number; message?: string }>
}

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

// ---------------------------------------------------------------------------
// Field-level toggle mutations
// ---------------------------------------------------------------------------

export function useToggleAccountPin() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, isPinned }: { id: number; isPinned: boolean }) => {
      const result = await api.updateAccount(id, { isPinned })
      return assertBusinessOk(result, 'accounts.toast.pinFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
    },
  })
}

export function useToggleAccountStatus() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      id,
      status,
    }: {
      id: number
      status: AccountStatus
    }) => {
      const result = await api.updateAccount(id, { status })
      return assertBusinessOk(result, 'accounts.toast.statusFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
    },
  })
}

export function useToggleAccountCheckin() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      id,
      checkinEnabled,
    }: {
      id: number
      checkinEnabled: boolean
    }) => {
      const result = await api.updateAccount(id, { checkinEnabled })
      return assertBusinessOk(result, 'accounts.toast.checkinFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
    },
  })
}

// ---------------------------------------------------------------------------
// Convenience selector
// ---------------------------------------------------------------------------
