/* eslint-disable no-nested-ternary -- status-select display uses chained ternaries */
// metapi-go/features/proxy-logs/components — proxy logs list page.
// i18n: all user-visible strings migrated to t() calls.

import type { Row } from '@tanstack/react-table'
import { useNavigate } from '@tanstack/react-router'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import {
  DataTablePage,
  type UrlTableState,
  type UrlTableStateUpdate,
  useDataTable,
  useUrlTableState,
} from '@/components/data-table'
import { DataTableRow } from '@/components/data-table/core/data-table-row'
import type { DataTableRenderRowHelpers } from '@/components/data-table/core/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { api } from '@/lib/api'
import { formatCurrency, formatInt } from '@/lib/format'
import { asStringParam } from '@/lib/helpers/searchParams'
import { toast } from '@/lib/toast'

import { useProxyLogs, useProxyLogsMeta } from '../api'
import { proxyLogsToCsv } from '../lib/proxy-logs-csv'
import {
  PROXY_LOG_STATUS_FILTER_OPTIONS,
  proxyLogsSearchSchema,
  type ProxyLogsSearch,
} from '../lib/proxy-logs-schema'
import { useProxyLogsAutoRefresh } from '../lib/use-proxy-logs-auto-refresh'
import type { ProxyLog, ProxyLogFilters } from '../types'
import { LatencyBadge } from './latency-badge'
import { ProxyLogDetailSheet } from './proxy-log-detail-sheet'
import { ProxyLogsAutoRefreshToggle } from './proxy-logs-auto-refresh-toggle'
import {
  useProxyLogsColumns,
  type ProxyLogsColumnActions,
} from './proxy-logs-columns'
import { ProxyLogsHeaderActions } from './proxy-logs-header-actions'

const PROXY_LOGS_COLUMN_VISIBILITY_STORAGE_KEY =
  'metapi-go:proxy-logs:column-visibility'
const PROXY_LOGS_COLUMN_SIZING_STORAGE_KEY =
  'metapi-go:proxy-logs:column-sizing'
const DEFAULT_PAGE_SIZE = 20
const PROXY_LOGS_CSV_EXPORT_LIMIT = 10_000

/** Page-specific URL filters (all strings for URL round-trip simplicity). */
type ProxyLogsUrlFilters = {
  status: string
  siteId: string
  channelId: string
  client: string
  from: string
  to: string
  latencyMin: string
  latencyMax: string
}

/**
 * Parse the raw search string into URL table state, reusing the route's
 * `proxyLogsSearchSchema` so the derived query payload matches the prefetched
 * cache key (no double fetch). Returns schema defaults on a malformed string.
 */
function readProxyLogsSearch(
  searchString: string
): UrlTableState<ProxyLogsUrlFilters> {
  const entries = Object.fromEntries(
    new URLSearchParams(
      searchString.startsWith('?') ? searchString.slice(1) : searchString
    ).entries()
  )
  const parsed = proxyLogsSearchSchema.safeParse(entries)
  const search: ProxyLogsSearch | null = parsed.success ? parsed.data : null
  return {
    q: asStringParam(search?.q) ?? '',
    // The proxy-logs URL is 0-based (`page`), unlike the other list pages.
    pageIndex: search?.page ?? 0,
    pageSize: search?.pageSize ?? DEFAULT_PAGE_SIZE,
    sorting: [],
    filters: {
      status: search?.status ?? 'all',
      siteId: search?.siteId === undefined ? '' : String(search.siteId),
      channelId:
        search?.channelId === undefined ? '' : String(search.channelId),
      client: asStringParam(search?.client) ?? '',
      from: asStringParam(search?.from) ?? '',
      to: asStringParam(search?.to) ?? '',
      latencyMin:
        search?.latencyMin === undefined ? '' : String(search.latencyMin),
      latencyMax:
        search?.latencyMax === undefined ? '' : String(search.latencyMax),
    },
  }
}

/** Serialize a partial state update back to the proxy-logs href, merging over
 *  the CURRENT url state so a single filter change preserves all others. */
