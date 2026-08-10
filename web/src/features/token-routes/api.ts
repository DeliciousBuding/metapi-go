// metapi-go features/token-routes/api — TanStack Query hooks for the route
// configuration domain. Establishes the query-key conventions for the rewrite:
//   ['routes']                    — summary list (GET /api/routes/summary)
//   ['routes', 'candidates']      — model token candidates (GET /api/models/token-candidates)
//   ['routes', 'channels', id]    — channels for a route (GET /api/routes/:id/channels)
//
// Mutations wrap the flat `api` object from @/lib/api. The shared axios layer
// in @/lib/http-client already toasts business errors ({success:false}) and
// HTTP failures, so these hooks keep their own error handling minimal: the
// mutationFn throws on a `success:false` body so useMutation transitions to
// error state (and so mutateAsync rejects in the form handler), while
// success-side cache invalidation + UI toasts live in the components.
//
// The zero-channel placeholder adapter (`useZeroChannelRoutes`) merges the
// summary list with the missing-token hints from the candidates endpoint via
// the migrated `@/lib/helpers/zeroChannelRoutes` helper, producing the virtual
// `RouteSummaryRow[]` the data-table renders (kind: 'zero_channel').

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
// Envelope helper — backend returns {success, message, data} for writes.
// http-client already toasted the failure; we only throw to flip the mutation
// to its error state and reject mutateAsync.
// ---------------------------------------------------------------------------

function assertBusinessOk<T>(result: unknown, fallback: string): T {
  const envelope = result as { success?: unknown; message?: unknown; data?: unknown }
  if (
    envelope &&
    typeof envelope.success === 'boolean' &&
    !envelope.success
  ) {
    throw new Error(typeof envelope.message === 'string' ? envelope.message : fallback)
  }
  return (result as T) ?? (envelope?.data as T)
}

// ---------------------------------------------------------------------------
// Model token candidates — the shape returned by GET /api/models/token-candidates
// (verified against legacy master:web/pages/TokenRoutes.tsx:264-273).
// ---------------------------------------------------------------------------

export type ModelTokenCandidatesResponse = {
  models?: Record<string, unknown[]>
  modelsWithoutToken?: MissingTokenModelsByName
  modelsMissingTokenGroups?: MissingTokenModelsByName
  endpointTypesByModel?: Record<string, string[]>
}

// ---------------------------------------------------------------------------
// useRoutes — summary list (GET /api/routes/summary)
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
// useModelTokenCandidates — GET /api/models/token-candidates
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
// useRouteChannels — GET /api/routes/:id/channels (detail view)
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
// useCreateRoute — POST /api/routes
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
      return assertBusinessOk<CreateRouteResult>(result, '添加路由失败')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      // The create form owns the success surface — it fires the guided
      // "configuration complete" toast (route-completion-toast.tsx) rather
      // than a plain confirmation, so this hook stays toast-free.
    },
  })
}

// ---------------------------------------------------------------------------
// useUpdateRoute — PUT /api/routes/:id (sparse updates: enabled / strategy /
// contextLength / full form payload)
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
      return assertBusinessOk(result, '更新路由失败')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      toast.success('路由已更新')
    },
  })
}

// ---------------------------------------------------------------------------
// useDeleteRoute — DELETE /api/routes/:id
// ---------------------------------------------------------------------------

export function useDeleteRoute() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const result = await api.deleteRoute(id)
      return assertBusinessOk(result, '删除路由失败')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      toast.success('路由已删除')
    },
  })
}

// ---------------------------------------------------------------------------
// useBatchUpdateRoutes — POST /api/routes/batch { ids, action }
// action: 'enable' | 'disable'
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
      return assertBusinessOk(result, '批量操作失败')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
    },
  })
}

// ---------------------------------------------------------------------------
// useClearRouteCooldown — POST /api/routes/:id/cooldown/clear
// ---------------------------------------------------------------------------

export function useClearRouteCooldown() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (id: number) => {
      const result = await api.clearRouteCooldown(id)
      return assertBusinessOk(result, '清除冷却失败')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      toast.success('已清除冷却')
    },
  })
}

// ---------------------------------------------------------------------------
// useBatchAddChannels — POST /api/routes/:id/channels/batch
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
      return assertBusinessOk<BatchAddChannelsResult>(result, '添加通道失败')
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
// useRebuildRoutes — POST /api/routes/rebuild (async, server-side)
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
      return assertBusinessOk<RebuildRoutesResult>(result, '重建路由失败')
    },
    onSuccess: (data) => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      if (data?.queued) {
        toast.info('已开始重建路由，请稍后查看日志')
      } else {
        toast.success(
          `自动重建完成（新增 ${data?.created ?? 0} 条路由 / ${data?.channelCount ?? 0} 个通道）`,
        )
      }
    },
  })
}

// ---------------------------------------------------------------------------
// useRefreshRouteDecisions — POST /api/routes/decision/refresh (async job)
// ---------------------------------------------------------------------------

export function useRefreshRouteDecisions() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async () => {
      const result = await api.refreshRouteDecisionSnapshots()
      return assertBusinessOk<{ jobId?: string }>(result, '刷新决策失败')
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: routeQueryKeys.all })
      toast.info('已开始后台刷新路由选中概率，可稍后返回查看')
    },
  })
}

// ---------------------------------------------------------------------------
// useZeroChannelRoutes — selector hook that merges the summary list with the
// missing-token hints (built by `buildZeroChannelPlaceholderRoutes`).
// Returns the combined `RouteSummaryRow[]` the data-table renders. Toggle
// `showZeroChannel` to include/exclude the virtual placeholder rows.
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
