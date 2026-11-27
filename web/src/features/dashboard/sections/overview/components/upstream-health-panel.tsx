import { Link } from '@tanstack/react-router'
import { Globe } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { toBcp47 } from '@/i18n/languages'
import type { OverviewPeriod, OverviewSiteSample } from '@/lib/api'
import { formatInt, formatLatency, formatRatio } from '@/lib/format'
import { cn } from '@/lib/utils'

import { useOverviewReport } from '../use-overview-report'

function formatSampleBucketUtc(iso: string): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return iso
  return `${String(date.getUTCMonth() + 1).padStart(2, '0')}-${String(
    date.getUTCDate()
  ).padStart(2, '0')} ${String(date.getUTCHours()).padStart(2, '0')}:00 UTC`
}

function sampleBarColorClass(sample: OverviewSiteSample): string {
  if (sample.success >= sample.total) return 'bg-success'
  if (sample.success <= 0) return 'bg-destructive'
  return 'bg-warning'
}

function SiteSamplesStrip({
  samples,
  siteName,
  locale,
}: {
  samples: OverviewSiteSample[] | undefined
  siteName: string
  locale: string
}) {
  const { t } = useTranslation()
  // Only render real non-empty UTC hour buckets — never pad with fake gray bars.
  const realSamples = (samples ?? []).filter((item) => item.total > 0)
  if (realSamples.length === 0) {
    return (
      <span className='text-muted-foreground text-xs'>
        {t('dashboard.overviewInsights.samplesEmpty')}
      </span>
    )
  }

  return (
    <TooltipProvider delay={150}>
      <div
        role='group'
        aria-label={t('dashboard.overviewInsights.samplesAria', {
          name: siteName,
          count: realSamples.length,
        })}
        className='inline-flex h-4 items-end gap-[2px]'
      >
        {realSamples.map((sample) => {
          const bucketLabel = formatSampleBucketUtc(sample.bucketTs)
          const latencyLabel =
            sample.avgLatencyMs != null
              ? formatLatency(sample.avgLatencyMs, {
                  autoSeconds: true,
                  secondsDigits: 1,
                  spaced: true,
                })
              : '—'
          const tooltipText = t('dashboard.overviewInsights.sampleTooltip', {
            bucket: bucketLabel,
            success: formatInt(sample.success, locale),
            total: formatInt(sample.total, locale),
            latency: latencyLabel,
          })
          return (
            <Tooltip key={sample.bucketTs}>
              <TooltipTrigger
                render={
                  <button
                    type='button'
                    aria-label={tooltipText}
                    className={cn(
                      'focus-visible:ring-ring h-3.5 w-[4px] rounded-[1px] transition-opacity outline-none hover:opacity-80 focus-visible:ring-2',
                      sampleBarColorClass(sample)
                    )}
                  />
                }
              />
              <TooltipContent side='top' className='text-xs tabular-nums'>
                {tooltipText}
              </TooltipContent>
            </Tooltip>
          )
        })}
      </div>
    </TooltipProvider>
  )
}

