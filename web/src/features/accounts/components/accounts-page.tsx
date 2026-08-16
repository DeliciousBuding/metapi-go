// metapi-go features/accounts/components — the accounts list page.
//
// Wires the data-table package to the accounts snapshot query with client-side
// filtering/sorting/pagination and URL-owned table state. The URL is the single
// source of truth: do not mirror it into local state and write it back from an
// effect. That pattern can create a router <-> table feedback loop when a
// controlled callback changes identity during render.

import type {
  ColumnFiltersState,
  FilterFn,
  Table,
} from '@tanstack/react-table'
import {
  Loader2,
  Plus,
  Power,
  RefreshCw,
  Trash2,
  Upload as UploadIcon,
} from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import {
  DataTableBulkActions,
  DataTablePage,
  encodeSorting,
  useDataTable,
  useUrlTableState,
  type UrlTableState,
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
import { ImportWizardDialog } from '@/features/import'
import { asStringParam, parseSortingParam } from '@/lib/helpers/searchParams'
import { toast } from '@/lib/toast'

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
  accountsSearchSchema,
  DEFAULT_ACCOUNTS_PAGE_SIZE,
} from '../lib/accounts-search-schema'
import { type Account, type AccountRowActions, accountSchema } from '../types'
import { AccountDetailSheet } from './account-detail-sheet'
import { AccountFormDialog } from './account-form-dialog'
import { useAccountsColumns } from './accounts-columns'

type AccountsUrlFilters = {
  status: string | undefined
  site: string | undefined
}

const EMPTY_ACCOUNTS_URL_STATE: UrlTableState<AccountsUrlFilters> = {
  q: '',
  pageIndex: 0,
  pageSize: DEFAULT_ACCOUNTS_PAGE_SIZE,
  sorting: [],
  filters: { status: undefined, site: undefined },
}

function readAccountsSearch(
  searchString?: string
): UrlTableState<AccountsUrlFilters> {
  if (typeof window === 'undefined' && searchString === undefined) {
    return EMPTY_ACCOUNTS_URL_STATE
  }

  const params = new URLSearchParams(
    searchString ?? (typeof window !== 'undefined' ? window.location.search : '')
  )
  const parsed = accountsSearchSchema.safeParse({
    q: params.get('q') ?? undefined,
    page: params.get('page') ?? undefined,
    pageSize: params.get('pageSize') ?? undefined,
    sort: params.get('sort') ?? undefined,
    status: params.get('status') ?? undefined,
    site: params.get('site') ?? undefined,
  })
  if (!parsed.success) return EMPTY_ACCOUNTS_URL_STATE

  const data = parsed.data
  return {
    q: asStringParam(data.q) ?? '',
    pageIndex: Math.max(0, (data.page ?? 1) - 1),
    pageSize: data.pageSize ?? DEFAULT_ACCOUNTS_PAGE_SIZE,
    sorting: parseSortingParam(data.sort),
    filters: {
      status: asStringParam(data.status),
      site: asStringParam(data.site),
    },
  }
}

function buildAccountsHref(
  next: Partial<UrlTableState<AccountsUrlFilters>>
): string {
  // Read the browser's current URL at callback time rather than closing over a
  // render snapshot. TanStack can emit several controlled-state updates in one
  // turn; merging against the latest URL prevents one update from erasing
  // another (for example page-size change dropping an active site filter).
  const current = readAccountsSearch()
  const merged: UrlTableState<AccountsUrlFilters> = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }

  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex + 1))
  if (merged.pageSize !== DEFAULT_ACCOUNTS_PAGE_SIZE) {
    params.set('pageSize', String(merged.pageSize))
  }
  const sort = encodeSorting(merged.sorting)
  if (sort) params.set('sort', sort)
  if (merged.filters.status) params.set('status', merged.filters.status)
  if (merged.filters.site) params.set('site', merged.filters.site)

  const queryString = params.toString()
  return queryString ? `/accounts?${queryString}` : '/accounts'
}

function splitFilter(value: string | undefined): string[] {
  return value?.split(',').map((item) => item.trim()).filter(Boolean) ?? []
}