function buildProxyLogsHref(
  next: UrlTableStateUpdate<ProxyLogsUrlFilters>
): string {
  const current = readProxyLogsSearch(window.location.search)
  const merged: UrlTableState<ProxyLogsUrlFilters> = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex))
  if (merged.pageSize !== DEFAULT_PAGE_SIZE) {
    params.set('pageSize', String(merged.pageSize))
  }
  if (merged.filters.status !== 'all') {
    params.set('status', merged.filters.status)
  }
  if (merged.filters.siteId) params.set('siteId', merged.filters.siteId)
  // Only a positive channel id is a real filter — the backend reads 0 as
  // "unset", so writing `channelId=0` would leave the URL disagreeing with
  // the rendered (unfiltered) list.
  if (Number(merged.filters.channelId) > 0) {
    params.set('channelId', merged.filters.channelId)
  }
  if (merged.filters.client) params.set('client', merged.filters.client)
  if (merged.filters.from) params.set('from', merged.filters.from)
  if (merged.filters.to) params.set('to', merged.filters.to)
  if (merged.filters.latencyMin) {
    params.set('latencyMin', merged.filters.latencyMin)
  }
  if (merged.filters.latencyMax) {
    params.set('latencyMax', merged.filters.latencyMax)
  }
  const queryString = params.toString()
  return queryString ? `/proxy-logs?${queryString}` : '/proxy-logs'
}

function useProxyLogsUrlState() {
  return useUrlTableState<ProxyLogsUrlFilters>({
    basePath: '/proxy-logs',
    read: readProxyLogsSearch,
    buildHref: buildProxyLogsHref,
    // No TanStack column filters on this page: status / site / client /
    // date / latency are all page-specific controls driven by `filters`.
    toColumnFilters: () => [],
    fromColumnFilters: () => ({}),
    resetPageIndexOnFilterChange: true,
  })
}

