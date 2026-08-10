// metapi-go features/accounts/components — the accounts list page.
//
// Wires the data-table four-layer package (useDataTable + DataTablePage) to
// the useAccounts snapshot query, with client-side pagination/filtering/
// sorting and URL-synced table state (page / pageSize / global search /
// status filter / site filter). Mobile card degradation is handled
// automatically by DataTablePage. Row actions + bulk actions call the
// TanStack Query mutation hooks; the create/edit form, detail sheet, and
// delete confirm live as siblings of the table.

import { useEffect, useMemo, useState } from 'react'
import {
  type ColumnFiltersState,
  type OnChangeFn,
  type PaginationState,
  type Table,
} from '@tanstack/react-table'
import { Loader2, Plus, Power, RefreshCw, Trash2 } from 'lucide-react'

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

import {
  type BatchAccountAction,
  useAccounts,
  useBatchUpdateAccounts,
  useDeleteAccount,
  useRefreshAccount,
  useToggleAccountCheckin,
  useToggleAccountPin,
  useToggleAccountStatus,
} from '../api'
import {
  type Account,
  type AccountRowActions,
  accountSchema,
} from '../types'
import { useAccountsColumns } from './accounts-columns'
import { AccountDetailSheet } from './account-detail-sheet'
import { AccountFormDialog } from './account-form-dialog'

// ---------------------------------------------------------------------------
// URL state helpers — read initial table state from the query string on mount
// and write subsequent changes back via history.replaceState. Keeping the sync
// framework-agnostic (window.history) avoids coupling to TanStack Router's
// generated route tree while /token-routes etc. are still unregistered.
// ---------------------------------------------------------------------------

interface InitialUrlState {
  page: number
  pageSize: number
  search: string
  status: string[]
  siteIds: string[]
}

const DEFAULT_PAGE_SIZE = 20

