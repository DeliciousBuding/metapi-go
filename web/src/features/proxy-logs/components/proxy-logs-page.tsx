/* eslint-disable no-nested-ternary -- status-icon selection uses chained ternaries */
// metapi-go/features/proxy-logs/components — proxy logs list page.
// i18n: all user-visible strings migrated to t() calls.

import type { OnChangeFn, PaginationState } from '@tanstack/react-table'
import { Download as DownloadIcon, RefreshCw } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { DataTablePage, useDataTable } from '@/components/data-table'
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
import { toast } from '@/lib/toast'

import { useProxyLogs, useProxyLogsMeta } from '../api'
import {
  PROXY_LOG_STATUS_FILTER_OPTIONS,
  proxyLogsSearchSchema,
} from '../lib/proxy-logs-schema'
import { useProxyLogsAutoRefresh } from '../lib/use-proxy-logs-auto-refresh'
import type { ProxyLog, ProxyLogDetail, ProxyLogFilters } from '../types'
import { LatencyBadge } from './latency-badge'
import { ProxyLogDetailSheet } from './proxy-log-detail-sheet'
import { ProxyLogsAutoRefreshToggle } from './proxy-logs-auto-refresh-toggle'
import {
  useProxyLogsColumns,
  type ProxyLogsColumnActions,
} from './proxy-logs-columns'

const PROXY_LOGS_COLUMN_VISIBILITY_STORAGE_KEY =
  'metapi-go:proxy-logs:column-visibility'
const PROXY_LOGS_COLUMN_SIZING_STORAGE_KEY =
  'metapi-go:proxy-logs:column-sizing'
const DEFAULT_PAGE_SIZE = 20
const PROXY_LOGS_CSV_EXPORT_LIMIT = 10_000

type ResolvedUrlState = {
  pageIndex: number
  pageSize: number
  search: string
  status: ProxyLogFilters['status']
  siteId: number | null
  client: string
  from: string
  to: string
  latencyMin: number | null
  latencyMax: number | null
}

function readUrlState(): ResolvedUrlState {
  if (typeof window === 'undefined') {
    return {
      pageIndex: 0,
      pageSize: DEFAULT_PAGE_SIZE,
      search: '',
      status: 'all',
      siteId: null,
      client: '',
      from: '',
      to: '',
      latencyMin: null,
      latencyMax: null,
    }
  }
  const params = new URLSearchParams(window.location.search)
  const parsed = proxyLogsSearchSchema.safeParse({
    q: params.get('q') ?? undefined,
    page: params.get('page') ?? undefined,
    pageSize: params.get('pageSize') ?? undefined,
    status: params.get('status') ?? undefined,
    siteId: params.get('siteId') ?? undefined,
    client: params.get('client') ?? undefined,
    from: params.get('from') ?? undefined,
    to: params.get('to') ?? undefined,
    latencyMin: params.get('latencyMin') ?? undefined,
    latencyMax: params.get('latencyMax') ?? undefined,
  })
  if (!parsed.success) {
    return {
      pageIndex: 0,
      pageSize: DEFAULT_PAGE_SIZE,
      search: '',
      status: 'all',
      siteId: null,
      client: '',
      from: '',
      to: '',
      latencyMin: null,
      latencyMax: null,
    }
  }
  const data = parsed.data
  return {
    pageIndex: data.page ?? 0,
    pageSize: data.pageSize ?? DEFAULT_PAGE_SIZE,
    search: data.q ?? '',
    status: data.status ?? 'all',
    siteId: data.siteId ?? null,
    client: data.client ?? '',
    from: data.from ?? '',
    to: data.to ?? '',
    latencyMin: data.latencyMin ?? null,
    latencyMax: data.latencyMax ?? null,
  }
}

function writeUrlState(state: ResolvedUrlState) {
  if (typeof window === 'undefined') return
  const params = new URLSearchParams()
  if (state.search) params.set('q', state.search)
  if (state.pageIndex > 0) params.set('page', String(state.pageIndex))
  if (state.pageSize !== DEFAULT_PAGE_SIZE) {
    params.set('pageSize', String(state.pageSize))
  }
  if (state.status && state.status !== 'all') params.set('status', state.status)
  if (state.siteId !== null) params.set('siteId', String(state.siteId))
  if (state.client) params.set('client', state.client)
  if (state.from) params.set('from', state.from)
  if (state.to) params.set('to', state.to)
  if (state.latencyMin !== null) {
    params.set('latencyMin', String(state.latencyMin))
  }
  if (state.latencyMax !== null) {
    params.set('latencyMax', String(state.latencyMax))
  }
  const query = params.toString()
  const url = query
    ? `${window.location.pathname}?${query}`
    : window.location.pathname
  window.history.replaceState(null, '', url)
}

