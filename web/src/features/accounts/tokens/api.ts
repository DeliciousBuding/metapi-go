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
// Envelope helper (mirrors accounts/api.assertBusinessOk)
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
      typeof envelope.message === 'string' ? envelope.message : fallback,
    )
  }
  return (result as T) ?? (envelope?.data as T)
}

// Normalise the getAccountTokens response — backend has shipped both a bare
// array and a {tokens: [...]} envelope across versions, so accept either.
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
        '获取令牌值失败',
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
      return assertBusinessOk(result, '添加令牌失败')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountTokenQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      toast.success('令牌已添加')
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
      return assertBusinessOk(result, '更新令牌失败')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountTokenQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      toast.success('令牌已更新')
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
      return assertBusinessOk(result, '删除令牌失败')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountTokenQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      toast.success('令牌已删除')
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
      return assertBusinessOk(result, '设置默认令牌失败')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountTokenQueryKeys.all })
      toast.success('已设为默认令牌')
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
      return assertBusinessOk(result, '同步令牌失败')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountTokenQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
      toast.success('令牌已同步')
    },
  })
}

// ---------------------------------------------------------------------------
// useToggleAccountTokenEnabled — convenience wrapper around
// updateAccountToken that flips the `enabled` flag.
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
      return assertBusinessOk(result, '更新令牌状态失败')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: accountTokenQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: accountQueryKeys.all })
    },
  })
}
