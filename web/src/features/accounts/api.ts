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
  type QueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'

import i18n from '@/i18n/config'
import { api } from '@/lib/api'
import { assertBusinessOk } from '@/lib/assert-business-ok'
import { toast } from '@/lib/toast'

import type {
  Account,
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

/**
 * Post-create token sync statuses reported truthfully by the backend
 * (handler/admin/account_tokens_sync.go). `synced`/`empty`/`skipped` are
 * informational; `failed` means partial initialization — the account row is
 * persisted but its upstream tokens did not sync.
 */
export type TokenSyncStatus = 'synced' | 'empty' | 'failed' | 'skipped'

export interface CreateAccountResult {
  success?: boolean
  message?: string
  id?: number
  items?: Array<{
    id?: number
    status?: 'created' | 'failed'
  }>
  tokenCount?: number
  tokenSyncStatus?: TokenSyncStatus
  tokenSyncMessage?: string
}

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
  tokenCount?: number
  tokenSyncStatus?: TokenSyncStatus
  tokenSyncMessage?: string
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
// useVerifyAccountToken — POST /api/accounts/verify-token (inline credential
// check before save). Passes skipErrorHandler so the shared axios layer does
// not toast a global error: the form renders the verified/failed state inline
// instead. The backend returns non-2xx {"error"} on failure, so assertBusinessOk
// only ever sees success envelopes here.
// ---------------------------------------------------------------------------

export interface VerifyTokenResult {
  tokenType?: string
  modelCount?: number
  models?: unknown[]
  userInfo?: unknown
  balance?: unknown
  apiToken?: string
  apiTokenFound?: boolean
}

export function useVerifyAccountToken() {
  return useMutation({
    mutationFn: async (payload: {
      siteId: number
      accessToken: string
      credentialMode: 'session' | 'apikey'
    }) => {
      const result = await api.verifyToken(payload, { skipErrorHandler: true })
      return assertBusinessOk<VerifyTokenResult>(
        result,
        'accounts.verify.failed'
      )
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
//
// All three toggles run the sites-feature optimistic pattern: onMutate
// patches the snapshot row in place (instant flip), onError rolls the cache
// back to the pre-mutation snapshot, and onSettled invalidates so the server
// truth wins either way. The row stays interactive — the optimistic flip IS
// the feedback; the accounts page keeps its mutation-derived per-row pending
// spinner for the status toggle.
// ---------------------------------------------------------------------------

type AccountToggleContext = { previous: AccountsSnapshot | undefined }

function patchAccountInSnapshot(
  queryClient: QueryClient,
  accountId: number,
  patch: Partial<Account>
) {
  queryClient.setQueryData<AccountsSnapshot>(
    accountQueryKeys.snapshot(),
    (current) =>
      current
        ? {
            ...current,
            accounts: current.accounts.map((account) =>
              account.id === accountId ? { ...account, ...patch } : account
            ),
          }
        : current
  )
}

function rollbackSnapshot(
  queryClient: QueryClient,
  context: AccountToggleContext | undefined
) {
  if (context?.previous) {
    queryClient.setQueryData(accountQueryKeys.snapshot(), context.previous)
  }
}

export function useToggleAccountPin() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({ id, isPinned }: { id: number; isPinned: boolean }) => {
      const result = await api.updateAccount(id, { isPinned })
      return assertBusinessOk(result, 'accounts.toast.pinFailed')
    },
    onMutate: async ({ id, isPinned }) => {
      await queryClient.cancelQueries({ queryKey: accountQueryKeys.snapshot() })
      const previous = queryClient.getQueryData<AccountsSnapshot>(
        accountQueryKeys.snapshot()
      )
      patchAccountInSnapshot(queryClient, id, { isPinned })
      return { previous }
    },
    onError: (_error, _variables, context) => {
      rollbackSnapshot(queryClient, context)
    },
    onSettled: () => {
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
    onMutate: async ({ id, status }) => {
      await queryClient.cancelQueries({ queryKey: accountQueryKeys.snapshot() })
      const previous = queryClient.getQueryData<AccountsSnapshot>(
        accountQueryKeys.snapshot()
      )
      patchAccountInSnapshot(queryClient, id, { status })
      return { previous }
    },
    onError: (_error, _variables, context) => {
      rollbackSnapshot(queryClient, context)
    },
    onSettled: () => {
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
    onMutate: async ({ id, checkinEnabled }) => {
      await queryClient.cancelQueries({ queryKey: accountQueryKeys.snapshot() })
      const previous = queryClient.getQueryData<AccountsSnapshot>(
        accountQueryKeys.snapshot()
      )
      patchAccountInSnapshot(queryClient, id, { checkinEnabled })
      return { previous }
    },
    onError: (_error, _variables, context) => {
      rollbackSnapshot(queryClient, context)
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
    },
  })
}

// ---------------------------------------------------------------------------
// Convenience selector
// ---------------------------------------------------------------------------
