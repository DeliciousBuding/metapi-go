// metapi-go features/token-routes/components — the routes list page.
//
// Wires the data-table four-layer package (useDataTable + DataTablePage) to
// the useRoutes summary query + useModelTokenCandidates query, with
// client-side pagination/filtering/sorting and URL-synced table state
// (page / pageSize / global search / enabled filter / accountId / siteId —
// the last two support the deep-link from the accounts page's guided toast).
// Mobile card degradation is handled automatically by DataTablePage.
//
// Zero-channel placeholder rows (kind: 'zero_channel') are merged in via the
// `useZeroChannelRoutes` adapter when the operator toggles the "显示零通道"
// header button — they render as muted, read-only rows with a "未生成" badge
// and are excluded from selection / batch actions / the enabled filter.
//
// Header actions: add route (opens the form Sheet), rebuild routes, refresh
// decision snapshots, toggle zero-channel visibility. Bulk actions: batch
// enable / disable.

import { useEffect, useMemo, useState } from 'react'
import {
  type ColumnFiltersState,
  type OnChangeFn,
  type PaginationState,
  type Table,
} from '@tanstack/react-table'
import {
  Loader2,
  Plus,
  Power,
  RefreshCw,
  Zap,
} from 'lucide-react'

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
import {
  type RouteRowActions,
  type RouteSummaryRow,
} from '../types'
import {
  isExplicitGroupRoute,
  isExactModelPattern,
  resolveRouteTitle,
} from '../utils'
import { useRoutesColumns } from './routes-columns'
import { RouteDetailSheet } from './route-detail-sheet'
import {
  RouteFormDialog,
  type RouteAccountOption,
} from './route-form-dialog'

// ---------------------------------------------------------------------------
// URL state helpers — read initial table state from the query string on mount
// and write subsequent changes back via history.replaceState. The `accountId`
// and `siteId` params are the deep-link context from the accounts page guided
// toast (step 2 → step 3 of the site → account → route chain).
// ---------------------------------------------------------------------------

interface InitialUrlState {
  page: number
  pageSize: number
  search: string
  enabled: string[]
  accountId?: number
  siteId?: number
}

const DEFAULT_PAGE_SIZE = 20