export function UpstreamHealthPanel({
  period = '7d',
}: {
  period?: OverviewPeriod
}) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const query = useOverviewReport(period)
  const snapshot = query.data
  const unavailable =
    !query.isPending &&
    !query.error &&
    !Array.isArray(snapshot?.siteAvailability)
  const range = snapshot?.window
  const sites = [...(snapshot?.siteAvailability ?? [])]
    .sort(
      (a, b) =>
        b.failedCount - a.failedCount ||
        b.totalRequests - a.totalRequests ||
        a.siteName.localeCompare(b.siteName)
    )
    .slice(0, 8)
  const showSamplesColumn = sites.some(
    (site) => site.recentSamples !== undefined
  )

  return (
    <Card className='h-full min-w-0'>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          <Globe className='text-muted-foreground size-4' />
          {t('dashboard.overviewInsights.upstreamTitle')}
        </CardTitle>
        <CardDescription className='text-xs'>
          {t('dashboard.overviewInsights.upstreamDescription')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <QueryErrorBanner
          error={query.error}
          messageKey='dashboard.overviewInsights.loadError'
          onRetry={() => void query.refetch()}
          isRetrying={query.isFetching}
        />
        {query.isPending && <Skeleton className='h-32 w-full' />}
        {unavailable && (
          <p role='status' className='text-muted-foreground py-3 text-sm'>
            {t('dashboard.overviewInsights.unavailable')}
          </p>
        )}
        {!query.error &&
          !unavailable &&
          !query.isPending &&
          (sites.length === 0 ? (
            <p className='text-muted-foreground py-3 text-sm'>
              {t('dashboard.overviewInsights.upstreamEmpty')}
            </p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className='pl-0 text-xs'>
                    {t('dashboard.overviewInsights.site')}
                  </TableHead>
                  {showSamplesColumn && (
                    <TableHead className='text-xs'>
                      {t('dashboard.overviewInsights.recentSamples')}
                    </TableHead>
                  )}
                  <TableHead className='text-right text-xs'>
                    {t('dashboard.overviewInsights.calls')}
                  </TableHead>
                  <TableHead className='text-right text-xs'>
                    {t('dashboard.overviewInsights.failed')}
                  </TableHead>
                  <TableHead className='text-right text-xs'>
                    {t('dashboard.overviewInsights.successRate')}
                  </TableHead>
                  <TableHead className='text-right text-xs'>
                    {t('dashboard.overviewInsights.latency')}
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {sites.map((site) => {
                  const avgLatency =
                    site.totalRequests > 0 && site.averageLatencyMs != null
                      ? `${formatInt(Math.round(site.averageLatencyMs), locale)} ms`
                      : '—'
                  const avgFirstByte =
                    site.totalRequests > 0 && site.averageFirstByteMs != null
                      ? formatLatency(site.averageFirstByteMs, {
                          autoSeconds: true,
                          secondsDigits: 1,
                          spaced: true,
                        })
                      : null
                  return (
                    <TableRow key={site.siteId}>
                      <TableCell className='max-w-48 truncate pl-0 font-medium'>
                        {range ? (
                          <Link
                            to='/proxy-logs'
                            search={{ siteId: site.siteId, ...range }}
                            className='hover:text-primary hover:underline'
                          >
                            {site.siteName}
                          </Link>
                        ) : (
                          site.siteName
                        )}
                      </TableCell>
                      {showSamplesColumn && (
                        <TableCell>
                          <SiteSamplesStrip
                            samples={site.recentSamples}
                            siteName={site.siteName}
                            locale={locale}
                          />
                        </TableCell>
                      )}
                      <TableCell className='text-right tabular-nums'>
                        {formatInt(site.totalRequests, locale)}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {range && site.failedCount > 0 ? (
                          <Link
                            to='/proxy-logs'
                            search={{
                              siteId: site.siteId,
                              status: 'failed',
                              ...range,
                            }}
                            aria-label={t(
                              'dashboard.overviewInsights.failedLink',
                              { name: site.siteName, count: site.failedCount }
                            )}
                            className='text-destructive hover:underline'
                          >
                            {formatInt(site.failedCount, locale)}
                          </Link>
                        ) : (
                          formatInt(site.failedCount, locale)
                        )}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {formatRatio(site.successCount, site.totalRequests)}
                      </TableCell>
                      <TableCell className='text-muted-foreground text-right tabular-nums'>
                        <div className='flex flex-col items-end gap-0.5'>
                          <span>{avgLatency}</span>
                          {avgFirstByte && (
                            <span className='text-2xs'>
                              {t('dashboard.overviewInsights.firstByte', {
                                value: avgFirstByte,
                              })}
                            </span>
                          )}
                        </div>
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          ))}
      </CardContent>
    </Card>
  )
}