function toAccountsColumnFilters(
  filters: AccountsUrlFilters
): ColumnFiltersState {
  const result: ColumnFiltersState = []
  const statuses = splitFilter(filters.status)
  const sites = splitFilter(filters.site)
  if (statuses.length > 0) result.push({ id: 'status', value: statuses })
  if (sites.length > 0) result.push({ id: 'site', value: sites })
  return result
}

function filterValueToString(value: unknown): string | undefined {
  if (Array.isArray(value)) {
    const values = value
      .filter((item): item is string => typeof item === 'string')
      .filter(Boolean)
    return values.length > 0 ? values.join(',') : undefined
  }
  return typeof value === 'string' && value ? value : undefined
}

function fromAccountsColumnFilters(
  filters: ColumnFiltersState
): Partial<UrlTableState<AccountsUrlFilters>> {
  return {
    filters: {
      status: filterValueToString(
        filters.find((filter) => filter.id === 'status')?.value
      ),
      site: filterValueToString(
        filters.find((filter) => filter.id === 'site')?.value
      ),
    },
  }
}

function useAccountsUrlState() {
  return useUrlTableState<AccountsUrlFilters>({
    basePath: '/accounts',
    read: readAccountsSearch,
    buildHref: buildAccountsHref,
    toColumnFilters: toAccountsColumnFilters,
    fromColumnFilters: fromAccountsColumnFilters,
  })
}

