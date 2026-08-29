/* eslint-disable no-nested-ternary -- empty-state text uses chained ternary */
// metapi-go features/token-routes/components — the routes list page.
// i18n: all user-visible strings migrated to t() calls.

import { useNavigate, useSearch } from '@tanstack/react-router'
import type { ColumnFiltersState, Table } from '@tanstack/react-table'
import { Plus, Power, Zap } from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { ConfirmDialog } from '@/components/common/confirm-dialog'
import {
  DataTableBulkActions,
  DataTablePage,
  type UrlTableState,
  type UrlTableStateUpdate,
  useDataTable,
  useUrlTableState,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Spinner } from '@/components/ui/spinner'
import { Switch } from '@/components/ui/switch'
import { useAccounts } from '@/features/accounts/api'
import { useChannels } from '@/features/channels/api'
import { useSites } from '@/features/sites/api'
import { asStringParam } from '@/lib/helpers/searchParams'

import {
  type BatchRouteAction,
  useBatchUpdateRoutes,
  useClearRouteCooldown,
  useDeleteRoute,
  useModelTokenCandidates,
  useRebuildRoutes,
  useRefreshRouteDecisions,
  useRoutes,
  useUpdateRoute,
  useZeroChannelRoutes,
} from '../api'
import { routesSearchSchema } from '../lib/routes-schema'
import { useShowZeroChannelPreference } from '../lib/use-show-zero-channel'
import type { RouteRowActions, RouteSummaryRow } from '../types'
import {
  isExplicitGroupRoute,
  isExactModelPattern,
  resolveRouteTitle,
} from '../utils'
import { RouteDetailSheet } from './route-detail-sheet'
import { RouteFormDialog, type RouteAccountOption } from './route-form-dialog'
import { useRoutesColumns } from './routes-columns'
import { RoutesHeaderActions } from './routes-header-actions'

const DEFAULT_PAGE_SIZE = 20

/** Page-specific URL filters: the `enabled` column filter + chain context.
 * `routeId` is a one-shot drilldown param (proxy-log detail sheet) that the
 * page consumes into the detail sheet and then strips; it is deliberately
 * never serialized back into hrefs. */
type TokenRoutesUrlFilters = {
  enabled: string
  accountId: string
  siteId: string
  routeId: string
}

/**
 * Parse the raw search string into URL table state, reusing the route's
 * `routesSearchSchema` so the derived state matches the loader's validated
 * search. The page uses a 1-based `page` param.
 */
function readTokenRoutesSearch(
  searchString: string
): UrlTableState<TokenRoutesUrlFilters> {
  const entries = Object.fromEntries(
    new URLSearchParams(
      searchString.startsWith('?') ? searchString.slice(1) : searchString
    ).entries()
  )
  const parsed = routesSearchSchema.safeParse(entries)
  const search = parsed.success ? parsed.data : null
  return {
    q: asStringParam(search?.q) ?? '',
    pageIndex: (search?.page ?? 1) - 1,
    pageSize: search?.pageSize ?? DEFAULT_PAGE_SIZE,
    sorting: [],
    filters: {
      enabled: asStringParam(search?.enabled) ?? '',
      accountId:
        search?.accountId === undefined ? '' : String(search.accountId),
      siteId: search?.siteId === undefined ? '' : String(search.siteId),
      routeId: search?.routeId === undefined ? '' : String(search.routeId),
    },
  }
}

/** Serialize a partial state update back to the token-routes href, merging
 *  over the CURRENT url state (preserves chain context accountId/siteId). */
