import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'
import { useMemo } from 'react'

import i18n from '@/i18n/config'
import { api } from '@/lib/api'
import { assertBusinessOk } from '@/lib/assert-business-ok'
import {
  normalizeMissingTokenModels,
  type MissingTokenModelsByName,
} from '@/lib/helpers/routeMissingTokenHints'
import { buildZeroChannelPlaceholderRoutes } from '@/lib/helpers/zeroChannelRoutes'
import { toast } from '@/lib/toast'

import { routeQueryKeys } from './route-query-keys'
import type {
  RouteFormPayload,
  RouteRoutingStrategy,
  RouteSummaryRow,
} from './types'

export type ModelTokenCandidatesResponse = {
  models?: Record<string, unknown[]>
  modelsWithoutToken?: MissingTokenModelsByName
  modelsMissingTokenGroups?: MissingTokenModelsByName
  endpointTypesByModel?: Record<string, string[]>
}

export function useRoutes(
  options?: Omit<UseQueryOptions<RouteSummaryRow[]>, 'queryKey' | 'queryFn'>
) {
  return useQuery({
    queryKey: routeQueryKeys.summary(),
    queryFn: async () => {
      const rows = await api.getRoutesSummary()
      if (!Array.isArray(rows)) {
        throw new Error('Failed to load routes')
      }
      return rows as RouteSummaryRow[]
    },
    staleTime: 10 * 1000,
    ...options,
  })
}

export function useModelTokenCandidates(
  options?: Omit<
    UseQueryOptions<ModelTokenCandidatesResponse>,
    'queryKey' | 'queryFn'
  >
) {
  return useQuery({
    queryKey: routeQueryKeys.candidates(),
    queryFn: async () => {
      const response =
        (await api.getModelTokenCandidates()) as ModelTokenCandidatesResponse
      return {
        models: response?.models ?? {},
        modelsWithoutToken: normalizeMissingTokenModels(
          response?.modelsWithoutToken ?? {}
        ),
        modelsMissingTokenGroups: normalizeMissingTokenModels(
          response?.modelsMissingTokenGroups ?? {}
        ),
        endpointTypesByModel: response?.endpointTypesByModel ?? {},
      }
    },
    staleTime: 30 * 1000,
    ...options,
  })
}

export interface CreateRouteResult {
  success?: boolean
  message?: string
  id?: number
}

export function resolveCreatedRouteId(
  result: CreateRouteResult | undefined
): number | undefined {
  return result?.id && result.id > 0 ? result.id : undefined
}

export function useCreateRoute() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (payload: RouteFormPayload) => {
      const result = await api.addRoute(payload)
      return assertBusinessOk<CreateRouteResult>(
        result,
        'tokenRoutes.toast.createFailed'
      )
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
    },
  })
}

export function useUpdateRoute() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      id,
      payload,
    }: {
      id: number
      payload: Partial<RouteFormPayload> & {
        enabled?: boolean
        routingStrategy?: RouteRoutingStrategy
      }
    }) => {
      const result = await api.updateRoute(id, payload)
      return assertBusinessOk(result, 'tokenRoutes.toast.updateFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      toast.success(i18n.t('tokenRoutes.toast.updated'))
    },
  })
}

export type BatchRouteAction = 'enable' | 'disable'

export function useBatchUpdateRoutes() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      ids,
      action,
    }: {
      ids: number[]
      action: BatchRouteAction
    }) => {
      const result = await api.batchUpdateRoutes({ ids, action })
      return assertBusinessOk<{ updatedCount?: number }>(
        result,
        'tokenRoutes.toast.batchFailed'
      )
    },
    onSuccess: (result, variables) => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      // The backend reports how many rows it actually flipped; anything short
      // of the requested count is a partial success and gets a warning.
      const requestedCount = variables.ids.length
      const updatedCount = result?.updatedCount ?? 0
      if (updatedCount >= requestedCount) {
        toast.success(
          i18n.t('tokenRoutes.toast.batchSucceeded', { count: updatedCount })
        )
      } else {
        toast.warning(
          i18n.t('tokenRoutes.toast.batchPartial', {
            success: updatedCount,
            failed: requestedCount - updatedCount,
          })
        )
      }
    },
  })
}

/**
 * Merge zero-channel placeholder routes into the summary list. Memoized so
 * the returned array keeps a stable identity across renders that change
 * none of the inputs — an unstable reference re-resolves the routes table
 * on every parent render (and re-runs TanStack's autoResetPageIndex).
 */

export function useZeroChannelRoutes(
  routes: RouteSummaryRow[] | undefined,
  candidates: ModelTokenCandidatesResponse | undefined,
  showZeroChannel: boolean
): RouteSummaryRow[] {
  return useMemo(() => {
    const base = routes ?? []
    if (!showZeroChannel || !candidates) return base
    const placeholders = buildZeroChannelPlaceholderRoutes(
      base,
      candidates.modelsWithoutToken ?? {},
      candidates.modelsMissingTokenGroups ?? {}
    )
    return [...base, ...placeholders]
  }, [routes, candidates, showZeroChannel])
}
