import { Globe, Plus as PlusIcon, Upload as UploadIcon } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { PageHeader } from '@/components/common/page-header'
import { DataTablePage, useDataTable } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { useAccounts } from '@/features/accounts'

import { useSites } from '../api'
import type { Site } from '../types'
import { SITES_STATUS_FILTER_OPTIONS } from './sites-columns'
import { useSiteRowColumns } from './use-site-row-columns'
import { useSitesBulkActions } from './use-sites-bulk-actions'
import { useSitesPageDialogs } from './use-sites-page-dialogs'
import { useSitesUrlState } from './use-sites-url-state'

const SITES_COLUMN_VISIBILITY_STORAGE_KEY = 'metapi-go:sites:column-visibility'
const SITES_COLUMN_SIZING_STORAGE_KEY = 'metapi-go:sites:column-sizing'

export function SitesPage() {
  const { t } = useTranslation()
  const sitesQuery = useSites()
  // The /api/sites endpoint does not include account counts; enrich the rows
  // from the already-cached accounts snapshot (the documented plan in
  // types.ts). If the snapshot is unavailable the column falls back to '—'.
  const accountsQuery = useAccounts()
  const accountCountBySite = useMemo(() => {
    const counts = new Map<number, number>()
    for (const account of accountsQuery.data?.accounts ?? []) {
      const siteId = Number(account.siteId)
      if (Number.isFinite(siteId) && siteId > 0) {
        counts.set(siteId, (counts.get(siteId) ?? 0) + 1)
      }
    }
    return counts
  }, [accountsQuery.data])
  const sites = useMemo(() => {
    const rows = sitesQuery.data ?? []
    if (accountCountBySite.size === 0) return rows
    return rows.map((site) => ({
      ...site,
      accountCount:
        typeof site.accountCount === 'number'
          ? site.accountCount
          : (accountCountBySite.get(site.id) ?? 0),
    }))
  }, [sitesQuery.data, accountCountBySite])
  const dialogs = useSitesPageDialogs(sitesQuery.data, sitesQuery.isLoading)
  const columns = useSiteRowColumns(dialogs.rowActions)
  const urlState = useSitesUrlState()

  const { table } = useDataTable<Site>({
    data: sites,
    columns,
    enableRowSelection: true,
    enableColumnResizing: true,
    // New optional columns (system proxy / external check-in URL / created
    // at) start hidden so the default layout keeps its density; they can be
    // re-enabled via the View column switcher.
    initialColumnVisibility: {
      useSystemProxy: false,
      externalCheckinUrl: false,
      createdAt: false,
    },
    columnVisibilityStorageKey: SITES_COLUMN_VISIBILITY_STORAGE_KEY,
    columnSizingStorageKey: SITES_COLUMN_SIZING_STORAGE_KEY,
    globalFilter: urlState.globalFilter,
    onGlobalFilterChange: urlState.onGlobalFilterChange,
    pagination: urlState.pagination,
    onPaginationChange: urlState.onPaginationChange,
    sorting: urlState.sorting,
    onSortingChange: urlState.onSortingChange,
    columnFilters: urlState.columnFilters,
    onColumnFiltersChange: urlState.onColumnFiltersChange,
    ensurePageInRange: urlState.ensurePageInRange,
    // URL state owns pagination. A fresh URL-derived filters array must not
    // let TanStack bounce an explicit page selection back to page 0.
    autoResetPageIndex: false,
    getRowId: (row) => String(row.id),
  })

  const bulk = useSitesBulkActions(table)
  const statusFilters = SITES_STATUS_FILTER_OPTIONS.map((option) => ({
    label: t(option.label),
    value: option.value,
  }))

  return (
    <div className='page-padding flex min-h-full flex-col gap-3 min-[641px]:h-full'>
      <PageHeader
        title={t('sites.page.title')}
        description={t('sites.page.description')}
        actions={
          <>
            <Button onClick={dialogs.openCreate}>
              <PlusIcon className='size-4' />
              {t('sites.toolbar.addSite')}
            </Button>
            {/* The wizard was only reachable from the empty-state CTA,
                    i.e. unreachable once the first site existed — keep a
                    permanent toolbar entry. The wizard is the only flow that
                    creates sites together with their accounts in one batch. */}
            <Button variant='outline' onClick={dialogs.openImport}>
              <UploadIcon className='size-4' />
              {t('sites.toolbar.import')}
            </Button>
          </>
        }
      />
      <div
        role='group'
        aria-label={t('sites.columns.status')}
        className='flex flex-wrap gap-2'
      >
        {['all', 'active', 'disabled'].map((value) => {
          const selected = table.getColumn('status')?.getFilterValue()
          const current = Array.isArray(selected) ? selected[0] : selected
          const pressed = value === 'all' ? !current : current === value
          const count =
            value === 'all'
              ? sites.length
              : sites.filter((site) => (site.status ?? 'active') === value)
                  .length
          return (
            <Button
              key={value}
              size='sm'
              variant={pressed ? 'default' : 'outline'}
              className='rounded-full'
              aria-pressed={pressed}
              disabled={sitesQuery.isLoading || !!sitesQuery.error}
              onClick={() =>
                table
                  .getColumn('status')
                  ?.setFilterValue(value === 'all' ? undefined : [value])
              }
            >
              {value === 'all'
                ? t('sites.page.allSites')
                : statusFilters.find((option) => option.value === value)?.label}
              <span
                className={
                  pressed
                    ? 'bg-primary-foreground/20 rounded-full px-1.5 text-xs tabular-nums'
                    : 'bg-muted text-muted-foreground rounded-full px-1.5 text-xs tabular-nums'
                }
              >
                {sitesQuery.isLoading || sitesQuery.error ? '—' : count}
              </span>
            </Button>
          )
        })}
      </div>

      <DataTablePage
        table={table}
        isLoading={sitesQuery.isLoading}
        isFetching={sitesQuery.isFetching}
        error={sitesQuery.error as Error | null}
        errorMessageKey='sites.page.loadError'
        onErrorRetry={() => sitesQuery.refetch()}
        isErrorRetrying={sitesQuery.isFetching}
        emptyIcon={<Globe className='size-6' />}
        emptyTitle={t('sites.empty.title')}
        emptyDescription={t('sites.empty.description')}
        emptyAction={
          <>
            <Button onClick={dialogs.openImport}>
              <UploadIcon className='size-4' />
              {t('sites.empty.import')}
            </Button>
            <Button variant='outline' onClick={dialogs.openCreate}>
              <PlusIcon className='size-4' />
              {t('sites.empty.addSite')}
            </Button>
          </>
        }
        skeletonKeyPrefix='site-skeleton'
        toolbarProps={{
          searchPlaceholder: t('sites.toolbar.searchPlaceholder'),
          searchDebounceMs: 400,
          filters: [
            {
              columnId: 'status',
              title: t('sites.columns.status'),
              options: statusFilters,
              singleSelect: true,
            },
          ],
        }}
        bulkActions={bulk.toolbar}
      />
      {dialogs.dialogs}
      {bulk.dialog}
    </div>
  )
}
