// metapi-go/features/channels — read-only list page.
// Wires the shared data-table to `useChannels` and mirrors search/page/sort
// state to the URL. No mutation surfaces: this page is intentionally read-only
// (soft isolation only — never hard-disable a channel). A detail sheet (opened
// from the row eye action) surfaces the routing-health fields the columns
// already render, mirroring the model / route / account detail pattern.

import { useNavigate, useSearch } from '@tanstack/react-router'
import type { ColumnFiltersState } from '@tanstack/react-table'
import { Users } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import {
  DataTablePage,
  encodeSorting,
  useDataTable,
  useUrlTableState,
  type UrlTableState,
  type UrlTableStateUpdate,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { asStringParam, parseSortingParam } from '@/lib/helpers/searchParams'

import { useChannels } from '../api'
import { channelsSearchSchema } from '../lib/channels-schema'
import type { ChannelRow } from '../types'
import { ChannelDetailSheet } from './channel-detail-sheet'
import {
  useChannelsColumns,
  type ChannelsColumnActions,
} from './channels-columns'

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

function buildHref(next: UrlTableStateUpdate<ChannelsUrlFilters>): string {
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
  const search = useSearch({ from: '/_authenticated/channels' })
  const navigate = useNavigate()
  const channelsQuery = useChannels()
  const urlState = useChannelsUrlState()
  const [detailChannel, setDetailChannel] = useState<ChannelRow | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)

  // One-shot channel drilldown (proxy-log detail -> `?channelId=N`): wait
  // for the list, open the detail sheet for the referenced channel, then
  // strip the param so a refetch or remount never reopens the sheet. A
  // stale or unknown id is stripped without opening anything.
  const drilldownConsumed = useRef(false)
  useEffect(() => {
    if (drilldownConsumed.current || !search.channelId) return
    if (channelsQuery.isLoading) return

    const target = (channelsQuery.data ?? []).find(
      (channel) => channel.id === search.channelId
    )
    drilldownConsumed.current = true
    if (target) {
      setDetailChannel(target)
      setDetailOpen(true)
    }
    navigate({
      to: '/channels',
      search: { ...search, channelId: undefined },
      replace: true,
    })
  }, [search, channelsQuery.isLoading, channelsQuery.data, navigate])

  const columnActions: ChannelsColumnActions = {
    onView: (channel) => {
      setDetailChannel(channel)
      setDetailOpen(true)
    },
  }
  const columns = useChannelsColumns(columnActions)

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
    ensurePageInRange: urlState.ensurePageInRange,
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

      <QueryErrorBanner
        error={channelsQuery.error as Error | null}
        messageKey='channels.page.loadError'
        onRetry={() => channelsQuery.refetch()}
        isRetrying={channelsQuery.isFetching}
      />

      <DataTablePage
        table={table}
        columns={columns}
        isLoading={channelsQuery.isLoading}
        isFetching={channelsQuery.isFetching}
        emptyTitle={t('channels.empty.title')}
        emptyDescription={t('channels.empty.description')}
        emptyAction={
          <Button
            variant='outline'
            onClick={() => void navigate({ to: '/accounts' })}
          >
            <Users className='size-4' />
            {t('channels.empty.manageAccounts')}
          </Button>
        }
        skeletonKeyPrefix='channel-skeleton'
        toolbarProps={{
          searchPlaceholder: t('channels.toolbar.searchPlaceholder'),
          searchDebounceMs: 400,
        }}
      />

      <ChannelDetailSheet
        channel={detailChannel}
        open={detailOpen}
        onOpenChange={setDetailOpen}
        onEdit={(channel) => {
          setDetailOpen(false)
          // One-shot `edit` deep link: the routes page opens the edit
          // dialog for this route (a `routeId` link would only open the
          // read-only detail sheet).
          void navigate({
            to: '/token-routes',
            search: { edit: channel.routeId },
          })
        }}
      />
    </div>
  )
}
