import { Plus, Upload as UploadIcon, Users } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { PageHeader } from '@/components/common/page-header'
import { useProbeHistory } from '@/components/common/use-probe-history'
import { DataTablePage, useDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'

import { useAccountsPage } from '../api'
import { resolveAccountDisplayName } from '../lib/accounts-display-name'
import { type Account, accountSchema } from '../types'
import { AccountInlineDetails } from './account-inline-details'
import { AccountsBulkActions } from './accounts-bulk-actions'
import { useAccountsColumns } from './accounts-columns'
import { useAccountRowActions } from './use-account-row-actions'
import { useAccountsPageDialogs } from './use-accounts-page-dialogs'
import { useAccountsUrlState } from './use-accounts-url-state'

// A stable filter lets the table search already-parsed rows on each keystroke.
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
  const urlState = useAccountsUrlState()
  const accountsPageQuery = useAccountsPage({
    pageIndex: urlState.pagination.pageIndex,
    pageSize: urlState.pagination.pageSize,
    // Server-side filters (#1108): q/status/site narrow the whole fleet in
    // SQL, so rows on other pages match — unlike the old client-side
    // filters over the loaded page only.
    q: urlState.globalFilter,
    status: urlState.filters.status,
    site: urlState.filters.site,
  })
  const { data, isLoading, isFetching, error, refetch } = accountsPageQuery
  const probeHistoryQuery = useProbeHistory('accounts')
  // Parse the raw server-page rows once (WeakMap-cached per raw object)
  // instead of per cell/per accessor/per filter — see parseAccountRow above.
  const accounts = useMemo(() => {
    // Production page data is already server-paged. Snapshot-shaped fixtures
    // (kept for legacy tests) are sliced here so they exercise the same
    // URL-controlled page view without a dedicated data source.
    const rawRows = data?.items ?? data?.accounts ?? []
    const rows = data?.items
      ? rawRows
      : rawRows.slice(
          urlState.pagination.pageIndex * urlState.pagination.pageSize,
          (urlState.pagination.pageIndex + 1) * urlState.pagination.pageSize
        )
    return rows.map(parseAccountRow)
  }, [data, urlState.pagination])
  const sites = data?.sites ?? []

  const { openCreate, openImport, openEdit, openDelete, openDetail, dialogs } =
    useAccountsPageDialogs(accounts, accountsPageQuery)
  const {
    rowActions,
    pendingStatusId,
    pendingCheckinId,
    pendingPinId,
    pendingManualCheckinId,
  } = useAccountRowActions({ openEdit, openDelete, openDetail })
  const [expandedAccountIds, setExpandedAccountIds] = useState<
    ReadonlySet<number>
  >(() => new Set())
  const toggleAccountExpansion = useCallback((id: number) => {
    setExpandedAccountIds((previous) => {
      const next = new Set(previous)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])
  const accountDisclosureContentId = useCallback(
    (id: number) => `account-details-${id}`,
    []
  )
  const disclosureOptions = useMemo(
    () => ({
      expandedIds: expandedAccountIds,
      toggle: toggleAccountExpansion,
      getContentId: accountDisclosureContentId,
    }),
    [expandedAccountIds, toggleAccountExpansion, accountDisclosureContentId]
  )

  const columns = useAccountsColumns(
    rowActions,
    pendingStatusId,
    pendingCheckinId,
    pendingPinId,
    pendingManualCheckinId,
    // Row-level probe history: ONE batch fetch per page render, rendered as
    // health bars; a failed fetch only hides the bars, never the table.
    probeHistoryQuery.data,
    disclosureOptions
  )

  const { table } = useDataTable({
    data: accounts,
    columns,
    getRowId: (account) => String(account.id),
    enableRowSelection: true,
    // Server-side pagination + filtering (#1108): URL pagination drives a
    // page query and the true fleet total drives the pager; q/status/site
    // are SQL filters so a page only ever holds already-matching rows.
    // Sorting stays client-side over the current page.
    manualPagination: true,
    manualSorting: true,
    manualFiltering: true,
    totalCount: data?.total ?? data?.accounts?.length ?? 0,
    globalFilter: urlState.globalFilter,
    onGlobalFilterChange: urlState.onGlobalFilterChange,
    columnFilters: urlState.columnFilters,
    onColumnFiltersChange: urlState.onColumnFiltersChange,
    pagination: urlState.pagination,
    onPaginationChange: urlState.onPaginationChange,
    sorting: urlState.sorting,
    onSortingChange: urlState.onSortingChange,
    ensurePageInRange: urlState.ensurePageInRange,
    // URL state owns pagination. A fresh URL-derived filters array must not
    // let TanStack bounce an explicit page selection back to page 0.
    autoResetPageIndex: false,
    globalFilterFn: accountsGlobalFilterFn,
  })

  return (
    <div className='page-padding flex min-h-full flex-col gap-3 min-[641px]:h-full'>
      <PageHeader
        title={t('accounts.page.title')}
        description={t('accounts.page.description')}
        actions={
          <>
            {/* `sites` is `[]` both while the snapshot is still fetching and for
            a genuinely empty library; pinning disabled to the raw array kept
            the button inoperable for the whole fetch window right after
            creating a fresh site. Only the loaded-and-empty state disables
            it — during load the dialog opens and picks up the sites as the
            snapshot lands (the form consumes the live `sites` prop). */}
            {/* Disabled lives on a wrapping span so the tooltip explaining the
            precondition still works (a disabled button swallows pointer
            events, and with them any title of its own). */}
            <span
              title={
                !isLoading && sites.length === 0
                  ? t('accounts.page.addDisabledHint')
                  : undefined
              }
            >
              <Button
                onClick={openCreate}
                disabled={!isLoading && sites.length === 0}
              >
                <Plus />
                {t('accounts.page.addButton')}
              </Button>
            </span>
          </>
        }
      />

      <div
        role='group'
        aria-label={t('accounts.page.filterStatusTitle')}
        className='flex flex-wrap gap-2'
      >
        {(
          [
            { value: 'all', labelKey: 'userAdmin.allStatuses' },
            { value: 'active', labelKey: 'accounts.page.filterStatusActive' },
            {
              value: 'disabled',
              labelKey: 'accounts.page.filterStatusDisabled',
            },
            {
              value: 'expired',
              labelKey: 'accounts.page.filterStatusExpired',
            },
          ] as const
        ).map((option) => {
          const currentStatus = urlState.filters.status
            .split(',')
            .filter(Boolean)[0]
          const pressed =
            option.value === 'all'
              ? !currentStatus
              : currentStatus === option.value
          return (
            <Button
              key={option.value}
              size='sm'
              variant={pressed ? 'default' : 'outline'}
              className='rounded-full'
              aria-pressed={pressed}
              disabled={isLoading || !!error}
              onClick={() =>
                urlState.updateUrlState({
                  pageIndex: 0,
                  filters: {
                    status: option.value === 'all' ? '' : option.value,
                  },
                })
              }
            >
              {t(option.labelKey)}
            </Button>
          )
        })}
      </div>

      <DataTablePage
        table={table}
        isLoading={isLoading}
        isFetching={isFetching}
        error={error as Error | null}
        errorMessageKey='accounts.page.loadError'
        onErrorRetry={() => refetch()}
        isErrorRetrying={isFetching}
        rowDisclosure={{
          isExpanded: (row) => expandedAccountIds.has(row.original.id),
          onToggle: (row) => toggleAccountExpansion(row.original.id),
          getContentId: (row) => accountDisclosureContentId(row.original.id),
          getLabel: (row) =>
            resolveAccountDisplayName(
              row.original,
              t('accounts.columns.fallbackApiKey'),
              t('accounts.columns.fallbackUnnamed')
            ),
          renderContent: (row) => (
            <AccountInlineDetails
              account={row.original}
              onViewDetail={rowActions.onViewDetail}
              onRefresh={rowActions.onRefresh}
            />
          ),
        }}
        emptyIcon={<Users className='size-6' />}
        emptyTitle={t('accounts.page.emptyTitle')}
        emptyDescription={t('accounts.page.emptyDescription')}
        emptyAction={
          <div className='flex flex-wrap items-center justify-center gap-2'>
            <Button onClick={openImport}>
              <UploadIcon className='size-4' />
              {t('accounts.page.emptyImport')}
            </Button>
            <span
              title={
                !isLoading && sites.length === 0
                  ? t('accounts.page.addDisabledHint')
                  : undefined
              }
            >
              <Button
                variant='outline'
                onClick={openCreate}
                disabled={!isLoading && sites.length === 0}
              >
                <Plus className='size-4' />
                {t('accounts.page.addButton')}
              </Button>
            </span>
          </div>
        }
        skeletonKeyPrefix='accounts-skeleton'
        toolbarProps={{
          searchPlaceholder: t('accounts.page.searchPlaceholder'),
          searchDebounceMs: 300,
          // The wizard was only reachable from the empty-state CTA, i.e.
          // unreachable once the first account existed. The accounts toolbar
          // keeps a permanent Import entry (same batch site+account path the
          // sites page exposes), reusing the mounted wizard.
          preActions: (
            <Button variant='outline' onClick={openImport}>
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

      {dialogs}
    </div>
  )
}
