// metapi-go/features/site-announcements — list page.
//
// Wires the four-layer data-table to the announcements query and the URL.
// Table state (search query / page / page-size / sorting / severity faceted
// filter / enabled filter) is mirrored to the URL search string so a deep
// link restores the exact view. The add/edit dialog handles both create and
// edit; a separate confirm dialog guards deletion. Mobile cards are handled
// by `DataTablePage` via the column `meta` flags.

import { useLocation, useNavigate } from '@tanstack/react-router'
import type {
  ColumnFiltersState,
  PaginationState,
  SortingState,
  Updater,
} from '@tanstack/react-table'
import { Plus as PlusIcon } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
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

import { parseSortingParam } from '@/lib/helpers/searchParams'

import {
  useAnnouncements,
  useDeleteAnnouncement,
} from '../api'
import { announcementsSearchSchema } from '../lib/announcements-schema'
import type { SiteAnnouncement } from '../types'
import { AnnouncementFormDialog } from './announcement-form-dialog'
import {
  ANNOUNCEMENTS_ENABLED_FILTER_OPTIONS,
  ANNOUNCEMENTS_SEVERITY_FILTER_OPTIONS,
  useAnnouncementsColumns,
} from './announcements-columns'

const ANNOUNCEMENTS_COLUMN_VISIBILITY_STORAGE_KEY =
  'metapi-go:site-announcements:column-visibility'
const ANNOUNCEMENTS_COLUMN_SIZING_STORAGE_KEY =
  'metapi-go:site-announcements:column-sizing'

type ResolvedSearch = {
  q: string
  pageIndex: number
  pageSize: number
  sorting: SortingState
  severity: string | undefined
  enabled: string | undefined
}

function resolveUpdater<TValue>(updater: Updater<TValue>, previous: TValue): TValue {
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
    return {
      q: '',
      pageIndex: 0,
      pageSize: 20,
      sorting: [],
      severity: undefined,
      enabled: undefined,
    }
  }
  const params = new URLSearchParams(searchString ?? window.location.search)
  const parsed = announcementsSearchSchema.safeParse({
    q: params.get('q') ?? undefined,
    page: params.get('page') ?? undefined,
    pageSize: params.get('pageSize') ?? undefined,
    sort: params.get('sort') ?? undefined,
    severity: params.get('severity') ?? undefined,
    enabled: params.get('enabled') ?? undefined,
  })
  if (!parsed.success) {
    return {
      q: '',
      pageIndex: 0,
      pageSize: 20,
      sorting: [],
      severity: undefined,
      enabled: undefined,
    }
  }
  const data = parsed.data
  return {
    q: data.q ?? '',
    pageIndex: data.page ?? 0,
    pageSize: data.pageSize ?? 20,
    sorting: parseSortingParam(data.sort),
    severity: data.severity,
    enabled: data.enabled,
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
  if (merged.severity) params.set('severity', merged.severity)
  if (merged.enabled) params.set('enabled', merged.enabled)
  const queryString = params.toString()
  return queryString ? `/site-announcements?${queryString}` : '/site-announcements'
}

