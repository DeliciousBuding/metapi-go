// metapi-go/features/sites — list page.
//
// Wires the four-layer data-table to the sites query and the URL. Table
// state (search query / page / page-size / sorting / status faceted filter)
// is mirrored to the URL search string so a deep link restores the exact
// view. The hook below is the "feature useSearch" stage of the three-stage
// pattern (route validateSearch -> feature useSearch -> useDataTable); it
// safe-parses `window.location.search` directly so the page works before the
// `/sites` route file lands its own `validateSearch`. Mobile cards are
// handled by `DataTablePage` via the column `meta` flags.

import { useLocation, useNavigate } from '@tanstack/react-router'
import type {
  ColumnFiltersState,
  PaginationState,
  SortingState,
  Updater,
} from '@tanstack/react-table'
import { Plus as PlusIcon, Trash2 as Trash2Icon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

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
import { Spinner } from '@/components/ui/spinner'
import { parseSortingParam } from '@/lib/helpers/searchParams'

import {
  useBatchUpdateSites,
  useDeleteSite,
  useSites,
  useUpdateSite,
} from '../api'
import { sitesSearchSchema } from '../lib/sites-schema'
import type { Site } from '../types'
import { SiteCreatedModal } from './site-created-modal'
import { SiteDetailSheet } from './site-detail-sheet'
import { SiteFormDialog } from './site-form-dialog'
import { SITES_STATUS_FILTER_OPTIONS, useSitesColumns } from './sites-columns'

const SITES_COLUMN_VISIBILITY_STORAGE_KEY = 'metapi-go:sites:column-visibility'
const SITES_COLUMN_SIZING_STORAGE_KEY = 'metapi-go:sites:column-sizing'

type ResolvedSearch = {
  q: string
  pageIndex: number
  pageSize: number
  sorting: SortingState
  status: string | undefined
}

function resolveUpdater<TValue>(
  updater: Updater<TValue>,
  previous: TValue
): TValue {
  return typeof updater === 'function'
    ? (updater as (old: TValue) => TValue)(previous)
    : updater
}

function encodeSorting(sorting: SortingState): string {
  return sorting
    .map((item) => `${item.id}:${item.desc ? 'desc' : 'asc'}`)
    .join(',')
}

function readSearch(searchString?: string): ResolvedSearch {
  if (typeof window === 'undefined') {
    return { q: '', pageIndex: 0, pageSize: 20, sorting: [], status: undefined }
  }
  const params = new URLSearchParams(searchString ?? window.location.search)
  const parsed = sitesSearchSchema.safeParse({
    q: params.get('q') ?? undefined,
    page: params.get('page') ?? undefined,
    pageSize: params.get('pageSize') ?? undefined,
    sort: params.get('sort') ?? undefined,
    status: params.get('status') ?? undefined,
  })
  if (!parsed.success) {
    return { q: '', pageIndex: 0, pageSize: 20, sorting: [], status: undefined }
  }
  const data = parsed.data
  return {
    q: data.q ?? '',
    pageIndex: data.page ?? 0,
    pageSize: data.pageSize ?? 20,
    sorting: parseSortingParam(data.sort),
    status: data.status,
  }
}

function buildHref(next: Partial<ResolvedSearch>): string {
  const current = readSearch()
  const merged: ResolvedSearch = { ...current, ...next }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex))
  if (merged.pageSize !== 20) params.set('pageSize', String(merged.pageSize))
  const sortString = encodeSorting(merged.sorting)
  if (sortString) params.set('sort', sortString)
  if (merged.status) params.set('status', merged.status)
  const queryString = params.toString()
  return queryString ? `/sites?${queryString}` : '/sites'
}

/**
 * The "feature useSearch" stage. Reads the URL on every render (cheap) and
 * hands the data-table controlled state + navigation-backed setters. Because
 * `useNavigate` re-renders the page after each navigation, the next render
 * re-reads the URL — keeping a single source of truth.
 */
