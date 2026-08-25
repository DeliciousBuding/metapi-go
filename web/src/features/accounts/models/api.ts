// metapi-go features/accounts/models/api — TanStack Query hooks for the
// account Models panel (#998). The panel reuses the existing owners instead
// of adding new ones:
//   - upstream refresh → POST /api/models/check/{accountId} (the manual
//     refresh entrypoint that persists into model_availability and rebuilds
//     routes exactly once per action);
//   - manual add/remove → POST /api/accounts/{id}/models/manual
//     ({models} adds, {remove} deletes manual rows explicitly).
// Transport goes through the shared `request` helper; the hooks live in the
// feature module so the accounts lib barrel stays integration-owned.

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'

import i18n from '@/i18n/config'
import { request } from '@/lib/api/transport'
import { assertBusinessOk } from '@/lib/assert-business-ok'
import { toast } from '@/lib/toast'

import { accountQueryKeys } from '../api'

// ---------------------------------------------------------------------------
// Wire types (camelCase, matching the Go handlers)
// ---------------------------------------------------------------------------

export interface AccountModelEntry {
  name: string
  available: boolean
  latencyMs?: number | null
  disabled?: boolean
  isManual?: boolean
  checkedAt?: string | null
}

export interface AccountModelsResponse {
  siteId?: number
  siteName?: string
  models?: AccountModelEntry[]
  totalCount?: number
  disabledCount?: number
}

export interface ManualModelsResult {
  success?: boolean
  added?: number
  removed?: number
  message?: string
}

export interface RefreshModelsResult {
  success?: boolean
  message?: string
  refresh?: {
    id?: number
    status?: string
    modelCount?: number
    models?: string[]
    checkedAt?: string
    errorCode?: string
    errorMessage?: string
  }
  rebuild?: Record<string, unknown>
}

export interface ManualModelsPayload {
  models?: string[]
  remove?: string[]
}

// ---------------------------------------------------------------------------
// Query keys: ['account-models', 'list', accountId]
// ---------------------------------------------------------------------------

const accountModelQueryKeys = {
  all: ['account-models'] as const,
  list: (accountId?: number) =>
    ['account-models', 'list', accountId ?? 'none'] as const,
}

function normalizeModels(raw: unknown): AccountModelEntry[] {
  if (!Array.isArray(raw)) return []
  return raw
    .filter(
      (item): item is Record<string, unknown> =>
        typeof item === 'object' && item !== null
    )
    .map((item) => ({
      name: typeof item.name === 'string' ? item.name : '',
      available: item.available === true,
      latencyMs: typeof item.latencyMs === 'number' ? item.latencyMs : null,
      disabled: item.disabled === true,
      isManual: item.isManual === true,
      checkedAt: typeof item.checkedAt === 'string' ? item.checkedAt : null,
    }))
    .filter((entry) => entry.name !== '')
}

// ---------------------------------------------------------------------------
// useAccountModels — GET /api/accounts/{id}/models
// ---------------------------------------------------------------------------

export function useAccountModels(
  accountId?: number,
  options?: Omit<UseQueryOptions<AccountModelsResponse>, 'queryKey' | 'queryFn'>
) {
  return useQuery({
    queryKey: accountModelQueryKeys.list(accountId),
    queryFn: async () => {
      const result = await request<AccountModelsResponse>(
        `/api/accounts/${accountId}/models`
      )
      return {
        siteId: result?.siteId,
        siteName: result?.siteName,
        totalCount: result?.totalCount,
        disabledCount: result?.disabledCount,
        models: normalizeModels(result?.models),
      }
    },
    enabled: accountId !== undefined && accountId > 0,
    staleTime: 10 * 1000,
    ...options,
  })
}

// ---------------------------------------------------------------------------
// useRefreshAccountModels — POST /api/models/check/{accountId}
//
// Honest outcome contract: the endpoint answers 200 with success:false on
// upstream failure, so skipBusinessError keeps the shared layer from
// toasting and the panel surfaces the failure inline (mutation error state).
// ---------------------------------------------------------------------------

export function useRefreshAccountModels(accountId?: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      if (!accountId) throw new Error('missing account id')
      const result = await request<RefreshModelsResult>(
        `/api/models/check/${accountId}`,
        { method: 'POST', skipBusinessError: true }
      )
      return assertBusinessOk<RefreshModelsResult>(
        result,
        'accounts.models.refreshFailed'
      )
    },
    onSuccess: (result) => {
      void queryClient.invalidateQueries({
        queryKey: accountModelQueryKeys.all,
      })
      void queryClient.invalidateQueries({
        queryKey: accountQueryKeys.all,
      })
      toast.success(
        i18n.t('accounts.models.refreshedToast', {
          count: result?.refresh?.modelCount ?? 0,
          defaultValue: 'Models refreshed ({{count}} available)',
        })
      )
    },
  })
}

// ---------------------------------------------------------------------------
// useUpdateManualModels — POST /api/accounts/{id}/models/manual
//
// One mutation covers both directions: {models:[…]} adds manual rows,
// {remove:[…]} deletes them explicitly. Success invalidates the models query
// so the list re-renders from persisted store truth — never from an
// optimistic guess.
// ---------------------------------------------------------------------------

export function useUpdateManualModels(accountId?: number) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (payload: ManualModelsPayload) => {
      if (!accountId) throw new Error('missing account id')
      const result = await request<ManualModelsResult>(
        `/api/accounts/${accountId}/models/manual`,
        { method: 'POST', body: JSON.stringify(payload) }
      )
      return assertBusinessOk<ManualModelsResult>(
        result,
        'accounts.models.manualFailed'
      )
    },
    onSuccess: (result, payload) => {
      void queryClient.invalidateQueries({
        queryKey: accountModelQueryKeys.all,
      })
      void queryClient.invalidateQueries({
        queryKey: accountQueryKeys.all,
      })
      if ((payload.remove?.length ?? 0) > 0) {
        toast.success(
          i18n.t('accounts.models.removedToast', {
            count: result?.removed ?? 0,
            defaultValue: 'Removed {{count}} manual model(s)',
          })
        )
      } else {
        toast.success(
          i18n.t('accounts.models.addedToast', {
            count: result?.added ?? 0,
            defaultValue: 'Added {{count}} manual model(s)',
          })
        )
      }
    },
  })
}
