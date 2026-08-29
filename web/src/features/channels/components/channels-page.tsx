// metapi-go/features/channels — read-only list page.
// Wires the shared data-table to `useChannels` and mirrors search/page/sort
// state to the URL. No mutation surfaces: this page is intentionally read-only
// (soft isolation only — never hard-disable a channel). A detail sheet (opened
// from the row eye action) surfaces the routing-health fields the columns
// already render, mirroring the model / route / account detail pattern.

import { useNavigate, useSearch } from '@tanstack/react-router'
import type { ColumnFiltersState } from '@tanstack/react-table'
import { Users } from 'lucide-react'
import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { useProbeHistory } from '@/components/common/use-probe-history'
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

import { useChannels, useChannelsErrorSummary, useChannelsPage } from '../api'
import { channelsSearchSchema } from '../lib/channels-schema'
import {
  CHANNELS_ERROR_STATUS_FILTER,
  isErrorOnlyStatusFilter,
} from '../lib/error-statuses'
import type { ChannelRow } from '../types'
import { ChannelDetailSheet } from './channel-detail-sheet'
import {
  CHANNELS_STATUS_FILTER_OPTIONS,
  useChannelsColumns,
  type ChannelsColumnActions,
} from './channels-columns'
import { ChannelsErrorBanner } from './channels-error-banner'
import { CooldownReasonDialog } from './cooldown-reason-dialog'

const CHANNELS_COLUMN_VISIBILITY_STORAGE_KEY =
  'metapi-go:channels:column-visibility'
const CHANNELS_COLUMN_SIZING_STORAGE_KEY = 'metapi-go:channels:column-sizing'

/** Page-specific URL filters: comma-separated status list for the facet. */
type ChannelsUrlFilters = {
  status: string
}

const EMPTY_URL_STATE: UrlTableState<ChannelsUrlFilters> = {
  q: '',
  pageIndex: 0,
  pageSize: 20,
  sorting: [],
  filters: { status: '' },
}

function readSearch(searchString?: string): UrlTableState<ChannelsUrlFilters> {
  if (typeof window === 'undefined') {
    return EMPTY_URL_STATE
  }
  const params = new URLSearchParams(searchString ?? window.location.search)
  const parsed = channelsSearchSchema.safeParse({
    q: params.get('q') ?? undefined,
    page: params.get('page') ?? undefined,
    pageSize: params.get('pageSize') ?? undefined,
    sort: params.get('sort') ?? undefined,
    status: params.get('status') ?? undefined,
  })
  if (!parsed.success) {
    return EMPTY_URL_STATE
  }
  const data = parsed.data
  return {
    q: asStringParam(data.q) ?? '',
    pageIndex: data.page ?? 0,
    pageSize: data.pageSize ?? 20,
    sorting: parseSortingParam(data.sort),
    filters: { status: asStringParam(data.status) ?? '' },
  }
}

function buildHref(next: UrlTableStateUpdate<ChannelsUrlFilters>): string {
  const current = readSearch()
  const merged: UrlTableState<ChannelsUrlFilters> = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex))
  if (merged.pageSize !== 20) params.set('pageSize', String(merged.pageSize))
  const sortString = encodeSorting(merged.sorting)
  if (sortString) params.set('sort', sortString)
  if (merged.filters.status) params.set('status', merged.filters.status)
  const queryString = params.toString()
  return queryString ? `/channels?${queryString}` : '/channels'
}

function useChannelsUrlState() {
  return useUrlTableState<ChannelsUrlFilters>({
    basePath: '/channels',
    read: readSearch,
    buildHref,
    toColumnFilters: (filters) => {
      const statusValues = filters.status.split(',').filter(Boolean)
      if (statusValues.length === 0) return [] as ColumnFiltersState
      return [{ id: 'status', value: statusValues }]
    },
    fromColumnFilters: (columnFilters) => {
      const statusEntry = columnFilters.find((filter) => filter.id === 'status')
      return {
        filters: {
          status: Array.isArray(statusEntry?.value)
            ? statusEntry.value.join(',')
            : '',
        },
      }
    },
  })
}

