/* eslint-disable no-nested-ternary -- empty-state text uses chained ternary */
// metapi-go features/token-routes/components — the routes list page.
// i18n: all user-visible strings migrated to t() calls.

import type { ColumnFiltersState, Table } from '@tanstack/react-table'
import { Loader2, Plus, Power, RefreshCw, RotateCcw, Zap } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

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
import { Switch } from '@/components/ui/switch'
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
import type { RouteRowActions, RouteSummaryRow } from '../types'
import {
  isExplicitGroupRoute,
  isExactModelPattern,
  resolveRouteTitle,
} from '../utils'
import { RouteDetailSheet } from './route-detail-sheet'
import { RouteFormDialog, type RouteAccountOption } from './route-form-dialog'
import { useRoutesColumns } from './routes-columns'

const DEFAULT_PAGE_SIZE = 20

/** Page-specific URL filters: the `enabled` column filter + chain context. */
type TokenRoutesUrlFilters = {
  enabled: string
  accountId: string
  siteId: string
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

  // Chain context (account/site deep-link) is read from the URL-owned filters;
  // it is preserved across every navigation but never modified by this page.
  const accountId = filters.accountId ? Number(filters.accountId) : undefined
  const siteId = filters.siteId ? Number(filters.siteId) : undefined

  const [showZeroChannel, setShowZeroChannel] = useState(false)
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

  const columns = useRoutesColumns(rowActions)

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
      <div className='flex items-center justify-between gap-4'>
        <div>
          <h1 className='text-lg font-normal'>{t('tokenRoutes.page.title')}</h1>
          <p className='text-muted-foreground text-sm'>
            {t('tokenRoutes.page.description')}
          </p>
        </div>
        <div className='flex items-center gap-2'>
          <Button
            variant='outline'
            size='sm'
            onClick={() => rebuildMutation.mutate({ refreshModels: true })}
            disabled={rebuildMutation.isPending}
          >
            {rebuildMutation.isPending ? (
              <Loader2 className='animate-spin' />
            ) : (
              <Zap />
            )}
            {t('tokenRoutes.page.rebuild')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            onClick={() => refreshDecisionsMutation.mutate()}
            disabled={refreshDecisionsMutation.isPending}
          >
            <RefreshCw
              className={
                refreshDecisionsMutation.isPending ? 'animate-spin' : undefined
              }
            />
            {t('tokenRoutes.page.refreshDecisions')}
          </Button>
          <Button onClick={openCreate}>
            <Plus />
            {t('tokenRoutes.page.addButton')}
          </Button>
        </div>
      </div>

      {(accountId || siteId) && (
        <div className='bg-muted/40 text-muted-foreground rounded-lg border p-2 text-sm'>
          {t('tokenRoutes.page.chainContext')}
          {accountId
            ? ` ${t('tokenRoutes.page.chainContextAccount', { id: accountId })}`
            : ''}
          {siteId
            ? ` / ${t('tokenRoutes.page.chainContextSite', { id: siteId })} `
            : ' '}
          {t('tokenRoutes.page.chainContextSuffix')}
        </div>
      )}

      {error ? (
        <div className='flex flex-col gap-3'>
          <div className='border-destructive/40 bg-destructive/10 text-destructive-soft-fg rounded-lg border p-3 text-sm'>
            {t('tokenRoutes.page.loadError', {
              message: (error as Error).message,
            })}
          </div>
          <div>
            <Button
              variant='secondary'
              onClick={() => refetch()}
              disabled={isFetching}
            >
              {isFetching ? (
                <Loader2 className='animate-spin' />
              ) : (
                <RotateCcw className='size-4' />
              )}
              {t('tokenRoutes.page.retry')}
            </Button>
          </div>
        </div>
      ) : (
        <DataTablePage
          table={table}
          columns={columns}
          isLoading={isLoading}
          isFetching={isFetching}
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
                onClick={() => rebuildMutation.mutate({ refreshModels: true })}
                disabled={rebuildMutation.isPending}
              >
                {rebuildMutation.isPending ? (
                  <Loader2 className='animate-spin' />
                ) : (
                  <Zap />
                )}
                {t('tokenRoutes.page.rebuild')}
              </Button>
            </div>
          }
          skeletonKeyPrefix='routes-skeleton'
          toolbarProps={{
            searchPlaceholder: t('tokenRoutes.page.searchPlaceholder'),
            searchDebounceMs: 300,
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
      )}

      <label className='text-muted-foreground flex items-center gap-2 text-sm'>
        <Switch
          checked={showZeroChannel}
          onCheckedChange={setShowZeroChannel}
        />
        {t('tokenRoutes.page.showZeroChannel')}
      </label>

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
              {deleteMutation.isPending && <Loader2 className='animate-spin' />}
              {t('common.delete')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function RoutesBulkActions({ table }: { table: Table<RouteSummaryRow> }) {
  const { t } = useTranslation()
  const batchMutation = useBatchUpdateRoutes()

  const selectedIds = useMemo(
    () =>
      table
        .getFilteredSelectedRowModel()
        .rows.map((row) => (row.original as RouteSummaryRow).id)
        .filter((id) => id > 0),
    [table]
  )

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
    <DataTableBulkActions
      table={table}
      entityName={t('tokenRoutes.page.bulkEntityName')}
    >
      <Button
        size='xs'
        variant='outline'
        onClick={() => runBatch('enable')}
        disabled={batchMutation.isPending}
      >
        <Power />
        {t('tokenRoutes.page.bulkEnable')}
      </Button>
      <Button
        size='xs'
        variant='outline'
        onClick={() => runBatch('disable')}
        disabled={batchMutation.isPending}
      >
        {t('tokenRoutes.page.bulkDisable')}
      </Button>
    </DataTableBulkActions>
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
