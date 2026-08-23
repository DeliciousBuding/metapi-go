// metapi-go features/token-routes/api — TanStack Query hooks for the route
// configuration domain.
//
// Toast messages use `i18n.t()` directly (mutations fire outside React
// render cycle; toasts are ephemeral so language-change reactivity is moot).

import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from '@tanstack/react-query'
import { useMemo } from 'react'

import { channelsKeys } from '@/features/channels/types'
import i18n from '@/i18n/config'
import { api } from '@/lib/api'
import { assertBusinessOk } from '@/lib/assert-business-ok'
import {
  normalizeMissingTokenModels,
  type MissingTokenModelsByName,
} from '@/lib/helpers/routeMissingTokenHints'
import { buildZeroChannelPlaceholderRoutes } from '@/lib/helpers/zeroChannelRoutes'
import { toast } from '@/lib/toast'

import type {
  RouteChannel,
  RouteFormPayload,
  RouteRoutingStrategy,
  RouteSummaryRow,
} from './types'

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const routeQueryKeys = {
  all: ['routes'] as const,
  summary: () => [...routeQueryKeys.all, 'summary'] as const,
  candidates: () => [...routeQueryKeys.all, 'candidates'] as const,
  channels: (id: number) => ['routes', 'channels', id] as const,
  channelsAll: () => ['routes', 'channels'] as const,
}

// ---------------------------------------------------------------------------
// Model token candidates
// ---------------------------------------------------------------------------

export type ModelTokenCandidatesResponse = {
  models?: Record<string, unknown[]>
  modelsWithoutToken?: MissingTokenModelsByName
  modelsMissingTokenGroups?: MissingTokenModelsByName
  endpointTypesByModel?: Record<string, string[]>
}

// ---------------------------------------------------------------------------
// useRoutes — summary list
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// useModelTokenCandidates
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// useRouteChannels
// ---------------------------------------------------------------------------

export function useRouteChannels(
  routeId: number | null,
  options?: Omit<
    UseQueryOptions<RouteChannel[]>,
    'queryKey' | 'queryFn' | 'enabled'
  >
) {
  return useQuery({
    queryKey: routeQueryKeys.channels(routeId ?? 0),
    queryFn: async () => {
      const channels = await api.getRouteChannels(routeId as number)
      if (!Array.isArray(channels)) return [] as RouteChannel[]
      return channels as RouteChannel[]
    },
    enabled: routeId != null,
    staleTime: 15 * 1000,
    ...options,
  })
}

// ---------------------------------------------------------------------------
// useCreateRoute
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// useUpdateRoute
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// useDeleteRoute
// ---------------------------------------------------------------------------

export function useDeleteRoute() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const result = await api.deleteRoute(id)
      return assertBusinessOk(result, 'tokenRoutes.toast.deleteFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      toast.success(i18n.t('tokenRoutes.toast.deleted'))
    },
  })
}

// ---------------------------------------------------------------------------
// useBatchUpdateRoutes
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// useClearRouteCooldown
// ---------------------------------------------------------------------------

export function useClearRouteCooldown() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const result = await api.clearRouteCooldown(id)
      return assertBusinessOk(result, 'tokenRoutes.toast.cooldownClearFailed')
    },
    onSuccess: (_data, routeId) => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      // The route detail sheet renders cooldown badges from the per-route
      // channel list; invalidate it so the badges (and the gated clear
      // button) flip back to enabled immediately instead of after the
      // 15s staleTime.
      void queryClient.invalidateQueries({
        queryKey: routeQueryKeys.channels(routeId),
      })
      // Channels page status badges (`cooldown`/`breaker_open`) derive from
      // the same rows — refresh them too.
      void queryClient.invalidateQueries({ queryKey: channelsKeys.list() })
      toast.success(i18n.t('tokenRoutes.toast.cooldownCleared'))
    },
  })
}

// ---------------------------------------------------------------------------
// useUpdateChannel
// ---------------------------------------------------------------------------

/**
 * Per-channel operator edits (weight / priority / enabled). The backend
 * marks `manual_override=true` on any edit so a route rebuild cannot wipe
 * operator tuning; the route detail sheet and channel detail sheet surface
 * the same fields, so the summary + both channel lists are invalidated.
 */
export function useUpdateChannel() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      id,
      data,
    }: {
      id: number
      data: { weight?: number; priority?: number; enabled?: boolean }
    }) => {
      const result = await api.updateChannel(id, data)
      return assertBusinessOk(result, 'tokenRoutes.toast.channelUpdateFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.channelsAll() })
      void queryClient.invalidateQueries({ queryKey: channelsKeys.list() })
      toast.success(i18n.t('tokenRoutes.toast.channelUpdated'))
    },
  })
}

// ---------------------------------------------------------------------------
// useDeleteChannel
// ---------------------------------------------------------------------------

export function useDeleteChannel() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const result = await api.deleteChannel(id)
      return assertBusinessOk(result, 'tokenRoutes.toast.channelDeleteFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.channelsAll() })
      void queryClient.invalidateQueries({ queryKey: channelsKeys.list() })
      toast.success(i18n.t('tokenRoutes.toast.channelDeleted'))
    },
  })
}

// ---------------------------------------------------------------------------
// useBatchAddChannels
// ---------------------------------------------------------------------------

export interface BatchAddChannelsResult {
  created?: number
  skipped?: number
  errors?: string[]
}

export function useBatchAddChannels() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      routeId,
      channels,
    }: {
      routeId: number
      channels: Array<{
        accountId: number
        tokenId?: number
        sourceModel?: string
      }>
    }) => {
      const result = await api.batchAddChannels(routeId, channels)
      return assertBusinessOk<BatchAddChannelsResult>(
        result,
        'tokenRoutes.toast.channelAddFailed'
      )
    },
    onSuccess: (_data, variables) => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      void queryClient.invalidateQueries({
        queryKey: routeQueryKeys.channels(variables.routeId),
      })
    },
  })
}

// ---------------------------------------------------------------------------
// useRebuildRoutes
// ---------------------------------------------------------------------------

export interface RebuildRoutesResult {
  queued?: boolean
  created?: number
  channelCount?: number
}

export function useRebuildRoutes() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (options?: {
      refreshModels?: boolean
      wait?: boolean
    }) => {
      const result = await api.rebuildRoutes(
        options?.refreshModels ?? true,
        options?.wait ?? false
      )
      return assertBusinessOk<RebuildRoutesResult>(
        result,
        'tokenRoutes.toast.rebuildFailed'
      )
    },
    onSuccess: (data) => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      if (data?.queued) {
        toast.info(i18n.t('tokenRoutes.toast.rebuildStarted'))
      } else {
        toast.success(
          i18n.t('tokenRoutes.toast.rebuildComplete', {
            created: data?.created ?? 0,
            channels: data?.channelCount ?? 0,
          })
        )
      }
    },
  })
}

// ---------------------------------------------------------------------------
// useRefreshRouteDecisions
// ---------------------------------------------------------------------------

export function useRefreshRouteDecisions() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const result = await api.refreshRouteDecisionSnapshots()
      return assertBusinessOk<{ jobId?: string }>(
        result,
        'tokenRoutes.toast.refreshFailed'
      )
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      toast.info(i18n.t('tokenRoutes.toast.refreshStarted'))
    },
  })
}

// ---------------------------------------------------------------------------
// useZeroChannelRoutes
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Convenience selectors
// ---------------------------------------------------------------------------