function useSitesUrlState() {
  const navigate = useNavigate()
  // Subscribe to the router location: TanStack Router does not re-render a
  // route component on same-path search-only navigation unless a hook
  // consumes the location, and readSearch() reads the URL on render — without
  // this the table would only catch up on the next unrelated re-render.
  const searchStr = useLocation({ select: (loc) => loc.searchStr })
  const search = readSearch(searchStr)

  const columnFilters: ColumnFiltersState = useMemo(() => {
    if (!search.status) return []
    return [{ id: 'status', value: search.status }]
  }, [search.status])

  // URL-sync guard: table state callbacks can fire while the router is
  // navigating away (the useLocation subscription re-renders this page with
  // the *next* location's search string). Without the pathname check the
  // callback would navigate straight back, hijacking the in-flight
  // navigation — the "clicked a sidebar link but the page snapped back"
  // bug. Only sync when we are still on this page.
  function syncUrl(next: Partial<ResolvedSearch>) {
    const href = buildHref(next)
    if (!href.startsWith(window.location.pathname)) return
    navigate({ href, replace: true })
  }

  const onGlobalFilterChange = (updater: Updater<string>) => {
    const next = resolveUpdater(updater, search.q)
    syncUrl({ q: next })
  }
  const onPaginationChange = (updater: Updater<PaginationState>) => {
    const next = resolveUpdater(updater, {
      pageIndex: search.pageIndex,
      pageSize: search.pageSize,
    })
    syncUrl({ pageIndex: next.pageIndex, pageSize: next.pageSize })
  }
  const onSortingChange = (updater: Updater<SortingState>) => {
    const next = resolveUpdater(updater, search.sorting)
    syncUrl({ sorting: next })
  }
  const onColumnFiltersChange = (updater: Updater<ColumnFiltersState>) => {
    const next = resolveUpdater(updater, columnFilters)
    const statusEntry = next.find((filter) => filter.id === 'status')
    const statusValue =
      statusEntry && Array.isArray(statusEntry.value)
        ? (statusEntry.value as string[])[0]
        : (statusEntry?.value as string | undefined)
    syncUrl({ status: statusValue })
  }

  return {
    globalFilter: search.q,
    onGlobalFilterChange,
    pagination: {
      pageIndex: search.pageIndex,
      pageSize: search.pageSize,
    } as PaginationState,
    onPaginationChange,
    sorting: search.sorting,
    onSortingChange,
    columnFilters,
    onColumnFiltersChange,
  }
}

