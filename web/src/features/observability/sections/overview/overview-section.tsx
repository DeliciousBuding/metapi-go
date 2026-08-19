// metapi-go/features/observability/sections/overview — slow-request TOP list
// + usage heatmap. Both consume existing /api/stats endpoints (no new
// backend surface) so the Overview reads like a curated lens over proxy data.

import { BarChart3, Flame } from 'lucide-react'
import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatLatency } from '@/lib/format'

import { StatusBadge } from '@/features/proxy-logs/components/status-badge'

import { useSlowRequests, useUsageHeatmap } from '../../api'
import { ObservabilityErrorBanner } from '../../components/observability-error-banner'
import type { SlowRequestItem, UsageHeatmapCell } from '../../types'

const MAX_HEATMAP_KEYS = 16

function formatClock(iso: string): string {
  if (!iso) return '—'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  return date.toLocaleString()
}

function formatBucket(iso: string): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  return `${String(date.getUTCMonth() + 1).padStart(2, '0')}-${String(
    date.getUTCDate()
  ).padStart(2, '0')} ${String(date.getUTCHours()).padStart(2, '0')}:00`
}

function heatmapLayout(cells: UsageHeatmapCell[]) {
  const buckets = [...new Set(cells.map((cell) => cell.bucket))].sort()
  const totals = new Map<string, number>()
  for (const cell of cells) {
    totals.set(cell.key, (totals.get(cell.key) ?? 0) + cell.calls)
  }
  const keys = [...totals.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, MAX_HEATMAP_KEYS)
    .map(([key]) => key)

  const index = new Map<string, UsageHeatmapCell>()
  for (const cell of cells) {
    index.set(`${cell.bucket}\u0000${cell.key}`, cell)
  }

  let maxCalls = 1
  for (const cell of cells) {
    if (cell.calls > maxCalls) maxCalls = cell.calls
  }

  return { buckets, keys, index, maxCalls }
}