function readInitialFromUrl(): InitialUrlState {
  if (typeof window === 'undefined') {
    return { page: 1, pageSize: DEFAULT_PAGE_SIZE, search: '', status: [], siteIds: [] }
  }
  const params = new URLSearchParams(window.location.search)
  const page = Math.max(1, Number(params.get('page')) || 1)
  const pageSize = Number(params.get('pageSize')) || DEFAULT_PAGE_SIZE
  const search = params.get('q') ?? ''
  const status = params.get('status')?.split(',').filter(Boolean) ?? []
  const siteIds = params.get('site')?.split(',').filter(Boolean) ?? []
  return { page, pageSize, search, status, siteIds }
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export function AccountsPage() {
  const { data, isLoading, isFetching, error } = useAccounts()
  const accounts = data?.accounts ?? []
  const sites = data?.sites ?? []

  const refreshMutation = useRefreshAccount()
  const deleteMutation = useDeleteAccount()
  const pinMutation = useToggleAccountPin()
  const statusMutation = useToggleAccountStatus()
  const checkinMutation = useToggleAccountCheckin()

  // --- table state (client-side, URL-synced) ---
  const initial = useMemo(readInitialFromUrl, [])
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: initial.page - 1,
    pageSize: initial.pageSize,
  })
  const [globalFilter, setGlobalFilter] = useState(initial.search)
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>(() => {
    const filters: ColumnFiltersState = []
    if (initial.status.length) filters.push({ id: 'status', value: initial.status })
    if (initial.siteIds.length) filters.push({ id: 'site', value: initial.siteIds })
    return filters
  })

  // Write state back to the URL (debounce-free replaceState is cheap enough).
  useEffect(() => {
    if (typeof window === 'undefined') return
    const params = new URLSearchParams()
    if (pagination.pageIndex > 0) params.set('page', String(pagination.pageIndex + 1))
    if (pagination.pageSize !== DEFAULT_PAGE_SIZE)
      params.set('pageSize', String(pagination.pageSize))
    if (globalFilter) params.set('q', globalFilter)
    const statusFilter = columnFilters.find((filter) => filter.id === 'status')
    if (statusFilter && Array.isArray(statusFilter.value) && statusFilter.value.length)
      params.set('status', statusFilter.value.join(','))
    const siteFilter = columnFilters.find((filter) => filter.id === 'site')
    if (siteFilter && Array.isArray(siteFilter.value) && siteFilter.value.length)
      params.set('site', siteFilter.value.join(','))
    const query = params.toString()
    const url = query ? `${window.location.pathname}?${query}` : window.location.pathname
    window.history.replaceState(null, '', url)
  }, [pagination, globalFilter, columnFilters])

  // onChange wrappers that reset to the first page on filter changes.
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
  const [editAccount, setEditAccount] = useState<Account | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailAccount, setDetailAccount] = useState<Account | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteAccount, setDeleteAccount] = useState<Account | null>(null)

  const openCreate = () => {
    setFormMode('create')
    setEditAccount(null)
    setFormOpen(true)
  }

  const openEdit = (account: Account) => {
    setFormMode('edit')
    setEditAccount(account)
    setFormOpen(true)
  }

  // --- row actions (handed to the columns hook) ---
  const rowActions: AccountRowActions = {
    onEdit: openEdit,
    onDelete: (account) => {
      setDeleteAccount(account)
      setDeleteOpen(true)
    },
    onRefresh: (account) => refreshMutation.mutate(account.id),
    onViewDetail: (account) => {
      setDetailAccount(account)
      setDetailOpen(true)
    },
    onTogglePin: (account) =>
      pinMutation.mutate({ id: account.id, isPinned: !account.isPinned }),
    onToggleStatus: (account) =>
      statusMutation.mutate({
        id: account.id,
        status: account.status === 'active' ? 'disabled' : 'active',
      }),
    onToggleCheckin: (account) =>
      checkinMutation.mutate({
        id: account.id,
        checkinEnabled: !account.checkinEnabled,
      }),
  }

  const columns = useAccountsColumns(rowActions)

  const { table } = useDataTable({
    data: accounts,
    columns,
    enableRowSelection: true,
    globalFilter,
    onGlobalFilterChange,
    columnFilters,
    onColumnFiltersChange,
    pagination,
    onPaginationChange: setPagination,
    // Client-side global search across username / site name / platform /
    // url / tags. Passed inline so the option's contextual typing drives the
    // param types (avoids implicit-any + keeps the filter stable enough).
    globalFilterFn: (row, _columnId, filterValue) => {
      const account = accountSchema.parse(row.original)
      const haystack = [
        account.username ?? '',
        account.site?.name ?? '',
        account.site?.platform ?? '',
        account.site?.url ?? '',
        ...(account.tags ?? []),
      ]
        .join(' ')
        .toLowerCase()
      return haystack.includes(String(filterValue).toLowerCase())
    },
  })

  const confirmDelete = async () => {
    if (!deleteAccount) return
    try {
      await deleteMutation.mutateAsync(deleteAccount.id)
      setDeleteOpen(false)
      setDeleteAccount(null)
    } catch {
      // http-client toasted
    }
  }

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div className='flex items-center justify-between'>
        <div>
          <h1 className='text-lg font-semibold'>账号管理</h1>
          <p className='text-sm text-muted-foreground'>
            管理站点连接：Session 账号用于签到/余额，API Key 账号用于代理转发。
          </p>
        </div>
        <Button onClick={openCreate} disabled={sites.length === 0}>
          <Plus />
          添加账号
        </Button>
      </div>

      {error && (
        <div className='rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive'>
          加载账号失败：{(error as Error).message}
        </div>
      )}

      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle='暂无账号'
        emptyDescription='添加一个站点账号以开始管理连接，或先到「站点」页添加站点。'
        skeletonKeyPrefix='accounts-skeleton'
        toolbarProps={{
          searchPlaceholder: '搜索账号名称 / 站点…',
          searchDebounceMs: 300,
          filters: [
            {
              columnId: 'status',
              title: '状态',
              singleSelect: true,
              options: [
                { label: '启用', value: 'active' },
                { label: '禁用', value: 'disabled' },
                { label: '过期', value: 'expired' },
              ],
            },
            ...(sites.length > 0
              ? [
                  {
                    columnId: 'site',
                    title: '站点',
                    options: sites.map((site) => ({
                      label: site.name || site.url || `#${site.id}`,
                      value: String(site.id),
                    })),
                  },
                ]
              : []),
          ],
        }}
        bulkActions={<AccountsBulkActions table={table} />}
      />

      {/* Create / edit form (Sheet) */}
      <AccountFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        mode={formMode}
        account={editAccount}
        sites={sites}
      />

      {/* Detail sheet (embeds the tokens sub-module) */}
      <AccountDetailSheet
        account={detailAccount}
        open={detailOpen}
        onOpenChange={setDetailOpen}
      />

      {/* Delete confirmation (Dialog) */}
      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除账号</DialogTitle>
            <DialogDescription>
              确定删除账号「{deleteAccount?.username || `#${deleteAccount?.id}`}」？该操作不可撤销，相关令牌将一并清除。
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
// Bulk actions toolbar — batch enable / disable / refresh / delete.
// ---------------------------------------------------------------------------

function AccountsBulkActions({ table }: { table: Table<Account> }) {
  const batchMutation = useBatchUpdateAccounts()

  const selectedIds = useMemo(
    () =>
      table
        .getFilteredSelectedRowModel()
        .rows.map((row) => accountSchema.parse(row.original).id),
    [table],
  )

  const runBatch = async (action: BatchAccountAction) => {
    if (selectedIds.length === 0) return
    try {
      await batchMutation.mutateAsync({ ids: selectedIds, action })
      table.resetRowSelection()
    } catch {
      // http-client toasted
    }
  }

  return (
    <DataTableBulkActions table={table} entityName='账号'>
      <Button
        size='xs'
        variant='outline'
        onClick={() => runBatch('refreshBalance')}
        disabled={batchMutation.isPending}
      >
        <RefreshCw />
        刷新余额
      </Button>
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
      <Button
        size='xs'
        variant='destructive'
        onClick={() => runBatch('delete')}
        disabled={batchMutation.isPending}
      >
        <Trash2 />
        删除
      </Button>
    </DataTableBulkActions>
  )
}
