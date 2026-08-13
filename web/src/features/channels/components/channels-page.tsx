// metapi-go/features/channels — read-only list page.
// Wires the shared data-table to `useChannels` and mirrors search/page/sort
// state to the URL. No mutation surfaces: this page is intentionally read-only
// (soft isolation only — never hard-disable a channel).

import type { ColumnFiltersState } from '@tanstack/react-table'
import { useTranslation } from 'react-i18next'

import {
  DataTablePage,
  encodeSorting,
  useDataTable,
  useUrlTableState,
  type UrlTableState,
} from '@/components/data-table'
import { asStringParam, parseSortingParam } from '@/lib/helpers/searchParams'

import { useChannels } from '../api'
import { channelsSearchSchema } from '../lib/channels-schema'
import type { ChannelRow } from '../types'
import { useChannelsColumns } from './channels-columns'

const CHANNELS_COLUMN_VISIBILITY_STORAGE_KEY =
  'metapi-go:channels:column-visibility'
const CHANNELS_COLUMN_SIZING_STORAGE_KEY = 'metapi-go:channels:column-sizing'

type ChannelsUrlFilters = Record<string, never>

function readSearch(searchString?: string): UrlTableState<ChannelsUrlFilters> {
  if (typeof window === 'undefined') {
    return { q: '', pageIndex: 0, pageSize: 20, sorting: [], filters: {} }
  }
  const params = new URLSearchParams(searchString ?? window.location.search)
  const parsed = channelsSearchSchema.safeParse({
    q: params.get('q') ?? undefined,
    page: params.get('page') ?? undefined,
    pageSize: params.get('pageSize') ?? undefined,
    sort: params.get('sort') ?? undefined,
  })
  if (!parsed.success) {
    return { q: '', pageIndex: 0, pageSize: 20, sorting: [], filters: {} }
  }
  const data = parsed.data
  return {
    q: asStringParam(data.q) ?? '',
    pageIndex: data.page ?? 0,
    pageSize: data.pageSize ?? 20,
    sorting: parseSortingParam(data.sort),
    filters: {},
  }
}

function buildHref(next: Partial<UrlTableState<ChannelsUrlFilters>>): string {
  const current = readSearch()
  const merged = { ...current, ...next }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex))
  if (merged.pageSize !== 20) params.set('pageSize', String(merged.pageSize))
  const sortString = encodeSorting(merged.sorting)
  if (sortString) params.set('sort', sortString)
  const queryString = params.toString()
  return queryString ? `/channels?${queryString}` : '/channels'
}

function useChannelsUrlState() {
  return useUrlTableState<ChannelsUrlFilters>({
    basePath: '/channels',
    read: readSearch,
    buildHref,
    toColumnFilters: () => [] as ColumnFiltersState,
    fromColumnFilters: () => ({}),
  })
}

export function ChannelsPage() {
  const { t } = useTranslation()
  const channelsQuery = useChannels()
  const urlState = useChannelsUrlState()
  const columns = useChannelsColumns()

  const { table } = useDataTable<ChannelRow>({
    data: channelsQuery.data ?? [],
    columns,
    enableColumnResizing: true,
    columnVisibilityStorageKey: CHANNELS_COLUMN_VISIBILITY_STORAGE_KEY,
    columnSizingStorageKey: CHANNELS_COLUMN_SIZING_STORAGE_KEY,
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

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div>
        <h1 className='text-lg font-normal'>{t('channels.page.title')}</h1>
        <p className='text-muted-foreground text-sm'>
          {t('channels.page.description')}
        </p>
      </div>

      {channelsQuery.error && (
        <div className='border-destructive/40 bg-destructive/10 text-destructive-soft-fg rounded-lg border p-3 text-sm'>
          {t('channels.page.loadError', {
            message: (channelsQuery.error as Error).message,
          })}
        </div>
      )}

      <DataTablePage
        table={table}
        columns={columns}
        isLoading={channelsQuery.isLoading}
        isFetching={channelsQuery.isFetching}
        emptyTitle={t('channels.empty.title')}
        emptyDescription={t('channels.empty.description')}
        skeletonKeyPrefix='channel-skeleton'
        toolbarProps={{
          searchPlaceholder: t('channels.toolbar.searchPlaceholder'),
          searchDebounceMs: 400,
        }}
      />
    </div>
  )
}