export function ChannelsPage() {
  const { t } = useTranslation()
  const search = useSearch({ from: '/_authenticated/channels' })
  const navigate = useNavigate()
  const urlState = useChannelsUrlState()
  const channelsPageQuery = useChannelsPage({
    pageIndex: urlState.pagination.pageIndex,
    pageSize: urlState.pagination.pageSize,
    status: urlState.filters.status || undefined,
  })
  // Legacy full-list query is enabled only for one-shot `?channelId=`
  // drilldown; the normal page never transfers the whole fleet.
  const channelsQuery = useChannels({
    enabled: Boolean(search.channelId),
  })
  const errorSummaryQuery = useChannelsErrorSummary()
  // Row-level probe history is secondary decoration: fetched in ONE batch
  // (never per row) and rendered as health bars; a failed fetch only hides
  // the bars, never the channels table.
  const probeHistoryQuery = useProbeHistory('channels')

  // P1-4 closure (competitor-study-2026-08): the error banner doubles as
  // the filter entry. The count derives from the loaded list; the mode
  // derives from the URL status facet — persistent, shareable state per
  // state-stability.md R1, never a one-shot param the page strips.
  const [detailChannel, setDetailChannel] = useState<ChannelRow | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [reasonChannel, setReasonChannel] = useState<ChannelRow | null>(null)
  const [reasonOpen, setReasonOpen] = useState(false)

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

  const errorChannelCount = errorSummaryQuery.data?.errorCount ?? 0
  const showErrorOnly = isErrorOnlyStatusFilter(
    asStringParam(search.status) ?? ''
  )

  const handleFilterErrors = useCallback(() => {
    void navigate({
      to: '/channels',
      search: { ...search, status: CHANNELS_ERROR_STATUS_FILTER, page: 0 },
    })
  }, [navigate, search])

  const handleExitErrorOnly = useCallback(() => {
    void navigate({
      to: '/channels',
      search: { ...search, status: undefined, page: 0 },
    })
  }, [navigate, search])

  const columnActions: ChannelsColumnActions = {
    onView: (channel) => {
      setDetailChannel(channel)
      setDetailOpen(true)
    },
    onShowReason: (channel) => {
      setReasonChannel(channel)
      setReasonOpen(true)
    },
  }
  const columns = useChannelsColumns(columnActions, probeHistoryQuery.data)

  const { table } = useDataTable<ChannelRow>({
    data: channelsPageQuery.data?.items ?? [],
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
    manualPagination: true,
    manualSorting: true,
    totalCount: channelsPageQuery.data?.total ?? 0,
  })

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div>
        <h1 className='text-lg font-normal'>{t('channels.page.title')}</h1>
        <p className='text-muted-foreground text-sm'>
          {t('channels.page.description')}
        </p>
      </div>

      {channelsPageQuery.error ? (
        <QueryErrorBanner
          error={channelsPageQuery.error as Error | null}
          messageKey='channels.page.loadError'
          onRetry={() => {
            void channelsPageQuery.refetch()
            void errorSummaryQuery.refetch()
          }}
          isRetrying={channelsPageQuery.isFetching}
        />
      ) : (
        <>
          {errorSummaryQuery.error && !errorSummaryQuery.data ? (
            <QueryErrorBanner
              error={errorSummaryQuery.error as Error | null}
              messageKey='channels.page.loadError'
              onRetry={() => errorSummaryQuery.refetch()}
              isRetrying={errorSummaryQuery.isFetching}
            />
          ) : (
            <ChannelsErrorBanner
              errorCount={errorChannelCount}
              showErrorOnly={showErrorOnly}
              onFilterErrors={handleFilterErrors}
              onExitErrorOnly={handleExitErrorOnly}
            />
          )}
          <DataTablePage
            table={table}
            columns={columns}
            isLoading={channelsPageQuery.isLoading}
            isFetching={channelsPageQuery.isFetching}
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
              filters: [
                {
                  columnId: 'status',
                  title: t('channels.columns.status'),
                  options: CHANNELS_STATUS_FILTER_OPTIONS.map((option) => ({
                    label: t(option.labelKey),
                    value: option.value,
                  })),
                },
              ],
            }}
          />
        </>
      )}

      <CooldownReasonDialog
        channel={reasonChannel}
        open={reasonOpen}
        onOpenChange={setReasonOpen}
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
