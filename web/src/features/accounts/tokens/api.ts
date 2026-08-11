// metapi-go features/accounts/tokens/api — TanStack Query hooks for the
// account-tokens sub-module. These power the TokensPanel embedded inside
// the account detail sheet (not a standalone page — aligned with the
// legacy metapi design where tokens live inside accounts management).
//
// Query keys: ['account-tokens', 'list', accountId].
// Token mutations also invalidate ['accounts'] so the snapshot (which
// carries token counts per account) stays fresh.

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'
import { toast } from 'sonner'

import { api } from '@/lib/api'
import i18n from '@/i18n/config'
import { accountQueryKeys } from '../api'

import { type AccountToken, accountTokenSchema } from '../types'
import type { AccountTokenPayload } from './lib/tokens-schema'

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const accountTokenQueryKeys = {
  all: ['account-tokens'] as const,
  list: (accountId?: number) =>
    ['account-tokens', 'list', accountId ?? 'all'] as const,
  value: (id: number) => ['account-tokens', 'value', id] as const,
}

// ---------------------------------------------------------------------------
// Envelope helper
// ---------------------------------------------------------------------------

function assertBusinessOk<T>(result: unknown, fallback: string): T {
  const envelope = result as {
    success?: unknown
    message?: unknown
    data?: unknown
  }
  if (
    envelope &&
    typeof envelope.success === 'boolean' &&
    !envelope.success
  ) {
    throw new Error(
      typeof envelope.message === 'string' ? envelope.message : i18n.t(fallback),
    )
  }
  return (result as T) ?? (envelope?.data as T)
}

function normalizeTokenList(raw: unknown): AccountToken[] {
  if (Array.isArray(raw)) {
    return raw.map((item) => accountTokenSchema.parse(item))
  }
  const envelope = raw as { tokens?: unknown; data?: { tokens?: unknown } }
  const list = envelope?.tokens ?? envelope?.data?.tokens
  if (Array.isArray(list)) {
    return list.map((item) => accountTokenSchema.parse(item))
  }
  return []
}

// ---------------------------------------------------------------------------
// useAccountTokens — GET /api/account-tokens?accountId=N
// ---------------------------------------------------------------------------

export function useAccountTokens(
  accountId?: number,
  options?: Omit<
    UseQueryOptions<AccountToken[]>,
    'queryKey' | 'queryFn'
  >,
) {
  return useQuery({
    queryKey: accountTokenQueryKeys.list(accountId),
    queryFn: async () => {
      const raw = await api.getAccountTokens(accountId)
      return normalizeTokenList(raw)
    },
    enabled: accountId !== undefined && accountId > 0,
    staleTime: 10 * 1000,
    ...options,
  })
}

// ---------------------------------------------------------------------------
// useAccountTokenValue — GET /api/account-tokens/:id/value (reveal)
// ---------------------------------------------------------------------------

export function useAccountTokenValue(
  id: number | null,
  options?: Omit<
    UseQueryOptions<{ token?: string } & Record<string, unknown>>,
    'queryKey' | 'queryFn'
  >,
) {
  return useQuery({
    queryKey: accountTokenQueryKeys.value(id ?? 0),
    queryFn: async () => {
      const raw = await api.getAccountTokenValue(id as number)
      return assertBusinessOk<{ token?: string } & Record<string, unknown>>(
        raw,
        'accounts.tokens.toast.valueFailed',
      )
    },
    enabled: id !== null && id > 0,
    staleTime: 0,
    ...options,
  })
}

// ---------------------------------------------------------------------------
// useCreateAccountToken — POST /api/account-tokens
// ---------------------------------------------------------------------------

export function useCreateAccountToken() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (payload: AccountTokenPayload) => {
      const result = await api.addAccountToken(payload)
      return assertBusinessOk(result, 'accounts.tokens.toast.createFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountTokenQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      toast.success(i18n.t('accounts.tokens.toast.created'))
    },
  })
}

// ---------------------------------------------------------------------------
// useUpdateAccountToken — PUT /api/account-tokens/:id
// ---------------------------------------------------------------------------

export function useUpdateAccountToken() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      id,
      payload,
    }: {
      id: number
      payload: Partial<AccountTokenPayload>
    }) => {
      const result = await api.updateAccountToken(id, payload)
      return assertBusinessOk(result, 'accounts.tokens.toast.updateFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountTokenQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      toast.success(i18n.t('accounts.tokens.toast.updated'))
    },
  })
}

// ---------------------------------------------------------------------------
// useDeleteAccountToken — DELETE /api/account-tokens/:id
// ---------------------------------------------------------------------------

export function useDeleteAccountToken() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const result = await api.deleteAccountToken(id)
      return assertBusinessOk(result, 'accounts.tokens.toast.deleteFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountTokenQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      toast.success(i18n.t('accounts.tokens.toast.deleted'))
    },
  })
}

// ---------------------------------------------------------------------------
// useSetDefaultAccountToken — POST /api/account-tokens/:id/default
// ---------------------------------------------------------------------------

export function useSetDefaultAccountToken() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const result = await api.setDefaultAccountToken(id)
      return assertBusinessOk(result, 'accounts.tokens.toast.defaultFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountTokenQueryKeys.all })
      toast.success(i18n.t('accounts.tokens.toast.defaultSet'))
    },
  })
}

// ---------------------------------------------------------------------------
// useSyncAccountTokens — POST /api/account-tokens/sync/:accountId
// ---------------------------------------------------------------------------

export function useSyncAccountTokens() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (accountId: number) => {
      const result = await api.syncAccountTokens(accountId)
      return assertBusinessOk(result, 'accounts.tokens.toast.syncFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountTokenQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      toast.success(i18n.t('accounts.tokens.toast.synced'))
    },
  })
}

// ---------------------------------------------------------------------------
// useToggleAccountTokenEnabled
// ---------------------------------------------------------------------------

export function useToggleAccountTokenEnabled() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      id,
      enabled,
    }: {
      id: number
      enabled: boolean
    }) => {
      const result = await api.updateAccountToken(id, { enabled })
      return assertBusinessOk(result, 'accounts.tokens.toast.statusFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountTokenQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
    },
  })
}
