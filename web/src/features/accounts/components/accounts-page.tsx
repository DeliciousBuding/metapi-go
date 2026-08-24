// metapi-go features/accounts/components — the accounts list page.
//
// Wires the data-table four-layer package (useDataTable + DataTablePage) to
// the useAccounts snapshot query, with client-side pagination/filtering/
// sorting and URL-synced table state (page / pageSize / global search /
// status filter / site filter). Mobile card degradation is handled
// automatically by DataTablePage. Row actions + bulk actions call the
// TanStack Query mutation hooks; the create/edit form, detail sheet, and
// delete confirm live as siblings of the table.
//
// URL state uses the shared useUrlTableState hook (same as sites/oauth/
// models): the URL is the single source of truth and the callbacks navigate,
// so there is no local state + navigate effect that can feed back into an
// infinite render loop (the accounts page previously froze the renderer on
// any interaction — see the URL-state loop fix).

import { useNavigate, useSearch } from '@tanstack/react-router'
import type { ColumnFiltersState, Table } from '@tanstack/react-table'
import {
  Plus,
  Power,
  RefreshCw,
  Trash2,
  Upload as UploadIcon,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import {
  DataTableBulkActions,
  DataTablePage,
  encodeSorting,
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
import { ImportWizardDialog } from '@/features/import'
import { parseSortingParam } from '@/lib/helpers/searchParams'
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
  resolveDeepLinkCredentialMode,
  resolveDeepLinkPreselect,
} from '../lib/accounts-deep-link'
import { resolveAccountDisplayName } from '../lib/accounts-display-name'
import {
  type Account,
  type AccountRowActions,
  type CredentialMode,
  accountSchema,
} from '../types'
import { AccountDetailSheet } from './account-detail-sheet'
import { AccountFormDialog } from './account-form-dialog'
import { useAccountsColumns } from './accounts-columns'

const DEFAULT_PAGE_SIZE = 20

/** Page-specific URL filters: comma-separated status + site id lists. */
type AccountsUrlFilters = {
  status: string
  site: string
}

/**
 * Parse the raw search string into URL table state. The accounts URL uses a
 * 1-based `page` param (route schema: accountsSearchSchema); `q` / `status` /
 * `site` are plain strings.
 */
function readAccountsSearch(
  searchString: string
): UrlTableState<AccountsUrlFilters> {
  const params = new URLSearchParams(searchString)
  const rawPage = Number(params.get('page') ?? '1')
  const rawPageSize = Number(
    params.get('pageSize') ?? String(DEFAULT_PAGE_SIZE)
  )
  const pageIndex =
    Number.isFinite(rawPage) && rawPage > 0 ? Math.max(0, rawPage - 1) : 0
  const pageSize =
    Number.isFinite(rawPageSize) && rawPageSize > 0
      ? Math.min(100, Math.max(1, rawPageSize))
      : DEFAULT_PAGE_SIZE
  return {
    q: params.get('q') ?? '',
    pageIndex,
    pageSize,
    sorting: parseSortingParam(params.get('sort') ?? undefined),
    filters: {
      status: params.get('status') ?? '',
      site: params.get('site') ?? '',
    },
  }
}

/** Serialize a partial state update back to the 1-based accounts href,
 *  merging over the CURRENT url state (so a pagination sync preserves q /
 *  status / site — stripping them re-triggered the toolbar's debounced
 *  search commit and looped the renderer). */
function buildAccountsHref(
  next: UrlTableStateUpdate<AccountsUrlFilters>,
  currentSearch?: string
): string {
  const currentSearchString = currentSearch ?? window.location.search
  const currentParams = new URLSearchParams(currentSearchString)
  const current = readAccountsSearch(currentSearchString)
  const merged: UrlTableState<AccountsUrlFilters> = {
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
  const sortString = encodeSorting(merged.sorting)
  if (sortString) params.set('sort', sortString)
  if (merged.filters.status) params.set('status', merged.filters.status)
  if (merged.filters.site) params.set('site', merged.filters.site)
  // Transient deep-link params are preserved across table-state syncs until
  // the page's one-shot consume effects below have stripped them (a toolbar
  // commit or page clamp firing first would otherwise drop the deep link).
  // Reads the router search string (not window.location, which lags until the
  // transition commits — right after the strip, a stale snapshot would
  // resurrect siteId/create and reopen the dialog on the next reload).
  const guidedSiteId = currentParams.get('siteId')
  const guidedCreate = currentParams.get('create')
  const attentionAccountId = currentParams.get('accountId')
  if (guidedSiteId) params.set('siteId', guidedSiteId)
  if (guidedCreate) params.set('create', guidedCreate)
  if (attentionAccountId) params.set('accountId', attentionAccountId)
  const queryString = params.toString()
  return queryString ? `/accounts?${queryString}` : '/accounts'
}

/** The "feature useSearch" stage, mirroring the sites page. */
function useAccountsUrlState() {
  return useUrlTableState<AccountsUrlFilters>({
    basePath: '/accounts',
    read: readAccountsSearch,
    buildHref: buildAccountsHref,
    toColumnFilters: (filters) => {
      const out: ColumnFiltersState = []
      const statusValues = filters.status.split(',').filter(Boolean)
      const siteIds = filters.site.split(',').filter(Boolean)
      if (statusValues.length) out.push({ id: 'status', value: statusValues })
      if (siteIds.length) out.push({ id: 'site', value: siteIds })
      return out
    },
    fromColumnFilters: (filters) => {
      const statusEntry = filters.find((filter) => filter.id === 'status')
      const siteEntry = filters.find((filter) => filter.id === 'site')
      return {
        filters: {
          status: Array.isArray(statusEntry?.value)
            ? statusEntry.value.join(',')
            : '',
          site: Array.isArray(siteEntry?.value)
            ? siteEntry.value.join(',')
            : '',
        },
      }
    },
  })
}

// Module-level so the table's globalFilterFn keeps a stable identity across
// renders (a fresh inline function would re-resolve the table every render).
// Rows arrive here already parsed (see parseAccountRow below), so the filter
// reads the typed fields directly instead of re-running the 25-field Zod
// schema per row per keystroke.
function accountsGlobalFilterFn(
  row: { original: unknown },
  _columnId: string,
  filterValue: string
): boolean {
  const account = row.original as Account
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
// Row parsing (once per raw object)
// ---------------------------------------------------------------------------

// The snapshot rows are raw JSON; every column cell / accessorFn / filterFn
// used to call accountSchema.parse(row.original) itself, i.e. ~900 full Zod
// parses per render for a 100-row table — the bulk of the ~2s main-thread
// freeze measured on the accounts page. Parse each raw row exactly once and
// hand the parsed Account[] to the table. The WeakMap keeps the cache
// keyed by the raw object identity, so the optimistic toggles (which replace
// only the touched row via spread) re-parse a single row instead of all 100.
const parsedAccountRowCache = new WeakMap<object, Account>()

function parseAccountRow(rawRow: object): Account {
  const cached = parsedAccountRowCache.get(rawRow)
  if (cached) return cached
  const parsed = accountSchema.parse(rawRow)
  parsedAccountRowCache.set(rawRow, parsed)
  return parsed
}

// ---------------------------------------------------------------------------
// Page
// ---------------------------------------------------------------------------

export function AccountsPage() {
  const { t } = useTranslation()
  const search = useSearch({ from: '/_authenticated/accounts' })
  const navigate = useNavigate()
  const urlState = useAccountsUrlState()
  const { data, isLoading, isFetching, error, refetch } = useAccounts()
  // Parse the raw snapshot rows once (WeakMap-cached per raw object) instead
  // of per cell/per accessor/per filter — see parseAccountRow above.
  const accounts = useMemo(
    () => (data?.accounts ?? []).map(parseAccountRow),
    [data]
  )
  const sites = data?.sites ?? []

  const { mutate: refreshAccount } = useRefreshAccount()
  const deleteMutation = useDeleteAccount()
  // Same shape as statusMutation: keep the full mutation object so the pin
  // dropdown item can derive `pendingPinId` for a per-row spinner (mirrors
  // the Power button's pendingStatusId flow; independent per toggle so
  // pin/check-in/status never cross-talk).
  const pinMutation = useToggleAccountPin()
  const toggleAccountPin = pinMutation.mutate
  // Keep the full mutation object so we can derive `pendingStatusId` from
  // `isPending` + `variables` — the inline enable/disable button in each row
  // uses it to show a per-row spinner without a separate state manager.
  const statusMutation = useToggleAccountStatus()
  // Destructure `mutate` to a stable top-level identifier. The
  // `react-hooks/exhaustive-deps` linter flags property accesses like
  // `statusMutation.mutate` inside the memo deps (it wants the whole
  // `statusMutation` object, which is NOT stable across renders and would
  // re-trigger the render loop). The mutate fn identity itself is stable,
  // so a top-level const keeps both the linter and the memo happy.
  const toggleStatusMutate = statusMutation.mutate
  // Same shape as statusMutation: keep the full mutation object so the inline
  // check-in badge button can derive `pendingCheckinId` for a per-row spinner
  // (mirrors the Power button's pendingStatusId flow).
  const checkinMutation = useToggleAccountCheckin()
  const toggleCheckinMutate = checkinMutation.mutate

  // --- dialog state ---
  const [formOpen, setFormOpen] = useState(false)
  const [formMode, setFormMode] = useState<'create' | 'edit'>('create')
  const [editAccount, setEditAccount] = useState<Account | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [detailAccount, setDetailAccount] = useState<Account | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteAccount, setDeleteAccount] = useState<Account | null>(null)
  const [importOpen, setImportOpen] = useState(false)
  const [preselectedSiteId, setPreselectedSiteId] = useState<
    number | undefined
  >(undefined)
  const [preselectedCredentialMode, setPreselectedCredentialMode] = useState<
    CredentialMode | undefined
  >(undefined)

  const openCreate = () => {
    setFormMode('create')
    setEditAccount(null)
    setPreselectedSiteId(undefined)
    setPreselectedCredentialMode(undefined)
    setFormOpen(true)
  }

  // Consume the one-shot site → account deep link exactly once: resolve the
  // referenced site against the loaded snapshot, open the create dialog with
  // it preselected (apikey mode when the CTA passed the segment hint), then
  // strip the transient params from the URL so a refetch or remount never
  // reopens the dialog. Waits for the snapshot so a stale or unknown
  // `siteId` falls back safely instead of creating data. The snapshot query
  // is shared with the sites page (10s staleTime), so right after a guided
  // create the cached snapshot may still miss the new site — retry with a
  // refetch (bounded) before treating the id as unknown.
  const deepLinkConsumed = useRef(false)
  const deepLinkRefetchAttempts = useRef(0)
  useEffect(() => {
    if (deepLinkConsumed.current || search.create !== true) return
    if (isLoading || !data) return

    const stripDeepLink = () => {
      deepLinkConsumed.current = true
      navigate({
        to: '/accounts',
        search: {
          ...search,
          siteId: undefined,
          create: undefined,
          segment: undefined,
        },
        replace: true,
      })
    }

    const resolvedSiteId = resolveDeepLinkPreselect(
      search.create,
      search.siteId,
      data?.sites ?? []
    )
    if (resolvedSiteId !== null) {
      const resolvedMode = resolveDeepLinkCredentialMode(search.segment)
      setPreselectedSiteId(resolvedSiteId)
      setPreselectedCredentialMode(resolvedMode ?? undefined)
      setFormMode('create')
      setEditAccount(null)
      setFormOpen(true)
      stripDeepLink()
      return
    }

    // Site missing from the snapshot. Refetch once, a few times max, so a
    // just-created site resolves; an unknown stays silently consumed.
    if (deepLinkRefetchAttempts.current >= 3) {
      stripDeepLink()
      return
    }
    deepLinkRefetchAttempts.current += 1
    void refetch().catch(() => {
      // Fetch failures fall through; the next data change re-runs this
      // effect and eventually the retry bound consumes the link.
    })
  }, [search, isLoading, data, refetch, navigate])

  // Consume the one-shot attention deep link exactly once (dashboard
  // attention items link `/accounts?accountId=N` for expired / low-balance
  // accounts): resolve the referenced id against the loaded snapshot, open
  // the detail sheet with the same state the row "view detail" action uses,
  // then strip the transient param so a refetch or remount never reopens
  // the sheet. Waits for the snapshot so a stale or unknown id is cleared
  // silently instead of opening a blank sheet — mirrors the channels page's
  // `channelId` drilldown. The `useRef` guard survives the strict-mode
  // double-invoke and the post-navigate re-render.
  const accountDeepLinkConsumed = useRef(false)
  useEffect(() => {
    if (accountDeepLinkConsumed.current || !search.accountId) return
    if (isLoading) return

    const targetAccount = (data?.accounts ?? []).find(
      (account) => account.id === search.accountId
    )
    accountDeepLinkConsumed.current = true
    if (targetAccount) {
      setDetailAccount(targetAccount)
      setDetailOpen(true)
    }
    navigate({
      to: '/accounts',
      search: { ...search, accountId: undefined },
      replace: true,
    })
  }, [search, isLoading, data, navigate])

  const openEdit = useCallback((account: Account) => {
    setFormMode('edit')
    setEditAccount(account)
    setFormOpen(true)
  }, [])

  // --- row actions (handed to the columns hook) ---
  // Memoized so the column defs keep a stable identity across renders; a
  // fresh object every render re-resolves the TanStack table instance and
  // re-runs its autoResetPageIndex effect (the old render-loop feedback).
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
        toggleAccountPin(
          { id: account.id, isPinned: !account.isPinned },
          {
            onSuccess: () =>
              toast.success(
                t('accounts.toast.pinToggled', {
                  name: resolveAccountDisplayName(
                    account,
                    t('accounts.columns.fallbackApiKey'),
                    t('accounts.columns.fallbackUnnamed')
                  ),
                })
              ),
          }
        ),
      onToggleStatus: (account) =>
        toggleStatusMutate({
          id: account.id,
          status: account.status === 'active' ? 'disabled' : 'active',
        }),
      onToggleCheckin: (account) =>
        toggleCheckinMutate(
          {
            id: account.id,
            checkinEnabled: !account.checkinEnabled,
          },
          {
            onSuccess: () =>
              toast.success(
                t('accounts.toast.checkinToggled', {
                  name: resolveAccountDisplayName(
                    account,
                    t('accounts.columns.fallbackApiKey'),
                    t('accounts.columns.fallbackUnnamed')
                  ),
                })
              ),
          }
        ),
    }),
    [
      openEdit,
      refreshAccount,
      toggleAccountPin,
      toggleStatusMutate,
      toggleCheckinMutate,
      t,
    ]
  )

  // Derive the per-row pending ids from each TanStack Query mutation's
  // `variables` (the last mutate input) — one id per toggle so pin,
  // check-in, and status spinners never cross-talk. Passing these as
  // SEPARATE args to useAccountsColumns is safe: useDataTable stabilizes
  // onChange callbacks via a ref, NOT the columns array ref — so the actions
  // handlers stay memoized and only the cell render closures capture the new
  // pending state. The table instance does NOT re-instantiate (no
  // autoResetPageIndex loop).
  const pendingStatusId = statusMutation.isPending
    ? (statusMutation.variables?.id ?? null)
    : null
  const pendingCheckinId = checkinMutation.isPending
    ? (checkinMutation.variables?.id ?? null)
    : null
  const pendingPinId = pinMutation.isPending
    ? (pinMutation.variables?.id ?? null)
    : null

  const columns = useAccountsColumns(
    rowActions,
    pendingStatusId,
    pendingCheckinId,
    pendingPinId
  )

  const { table } = useDataTable({
    data: accounts,
    columns,
    enableRowSelection: true,
    globalFilter: urlState.globalFilter,
    onGlobalFilterChange: urlState.onGlobalFilterChange,
    columnFilters: urlState.columnFilters,
    onColumnFiltersChange: urlState.onColumnFiltersChange,
    pagination: urlState.pagination,
    onPaginationChange: urlState.onPaginationChange,
    sorting: urlState.sorting,
    onSortingChange: urlState.onSortingChange,
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
        {/* `sites` is `[]` both while the snapshot is still fetching and for
            a genuinely empty library; pinning disabled to the raw array kept
            the button inoperable for the whole fetch window right after
            creating a fresh site. Only the loaded-and-empty state disables
            it — during load the dialog opens and picks up the sites as the
            snapshot lands (the form consumes the live `sites` prop). */}
        <Button
          onClick={openCreate}
          disabled={!isLoading && sites.length === 0}
        >
          <Plus />
          {t('accounts.page.addButton')}
        </Button>
      </div>

      {error ? (
        <QueryErrorBanner
          error={error as Error | null}
          messageKey='accounts.page.loadError'
          onRetry={() => refetch()}
          isRetrying={isFetching}
        />
      ) : (
        <DataTablePage
          table={table}
          columns={columns}
          isLoading={isLoading}
          isFetching={isFetching}
          emptyTitle={t('accounts.page.emptyTitle')}
          emptyDescription={t('accounts.page.emptyDescription')}
          emptyAction={
            <Button onClick={() => setImportOpen(true)}>
              <UploadIcon className='size-4' />
              {t('accounts.page.emptyImport')}
            </Button>
          }
          skeletonKeyPrefix='accounts-skeleton'
          toolbarProps={{
            searchPlaceholder: t('accounts.page.searchPlaceholder'),
            searchDebounceMs: 300,
            // The wizard was only reachable from the empty-state CTA, i.e.
            // unreachable once the first account existed. The accounts toolbar
            // keeps a permanent Import entry (same batch site+account path the
            // sites page exposes), reusing the already-mounted setImportOpen.
            preActions: (
              <Button variant='outline' onClick={() => setImportOpen(true)}>
                <UploadIcon className='size-4' />
                {t('accounts.page.toolbarImport')}
              </Button>
            ),
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
      )}

      <ImportWizardDialog open={importOpen} onOpenChange={setImportOpen} />

      {/* Create / edit form (Sheet) */}
      <AccountFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        mode={formMode}
        account={editAccount}
        sites={sites}
        initialSiteId={preselectedSiteId}
        initialCredentialMode={preselectedCredentialMode}
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
              {deleteMutation.isPending && <Spinner />}
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

  // Derived per render — `table` identity is stable across selection changes
  // (TanStack `useReactTable` memoizes the instance), so a `useMemo([table])`
  // here would freeze the ids at their mount-time (empty) value and every
  // batch action would silently no-op.
  const selectedIds = table
    .getFilteredSelectedRowModel()
    .rows.map((row) => row.original.id)

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
          size='sm'
          variant='outline'
          onClick={() => runBatch('refreshBalance')}
          disabled={batchMutation.isPending}
        >
          <RefreshCw />
          {t('accounts.bulk.refreshBalance')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          onClick={() => runBatch('enable')}
          disabled={batchMutation.isPending}
        >
          <Power />
          {t('accounts.bulk.enable')}
        </Button>
        <Button
          size='sm'
          variant='outline'
          onClick={() => runBatch('disable')}
          disabled={batchMutation.isPending}
        >
          {t('accounts.bulk.disable')}
        </Button>
        <Button
          size='sm'
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
              {batchMutation.isPending && <Spinner />}
              {t('accounts.bulk.deleteConfirmConfirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