function buildTokenRoutesHref(
  next: UrlTableStateUpdate<TokenRoutesUrlFilters>
): string {
  const current = readTokenRoutesSearch(window.location.search)
  const merged: UrlTableState<TokenRoutesUrlFilters> = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex + 1))
  if (merged.pageSize !== DEFAULT_PAGE_SIZE) {
    params.set('pageSize', String(merged.pageSize))
  }
  if (merged.filters.enabled) params.set('enabled', merged.filters.enabled)
  if (merged.filters.accountId) {
    params.set('accountId', merged.filters.accountId)
  }
  if (merged.filters.siteId) params.set('siteId', merged.filters.siteId)
  // Preserve the one-shot `edit` deep-link param across table-state
  // navigations (page-clamp / sort / filter) until the page's consumption
  // effect strips it — mirrors the sites page's buildHref guard for its
  // guided `create`/`edit` params.
  const guidedEdit = new URLSearchParams(window.location.search).get('edit')
  if (guidedEdit) params.set('edit', guidedEdit)
  const queryString = params.toString()
  return queryString ? `/token-routes?${queryString}` : '/token-routes'
}

function useTokenRoutesUrlState() {
  return useUrlTableState<TokenRoutesUrlFilters>({
    basePath: '/token-routes',
    read: readTokenRoutesSearch,
    buildHref: buildTokenRoutesHref,
    toColumnFilters: (filters) => {
      const out: ColumnFiltersState = []
      const enabledValues = filters.enabled.split(',').filter(Boolean)
      if (enabledValues.length) {
        out.push({ id: 'enabled', value: enabledValues })
      }
      return out
    },
    fromColumnFilters: (filters) => {
      const enabledEntry = filters.find((filter) => filter.id === 'enabled')
      return {
        filters: {
          enabled: Array.isArray(enabledEntry?.value)
            ? enabledEntry.value.join(',')
            : '',
        },
      }
    },
    resetPageIndexOnFilterChange: true,
  })
}

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