export function OverviewSection() {
  const { t } = useTranslation()
  const slow = useSlowRequests({ limit: 20, minLatencyMs: 1000, hours: 24 })
  const heatmap = useUsageHeatmap({ days: 7, dimension: 'site' })

  const layout = useMemo(
    () => heatmapLayout(heatmap.data?.cells ?? []),
    [heatmap.data]
  )

  const slowItems = slow.data?.items ?? []

  return (
    <div className='flex flex-col gap-4'>
      <Card>
        <CardHeader>
          <CardTitle className='flex items-center gap-2 text-sm font-medium'>
            <Flame className='size-4' />
            {t('observability.overview.slowRequests.title')}
          </CardTitle>
          <CardDescription className='text-xs'>
            {t('observability.overview.slowRequests.description')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {slow.isError && !slow.isLoading ? (
            <ObservabilityErrorBanner
              messageKey='observability.overview.slowRequests.loadFailed'
              isRetrying={slow.isFetching && !slow.isLoading}
              onRetry={() => void slow.refetch()}
            />
          ) : (
            renderSlowRequestsBody(slow.isLoading, slowItems, t)
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className='flex items-center gap-2 text-sm font-medium'>
            <BarChart3 className='size-4' />
            {t('observability.overview.heatmap.title')}
          </CardTitle>
          <CardDescription className='text-xs'>
            {t('observability.overview.heatmap.description')}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {heatmap.isError && !heatmap.isLoading ? (
            <ObservabilityErrorBanner
              messageKey='observability.overview.heatmap.loadFailed'
              isRetrying={heatmap.isFetching && !heatmap.isLoading}
              onRetry={() => void heatmap.refetch()}
            />
          ) : (
            renderHeatmapBody(heatmap.isLoading, layout, t)
          )}
        </CardContent>
      </Card>
    </div>
  )
}

function renderSlowRequestsBody(
  loading: boolean,
  items: SlowRequestItem[],
  t: (key: string) => string
): ReactNode {
  if (loading) {
    return <Skeleton className='h-48 w-full rounded-md' />
  }
  if (items.length === 0) {
    return (
      <div className='flex min-h-24 flex-col items-center justify-center gap-2 rounded-lg border border-dashed py-8 text-center'>
        <p className='text-muted-foreground text-sm'>
          {t('observability.overview.slowRequests.empty')}
        </p>
      </div>
    )
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>
            {t('observability.overview.slowRequests.colModel')}
          </TableHead>
          <TableHead>
            {t('observability.overview.slowRequests.colSite')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.overview.slowRequests.colLatency')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.overview.slowRequests.colFirstByte')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.overview.slowRequests.colHttp')}
          </TableHead>
          <TableHead>
            {t('observability.overview.slowRequests.colStatus')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.overview.slowRequests.colTime')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => (
          <TableRow key={item.id}>
            <TableCell className='max-w-56 truncate font-medium'>
              {item.model || '—'}
            </TableCell>
            <TableCell className='max-w-40 truncate'>
              {item.siteName || '—'}
            </TableCell>
            <TableCell className='text-right tabular-nums'>
              {formatLatency(item.latencyMs, {
                autoSeconds: true,
                secondsDigits: 1,
                spaced: true,
              })}
            </TableCell>
            <TableCell className='text-right tabular-nums'>
              {formatLatency(item.firstByteLatencyMs, {
                autoSeconds: true,
                secondsDigits: 1,
                spaced: true,
              })}
            </TableCell>
            <TableCell className='text-right tabular-nums'>
              {item.httpStatus || '—'}
            </TableCell>
            <TableCell>
              <StatusBadge
                httpStatus={item.httpStatus ?? null}
                status={item.status ?? null}
              />
            </TableCell>
            <TableCell className='text-right tabular-nums'>
              {formatClock(item.createdAt)}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

function renderHeatmapBody(
  loading: boolean,
  layout: ReturnType<typeof heatmapLayout>,
  t: (key: string) => string
): ReactNode {
  if (loading) {
    return <Skeleton className='h-48 w-full rounded-md' />
  }
  if (layout.keys.length === 0) {
    return (
      <div className='flex min-h-24 flex-col items-center justify-center gap-2 rounded-lg border border-dashed py-8 text-center'>
        <p className='text-muted-foreground text-sm'>
          {t('observability.overview.heatmap.empty')}
        </p>
      </div>
    )
  }
  return (
    <div className='overflow-x-auto'>
      <div className='min-w-max'>
        <div
          className='grid gap-px'
          style={{
            gridTemplateColumns: `160px repeat(${layout.buckets.length}, 14px)`,
          }}
        >
          <div />
          {layout.buckets.map((bucket) => (
            <div
              key={bucket}
              className='size-[14px]'
              title={formatBucket(bucket)}
              aria-label={formatBucket(bucket)}
            />
          ))}
          {layout.keys.map((key) => (
            <HeatmapRow
              key={key}
              keyName={key}
              buckets={layout.buckets}
              index={layout.index}
              maxCalls={layout.maxCalls}
            />
          ))}
        </div>
        <div className='text-muted-foreground mt-2 flex items-center justify-end gap-2 text-xs'>
          <span>{t('observability.overview.heatmap.legendLow')}</span>
          <div className='flex gap-px'>
            {[0, 0.25, 0.5, 0.75, 1].map((alpha) => (
              <div
                key={alpha}
                className='size-3 rounded-sm'
                style={{
                  backgroundColor: `color-mix(in srgb, var(--chart-1) ${Math.round(alpha * 100)}%, transparent)`,
                }}
              />
            ))}
          </div>
          <span>{t('observability.overview.heatmap.legendHigh')}</span>
        </div>
      </div>
    </div>
  )
}

function HeatmapRow({
  keyName,
  buckets,
  index,
  maxCalls,
}: {
  keyName: string
  buckets: string[]
  index: Map<string, UsageHeatmapCell>
  maxCalls: number
}) {
  const label = index.get(`${buckets[0]}\u0000${keyName}`)?.label ?? keyName
  return (
    <>
      <div className='flex items-center gap-1 truncate pr-2 text-xs'>
        <span className='truncate' title={label}>
          {label}
        </span>
      </div>
      {buckets.map((bucket) => {
        const cell = index.get(`${bucket}\u0000${keyName}`)
        const intensity = cell ? cell.calls / maxCalls : 0
        return (
          <div
            key={bucket}
            className='h-[14px] w-[14px] rounded-[2px]'
            style={{
              backgroundColor: `color-mix(in srgb, var(--chart-1) ${Math.round(intensity * 100)}%, transparent)`,
            }}
            title={cell ? `${label} · ${bucket} · ${cell.calls}` : undefined}
          />
        )
      })}
    </>
  )
}