function readInitialFromUrl(): InitialUrlState {
  if (typeof window === 'undefined') {
    return { page: 1, pageSize: DEFAULT_PAGE_SIZE, search: '', enabled: [] }
  }
  const params = new URLSearchParams(window.location.search)
  const page = Math.max(1, Number(params.get('page')) || 1)
  const pageSize = Number(params.get('pageSize')) || DEFAULT_PAGE_SIZE
  const search = params.get('q') ?? ''
  const enabled = params.get('enabled')?.split(',').filter(Boolean) ?? []
  const accountIdRaw = Number(params.get('accountId'))
  const siteIdRaw = Number(params.get('siteId'))
  return {
    page,
    pageSize,
    search,
    enabled,
    accountId: accountIdRaw > 0 ? accountIdRaw : undefined,
    siteId: siteIdRaw > 0 ? siteIdRaw : undefined,
  }
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export function RoutesPage() {
  const { data: routesData, isLoading, isFetching, error } = useRoutes()
  const candidatesQuery = useModelTokenCandidates()

  const deleteMutation = useDeleteRoute()
  const updateMutation = useUpdateRoute()
  const clearCooldownMutation = useClearRouteCooldown()
  const rebuildMutation = useRebuildRoutes()
  const refreshDecisionsMutation = useRefreshRouteDecisions()

  const routes = routesData ?? []
  const candidates = candidatesQuery.data

  // --- zero-channel toggle ---
  const [showZeroChannel, setShowZeroChannel] = useState(false)
  const rows = useZeroChannelRoutes(routes, candidates, showZeroChannel)

  // --- table state (client-side, URL-synced) ---
  const initial = useMemo(readInitialFromUrl, [])
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: initial.page - 1,
    pageSize: initial.pageSize,
  })
  const [globalFilter, setGlobalFilter] = useState(initial.search)
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>(() => {
    const filters: ColumnFiltersState = []
    if (initial.enabled.length) {
      filters.push({ id: 'enabled', value: initial.enabled })
    }
    return filters
  })

  // Write state back to the URL.
  useEffect(() => {
    if (typeof window === 'undefined') return
    const params = new URLSearchParams()
    if (pagination.pageIndex > 0) params.set('page', String(pagination.pageIndex + 1))
    if (pagination.pageSize !== DEFAULT_PAGE_SIZE)
      params.set('pageSize', String(pagination.pageSize))
    if (globalFilter) params.set('q', globalFilter)
    const enabledFilter = columnFilters.find((filter) => filter.id === 'enabled')
    if (enabledFilter && Array.isArray(enabledFilter.value) && enabledFilter.value.length)
      params.set('enabled', enabledFilter.value.join(','))
    if (initial.accountId) params.set('accountId', String(initial.accountId))
    if (initial.siteId) params.set('siteId', String(initial.siteId))
    const query = params.toString()
    const url = query ? `${window.location.pathname}?${query}` : window.location.pathname
    window.history.replaceState(null, '', url)
  }, [pagination, globalFilter, columnFilters, initial.accountId, initial.siteId])

  const onGlobalFilterChange = useMemo<OnChangeFn<string>>(
    () => (updater) => {
      setGlobalFilter((prev) =>
        updater instanceof Function ? updater(prev) : updater,
      )
      setPagination((prev) => ({ ...prev, pageIndex: 0 }))
    },
    [],
  )
  const onColumnFiltersChange = useMemo<OnChangeFn<ColumnFiltersState>>(
    () => (updater) => {
      setColumnFilters((prev) =>
        updater instanceof Function ? updater(prev) : updater,
      )
      setPagination((prev) => ({ ...prev, pageIndex: 0 }))
    },
    [],
  )

  // --- dialog state ---
  const [formOpen, setFormOpen] = useState(false)
  const [formMode, setFormMode] = useState<'create' | 'edit'>('create')
  const [editRoute, setEditRoute] = useState<RouteSummaryRow | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailRoute, setDetailRoute] = useState<RouteSummaryRow | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteRoute, setDeleteRoute] = useState<RouteSummaryRow | null>(null)

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

  // --- row actions (handed to the columns hook) ---
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
    // Client-side global search across modelPattern / displayName / siteNames.
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

  // --- form helpers: available source routes (exact-model) + account options
  const availableRoutes = useMemo(
    () =>
      routes.filter(
        (route) =>
          !isExplicitGroupRoute(route) &&
          isExactModelPattern(route.modelPattern) &&
          route.id !== editRoute?.id,
      ),
    [routes, editRoute],
  )

  const accountOptions = useMemo<RouteAccountOption[]>(
    () => buildAccountOptions(candidates),
    [candidates],
  )

  const chainContext = useMemo(
    () => ({
      accountId: initial.accountId,
      siteId: initial.siteId,
    }),
    [initial.accountId, initial.siteId],
  )

  const confirmDelete = async () => {
    if (!deleteRoute) return
    try {
      await deleteMutation.mutateAsync(deleteRoute.id)
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
          <h1 className='text-lg font-semibold'>路由管理</h1>
          <p className='text-sm text-muted-foreground'>
            配置模型路由：匹配规则 / 分组 / 通道策略，让请求按规则分发到账号通道。
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
            自动重建
          </Button>
          <Button
            variant='outline'
            size='sm'
            onClick={() => refreshDecisionsMutation.mutate()}
            disabled={refreshDecisionsMutation.isPending}
          >
            <RefreshCw
              className={refreshDecisionsMutation.isPending ? 'animate-spin' : undefined}
            />
            刷新决策
          </Button>
          <Button onClick={openCreate}>
            <Plus />
            添加路由
          </Button>
        </div>
      </div>

      {/* Chain context banner — shown when arrived from accounts guided toast */}
      {(initial.accountId || initial.siteId) && (
        <div className='rounded-lg border bg-muted/40 p-2 text-sm text-muted-foreground'>
          正在为
          {initial.accountId ? ` 账号 #${initial.accountId}` : ''}
          {initial.siteId ? ` / 站点 #${initial.siteId}` : ''} 配置路由。
          添加一条路由即可完成配置动线。
        </div>
      )}

      {error && (
        <div className='rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive'>
          加载路由失败：{(error as Error).message}
        </div>
      )}

      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle='暂无路由'
        emptyDescription='添加一条路由开始配置，或点击「自动重建」让系统按账号模型生成路由。'
        skeletonKeyPrefix='routes-skeleton'
        toolbarProps={{
          searchPlaceholder: '搜索模型 / 名称…',
          searchDebounceMs: 300,
          filters: [
            {
              columnId: 'enabled',
              title: '状态',
              singleSelect: true,
              options: [
                { label: '启用', value: 'enabled' },
                { label: '禁用', value: 'disabled' },
              ],
            },
          ],
        }}
        bulkActions={<RoutesBulkActions table={table} />}
      />

      {/* Zero-channel toggle (below the table, above pagination) */}
      <label className='flex items-center gap-2 text-sm text-muted-foreground'>
        <Switch
          checked={showZeroChannel}
          onCheckedChange={setShowZeroChannel}
        />
        显示零通道模型（账号有模型但未生成路由）
      </label>

      {/* Create / edit form (Sheet) */}
      <RouteFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        mode={formMode}
        route={editRoute}
        availableRoutes={availableRoutes}
        accountOptions={accountOptions}
        chainContext={chainContext}
      />

      {/* Detail sheet (embeds the channels sub-query + decision snapshot) */}
      <RouteDetailSheet
        route={detailRoute}
        open={detailOpen}
        onOpenChange={setDetailOpen}
      />

      {/* Delete confirmation (Dialog) */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除路由</DialogTitle>
            <DialogDescription>
              确定删除路由「{deleteRoute ? resolveRouteTitle(deleteRoute) : ''}」？该操作不可撤销，相关通道将一并清除。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setDeleteOpen(false)}
              disabled={deleteMutation.isPending}
            >
              取消
            </Button>
            <Button
              variant='destructive'
              onClick={confirmDelete}
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending && <Loader2 className='animate-spin' />}
              删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Bulk actions toolbar — batch enable / disable.
// ---------------------------------------------------------------------------

function RoutesBulkActions({ table }: { table: Table<RouteSummaryRow> }) {
  const batchMutation = useBatchUpdateRoutes()

  const selectedIds = useMemo(
    () =>
      table
        .getFilteredSelectedRowModel()
        .rows.map((row) => (row.original as RouteSummaryRow).id)
        .filter((id) => id > 0),
    [table],
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
    <DataTableBulkActions table={table} entityName='路由'>
      <Button
        size='xs'
        variant='outline'
        onClick={() => runBatch('enable')}
        disabled={batchMutation.isPending}
      >
        <Power />
        启用
      </Button>
      <Button
        size='xs'
        variant='outline'
        onClick={() => runBatch('disable')}
        disabled={batchMutation.isPending}
      >
        禁用
      </Button>
    </DataTableBulkActions>
  )
}

// ---------------------------------------------------------------------------
// Account options — derive the channel-draft picker source from the model
// token candidates response. Aggregates all accounts that carry any token
// (deduped by accountId). A follow-up can re-filter by the form's current
// model pattern via `matchesModelPattern`; for v1 the operator picks from the
// full account list.
// ---------------------------------------------------------------------------

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
    | undefined,
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

  return Array.from(accountMap.entries())
    .map(([id, label]) => ({ id, label }))
    .sort((left, right) => left.label.localeCompare(right.label, undefined, { sensitivity: 'base' }))
}
