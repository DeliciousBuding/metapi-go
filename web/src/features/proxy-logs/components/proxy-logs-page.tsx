// metapi-go/features/proxy-logs/components — proxy logs list page.
//
// Wires the four-layer data-table to the server-paginated proxy-logs query.
// Because the backend is paginated (`getProxyLogs` returns items + total +
// page + pageSize), the table runs in manual mode (`manualPagination` /
// `manualFiltering` / `manualSorting`) and the page owns the full filter
// state, translating it into the `ProxyLogsQuery` payload (limit/offset/
// status/search/client/siteId/from/to). The latency-range filter is applied
// client-side on the fetched page because the backend does not support
// latency filtering. Table state (search / page / pageSize / status / site
// / client / date range / latency range) is mirrored to the URL search
// string so a deep link restores the exact view. Mobile card degradation is
// handled by `DataTablePage` via the column `meta` flags.

import { useEffect, useMemo, useState } from 'react'
import type {
  OnChangeFn,
  PaginationState,
} from '@tanstack/react-table'
import { RefreshCw } from 'lucide-react'

import {
  DataTablePage,
  useDataTable,
} from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

import { useProxyLogs, useProxyLogsMeta } from '../api'
import {
  PROXY_LOG_STATUS_FILTER_OPTIONS,
  proxyLogsSearchSchema,
} from '../lib/proxy-logs-schema'
import type {
  ProxyLog,
  ProxyLogDetail,
  ProxyLogFilters,
} from '../types'
import { LatencyBadge } from './latency-badge'
import { ProxyLogDetailSheet } from './proxy-log-detail-sheet'
import {
  useProxyLogsColumns,
  type ProxyLogsColumnActions,
} from './proxy-logs-columns'

const PROXY_LOGS_COLUMN_VISIBILITY_STORAGE_KEY =
  'metapi-go:proxy-logs:column-visibility'
const PROXY_LOGS_COLUMN_SIZING_STORAGE_KEY =
  'metapi-go:proxy-logs:column-sizing'
const DEFAULT_PAGE_SIZE = 20

// --- URL state helpers ----------------------------------------------------

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
  if (state.pageSize !== DEFAULT_PAGE_SIZE)
    params.set('pageSize', String(state.pageSize))
  if (state.status && state.status !== 'all') params.set('status', state.status)
  if (state.siteId !== null) params.set('siteId', String(state.siteId))
  if (state.client) params.set('client', state.client)
  if (state.from) params.set('from', state.from)
  if (state.to) params.set('to', state.to)
  if (state.latencyMin !== null) params.set('latencyMin', String(state.latencyMin))
  if (state.latencyMax !== null) params.set('latencyMax', String(state.latencyMax))
  const query = params.toString()
  const url = query
    ? `${window.location.pathname}?${query}`
    : window.location.pathname
  window.history.replaceState(null, '', url)
}

// --- Page -----------------------------------------------------------------

