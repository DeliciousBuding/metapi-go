/* eslint-disable no-nested-ternary -- empty-state text uses chained ternary */
// metapi-go features/token-routes/components — the routes list page.
// i18n: all user-visible strings migrated to t() calls.

import { useNavigate, useSearch } from '@tanstack/react-router'
import type {
  ColumnFiltersState,
  OnChangeFn,
  PaginationState,
  Table,
} from '@tanstack/react-table'
import { Loader2, Plus, Power, RefreshCw, Zap } from 'lucide-react'
import { useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTableBulkActions,
  DataTablePage,
  useDataTable,
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

export function RoutesPage() {
  const { t } = useTranslation()
  // URL state is owned by the router: read the validated search via
  // `useSearch` (no `window.location.search`), write changes back via
  // `navigate({ search, replace: true })` (no `history.replaceState`).
  const urlSearch = useSearch({ from: '/_authenticated/token-routes' })
  const navigate = useNavigate()
  const { data: routesData, isLoading, isFetching, error } = useRoutes()
  const candidatesQuery = useModelTokenCandidates()

  const deleteMutation = useDeleteRoute()
  const updateMutation = useUpdateRoute()
  const clearCooldownMutation = useClearRouteCooldown()
  const rebuildMutation = useRebuildRoutes()
  const refreshDecisionsMutation = useRefreshRouteDecisions()

  const routes = useMemo(() => routesData ?? [], [routesData])
  const candidates = candidatesQuery.data

  // Chain context (account/site deep-link) is fixed for the session — read
  // once from the validated search.
  const accountId = urlSearch.accountId
  const siteId = urlSearch.siteId

  const [showZeroChannel, setShowZeroChannel] = useState(false)
  const rows = useZeroChannelRoutes(routes, candidates, showZeroChannel)

  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: (urlSearch.page ?? 1) - 1,
    pageSize: urlSearch.pageSize ?? DEFAULT_PAGE_SIZE,
  })
  const [globalFilter, setGlobalFilter] = useState(urlSearch.q ?? '')
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>(() => {
    const filters: ColumnFiltersState = []
    const enabledValues = urlSearch.enabled?.split(',').filter(Boolean) ?? []
    if (enabledValues.length) {
      filters.push({ id: 'enabled', value: enabledValues })
    }
    return filters
  })

  // Write state back to the URL through the router (single source of truth).
  // Skip the initial write — the URL already holds the search the state was
  // initialised from.
  const skipInitialWrite = useRef(true)
  useEffect(() => {
    if (skipInitialWrite.current) {
      skipInitialWrite.current = false
      return
    }
    const enabledFilter = columnFilters.find(
      (filter) => filter.id === 'enabled'
    )
    navigate({
      to: '/token-routes',
      search: {
        page: pagination.pageIndex > 0 ? pagination.pageIndex + 1 : undefined,
        pageSize:
          pagination.pageSize !== DEFAULT_PAGE_SIZE
            ? pagination.pageSize
            : undefined,
        q: globalFilter || undefined,
        enabled:
          Array.isArray(enabledFilter?.value) && enabledFilter.value.length
            ? enabledFilter.value.join(',')
            : undefined,
        accountId,
        siteId,
      },
      replace: true,
    })
  }, [pagination, globalFilter, columnFilters, accountId, siteId, navigate])

  const onGlobalFilterChange = useMemo<OnChangeFn<string>>(
    () => (updater) => {
      setGlobalFilter((prev) =>
        updater instanceof Function ? updater(prev) : updater
      )
      setPagination((prev) => ({ ...prev, pageIndex: 0 }))
    },
    []
  )
  const onColumnFiltersChange = useMemo<OnChangeFn<ColumnFiltersState>>(
    () => (updater) => {
      setColumnFilters((prev) =>
        updater instanceof Function ? updater(prev) : updater
      )
      setPagination((prev) => ({ ...prev, pageIndex: 0 }))
    },
    []
  )

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

  const openEdit = (route: RouteSummaryRow) => {
    setFormMode('edit')
    setEditRoute(route)
    setFormOpen(true)
  }

  const rowActions: RouteRowActions = {
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
  }

  const columns = useRoutesColumns(rowActions)

  const { table } = useDataTable({
    data: rows,
    columns,
    enableRowSelection: true,
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange: setPagination,
    globalFilterFn: (row, _columnId, filterValue) => {
      const route = row.original as RouteSummaryRow
      const haystack = [
        route.modelPattern,
        route.displayName ?? '',
        ...(route.siteNames ?? []),
      ]
        .join(' ')
        .toLowerCase()
      return haystack.includes(String(filterValue).toLowerCase())
    },
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

      {error && (
        <div className='border-destructive/40 bg-destructive/10 text-destructive-soft-fg rounded-lg border p-3 text-sm'>
          {t('tokenRoutes.page.loadError', {
            message: (error as Error).message,
          })}
        </div>
      )}

      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle={t('tokenRoutes.page.emptyTitle')}
        emptyDescription={t('tokenRoutes.page.emptyDescription')}
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
