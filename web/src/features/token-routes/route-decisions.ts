import {
  useMutation,
  useQuery,
  useQueryClient,
  type QueryClient,
} from '@tanstack/react-query'

import { accountQueryKeys } from '@/features/accounts'
import { channelsKeys } from '@/features/channels'
import { modelsKeys } from '@/features/models'
import i18n from '@/i18n/config'
import { api } from '@/lib/api'
import { assertBusinessOk } from '@/lib/assert-business-ok'
import { toast } from '@/lib/toast'

import {
  rememberRouteRebuild,
  routeRebuildTaskQueryKey,
} from './lib/route-rebuild-reference'
import { routeQueryKeys } from './route-query-keys'

// Explicit administrator preview; no polling or focus-triggered rechecks.

export function useRouteDecision(
  input: { model: string; endpoint: string } | null
) {
  return useQuery({
    queryKey: routeQueryKeys.decision(input),
    queryFn: async () => {
      if (!input) throw new Error('Route decision input is required')
      const response = await api.getRouteDecision(input)
      assertBusinessOk(response, 'tokenRoutes.preview.failed')
      return response.decision
    },
    enabled: input !== null,
    retry: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  })
}

/** The synchronous result, also returned as a completed rebuild task's result. */

export interface RebuildRoutesResult {
  success?: boolean
  queued?: boolean
  reused?: boolean
  status?: string
  message?: string
  taskId?: string
  jobId?: string
  routesConsidered?: number
  routesCreated?: number
  unsafeModelsSkipped?: number
  patternRoutes?: number
  groupRoutes?: number
  channelsInserted?: number
  channelsRemoved?: number
  channelsKept?: number
  changed?: boolean
  modelRefresh?: {
    total: number
    success: number
    failed: number
    notProcessed: number
  }
}

export function rebuildHasWarnings(result: RebuildRoutesResult): boolean {
  return (
    (result.unsafeModelsSkipped ?? 0) > 0 ||
    (result.modelRefresh?.failed ?? 0) > 0 ||
    (result.modelRefresh?.notProcessed ?? 0) > 0
  )
}

export function invalidateRouteRebuildQueries(queryClient: QueryClient): void {
  for (const queryKey of [
    routeQueryKeys.all,
    channelsKeys.all,
    accountQueryKeys.all,
    modelsKeys.all,
    // Account-model inventory owns this private query namespace.
    ['account-models'] as const,
  ]) {
    void queryClient.invalidateQueries({ queryKey })
  }
}

function showRebuildResult(result: RebuildRoutesResult): void {
  if (rebuildHasWarnings(result)) {
    toast.warning(i18n.t('tokenRoutes.rebuild.partial'))
    return
  }
  const routesConsidered = result.routesConsidered ?? 0
  const created = result.routesCreated ?? 0
  const inserted = result.channelsInserted ?? 0
  const removed = result.channelsRemoved ?? 0
  if (routesConsidered === 0 && created === 0) {
    toast.warning(i18n.t('tokenRoutes.toast.rebuildNoRoutes'))
  } else if (created === 0 && inserted === 0 && removed === 0) {
    toast.info(i18n.t('tokenRoutes.toast.rebuildNoChanges'))
  } else {
    toast.success(
      i18n.t('tokenRoutes.toast.rebuildComplete', {
        routes: routesConsidered,
        inserted,
        removed,
      })
    )
  }
}

export function useRebuildRoutes() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationKey: routeQueryKeys.rebuild(),
    mutationFn: async (options?: {
      refreshModels?: boolean
      wait?: boolean
    }) => {
      const wait = options?.wait ?? false
      const result = assertBusinessOk<RebuildRoutesResult>(
        await api.rebuildRoutes(options?.refreshModels ?? true, wait),
        'tokenRoutes.toast.rebuildFailed'
      )
      if (result?.success !== true) {
        throw new Error(i18n.t('tokenRoutes.rebuild.invalidResponse'))
      }
      if (result.queued) {
        const taskId = result.taskId
        if (typeof taskId !== 'string' || !taskId.trim()) {
          throw new Error(i18n.t('tokenRoutes.rebuild.invalidResponse'))
        }
        return { ...result, queued: true as const, taskId }
      }
      if (!wait) {
        throw new Error(i18n.t('tokenRoutes.rebuild.invalidResponse'))
      }
      return { ...result, queued: false as const }
    },
    onSuccess: (data, options) => {
      if (data.queued) {
        rememberRouteRebuild({
          taskId: data.taskId,
          refreshModels: options?.refreshModels ?? true,
        })
        // A deduplicated launch can return the same id after a failed GET.
        // Explicitly re-query it instead of leaving its observer in error.
        void queryClient.invalidateQueries({
          queryKey: routeRebuildTaskQueryKey(data.taskId),
        })
        toast.info(
          i18n.t(
            data.reused
              ? 'tokenRoutes.rebuild.resumed'
              : 'tokenRoutes.toast.rebuildStarted'
          )
        )
        return
      }
      // A 202 only acknowledges a launch; refresh data after a verified terminal
      // task (useRouteRebuildTask), or after an explicit synchronous completion.
      invalidateRouteRebuildQueries(queryClient)
      showRebuildResult(data)
    },
  })
}

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
