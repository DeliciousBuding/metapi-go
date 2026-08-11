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
import { toast } from 'sonner'

import { api } from '@/lib/api'
import {
  buildZeroChannelPlaceholderRoutes,
} from '@/lib/helpers/zeroChannelRoutes'
import {
  normalizeMissingTokenModels,
  type MissingTokenModelsByName,
} from '@/lib/helpers/routeMissingTokenHints'
import i18n from '@/i18n/config'

import {
  type RouteChannel,
  type RouteFormPayload,
  type RouteRoutingStrategy,
  type RouteSummaryRow,
} from './types'

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const routeQueryKeys = {
  all: ['routes'] as const,
  summary: () => [...routeQueryKeys.all, 'summary'] as const,
  candidates: () => [...routeQueryKeys.all, 'candidates'] as const,
  channels: (id: number) => ['routes', 'channels', id] as const,
}

// ---------------------------------------------------------------------------
// Envelope helper
// ---------------------------------------------------------------------------

function assertBusinessOk<T>(result: unknown, fallback: string): T {
  const envelope = result as { success?: unknown; message?: unknown; data?: unknown }
  if (
    envelope &&
    typeof envelope.success === 'boolean' &&
    !envelope.success
  ) {
    throw new Error(typeof envelope.message === 'string' ? envelope.message : i18n.t(fallback))
  }
  return (result as T) ?? (envelope?.data as T)
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
  options?: Omit<
    UseQueryOptions<RouteSummaryRow[]>,
    'queryKey' | 'queryFn'
  >,
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
  >,
) {
  return useQuery({
    queryKey: routeQueryKeys.candidates(),
    queryFn: async () => {
      const response = (await api.getModelTokenCandidates()) as ModelTokenCandidatesResponse
      return {
        models: response?.models ?? {},
        modelsWithoutToken: normalizeMissingTokenModels(
          response?.modelsWithoutToken ?? {},
        ),
        modelsMissingTokenGroups: normalizeMissingTokenModels(
          response?.modelsMissingTokenGroups ?? {},
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
  >,
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
  data?: { id?: number } & Record<string, unknown>
}

export function useCreateRoute() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (payload: RouteFormPayload) => {
      const result = await api.addRoute(payload)
      return assertBusinessOk<CreateRouteResult>(result, 'tokenRoutes.toast.createFailed')
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
      return assertBusinessOk(result, 'tokenRoutes.toast.batchFailed')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
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
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      toast.success(i18n.t('tokenRoutes.toast.cooldownCleared'))
    },
  })
}

// ---------------------------------------------------------------------------
// useBatchAddChannels
// ---------------------------------------------------------------------------

export interface BatchAddChannelsResult {
  created?: number
  skipped?: number
  errors?: Array<{ accountId?: number; message?: string }>
}

export function useBatchAddChannels() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async ({
      routeId,
      channels,
    }: {
      routeId: number
      channels: Array<{ accountId: number; tokenId?: number; sourceModel?: string }>
    }) => {
      const result = await api.batchAddChannels(routeId, channels)
      return assertBusinessOk<BatchAddChannelsResult>(result, 'tokenRoutes.toast.channelAddFailed')
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
    mutationFn: async (options?: { refreshModels?: boolean; wait?: boolean }) => {
      const result = await api.rebuildRoutes(
        options?.refreshModels ?? true,
        options?.wait ?? false,
      )
      return assertBusinessOk<RebuildRoutesResult>(result, 'tokenRoutes.toast.rebuildFailed')
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
          }),
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
      return assertBusinessOk<{ jobId?: string }>(result, 'tokenRoutes.toast.refreshFailed')
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

export function useZeroChannelRoutes(
  routes: RouteSummaryRow[] | undefined,
  candidates: ModelTokenCandidatesResponse | undefined,
  showZeroChannel: boolean,
): RouteSummaryRow[] {
  const base = routes ?? []
  if (!showZeroChannel || !candidates) return base
  const placeholders = buildZeroChannelPlaceholderRoutes(
    base,
    candidates.modelsWithoutToken ?? {},
    candidates.modelsMissingTokenGroups ?? {},
  )
  return [...base, ...placeholders]
}

// ---------------------------------------------------------------------------
// Convenience selectors
// ---------------------------------------------------------------------------

export function selectRouteById(
  routes: RouteSummaryRow[] | undefined,
  id: number,
): RouteSummaryRow | undefined {
  return routes?.find((route) => route.id === id)
}