export function ProxyLogsPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const {
    globalFilter,
    pagination,
    onGlobalFilterChange,
    onPaginationChange,
    filters,
    updateUrlState,
  } = useProxyLogsUrlState()

  // Derive the active server-side filter values directly from the URL-owned
  // filters (single source of truth — no local mirror to sync back).
  const status = filters.status as ProxyLogFilters['status']
  const siteId = filters.siteId ? Number(filters.siteId) : null
  const channelId = filters.channelId ? Number(filters.channelId) : null
  const client = filters.client
  const from = filters.from
  const to = filters.to
  const latencyMin = filters.latencyMin ? Number(filters.latencyMin) : null
  const latencyMax = filters.latencyMax ? Number(filters.latencyMax) : null

  const [detailLog, setDetailLog] = useState<ProxyLog | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [isExporting, setIsExporting] = useState(false)

  const queryPayload = useMemo(
    () => ({
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
      status: status === 'all' ? undefined : status,
      search: globalFilter.trim() || undefined,
      siteId: siteId ?? undefined,
      channelId: channelId ?? undefined,
      client: client || undefined,
      from: from || undefined,
      to: to || undefined,
      // Latency bounds are sent to the server so total + summary agree with
      // the returned items (manualPagination relies on the server total).
      latencyMin: latencyMin ?? undefined,
      latencyMax: latencyMax ?? undefined,
    }),
    [
      pagination,
      status,
      globalFilter,
      siteId,
      channelId,
      client,
      from,
      to,
      latencyMin,
      latencyMax,
    ]
  )

  const metaPayload = useMemo(
    () => ({
      status: queryPayload.status,
      search: queryPayload.search,
      siteId: queryPayload.siteId,
      channelId: queryPayload.channelId,
      client: queryPayload.client,
      from: queryPayload.from,
      to: queryPayload.to,
      latencyMin: queryPayload.latencyMin,
      latencyMax: queryPayload.latencyMax,
    }),
    [queryPayload]
  )
  const { intervalMs, setIntervalMs } = useProxyLogsAutoRefresh()
  const logsQuery = useProxyLogs(queryPayload, {
    refetchInterval: intervalMs === false ? false : intervalMs,
  })
  // The meta query is the single owner of the summary aggregate (the list
  // fetch is view=query and carries no summary) — refetch it on the same
  // interval so auto-refresh keeps the summary strip as fresh as the rows.
  const metaQuery = useProxyLogsMeta(metaPayload, {
    refetchInterval: intervalMs === false ? false : intervalMs,
  })
  const rawItems = useMemo(() => logsQuery.data?.items ?? [], [logsQuery.data])
  const total = logsQuery.data?.total ?? 0

  // The server now applies the latency filter, so the page items are used
  // directly — no client-side re-filter that would desync from `total`.
  const items = rawItems

  // Memoized so the column defs keep a stable identity across renders.
  const columnActions = useMemo<ProxyLogsColumnActions>(
    () => ({
      onView: (log) => {
        setDetailLog(log)
        setDetailOpen(true)
      },
    }),
    []
  )
  const columns = useProxyLogsColumns(columnActions)

  // Desktop row click opens the detail sheet directly — the failure reason is
  // now readable in the row itself, and one click (instead of the two-step
  // ⋯ → View details) reaches the full detail. Rows are keyboard-accessible
  // (focusable; Enter/Space opens the sheet). Clicks originating inside
  // nested interactive elements (the row ⋯ dropdown trigger, links, form
  // controls) bubble to the row — skip them so a nested control is never
  // swallowed by the row-level handler.
  const renderRow = (
    row: Row<ProxyLog>,
    helpers: DataTableRenderRowHelpers
  ) => (
    <DataTableRow
      row={row}
      className='cursor-pointer'
      tabIndex={0}
      onClick={(event) => {
        const target = event.target as HTMLElement
        if (target.closest('button, a, [role="menuitem"], input, select')) {
          return
        }
        columnActions.onView(row.original)
      }}
      onKeyDown={(event) => {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault()
          columnActions.onView(row.original)
        }
      }}
      getColumnClassName={(columnId) => helpers.getCellClassName(columnId)}
    />
  )

  const { table } = useDataTable<ProxyLog>({
    data: items,
    columns,
    manualPagination: true,
    manualFiltering: true,
    manualSorting: true,
    enableColumnResizing: true,
    columnVisibilityStorageKey: PROXY_LOGS_COLUMN_VISIBILITY_STORAGE_KEY,
    columnSizingStorageKey: PROXY_LOGS_COLUMN_SIZING_STORAGE_KEY,
    // The URL-synced callbacks already reset the page on every filter change
    // (resetPageIndexOnFilterChange), so disable TanStack's own auto-reset to
    // avoid a second redundant page update.
    autoResetPageIndex: false,
    globalFilter,
    onGlobalFilterChange,
    pagination,
    onPaginationChange,
    getRowId: (row) => String(row.id),
    totalCount: total,
  })

  const summary = metaQuery.data?.summary
  const clientOptions = metaQuery.data?.clientOptions ?? []
  const siteOptions = metaQuery.data?.sites ?? []

  function handleReset() {
    updateUrlState({
      q: '',
      pageIndex: 0,
      filters: {
        status: 'all',
        siteId: '',
        channelId: '',
        client: '',
        from: '',
        to: '',
        latencyMin: '',
        latencyMax: '',
      },
    })
  }

  async function handleExportCsv() {
    setIsExporting(true)
    try {
      const exportPayload = {
        ...queryPayload,
        limit: PROXY_LOGS_CSV_EXPORT_LIMIT,
        offset: 0,
      }
      const response = await api.getProxyLogsQuery(exportPayload)
      const rows = response.items ?? []
      const csv = proxyLogsToCsv(rows, t)
      const blob = new Blob([csv], { type: 'text/csv;charset=utf-8;' })
      const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a')
      anchor.href = url
      anchor.download = `metapi-proxy-logs-${formatExportStamp()}.csv`
      anchor.click()
      URL.revokeObjectURL(url)
      toast.success(t('proxyLogs.page.exportCsvToast', { count: rows.length }))
    } catch (exportError) {
      toast.error(
        t('proxyLogs.page.exportCsvFailed', {
          message:
            exportError instanceof Error ? exportError.message : 'unknown',
        })
      )
    } finally {
      setIsExporting(false)
    }
  }

  const hasLatencyFilter = latencyMin !== null || latencyMax !== null
  const slowOnly = latencyMin === 2000 && latencyMax === null

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div className='flex flex-wrap items-center justify-between gap-3'>
        <div>
          <h1 className='text-lg font-normal'>{t('proxyLogs.page.title')}</h1>
          <p className='text-muted-foreground text-sm'>
            {t('proxyLogs.page.description')}
          </p>
        </div>
        <div className='flex flex-wrap items-center gap-2'>
          <ProxyLogsAutoRefreshToggle
            intervalMs={intervalMs}
            setIntervalMs={setIntervalMs}
          />
          <ProxyLogsHeaderActions
            onExport={handleExportCsv}
            isExporting={isExporting}
            onRefresh={() => {
              // Refresh both halves of the split fetch: the list (view=query)
              // and the meta facets/summary (view=meta).
              logsQuery.refetch()
              metaQuery.refetch()
            }}
            isRefreshing={logsQuery.isFetching || metaQuery.isFetching}
          />
        </div>
      </div>

      {summary && (
        <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
          <SummaryCard
            label={t('proxyLogs.page.summaryTotal')}
            value={formatInt(summary.totalCount)}
          />
          <SummaryCard
            label={t('proxyLogs.page.summarySuccess')}
            value={formatInt(summary.successCount)}
            tone='success'
          />
          <SummaryCard
            label={t('proxyLogs.page.summaryFailed')}
            value={formatInt(summary.failedCount)}
            tone='danger'
          />
          <SummaryCard
            label={t('proxyLogs.page.summaryCost')}
            value={formatCurrency(summary.totalCost, { fractionDigits: 4 })}
          />
        </div>
      )}

      <QueryErrorBanner
        error={logsQuery.error as Error | null}
        messageKey='proxyLogs.page.loadError'
        onRetry={() => logsQuery.refetch()}
        isRetrying={logsQuery.isFetching}
      />

      <DataTablePage
        table={table}
        columns={columns}
        renderRow={renderRow}
        isLoading={logsQuery.isLoading}
        isFetching={logsQuery.isFetching}
        emptyTitle={t('proxyLogs.page.emptyTitle')}
        emptyDescription={t('proxyLogs.page.emptyDescription')}
        emptyAction={
          <Button
            variant='outline'
            onClick={() => void navigate({ to: '/token-routes' })}
          >
            {t('proxyLogs.page.emptyAction')}
          </Button>
        }
        skeletonKeyPrefix='proxy-log-skeleton'
        toolbarProps={{
          searchPlaceholder: t('proxyLogs.page.searchPlaceholder'),
          searchDebounceMs: 400,
          additionalSearch: (
            <>
              <Select
                value={status}
                onValueChange={(value) => {
                  updateUrlState({
                    filters: { status: value ?? 'all' },
                    pageIndex: 0,
                  })
                }}
              >
                <SelectTrigger
                  size='sm'
                  aria-label={t('proxyLogs.page.filterStatusPlaceholder')}
                  className='w-[120px]'
                >
                  <SelectValue>
                    {(selected) => {
                      const option = PROXY_LOG_STATUS_FILTER_OPTIONS.find(
                        (item) => item.value === selected
                      )
                      return option
                        ? t(option.labelKey)
                        : selected
                          ? String(selected)
                          : t('proxyLogs.filter.all')
                    }}
                  </SelectValue>
                </SelectTrigger>
                <SelectContent>
                  {PROXY_LOG_STATUS_FILTER_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {t(option.labelKey)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
              {siteOptions.length > 0 && (
                <Select
                  value={siteId === null ? 'all' : String(siteId)}
                  onValueChange={(value) => {
                    updateUrlState({
                      filters: {
                        siteId: !value || value === 'all' ? '' : value,
                      },
                      pageIndex: 0,
                    })
                  }}
                >
                  <SelectTrigger
                    size='sm'
                    aria-label={t('proxyLogs.page.filterSitePlaceholder')}
                    className='w-[160px]'
                  >
                    <SelectValue>
                      {(selected) => {
                        if (!selected || selected === 'all') {
                          return t('proxyLogs.page.filterAllSites')
                        }
                        const site = siteOptions.find(
                          (item) => String(item.id) === selected
                        )
                        return site
                          ? site.name || `#${site.id}`
                          : String(selected)
                      }}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>
                      {t('proxyLogs.page.filterAllSites')}
                    </SelectItem>
                    {siteOptions.map((site) => (
                      <SelectItem key={site.id} value={String(site.id)}>
                        {site.name || `#${site.id}`}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
              {clientOptions.length > 0 && (
                <Select
                  value={client || 'all'}
                  onValueChange={(value) => {
                    updateUrlState({
                      filters: {
                        client: !value || value === 'all' ? '' : value,
                      },
                      pageIndex: 0,
                    })
                  }}
                >
                  <SelectTrigger
                    size='sm'
                    aria-label={t('proxyLogs.page.filterClientPlaceholder')}
                    className='w-[160px]'
                  >
                    <SelectValue>
                      {(selected) => {
                        if (!selected || selected === 'all') {
                          return t('proxyLogs.page.filterAllClients')
                        }
                        const option = clientOptions.find(
                          (item) => item.value === selected
                        )
                        return option ? option.label : String(selected)
                      }}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>
                      {t('proxyLogs.page.filterAllClients')}
                    </SelectItem>
                    {clientOptions.map((option) => (
                      <SelectItem key={option.value} value={option.value}>
                        {option.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
              <Input
                type='number'
                min={1}
                aria-label={t('proxyLogs.page.filterChannelPlaceholder')}
                placeholder={t('proxyLogs.page.filterChannelPlaceholder')}
                value={filters.channelId}
                onChange={(event) => {
                  updateUrlState({
                    filters: { channelId: event.target.value },
                    pageIndex: 0,
                  })
                }}
                className='w-[130px]'
              />
              <Input
                type='datetime-local'
                aria-label={t('proxyLogs.page.startTime')}
                value={from}
                onChange={(event) => {
                  updateUrlState({
                    filters: { from: event.target.value },
                    pageIndex: 0,
                  })
                }}
                className='w-[180px]'
              />
              <Input
                type='datetime-local'
                aria-label={t('proxyLogs.page.endTime')}
                value={to}
                onChange={(event) => {
                  updateUrlState({
                    filters: { to: event.target.value },
                    pageIndex: 0,
                  })
                }}
                className='w-[180px]'
              />
            </>
          ),
          expandable: (
            <>
              <div className='flex items-center gap-1.5'>
                <label className='text-muted-foreground text-xs'>
                  {t('proxyLogs.page.latencyMin')}
                </label>
                <Input
                  type='number'
                  min={0}
                  placeholder={t('proxyLogs.page.latencyUnit')}
                  value={latencyMin ?? ''}
                  onChange={(event) => {
                    const value = event.target.value
                    updateUrlState({
                      filters: { latencyMin: value },
                      pageIndex: 0,
                    })
                  }}
                  className='w-[100px]'
                />
              </div>
              <div className='flex items-center gap-1.5'>
                <label className='text-muted-foreground text-xs'>
                  {t('proxyLogs.page.latencyMax')}
                </label>
                <Input
                  type='number'
                  min={0}
                  placeholder={t('proxyLogs.page.latencyUnit')}
                  value={latencyMax ?? ''}
                  onChange={(event) => {
                    const value = event.target.value
                    updateUrlState({
                      filters: { latencyMax: value },
                      pageIndex: 0,
                    })
                  }}
                  className='w-[100px]'
                />
              </div>
              <div className='flex items-center gap-1.5'>
                <Button
                  type='button'
                  variant={slowOnly ? 'default' : 'outline'}
                  size='sm'
                  onClick={() => {
                    if (slowOnly) {
                      updateUrlState({
                        filters: { latencyMin: '', latencyMax: '' },
                        pageIndex: 0,
                      })
                    } else {
                      updateUrlState({
                        filters: { latencyMin: '2000', latencyMax: '' },
                        pageIndex: 0,
                      })
                    }
                  }}
                >
                  {t('proxyLogs.page.slowOnly')}
                </Button>
              </div>
              {hasLatencyFilter && (
                <div className='flex items-center gap-1.5 text-xs'>
                  <span className='text-muted-foreground'>
                    {t('proxyLogs.page.latencyExample')}
                  </span>
                  <LatencyBadge latencyMs={latencyMin ?? 0} />
                  <span className='text-muted-foreground'>~</span>
                  <LatencyBadge latencyMs={latencyMax ?? 0} />
                </div>
              )}
            </>
          ),
          hasExpandedActiveFilters: hasLatencyFilter,
          hasAdditionalFilters:
            siteId !== null ||
            channelId !== null ||
            !!client ||
            !!from ||
            !!to ||
            status !== 'all',
          onReset: handleReset,
        }}
      />

      <ProxyLogDetailSheet
        log={detailLog}
        open={detailOpen}
        onOpenChange={setDetailOpen}
      />
    </div>
  )
}

function SummaryCard({
  label,
  value,
  tone,
}: {
  label: string
  value: string
  tone?: 'success' | 'danger'
}) {
  const toneClass =
    tone === 'success'
      ? 'border-success/30 bg-success/5'
      : tone === 'danger'
        ? 'border-destructive/30 bg-destructive/5'
        : 'border-border bg-muted/30'
  const valueToneClass =
    tone === 'success'
      ? 'text-success'
      : tone === 'danger'
        ? 'text-destructive'
        : 'text-foreground'
  return (
    <div className={`rounded-lg border p-2.5 ${toneClass}`}>
      <div className='text-muted-foreground text-[11px]'>{label}</div>
      <div className={`text-base font-semibold tabular-nums ${valueToneClass}`}>
        {value}
      </div>
    </div>
  )
}

function formatExportStamp(): string {
  const now = new Date()
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}`
}