export function ProxyLogsPage() {
  const { t } = useTranslation()
  const initial = useMemo(readUrlState, [])
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: initial.pageIndex,
    pageSize: initial.pageSize,
  })
  const [search, setSearch] = useState(initial.search)
  const [status, setStatus] = useState<ProxyLogFilters['status']>(
    initial.status
  )
  const [siteId, setSiteId] = useState<number | null>(initial.siteId)
  const [client, setClient] = useState(initial.client)
  const [from, setFrom] = useState(initial.from)
  const [to, setTo] = useState(initial.to)
  const [latencyMin, setLatencyMin] = useState<number | null>(
    initial.latencyMin
  )
  const [latencyMax, setLatencyMax] = useState<number | null>(
    initial.latencyMax
  )
  const [detailLog, setDetailLog] = useState<ProxyLog | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)
  const [isExporting, setIsExporting] = useState(false)

  useEffect(() => {
    writeUrlState({
      pageIndex: pagination.pageIndex,
      pageSize: pagination.pageSize,
      search,
      status,
      siteId,
      client,
      from,
      to,
      latencyMin,
      latencyMax,
    })
  }, [
    pagination,
    search,
    status,
    siteId,
    client,
    from,
    to,
    latencyMin,
    latencyMax,
  ])

  const queryPayload = useMemo(
    () => ({
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
      status: status === 'all' ? undefined : status,
      search: search.trim() || undefined,
      siteId: siteId ?? undefined,
      client: client || undefined,
      from: from || undefined,
      to: to || undefined,
    }),
    [pagination, status, search, siteId, client, from, to]
  )

  const metaPayload = useMemo(
    () => ({
      status: queryPayload.status,
      search: queryPayload.search,
      siteId: queryPayload.siteId,
      client: queryPayload.client,
      from: queryPayload.from,
      to: queryPayload.to,
    }),
    [queryPayload]
  )
  const { intervalMs } = useProxyLogsAutoRefresh()
  const logsQuery = useProxyLogs(queryPayload, {
    refetchInterval: intervalMs === false ? false : intervalMs,
  })
  const metaQuery = useProxyLogsMeta(metaPayload)
  const rawItems = useMemo(() => logsQuery.data?.items ?? [], [logsQuery.data])
  const total = logsQuery.data?.total ?? 0

  const items = useMemo(() => {
    if (latencyMin === null && latencyMax === null) return rawItems
    return rawItems.filter((log) => {
      const latency = log.latencyMs
      if (typeof latency !== 'number' || latency < 0) return false
      if (latencyMin !== null && latency < latencyMin) return false
      if (latencyMax !== null && latency > latencyMax) return false
      return true
    })
  }, [rawItems, latencyMin, latencyMax])

  const onGlobalFilterChange = useMemo<OnChangeFn<string>>(
    () => (updater) => {
      setSearch((prev) =>
        updater instanceof Function ? updater(prev) : updater
      )
      setPagination((prev) => ({ ...prev, pageIndex: 0 }))
    },
    []
  )
  const onPaginationChange = useMemo<OnChangeFn<PaginationState>>(
    () => (updater) => {
      setPagination((prev) =>
        updater instanceof Function ? updater(prev) : updater
      )
    },
    []
  )

  const columnActions: ProxyLogsColumnActions = {
    onView: (log) => {
      setDetailLog(log)
      setDetailOpen(true)
    },
  }
  const columns = useProxyLogsColumns(columnActions)

  const { table } = useDataTable<ProxyLog>({
    data: items,
    columns,
    manualPagination: true,
    manualFiltering: true,
    manualSorting: true,
    enableColumnResizing: true,
    columnVisibilityStorageKey: PROXY_LOGS_COLUMN_VISIBILITY_STORAGE_KEY,
    columnSizingStorageKey: PROXY_LOGS_COLUMN_SIZING_STORAGE_KEY,
    globalFilter: search,
    onGlobalFilterChange,
    pagination,
    onPaginationChange,
    getRowId: (row) => String(row.id),
    totalCount: total,
  })

  const summary = logsQuery.data?.summary
  const clientOptions = metaQuery.data?.clientOptions ?? []
  const siteOptions = metaQuery.data?.sites ?? []

  function handleReset() {
    setSearch('')
    setStatus('all')
    setSiteId(null)
    setClient('')
    setFrom('')
    setTo('')
    setLatencyMin(null)
    setLatencyMax(null)
    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
  }

  async function handleExportCsv() {
    setIsExporting(true)
    try {
      const exportPayload = {
        ...queryPayload,
        limit: PROXY_LOGS_CSV_EXPORT_LIMIT,
        offset: 0,
      }
      const response = await api.getProxyLogs(exportPayload)
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
      <div className='flex items-center justify-between'>
        <div>
          <h1 className='text-lg font-normal'>{t('proxyLogs.page.title')}</h1>
          <p className='text-muted-foreground text-sm'>
            {t('proxyLogs.page.description')}
          </p>
        </div>
        <div className='flex items-center gap-2'>
          <ProxyLogsAutoRefreshToggle />
          <Button
            variant='outline'
            size='sm'
            onClick={handleExportCsv}
            disabled={isExporting}
          >
            <DownloadIcon
              className={isExporting ? 'animate-pulse' : undefined}
            />
            {t('proxyLogs.page.exportCsv')}
          </Button>
          <Button
            variant='outline'
            size='sm'
            onClick={() => logsQuery.refetch()}
            disabled={logsQuery.isFetching}
          >
            <RefreshCw
              className={logsQuery.isFetching ? 'animate-spin' : undefined}
            />
            {t('proxyLogs.page.refresh')}
          </Button>
        </div>
      </div>

      {summary && (
        <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
          <SummaryCard
            label={t('proxyLogs.page.summaryTotal')}
            value={String(summary.totalCount)}
          />
          <SummaryCard
            label={t('proxyLogs.page.summarySuccess')}
            value={String(summary.successCount)}
            tone='success'
          />
          <SummaryCard
            label={t('proxyLogs.page.summaryFailed')}
            value={String(summary.failedCount)}
            tone='danger'
          />
          <SummaryCard
            label={t('proxyLogs.page.summaryCost')}
            value={`$${summary.totalCost.toFixed(4)}`}
          />
        </div>
      )}

      {logsQuery.error && (
        <div className='border-destructive/40 bg-destructive/10 text-destructive-soft-fg rounded-lg border p-3 text-sm'>
          {t('proxyLogs.page.loadError', {
            message: (logsQuery.error as Error).message,
          })}
        </div>
      )}

      <DataTablePage
        table={table}
        columns={columns}
        isLoading={logsQuery.isLoading}
        isFetching={logsQuery.isFetching}
        emptyTitle={t('proxyLogs.page.emptyTitle')}
        emptyDescription={t('proxyLogs.page.emptyDescription')}
        skeletonKeyPrefix='proxy-log-skeleton'
        toolbarProps={{
          searchPlaceholder: t('proxyLogs.page.searchPlaceholder'),
          searchDebounceMs: 400,
          additionalSearch: (
            <>
              <Select
                value={status}
                onValueChange={(value) => {
                  setStatus(value as ProxyLogFilters['status'])
                  setPagination((prev) => ({ ...prev, pageIndex: 0 }))
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
                    setSiteId(value === 'all' ? null : Number(value))
                    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
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
                    setClient(!value || value === 'all' ? '' : value)
                    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
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
                type='datetime-local'
                aria-label={t('proxyLogs.page.startTime')}
                value={from}
                onChange={(event) => {
                  setFrom(event.target.value)
                  setPagination((prev) => ({ ...prev, pageIndex: 0 }))
                }}
                className='w-[180px]'
              />
              <Input
                type='datetime-local'
                aria-label={t('proxyLogs.page.endTime')}
                value={to}
                onChange={(event) => {
                  setTo(event.target.value)
                  setPagination((prev) => ({ ...prev, pageIndex: 0 }))
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
                    setLatencyMin(value === '' ? null : Number(value))
                    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
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
                    setLatencyMax(value === '' ? null : Number(value))
                    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
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
                      setLatencyMin(null)
                    } else {
                      setLatencyMin(2000)
                      setLatencyMax(null)
                    }
                    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
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
            siteId !== null || !!client || !!from || !!to || status !== 'all',
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

// CSV export column header keys. The proxy_logs table does not persist the
// HTTP method or upstream path (only the downstream trace surface does), so
// the CSV sticks to the columns the /api/stats/proxy-logs response actually
// returns. httpStatus IS in the response (pl.*) even though the list type
// historically omitted it, so it is surfaced here as a bonus column.
const PROXY_LOGS_CSV_COLUMNS = [
  'timestamp',
  'httpStatus',
  'status',
  'model',
  'account',
  'site',
  'duration',
  'tokens',
  'estimatedCost',
] as const

function csvEscape(value: string | number | null | undefined): string {
  if (value === null || value === undefined) return ''
  const text = String(value)
  if (/[",\n\r]/.test(text)) {
    return `"${text.replaceAll('"', '""')}"`
  }
  return text
}

function proxyLogsToCsv(
  rows: ProxyLog[],
  t: (key: string, params?: Record<string, unknown>) => string
): string {
  const header = PROXY_LOGS_CSV_COLUMNS.map((column) =>
    csvEscape(
      t(`proxyLogs.page.exportCsv.column.${column}`, { defaultValue: column })
    )
  ).join(',')
  const body = rows
    .map((log) => {
      const accountLabel =
        log.username || (log.accountId ? `#${log.accountId}` : '')
      const siteLabel = log.siteName || (log.siteId ? `#${log.siteId}` : '')
      const modelLabel =
        log.modelActual?.trim() || log.modelRequested?.trim() || ''
      const cells = [
        log.createdAt,
        log.httpStatus ?? '',
        log.status,
        modelLabel,
        accountLabel,
        siteLabel,
        log.latencyMs,
        log.totalTokens ?? '',
        log.estimatedCost ?? '',
      ]
      return cells.map(csvEscape).join(',')
    })
    .join('\n')
  return `${header}\n${body}`
}

export type { ProxyLogDetail }