export function RoutesPage() {
  const { t } = useTranslation()
  const routerSearch = useSearch({ from: '/_authenticated/token-routes' })
  const navigate = useNavigate()
  const {
    globalFilter,
    pagination,
    columnFilters,
    onGlobalFilterChange,
    onPaginationChange,
    onColumnFiltersChange,
    filters,
  } = useTokenRoutesUrlState()

  const {
    data: routesData,
    isLoading,
    isFetching,
    error,
    refetch,
  } = useRoutes()
  const candidatesQuery = useModelTokenCandidates()

  const deleteMutation = useDeleteRoute()
  const updateMutation = useUpdateRoute()
  const clearCooldownMutation = useClearRouteCooldown()
  const rebuildMutation = useRebuildRoutes()
  const refreshDecisionsMutation = useRefreshRouteDecisions()

  const routes = useMemo(() => routesData ?? [], [routesData])
  const candidates = candidatesQuery.data

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

  // Chain context (account/site deep-link) is read from the URL-owned filters;
  // it is preserved across every navigation but never modified by this page.
  const accountId = filters.accountId ? Number(filters.accountId) : undefined
  const siteId = filters.siteId ? Number(filters.siteId) : undefined

  const { showZeroChannel, setShowZeroChannel } = useShowZeroChannelPreference()
  const rows = useZeroChannelRoutes(routes, candidates, showZeroChannel)

  const [formOpen, setFormOpen] = useState(false)
  const [formMode, setFormMode] = useState<'create' | 'edit'>('create')
  const [editRoute, setEditRoute] = useState<RouteSummaryRow | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailRoute, setDetailRoute] = useState<RouteSummaryRow | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteRouteState, setDeleteRoute] = useState<RouteSummaryRow | null>(
    null
  )
  const [rebuildConfirmOpen, setRebuildConfirmOpen] = useState(false)

  // One-shot route drilldown (proxy-log detail -> `?routeId=N`): wait for
  // the list, open the detail sheet for the referenced route, then strip
  // the param so a refetch or remount never reopens the sheet. A stale or
  // unknown id is stripped without opening anything.
  const routeDrilldownConsumed = useRef(false)
  useEffect(() => {
    if (routeDrilldownConsumed.current || !filters.routeId) return
    if (isLoading) return

    const targetId = Number(filters.routeId)
    const target = routes.find((route) => route.id === targetId)
    routeDrilldownConsumed.current = true
    if (target) {
      setDetailRoute(target)
      setDetailOpen(true)
    }
    navigate({
      to: '/token-routes',
      search: { ...routerSearch, routeId: undefined },
      replace: true,
    })
  }, [filters.routeId, isLoading, routes, routerSearch, navigate])

  const openCreate = () => {
    setFormMode('create')
    setEditRoute(null)
    setFormOpen(true)
  }

  const openEdit = useCallback((route: RouteSummaryRow) => {
    setFormMode('edit')
    setEditRoute(route)
    setFormOpen(true)
  }, [])

  // One-shot edit drilldown (channel detail sheet -> `?edit=N`): wait for
  // the list, open the edit dialog for the referenced route (the same state
  // the row edit action uses), then strip the param so a refetch or remount
  // never reopens it. A stale or unknown id is stripped without opening
  // anything — mirrors the sites page's `edit` consumption and this page's
  // `routeId` drilldown, including the strict-mode re-entry guard.
  const editDrilldownConsumed = useRef(false)
  useEffect(() => {
    if (editDrilldownConsumed.current || routerSearch.edit === undefined) {
      return
    }
    if (isLoading) return

    const target = routes.find((route) => route.id === routerSearch.edit)
    editDrilldownConsumed.current = true
    if (target) {
      openEdit(target)
    }
    navigate({
      to: '/token-routes',
      search: { ...routerSearch, edit: undefined },
      replace: true,
    })
  }, [routerSearch, isLoading, routes, navigate, openEdit])

  // Memoized so the column defs keep a stable identity across renders.
  const rowActions = useMemo<RouteRowActions>(
    () => ({
      onEdit: openEdit,
      onDelete: (route) => {
        setDeleteRoute(route)
        setDeleteOpen(true)
      },
      onToggleEnabled: (route) =>
        updateMutation.mutate({
          id: route.id,
          payload: { enabled: !route.enabled },
        }),
      onViewDetail: (route) => {
        setDetailRoute(route)
        setDetailOpen(true)
      },
      onClearCooldown: (route) => {
        clearCooldownMutation.mutate(route.id)
      },
      onRefreshDecision: () => {
        refreshDecisionsMutation.mutate()
      },
    }),
    [openEdit, updateMutation, clearCooldownMutation, refreshDecisionsMutation]
  )

  const columns = useRoutesColumns(
    rowActions,
    updateMutation.isPending ? (updateMutation.variables?.id ?? null) : null,
    clearCooldownMutation.isPending
      ? (clearCooldownMutation.variables ?? null)
      : null,
    cooldownRouteIds
  )

  const { table } = useDataTable({
    data: rows,
    columns,
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

  const availableRoutes = useMemo(
    () =>
      routes.filter(
        (route) =>
          !isExplicitGroupRoute(route) &&
          isExactModelPattern(route.modelPattern) &&
          route.id !== editRoute?.id
      ),
    [routes, editRoute]
  )

  const accountOptions = useMemo<RouteAccountOption[]>(
    () => buildAccountOptions(candidates),
    [candidates]
  )

  const chainContext = useMemo(
    () => ({
      accountId,
      siteId,
    }),
    [accountId, siteId]
  )

  // The loader already prefetches both the accounts snapshot and the sites
  // list into the query cache, so these are cache hits (no extra round-trip).
  // Resolve chain-context IDs to human-readable names with `#ID` fallback.
  const { data: accountsSnapshot } = useAccounts()
  const { data: sitesList } = useSites()
  const chainAccountName = useMemo(() => {
    if (!accountId) return undefined
    const match = accountsSnapshot?.accounts.find((a) => a.id === accountId)
    return match?.username ?? `#${accountId}`
  }, [accountId, accountsSnapshot])
  const chainSiteName = useMemo(() => {
    if (!siteId) return undefined
    const match = sitesList?.find((s) => s.id === siteId)
    return match?.name ?? `#${siteId}`
  }, [siteId, sitesList])

  const confirmDelete = async () => {
    if (!deleteRouteState) return
    try {
      await deleteMutation.mutateAsync(deleteRouteState.id)
      setDeleteOpen(false)
      setDeleteRoute(null)
    } catch {
      // http-client toasted
    }
  }

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div className='flex flex-wrap items-center justify-between gap-4'>
        <div>
          <h1 className='text-lg font-normal'>{t('tokenRoutes.page.title')}</h1>
          <p className='text-muted-foreground text-sm'>
            {t('tokenRoutes.page.description')}
          </p>
        </div>
        <RoutesHeaderActions
          onRebuild={() => setRebuildConfirmOpen(true)}
          isRebuildPending={rebuildMutation.isPending}
          onRefreshDecisions={() => refreshDecisionsMutation.mutate()}
          isRefreshDecisionsPending={refreshDecisionsMutation.isPending}
          onAddRoute={openCreate}
        />
      </div>

      {(accountId || siteId) && (
        <div className='bg-muted/40 text-muted-foreground rounded-lg border p-2 text-sm'>
          {t('tokenRoutes.page.chainContext')}
          {chainAccountName
            ? ` ${t('tokenRoutes.page.chainContextAccount', { name: chainAccountName })}`
            : ''}
          {chainSiteName
            ? ` / ${t('tokenRoutes.page.chainContextSite', { name: chainSiteName })} `
            : ' '}
          {t('tokenRoutes.page.chainContextSuffix')}
        </div>
      )}

      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        error={error as Error | null}
        errorMessageKey='tokenRoutes.page.loadError'
        onErrorRetry={() => refetch()}
        isErrorRetrying={isFetching}
        emptyTitle={t('tokenRoutes.page.emptyTitle')}
        emptyDescription={t('tokenRoutes.page.emptyDescription')}
        emptyAction={
          <div className='flex items-center gap-2'>
            <Button onClick={openCreate}>
              <Plus />
              {t('tokenRoutes.page.addButton')}
            </Button>
            <Button
              variant='outline'
              onClick={() => setRebuildConfirmOpen(true)}
              disabled={rebuildMutation.isPending}
            >
              {rebuildMutation.isPending ? <Spinner /> : <Zap />}
              {t('tokenRoutes.page.rebuild')}
            </Button>
          </div>
        }
        skeletonKeyPrefix='routes-skeleton'
        toolbarProps={{
          searchPlaceholder: t('tokenRoutes.page.searchPlaceholder'),
          searchDebounceMs: 300,
          viewToggle: (
            // min-w-0 + truncated span so the long "显示零通道模型…" label
            // can shrink on narrow viewports instead of pushing the View
            // Options button past the toolbar edge (375px overflow fix).
            <label
              className='text-muted-foreground flex min-w-0 items-center gap-1.5 text-sm'
              title={t('tokenRoutes.page.showZeroChannel')}
            >
              <Switch
                className='shrink-0'
                checked={showZeroChannel}
                onCheckedChange={setShowZeroChannel}
              />
              <span className='max-w-[150px] min-w-0 truncate sm:max-w-[280px]'>
                {t('tokenRoutes.page.showZeroChannel')}
              </span>
            </label>
          ),
          filters: [
            {
              columnId: 'enabled',
              title: t('tokenRoutes.page.filterStatusTitle'),
              singleSelect: true,
              options: [
                {
                  label: t('tokenRoutes.page.filterStatusEnabled'),
                  value: 'enabled',
                },
                {
                  label: t('tokenRoutes.page.filterStatusDisabled'),
                  value: 'disabled',
                },
              ],
            },
          ],
        }}
        bulkActions={<RoutesBulkActions table={table} />}
      />

      <RouteFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        mode={formMode}
        route={editRoute}
        availableRoutes={availableRoutes}
        accountOptions={accountOptions}
        chainContext={chainContext}
      />

      <RouteDetailSheet
        route={detailRoute}
        open={detailOpen}
        onOpenChange={setDetailOpen}
        onEdit={(route) => {
          setDetailOpen(false)
          openEdit(route)
        }}
      />

      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('tokenRoutes.page.deleteTitle')}</DialogTitle>
            <DialogDescription>
              {t('tokenRoutes.page.deleteDescription', {
                name: deleteRouteState
                  ? resolveRouteTitle(deleteRouteState)
                  : '',
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setDeleteOpen(false)}
              disabled={deleteMutation.isPending}
            >
              {t('common.cancel')}
            </Button>
            <Button
              variant='destructive'
              onClick={confirmDelete}
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending && <Spinner />}
              {t('common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={rebuildConfirmOpen}
        title={t('tokenRoutes.page.rebuildConfirmTitle')}
        description={t('tokenRoutes.page.rebuildConfirmDescription')}
        confirmLabel={t('tokenRoutes.page.rebuild')}
        cancelLabel={t('common.cancel')}
        destructive
        onConfirm={() => {
          setRebuildConfirmOpen(false)
          rebuildMutation.mutate({ refreshModels: true })
        }}
        onCancel={() => setRebuildConfirmOpen(false)}
      />
    </div>
  )
}

function RoutesBulkActions({ table }: { table: Table<RouteSummaryRow> }) {
  const { t } = useTranslation()
  const batchMutation = useBatchUpdateRoutes()
  const [confirmDisableOpen, setConfirmDisableOpen] = useState(false)

  // Derived per render — `table` identity is stable across selection changes
  // then a `useMemo([table])` would freeze the ids at their mount-time value
  // and every batch action would silently no-op.
  const selectedIds = table
    .getFilteredSelectedRowModel()
    .rows.map((row) => (row.original as RouteSummaryRow).id)
    .filter((id) => id > 0)

  const runBatch = async (action: BatchRouteAction) => {
    if (selectedIds.length === 0) return
    try {
      await batchMutation.mutateAsync({ ids: selectedIds, action })
      table.resetRowSelection()
    } catch {
      // http-client toasted
    }
  }

  return (
    <>
      <DataTableBulkActions
        table={table}
        entityName={t('tokenRoutes.page.bulkEntityName')}
      >
        <Button
          size='sm'
          variant='outline'
          onClick={() => runBatch('enable')}
          disabled={batchMutation.isPending}
        >
          <Power />
          {t('tokenRoutes.page.bulkEnable')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          onClick={() => setConfirmDisableOpen(true)}
          disabled={batchMutation.isPending}
        >
          {t('tokenRoutes.page.bulkDisable')}
        </Button>
      </DataTableBulkActions>

      <ConfirmDialog
        open={confirmDisableOpen}
        title={t('tokenRoutes.page.bulkDisableConfirmTitle')}
        description={t('tokenRoutes.page.bulkDisableConfirmDescription', {
          count: selectedIds.length,
        })}
        confirmLabel={t('tokenRoutes.page.bulkDisable')}
        cancelLabel={t('common.cancel')}
        destructive
        onConfirm={() => {
          setConfirmDisableOpen(false)
          void runBatch('disable')
        }}
        onCancel={() => setConfirmDisableOpen(false)}
      />
    </>
  )
}

type CandidateAccountLike = {
  accountId?: number
  username?: string | null
  siteName?: string | null
}

function buildAccountOptions(
  candidates:
    | {
        models?: Record<string, unknown[]>
      }
    | undefined
): RouteAccountOption[] {
  const models = candidates?.models
  if (!models || typeof models !== 'object') return []

  const accountMap = new Map<number, string>()
  for (const candidatesList of Object.values(models)) {
    if (!Array.isArray(candidatesList)) continue
    for (const raw of candidatesList) {
      const candidate = raw as CandidateAccountLike
      if (!candidate || typeof candidate.accountId !== 'number') continue
      if (accountMap.has(candidate.accountId)) continue
      const username = (candidate.username || '').trim()
      const siteName = (candidate.siteName || '').trim()
      const label = username
        ? siteName
          ? `${username} @ ${siteName}`
          : username
        : `account-${candidate.accountId}`
      accountMap.set(candidate.accountId, label)
    }
  }

  return [...accountMap.entries()]
    .map(([id, label]) => ({ id, label }))
    .sort((left, right) =>
      left.label.localeCompare(right.label, undefined, { sensitivity: 'base' })
    )
}