function useAnnouncementsUrlState() {
  const navigate = useNavigate()
  // Subscribe to the router location: TanStack Router does not re-render a
  // route component on same-path search-only navigation unless a hook
  // consumes the location, and readSearch() reads the URL on render.
  const searchStr = useLocation({ select: (loc) => loc.searchStr })
  const search = readSearch(searchStr)

  const columnFilters: ColumnFiltersState = useMemo(() => {
    const filters: ColumnFiltersState = []
    if (search.severity) filters.push({ id: 'severity', value: search.severity })
    if (search.enabled) filters.push({ id: 'enabled', value: search.enabled })
    return filters
  }, [search.severity, search.enabled])

  // URL-sync guard: table state callbacks can fire while the router is
  // navigating away (the useLocation subscription re-renders this page with
  // the *next* location's search string). Without the pathname check the
  // callback would navigate straight back, hijacking the in-flight
  // navigation. Only sync when we are still on this page.
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
    const severityEntry = next.find((filter) => filter.id === 'severity')
    const severityValue = Array.isArray(severityEntry?.value)
      ? (severityEntry?.value as string[] | undefined)?.[0]
      : (severityEntry?.value as string | undefined)
    const enabledEntry = next.find((filter) => filter.id === 'enabled')
    const enabledValue = Array.isArray(enabledEntry?.value)
      ? (enabledEntry?.value as string[] | undefined)?.[0]
      : (enabledEntry?.value as string | undefined)
    syncUrl({ severity: severityValue, enabled: enabledValue })
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

export function AnnouncementsPage() {
  const { t } = useTranslation()
  const announcementsQuery = useAnnouncements()
  const deleteAnnouncement = useDeleteAnnouncement()

  const [formOpen, setFormOpen] = useState(false)
  const [editingAnnouncement, setEditingAnnouncement] =
    useState<SiteAnnouncement | null>(null)
  const [deletingAnnouncement, setDeletingAnnouncement] =
    useState<SiteAnnouncement | null>(null)

  const urlState = useAnnouncementsUrlState()

  const columns = useAnnouncementsColumns({
    onEdit: (item) => {
      setEditingAnnouncement(item)
      setFormOpen(true)
    },
    onDelete: (item) => {
      setDeletingAnnouncement(item)
    },
  })

  const { table } = useDataTable<SiteAnnouncement>({
    data: announcementsQuery.data ?? [],
    columns,
    enableRowSelection: true,
    enableColumnResizing: true,
    columnVisibilityStorageKey: ANNOUNCEMENTS_COLUMN_VISIBILITY_STORAGE_KEY,
    columnSizingStorageKey: ANNOUNCEMENTS_COLUMN_SIZING_STORAGE_KEY,
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

  function handleAdd() {
    setEditingAnnouncement(null)
    setFormOpen(true)
  }

  async function confirmDelete() {
    if (!deletingAnnouncement) return
    const item = deletingAnnouncement
    try {
      await deleteAnnouncement.mutateAsync(item.id)
      toast.success(t('siteAnnouncements.toast.deleted', { title: item.title }))
    } catch {
      toast.error(t('siteAnnouncements.toast.deleteFailed'))
    } finally {
      setDeletingAnnouncement(null)
    }
  }

  const severityFilters = ANNOUNCEMENTS_SEVERITY_FILTER_OPTIONS.map((option) => ({
    label: t(option.label),
    value: option.value,
  }))

  const enabledFilters = ANNOUNCEMENTS_ENABLED_FILTER_OPTIONS.map((option) => ({
    label: t(option.label),
    value: option.value,
  }))

  return (
    <>
      <DataTablePage
        table={table}
        columns={columns}
        isLoading={announcementsQuery.isLoading}
        isFetching={announcementsQuery.isFetching}
        emptyTitle={t('siteAnnouncements.empty.title')}
        emptyDescription={t('siteAnnouncements.empty.description')}
        emptyAction={
          <Button onClick={handleAdd}>
            <PlusIcon className='mr-1 size-4' />
            {t('siteAnnouncements.empty.add')}
          </Button>
        }
        skeletonKeyPrefix='announcement-skeleton'
        toolbarProps={{
          searchPlaceholder: t('siteAnnouncements.toolbar.searchPlaceholder'),
          searchDebounceMs: 400,
          filters: [
            {
              columnId: 'severity',
              title: t('siteAnnouncements.columns.severity'),
              options: severityFilters,
              singleSelect: true,
            },
            {
              columnId: 'enabled',
              title: t('siteAnnouncements.columns.enabled'),
              options: enabledFilters,
              singleSelect: true,
            },
          ],
          preActions: (
            <Button onClick={handleAdd}>
              <PlusIcon className='mr-1 size-4' />
              {t('siteAnnouncements.toolbar.add')}
            </Button>
          ),
        }}
      />

      <AnnouncementFormDialog
        open={formOpen}
        onOpenChange={setFormOpen}
        editingAnnouncement={editingAnnouncement}
      />

      <Dialog
        open={deletingAnnouncement !== null}
        onOpenChange={(open) => {
          if (!open) setDeletingAnnouncement(null)
        }}
      >
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>{t('siteAnnouncements.delete.title')}</DialogTitle>
            <DialogDescription>
              {t('siteAnnouncements.delete.description', {
                title: deletingAnnouncement?.title ?? '',
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setDeletingAnnouncement(null)}
              disabled={deleteAnnouncement.isPending}
            >
              {t('siteAnnouncements.delete.cancel')}
            </Button>
            <Button
              variant='destructive'
              onClick={confirmDelete}
              disabled={deleteAnnouncement.isPending}
            >
              {t('siteAnnouncements.delete.confirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
