// metapi-go features/accounts/components — the accounts list page.
//
// Wires the data-table four-layer package (useDataTable + DataTablePage) to
// the useAccounts snapshot query, with client-side pagination/filtering/
// sorting and URL-synced table state (page / pageSize / global search /
// status filter / site filter). Mobile card degradation is handled
// automatically by DataTablePage. Row actions + bulk actions call the
// TanStack Query mutation hooks; the create/edit form, detail sheet, and
// delete confirm live as siblings of the table.

import { useNavigate, useSearch } from '@tanstack/react-router'
import type {
  ColumnFiltersState,
  OnChangeFn,
  PaginationState,
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
import { ImportWizardDialog } from '@/features/import'
import { asStringParam } from '@/lib/helpers/searchParams'
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
import { resolveDeepLinkPreselect } from '../lib/accounts-deep-link'
import { type Account, type AccountRowActions, accountSchema } from '../types'
import { AccountDetailSheet } from './account-detail-sheet'
import { AccountFormDialog } from './account-form-dialog'
import { useAccountsColumns } from './accounts-columns'

const DEFAULT_PAGE_SIZE = 20

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export function AccountsPage() {
  const { t } = useTranslation()
  // URL state is owned by the router: read the validated search via
  // `useSearch` (no `window.location.search`), and write changes back via
  // `navigate({ search, replace: true })` (no `history.replaceState`).
  const search = useSearch({ from: '/_authenticated/accounts' })
  const navigate = useNavigate()
  const { data, isLoading, isFetching, error } = useAccounts()
  const accounts = data?.accounts ?? []
  const sites = data?.sites ?? []

  const refreshMutation = useRefreshAccount()
  const deleteMutation = useDeleteAccount()
  const pinMutation = useToggleAccountPin()
  const statusMutation = useToggleAccountStatus()
  const checkinMutation = useToggleAccountCheckin()

  // --- table state (client-side, URL-synced) ---
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: (search.page ?? 1) - 1,
    pageSize: search.pageSize ?? DEFAULT_PAGE_SIZE,
  })
  const [globalFilter, setGlobalFilter] = useState(
    asStringParam(search.q) ?? ''
  )
  const [columnFilters, setColumnFilters] = useState<ColumnFiltersState>(() => {
    const filters: ColumnFiltersState = []
    const statusValues =
      asStringParam(search.status)?.split(',').filter(Boolean) ?? []
    const siteIds = asStringParam(search.site)?.split(',').filter(Boolean) ?? []
    if (statusValues.length) {
      filters.push({ id: 'status', value: statusValues })
    }
    if (siteIds.length) {
      filters.push({ id: 'site', value: siteIds })
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
    const statusFilter = columnFilters.find((filter) => filter.id === 'status')
    const siteFilter = columnFilters.find((filter) => filter.id === 'site')
    navigate({
      to: '/accounts',
      search: {
        page: pagination.pageIndex > 0 ? pagination.pageIndex + 1 : undefined,
        pageSize:
          pagination.pageSize !== DEFAULT_PAGE_SIZE
            ? pagination.pageSize
            : undefined,
        q: globalFilter || undefined,
        status:
          Array.isArray(statusFilter?.value) && statusFilter.value.length
            ? statusFilter.value.join(',')
            : undefined,
        site:
          Array.isArray(siteFilter?.value) && siteFilter.value.length
            ? siteFilter.value.join(',')
            : undefined,
      },
      replace: true,
    })
  }, [pagination, globalFilter, columnFilters, navigate])

  // onChange wrappers that reset to the first page on filter changes.
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

  // --- dialog state ---
  const [formOpen, setFormOpen] = useState(false)
  const [formMode, setFormMode] = useState<'create' | 'edit'>('create')
  const [editAccount, setEditAccount] = useState<Account | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailAccount, setDetailAccount] = useState<Account | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteAccount, setDeleteAccount] = useState<Account | null>(null)
  const [importOpen, setImportOpen] = useState(false)
  const [preselectedSiteId, setPreselectedSiteId] = useState<number | undefined>(
    undefined
  )

  const openCreate = () => {
    setFormMode('create')
    setEditAccount(null)
    setPreselectedSiteId(undefined)
    setFormOpen(true)
  }

  // Consume the one-shot site → account deep link exactly once: resolve the
  // referenced site against the loaded snapshot, open the create dialog with
  // it preselected, then strip the transient params from the URL so a refetch
  // or remount never reopens the dialog. Waits for the snapshot so a stale or
  // unknown `siteId` falls back safely instead of creating data.
  const deepLinkConsumed = useRef(false)
  useEffect(() => {
    if (deepLinkConsumed.current || search.create !== true) return
    if (isLoading) return

    const resolvedSiteId = resolveDeepLinkPreselect(
      search.create,
      search.siteId,
      data?.sites ?? []
    )
    if (resolvedSiteId !== null) {
      setPreselectedSiteId(resolvedSiteId)
      setFormMode('create')
      setEditAccount(null)
      setFormOpen(true)
    }

    deepLinkConsumed.current = true
    navigate({
      to: '/accounts',
      search: { ...search, siteId: undefined, create: undefined },
      replace: true,
    })
  }, [search, isLoading, data, navigate])

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

      {/* Create / edit form (Sheet) */}
      <AccountFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        mode={formMode}
        account={editAccount}
        sites={sites}
        initialSiteId={preselectedSiteId}
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