const accountsGlobalFilterFn: FilterFn<Account> = (
  row,
  _columnId,
  filterValue
) => {
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
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export function AccountsPage() {
  const { t } = useTranslation()
  const { data, isLoading, isFetching, error } = useAccounts()
  const accounts = data?.accounts ?? []
  const sites = data?.sites ?? []
  const urlState = useAccountsUrlState()

  const { mutate: refreshAccount } = useRefreshAccount()
  const deleteMutation = useDeleteAccount()
  const { mutate: togglePin } = useToggleAccountPin()
  const { mutate: toggleStatus } = useToggleAccountStatus()
  const { mutate: toggleCheckin } = useToggleAccountCheckin()

  // --- dialog state ---
  const [formOpen, setFormOpen] = useState(false)
  const [formMode, setFormMode] = useState<'create' | 'edit'>('create')
  const [editAccount, setEditAccount] = useState<Account | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailAccount, setDetailAccount] = useState<Account | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteAccount, setDeleteAccount] = useState<Account | null>(null)
  const [importOpen, setImportOpen] = useState(false)

  const openCreate = useCallback(() => {
    setFormMode('create')
    setEditAccount(null)
    setFormOpen(true)
  }, [])

  const openEdit = useCallback((account: Account) => {
    setFormMode('edit')
    setEditAccount(account)
    setFormOpen(true)
  }, [])

  // Column definitions are derived from these actions, so keep the action
  // object stable as well. Rebuilding columns on every render can cause table
  // feature state to re-register even when no visible data changed.
  const rowActions = useMemo<AccountRowActions>(
    () => ({
      onEdit: openEdit,
      onDelete: (account) => {
        setDeleteAccount(account)
        setDeleteOpen(true)
      },
      onRefresh: (account) => refreshAccount(account.id),
      onViewDetail: (account) => {
        setDetailAccount(account)
        setDetailOpen(true)
      },
      onTogglePin: (account) =>
        togglePin({ id: account.id, isPinned: !account.isPinned }),
      onToggleStatus: (account) =>
        toggleStatus({
          id: account.id,
          status: account.status === 'active' ? 'disabled' : 'active',
        }),
      onToggleCheckin: (account) =>
        toggleCheckin({
          id: account.id,
          checkinEnabled: !account.checkinEnabled,
        }),
    }),
    [openEdit, refreshAccount, toggleCheckin, togglePin, toggleStatus]
  )

  const columns = useAccountsColumns(rowActions)

  const { table } = useDataTable({
    data: accounts,
    columns,
    enableRowSelection: true,
    globalFilter: urlState.globalFilter,
    onGlobalFilterChange: urlState.onGlobalFilterChange,
    columnFilters: urlState.columnFilters,
    onColumnFiltersChange: urlState.onColumnFiltersChange,
    sorting: urlState.sorting,
    onSortingChange: urlState.onSortingChange,
    pagination: urlState.pagination,
    onPaginationChange: urlState.onPaginationChange,
    ensurePageInRange: urlState.ensurePageInRange,
    globalFilterFn: accountsGlobalFilterFn,
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
          <h1 className='text-lg font-normal'>{t('accounts.page.title')}</h1>
          <p className='text-muted-foreground text-sm'>
            {t('accounts.page.description')}
          </p>
        </div>
        <Button onClick={openCreate} disabled={sites.length === 0}>
          <Plus />
          {t('accounts.page.addButton')}
        </Button>
      </div>

      {error && (
        <div className='border-destructive/40 bg-destructive/10 text-destructive-soft-fg rounded-lg border p-3 text-sm'>
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
        emptyAction={
          <Button onClick={() => setImportOpen(true)}>
            <UploadIcon className='mr-1 size-4' />
            {t('accounts.page.emptyImport')}
          </Button>
        }
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
                {
                  label: t('accounts.page.filterStatusActive'),
                  value: 'active',
                },
                {
                  label: t('accounts.page.filterStatusDisabled'),
                  value: 'disabled',
                },
                {
                  label: t('accounts.page.filterStatusExpired'),
                  value: 'expired',
                },
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

      <ImportWizardDialog open={importOpen} onOpenChange={setImportOpen} />

      <AccountFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        mode={formMode}
        account={editAccount}
        sites={sites}
      />

      <AccountDetailSheet
        account={detailAccount}
        open={detailOpen}
        onOpenChange={setDetailOpen}
      />

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
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false)

  const selectedIds = useMemo(
    () =>
      table
        .getFilteredSelectedRowModel()
        .rows.map((row) => accountSchema.parse(row.original).id),
    [table]
  )

  const runBatch = async (action: BatchAccountAction) => {
    if (selectedIds.length === 0) return
    try {
      const result = await batchMutation.mutateAsync({
        ids: selectedIds,
        action,
      })
      // Partial failures are toasted by the mutation hook (failedItems);
      // only report success here to avoid double toasts.
      if ((result?.failedItems ?? []).length === 0) {
        toast.success(
          t('accounts.toast.bulkSucceeded', {
            count: result?.successIds?.length ?? selectedIds.length,
          })
        )
      }
      table.resetRowSelection()
    } catch {
      // http-client toasted
    }
  }

  const confirmBulkDelete = async () => {
    if (selectedIds.length === 0) return
    try {
      const result = await batchMutation.mutateAsync({
        ids: selectedIds,
        action: 'delete',
      })
      if ((result?.failedItems ?? []).length === 0) {
        toast.success(
          t('accounts.toast.bulkSucceeded', {
            count: result?.successIds?.length ?? selectedIds.length,
          })
        )
      }
      table.resetRowSelection()
      setConfirmDeleteOpen(false)
    } catch {
      // http-client toasted
      setConfirmDeleteOpen(false)
    }
  }

  return (
    <>
      <DataTableBulkActions
        table={table}
        entityName={t('accounts.bulk.entityName')}
      >
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
          onClick={() => setConfirmDeleteOpen(true)}
          disabled={batchMutation.isPending}
        >
          <Trash2 />
          {t('accounts.bulk.delete')}
        </Button>
      </DataTableBulkActions>

      <Dialog open={confirmDeleteOpen} onOpenChange={setConfirmDeleteOpen}>
        <DialogContent className='sm:max-w-sm'>
          <DialogHeader>
            <DialogTitle>{t('accounts.bulk.deleteConfirmTitle')}</DialogTitle>
            <DialogDescription>
              {t('accounts.bulk.deleteConfirmDescription', {
                count: selectedIds.length,
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setConfirmDeleteOpen(false)}
              disabled={batchMutation.isPending}
            >
              {t('accounts.bulk.deleteConfirmCancel')}
            </Button>
            <Button
              variant='destructive'
              onClick={confirmBulkDelete}
              disabled={batchMutation.isPending}
            >
              {batchMutation.isPending && <Loader2 className='animate-spin' />}
              {t('accounts.bulk.deleteConfirmConfirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
