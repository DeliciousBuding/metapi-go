import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { useUpstreamDeletion } from '@/components/common/upstream-deletion'
import { useDataTable } from '@/components/data-table'
import { useChannels } from '@/features/channels'
import { useUndoableDelete } from '@/lib/undoable-delete'

import {
  routeQueryKeys,
  useClearRouteCooldown,
  useModelTokenCandidates,
  useUpdateRoute,
  useZeroChannelRoutes,
} from '../api'
import { useShowZeroChannelPreference } from '../lib/use-show-zero-channel'
import type { RouteRowActions, RouteSummaryRow } from '../types'
import { resolveRouteTitle } from '../utils'
import { useRoutesColumns } from './routes-columns'
import type { useTokenRoutesUrlState } from './use-token-routes-url-state'

export const routeContentId = (id: number) => `route-channels-${id}`
const routeRowId = (route: RouteSummaryRow) => String(route.id)

// Module-level so the table's globalFilterFn keeps a stable identity across
// renders (a fresh inline function would re-resolve the table every render).
function routesGlobalFilterFn(
  row: { original: unknown },
  _columnId: string,
  filterValue: string
): boolean {
  const route = row.original as RouteSummaryRow
  const haystack = [
    route.modelPattern,
    route.displayName ?? '',
    ...(route.siteNames ?? []),
  ]
    .join(' ')
    .toLowerCase()
  return haystack.includes(String(filterValue).toLowerCase())
}

export function useRoutesTable({
  routes,
  urlState,
  openEdit,
  openDetail,
  refreshDecisions,
}: {
  routes: RouteSummaryRow[]
  urlState: ReturnType<typeof useTokenRoutesUrlState>
  openEdit: RouteRowActions['onEdit']
  openDetail: RouteRowActions['onViewDetail']
  refreshDecisions: () => void
}) {
  const { t } = useTranslation()
  const {
    globalFilter,
    pagination,
    columnFilters,
    onGlobalFilterChange,
    onPaginationChange,
    onColumnFiltersChange,
  } = urlState
  const { data: candidates } = useModelTokenCandidates()
  const updateMutation = useUpdateRoute()
  const clearCooldownMutation = useClearRouteCooldown()

  // Route ids with at least one channel in an ACTIVE persisted cooldown:
  // the channels list carries per-channel cooldownUntil, and the summary
  // rows do not. The row menu's "清除冷却" item is hidden for routes with
  // nothing to clear — same predicate as the route detail sheet gate
  // (cooldownUntil > now), so the two surfaces can never disagree.
  const { data: channelList } = useChannels()
  const cooldownRouteIds = useMemo(
    () =>
      new Set(
        (channelList ?? [])
          .filter(
            (c) =>
              Boolean(c.cooldownUntil) &&
              new Date(c.cooldownUntil as string) > new Date()
          )
          .map((c) => c.routeId)
      ),
    [channelList]
  )

  const { showZeroChannel, setShowZeroChannel } = useShowZeroChannelPreference()
  const rows = useZeroChannelRoutes(routes, candidates, showZeroChannel)

  const [expandedRouteIds, setExpandedRouteIds] = useState<ReadonlySet<number>>(
    () => new Set()
  )
  const toggleRouteExpansion = useCallback((id: number) => {
    setExpandedRouteIds((previous) => {
      const next = new Set(previous)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  // Native leaf routes retain undo; imported groups and other dependencies
  // require a preview of the full deletion before confirmation.
  const undoableDelete = useUndoableDelete()
  const {
    requestDeletion,
    dialog: deletionDialog,
    isPending: isDeletePending,
  } = useUpstreamDeletion()
  const deleteRouteUndoable = useCallback(
    (route: RouteSummaryRow) => {
      void requestDeletion(
        { kind: 'route', id: route.id, name: resolveRouteTitle(route) },
        (_preview, commit) =>
          undoableDelete<RouteSummaryRow[], RouteSummaryRow>({
            item: route,
            queryKey: routeQueryKeys.summary(),
            removeFromCache: (data, item) =>
              data.filter((entry) => entry.id !== item.id),
            deleteFn: commit,
            title: t('tokenRoutes.toast.deleted'),
            undoLabel: t('common.undo'),
            errorTitle: t('tokenRoutes.toast.deleteFailed'),
          })
      )
    },
    [requestDeletion, undoableDelete, t]
  )

  // Memoized so the column defs keep a stable identity across renders.
  const rowActions = useMemo<RouteRowActions>(
    () => ({
      isDeletePending,
      onEdit: openEdit,
      onDelete: deleteRouteUndoable,
      onToggleEnabled: (route) =>
        updateMutation.mutate({
          id: route.id,
          payload: { enabled: !route.enabled },
        }),
      onViewDetail: openDetail,
      onClearCooldown: (route) => {
        clearCooldownMutation.mutate(route.id)
      },
      onRefreshDecision: () => {
        refreshDecisions()
      },
    }),
    [
      isDeletePending,
      openEdit,
      openDetail,
      deleteRouteUndoable,
      updateMutation,
      clearCooldownMutation,
      refreshDecisions,
    ]
  )

  const columns = useRoutesColumns(
    rowActions,
    updateMutation.isPending ? (updateMutation.variables?.id ?? null) : null,
    clearCooldownMutation.isPending
      ? (clearCooldownMutation.variables ?? null)
      : null,
    cooldownRouteIds,
    {
      expandedIds: expandedRouteIds,
      toggle: toggleRouteExpansion,
      getContentId: routeContentId,
    }
  )

  const { table } = useDataTable({
    data: rows,
    columns,
    getRowId: routeRowId,
    enableRowSelection: true,
    // The URL-synced callbacks already reset the page on every filter change
    // (resetPageIndexOnFilterChange), so disable TanStack's own auto-reset to
    // avoid a second redundant page update.
    autoResetPageIndex: false,
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange,
    globalFilterFn: routesGlobalFilterFn,
  })
  return {
    table,
    expandedRouteIds,
    toggleRouteExpansion,
    showZeroChannel,
    setShowZeroChannel,
    deletionDialog,
  }
}