export function SitesPage() {
  const { t } = useTranslation()
  const sitesQuery = useSites()
  const deleteSite = useDeleteSite()
  const updateSite = useUpdateSite()
  const batchUpdateSites = useBatchUpdateSites()

  const [formOpen, setFormOpen] = useState(false)
  const [editingSite, setEditingSite] = useState<Site | null>(null)
  const [viewingSite, setViewingSite] = useState<Site | null>(null)
  const [createdSite, setCreatedSite] = useState<Site | null>(null)
  const [deletingSite, setDeletingSite] = useState<Site | null>(null)

  const urlState = useSitesUrlState()

  const columns = useSitesColumns({
    onEdit: (site) => {
      setEditingSite(site)
      setFormOpen(true)
    },
    onView: (site) => {
      setViewingSite(site)
    },
    onToggleStatus: (site) => {
      const nextStatus = site.status === 'disabled' ? 'active' : 'disabled'
      updateSite.mutate(
        { id: site.id, payload: { status: nextStatus } },
        {
          onSuccess: () =>
            toast.success(t('sites.toast.statusToggled', { name: site.name })),
          onError: () => toast.error(t('sites.toast.statusToggleFailed')),
        }
      )
    },
    onTogglePin: (site) => {
      updateSite.mutate(
        { id: site.id, payload: { isPinned: !site.isPinned } },
        {
          onSuccess: () =>
            toast.success(t('sites.toast.pinToggled', { name: site.name })),
          onError: () => toast.error(t('sites.toast.pinToggleFailed')),
        }
      )
    },
    onDelete: (site) => {
      setDeletingSite(site)
    },
  })

  const { table } = useDataTable<Site>({
    data: sitesQuery.data ?? [],
    columns,
    enableRowSelection: true,
    enableColumnResizing: true,
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
    getRowId: (row) => String(row.id),
  })

  function handleAddSite() {
    setEditingSite(null)
    setFormOpen(true)
  }

  function handleCreated(site: Site) {
    setCreatedSite(site)
  }

  async function confirmDelete() {
    if (!deletingSite) return
    const site = deletingSite
    try {
      await deleteSite.mutateAsync(site.id)
      toast.success(t('sites.toast.deleted', { name: site.name }))
    } catch {
      toast.error(t('sites.toast.deleteFailed'))
    } finally {
      setDeletingSite(null)
    }
  }

  async function handleBulkAction(action: 'enable' | 'disable' | 'delete') {
    const selectedRows = table.getFilteredSelectedRowModel().rows
    const ids = selectedRows.map((row) => row.original.id)
    if (ids.length === 0) return
    try {
      const result = await batchUpdateSites.mutateAsync({ ids, action })
      const successCount = result.successIds?.length ?? 0
      const failedCount = ids.length - successCount
      if (failedCount <= 0) {
        toast.success(t('sites.toast.bulkSucceeded', { count: successCount }))
      } else {
        toast.warning(
          t('sites.toast.bulkPartial', {
            success: successCount,
            failed: failedCount,
          })
        )
      }
      table.resetRowSelection()
    } catch {
      toast.error(t('sites.toast.bulkFailed'))
    }
  }

  const statusFilters = SITES_STATUS_FILTER_OPTIONS.map((option) => ({
    label: t(option.label),
    value: option.value,
  }))

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={sitesQuery.isLoading}
        isFetching={sitesQuery.isFetching}
        emptyTitle={t('sites.empty.title')}
        emptyDescription={t('sites.empty.description')}
        emptyAction={
          <Button onClick={handleAddSite}>
            <PlusIcon className='mr-1 size-4' />
            {t('sites.empty.addSite')}
          </Button>
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
          preActions: (
            <Button onClick={handleAddSite}>
              <PlusIcon className='mr-1 size-4' />
              {t('sites.toolbar.addSite')}
            </Button>
          ),
        }}
        bulkActions={
          <DataTableBulkActions
            table={table}
            entityName={t('sites.entityName')}
          >
            <Button
              variant='outline'
              size='sm'
              onClick={() => handleBulkAction('enable')}
              disabled={batchUpdateSites.isPending}
            >
              {t('sites.bulk.enable')}
            </Button>
            <Button
              variant='outline'
              size='sm'
              onClick={() => handleBulkAction('disable')}
              disabled={batchUpdateSites.isPending}
            >
              {t('sites.bulk.disable')}
            </Button>
            <Button
              variant='destructive'
              size='sm'
              onClick={() => handleBulkAction('delete')}
              disabled={batchUpdateSites.isPending}
            >
              <Trash2Icon className='mr-1 size-3.5' />
              {t('sites.bulk.delete')}
            </Button>
          </DataTableBulkActions>
        }
      />

      <SiteFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        editingSite={editingSite}
        onCreated={handleCreated}
      />

      <SiteCreatedModal
        site={createdSite}
        open={createdSite !== null}
        onOpenChange={(open) => {
          if (!open) setCreatedSite(null)
        }}
      />

      <SiteDetailSheet
        site={viewingSite}
        open={viewingSite !== null}
        onOpenChange={(open) => {
          if (!open) setViewingSite(null)
        }}
        onEdit={(site) => {
          setViewingSite(null)
          setEditingSite(site)
          setFormOpen(true)
        }}
      />

      <Dialog
        open={deletingSite !== null}
        onOpenChange={(open) => {
          if (!open) setDeletingSite(null)
        }}
      >
        <DialogContent className='sm:max-w-sm'>
          <DialogHeader>
            <DialogTitle>{t('sites.deleteConfirm.title')}</DialogTitle>
            <DialogDescription>
              {deletingSite
                ? t('sites.deleteConfirm.body', { name: deletingSite.name })
                : t('sites.deleteConfirm.bodyFallback')}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setDeletingSite(null)}
              disabled={deleteSite.isPending}
            >
              {t('sites.deleteConfirm.cancel')}
            </Button>
            <Button
              variant='destructive'
              onClick={confirmDelete}
              disabled={deleteSite.isPending}
            >
              {deleteSite.isPending && <Spinner className='mr-2' />}
              {t('sites.deleteConfirm.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
