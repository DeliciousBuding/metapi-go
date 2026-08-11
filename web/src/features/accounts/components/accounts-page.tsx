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
import type {
  ColumnFiltersState,
  OnChangeFn,
  PaginationState,
  Table,
} from '@tanstack/react-table'
import { Loader2, Plus, Power, RefreshCw, Trash2 } from 'lucide-react'
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
  const { t } = useTranslation()
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
    if (pagination.pageSize !== DEFAULT_PAGE_SIZE) {
      params.set('pageSize', String(pagination.pageSize))
    }
    if (globalFilter) params.set('q', globalFilter)
    const statusFilter = columnFilters.find((filter) => filter.id === 'status')
    if (statusFilter && Array.isArray(statusFilter.value) && statusFilter.value.length) {
      params.set('status', statusFilter.value.join(','))
    }
    const siteFilter = columnFilters.find((filter) => filter.id === 'site')
    if (siteFilter && Array.isArray(siteFilter.value) && siteFilter.value.length) {
      params.set('site', siteFilter.value.join(','))
    }
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
          <h1 className='text-lg font-semibold'>{t('accounts.page.title')}</h1>
          <p className='text-sm text-muted-foreground'>
            {t('accounts.page.description')}
          </p>
        </div>
        <Button onClick={openCreate} disabled={sites.length === 0}>
          <Plus />
          {t('accounts.page.addButton')}
        </Button>
      </div>

      {error && (
        <div className='rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive'>
          {t('accounts.page.loadError', { message: (error as Error).message })}
        </div>
      )}

      <DataTablePage
        table={table}
        columns={columns}
        isLoading={isLoading}
        isFetching={isFetching}
        emptyTitle={t('accounts.page.emptyTitle')}
        emptyDescription={t('accounts.page.emptyDescription')}
        skeletonKeyPrefix='accounts-skeleton'
        toolbarProps={{
          searchPlaceholder: t('accounts.page.searchPlaceholder'),
          searchDebounceMs: 300,
          filters: [
            {
              columnId: 'status',
              title: t('accounts.page.filterStatusTitle'),
              singleSelect: true,
              options: [
                { label: t('accounts.page.filterStatusActive'), value: 'active' },
                { label: t('accounts.page.filterStatusDisabled'), value: 'disabled' },
                { label: t('accounts.page.filterStatusExpired'), value: 'expired' },
              ],
            },
            ...(sites.length > 0
              ? [
                  {
                    columnId: 'site',
                    title: t('accounts.page.filterSiteTitle'),
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
            <DialogTitle>{t('accounts.page.deleteTitle')}</DialogTitle>
            <DialogDescription>
              {t('accounts.page.deleteDescription', {
                name: deleteAccount?.username || `#${deleteAccount?.id}`,
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

// ---------------------------------------------------------------------------
// Bulk actions toolbar — batch enable / disable / refresh / delete.
// ---------------------------------------------------------------------------

function AccountsBulkActions({ table }: { table: Table<Account> }) {
  const { t } = useTranslation()
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
    <DataTableBulkActions table={table} entityName={t('accounts.bulk.entityName')}>
      <Button
        size='xs'
        variant='outline'
        onClick={() => runBatch('refreshBalance')}
        disabled={batchMutation.isPending}
      >
        <RefreshCw />
        {t('accounts.bulk.refreshBalance')}
      </Button>
      <Button
        size='xs'
        variant='outline'
        onClick={() => runBatch('enable')}
        disabled={batchMutation.isPending}
      >
        <Power />
        {t('accounts.bulk.enable')}
      </Button>
      <Button
        size='xs'
        variant='outline'
        onClick={() => runBatch('disable')}
        disabled={batchMutation.isPending}
      >
        {t('accounts.bulk.disable')}
      </Button>
      <Button
        size='xs'
        variant='destructive'
        onClick={() => runBatch('delete')}
        disabled={batchMutation.isPending}
      >
        <Trash2 />
        {t('accounts.bulk.delete')}
      </Button>
    </DataTableBulkActions>
  )
}
