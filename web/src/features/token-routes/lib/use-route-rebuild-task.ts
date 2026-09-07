import { useIsMutating, useQuery, useQueryClient } from '@tanstack/react-query'
import { useEffect, useMemo, useRef, useSyncExternalStore } from 'react'

import i18n from '@/i18n/config'
import { api } from '@/lib/api'

import {
  invalidateRouteRebuildQueries,
  routeQueryKeys,
  type RebuildRoutesResult,
} from '../api'
import {
  parseRouteRebuildReference,
  readRouteRebuildSnapshot,
  rememberRouteRebuild,
  routeRebuildTaskQueryKey,
  subscribeRouteRebuild,
} from './route-rebuild-reference'

export type RouteRebuildTask = {
  id: string
  type: 'routes-rebuild'
  status: 'pending' | 'running' | 'succeeded' | 'failed'
  message?: string
  error?: string | null
  result?: RebuildRoutesResult | null
}

const TASK_STATUSES = new Set(['pending', 'running', 'succeeded', 'failed'])
const isTerminal = (task: RouteRebuildTask | undefined) =>
  task?.status === 'succeeded' || task?.status === 'failed'

function hasCompleteResult(
  result: RebuildRoutesResult | null | undefined
): boolean {
  if (!result || result.success !== true) return false
  const counts = [
    result.routesCreated,
    result.routesConsidered,
    result.channelsInserted,
    result.channelsRemoved,
    result.channelsKept,
    result.unsafeModelsSkipped,
  ]
  if (result.modelRefresh) {
    counts.push(
      result.modelRefresh.total,
      result.modelRefresh.success,
      result.modelRefresh.failed,
      result.modelRefresh.notProcessed
    )
  }
  return counts.every(
    (value) =>
      typeof value === 'number' && Number.isInteger(value) && value >= 0
  )
}

export function useRouteRebuildTask() {
  const snapshot = useSyncExternalStore(
    subscribeRouteRebuild,
    readRouteRebuildSnapshot,
    () => null
  )
  const reference = useMemo(
    () => parseRouteRebuildReference(snapshot),
    [snapshot]
  )
  const taskId = reference?.taskId ?? null
  const queryClient = useQueryClient()
  const submissions = useIsMutating({ mutationKey: routeQueryKeys.rebuild() })
  const query = useQuery({
    queryKey: routeRebuildTaskQueryKey(taskId),
    enabled: taskId !== null,
    queryFn: async ({ signal }): Promise<RouteRebuildTask> => {
      if (taskId === null) {
        throw new Error(i18n.t('tokenRoutes.rebuild.invalidResponse'))
      }
      const response = (await api.getTask(taskId, {
        signal,
        skipErrorHandler: true,
      })) as {
        success?: boolean
        task?: RouteRebuildTask
      }
      const task = response?.task
      if (
        response?.success !== true ||
        !task ||
        task.id !== taskId ||
        task.type !== 'routes-rebuild' ||
        !TASK_STATUSES.has(task.status) ||
        (task.status === 'succeeded' && !hasCompleteResult(task.result))
      ) {
        throw new Error(i18n.t('tokenRoutes.rebuild.invalidResponse'))
      }
      return task
    },
    retry: false,
    staleTime: 0,
    refetchOnMount: 'always',
    refetchInterval: (query) =>
      query.state.status === 'error' || isTerminal(query.state.data)
        ? false
        : 2000,
  })

  // Even a cached terminal task is unverified on a new page mount. Never turn
  // a stale snapshot or a failed GET into a completion signal.
  const task =
    query.isFetchedAfterMount && !query.isError ? query.data : undefined
  const terminal = isTerminal(task)
  const handledTerminal = useRef<string | null>(null)
  useEffect(() => {
    if (!task || !terminal) return
    const key = `${task.id}:${task.status}`
    if (handledTerminal.current === key) return
    handledTerminal.current = key
    invalidateRouteRebuildQueries(queryClient)
  }, [task, terminal, queryClient])

  return {
    reference,
    task,
    isBusy: submissions > 0 || (reference !== null && !terminal),
    isChecking: query.isFetching,
    queryError: query.isError,
    retryQuery: query.refetch,
    dismiss: () => {
      if (terminal) rememberRouteRebuild(null)
    },
  }
}