export function ProxyLogsPage() {
  const initial = useMemo(readUrlState, [])

  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: initial.pageIndex,
    pageSize: initial.pageSize,
  })
  const [search, setSearch] = useState(initial.search)
  const [status, setStatus] = useState<ProxyLogFilters['status']>(initial.status)
  const [siteId, setSiteId] = useState<number | null>(initial.siteId)
  const [client, setClient] = useState(initial.client)
  const [from, setFrom] = useState(initial.from)
  const [to, setTo] = useState(initial.to)
  const [latencyMin, setLatencyMin] = useState<number | null>(initial.latencyMin)
  const [latencyMax, setLatencyMax] = useState<number | null>(initial.latencyMax)

  const [detailLog, setDetailLog] = useState<ProxyLog | null>(null)
  const [detailOpen, setDetailOpen] = useState(false)

  // Sync the full state to the URL whenever it changes (replaceState is
  // cheap and keeps the address bar + deep links accurate).
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

  // Build the backend query payload from the filter state. Latency range is
  // NOT part of `ProxyLogsQuery` (backend doesn't support it) — it's applied
  // client-side below.
  const queryPayload = useMemo(() => {
    return {
      limit: pagination.pageSize,
      offset: pagination.pageIndex * pagination.pageSize,
      status: status === 'all' ? undefined : status,
      search: search.trim() || undefined,
      siteId: siteId ?? undefined,
      client: client || undefined,
      from: from || undefined,
      to: to || undefined,
    }
  }, [pagination, status, search, siteId, client, from, to])

  const metaPayload = useMemo(
    () => ({
      status: queryPayload.status,
      search: queryPayload.search,
      siteId: queryPayload.siteId,
      client: queryPayload.client,
      from: queryPayload.from,
      to: queryPayload.to,
    }),
    [queryPayload],
  )

  const logsQuery = useProxyLogs(queryPayload)
  const metaQuery = useProxyLogsMeta(metaPayload)

  const rawItems = logsQuery.data?.items ?? []
  const total = logsQuery.data?.total ?? 0

  // Client-side latency-range filter on the fetched page.
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

  // onChange wrappers that reset to the first page on filter changes so the
  // user never lands on a page index past the new result set.
  const onGlobalFilterChange = useMemo<OnChangeFn<string>>(
    () => (updater) => {
      setSearch((prev) => (updater instanceof Function ? updater(prev) : updater))
      setPagination((prev) => ({ ...prev, pageIndex: 0 }))
    },
    [],
  )
  const onPaginationChange = useMemo<OnChangeFn<PaginationState>>(
    () => (updater) => {
      setPagination((prev) =>
        updater instanceof Function ? updater(prev) : updater,
      )
    },
    [],
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
    onPaginationChange: onPaginationChange,
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

  const hasLatencyFilter = latencyMin !== null || latencyMax !== null

  return (
    <div className='flex h-full flex-col gap-3 p-4'>
      <div className='flex items-center justify-between'>
        <div>
          <h1 className='text-lg font-semibold'>代理日志</h1>
          <p className='text-muted-foreground text-sm'>
            代理请求日志，用于运维排查：时间 / 账号 / 站点 / 模型 / 状态 / 延迟 / 令牌。
          </p>
        </div>
        <Button
          variant='outline'
          size='sm'
          onClick={() => logsQuery.refetch()}
          disabled={logsQuery.isFetching}
        >
          <RefreshCw className={logsQuery.isFetching ? 'animate-spin' : undefined} />
          刷新
        </Button>
      </div>

      {summary && (
        <div className='grid grid-cols-2 gap-2 sm:grid-cols-4'>
          <SummaryCard label='总数' value={String(summary.totalCount)} />
          <SummaryCard
            label='成功'
            value={String(summary.successCount)}
            tone='success'
          />
          <SummaryCard
            label='失败'
            value={String(summary.failedCount)}
            tone='danger'
          />
          <SummaryCard
            label='总成本'
            value={`$${summary.totalCost.toFixed(4)}`}
          />
        </div>
      )}

      {logsQuery.error && (
        <div className='rounded-lg border border-destructive/40 bg-destructive/10 p-3 text-destructive text-sm'>
          加载代理日志失败：{(logsQuery.error as Error).message}
        </div>
      )}

      <DataTablePage
        table={table}
        columns={columns}
        isLoading={logsQuery.isLoading}
        isFetching={logsQuery.isFetching}
        emptyTitle='暂无代理日志'
        emptyDescription='调整筛选条件或稍后刷新以查看代理请求记录。'
        skeletonKeyPrefix='proxy-log-skeleton'
        toolbarProps={{
          searchPlaceholder: '搜索模型 / 账号 / 令牌…',
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
                <SelectTrigger size='sm' className='w-[120px]'>
                  <SelectValue placeholder='状态' />
                </SelectTrigger>
                <SelectContent>
                  {PROXY_LOG_STATUS_FILTER_OPTIONS.map((option) => (
                    <SelectItem key={option.value} value={option.value}>
                      {option.label}
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
                  <SelectTrigger size='sm' className='w-[160px]'>
                    <SelectValue placeholder='站点' />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>全部站点</SelectItem>
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
                  <SelectTrigger size='sm' className='w-[160px]'>
                    <SelectValue placeholder='客户端' />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='all'>全部客户端</SelectItem>
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
                aria-label='起始时间'
                value={from}
                onChange={(event) => {
                  setFrom(event.target.value)
                  setPagination((prev) => ({ ...prev, pageIndex: 0 }))
                }}
                className='w-[180px]'
              />
              <Input
                type='datetime-local'
                aria-label='结束时间'
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
                <label className='text-muted-foreground text-xs'>延迟 ≥</label>
                <Input
                  type='number'
                  min={0}
                  placeholder='ms'
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
                <label className='text-muted-foreground text-xs'>延迟 ≤</label>
                <Input
                  type='number'
                  min={0}
                  placeholder='ms'
                  value={latencyMax ?? ''}
                  onChange={(event) => {
                    const value = event.target.value
                    setLatencyMax(value === '' ? null : Number(value))
                    setPagination((prev) => ({ ...prev, pageIndex: 0 }))
                  }}
                  className='w-[100px]'
                />
              </div>
              {hasLatencyFilter && (
                <div className='flex items-center gap-1.5 text-xs'>
                  <span className='text-muted-foreground'>示例:</span>
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

// ---------------------------------------------------------------------------
// Summary card
// ---------------------------------------------------------------------------

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

// Re-export the detail type so consumers of the barrel get the merged shape.
export type { ProxyLogDetail }
