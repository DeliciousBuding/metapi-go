import { useQuery } from '@tanstack/react-query'
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
import { api } from '@/lib/api'
import { formatInt, formatRatio } from '@/lib/format'

type Site = {
  siteId: number
  siteName: string
  totalRequests: number
  successCount: number
  failedCount: number
  averageLatencyMs: number | null
}
type Insights = {
  generatedAt?: string
  siteAvailability?: Site[] | null
  dashboardStatus?: { status: string; failed?: string[] }
}

export function UpstreamHealthPanel() {
  const { t, i18n } = useTranslation()
  const query = useQuery({
    queryKey: ['dashboard', 'overview-insights'],
    queryFn: () =>
      api.getDashboardSnapshot({ view: 'insights' }) as Promise<Insights>,
    refetchInterval: 60_000,
  })
  const snapshot = query.data
  const unavailable =
    !query.isPending &&
    !query.error &&
    (!Array.isArray(snapshot?.siteAvailability) ||
      snapshot?.dashboardStatus?.failed?.includes('siteAvailability'))
  const timestamp = snapshot?.generatedAt
    ? Date.parse(snapshot.generatedAt)
    : Number.NaN
  const range = Number.isFinite(timestamp)
    ? {
        from: new Date(timestamp - 86_400_000).toISOString(),
        to: new Date(timestamp).toISOString(),
      }
    : null
  const sites = [...(snapshot?.siteAvailability ?? [])]
    .sort(
      (a, b) =>
        b.failedCount - a.failedCount ||
        b.totalRequests - a.totalRequests ||
        a.siteName.localeCompare(b.siteName)
    )
    .slice(0, 8)
  return (
    <Card size='sm' className='h-full min-w-0'>
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
        {!query.error && !unavailable && !query.isPending && (
          <>
            {snapshot?.dashboardStatus?.status === 'partial' && (
              <p role='status' className='text-muted-foreground mb-2 text-xs'>
                {t('dashboard.overviewInsights.partial')}
              </p>
            )}
            {sites.length === 0 ? (
              <p className='text-muted-foreground py-3 text-sm'>
                {t('dashboard.overviewInsights.upstreamEmpty')}
              </p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    {['site', 'calls', 'failed', 'successRate', 'latency'].map(
                      (key) => (
                        <TableHead
                          key={key}
                          className={
                            key === 'site'
                              ? 'pl-0 text-xs'
                              : 'text-right text-xs'
                          }
                        >
                          {t(`dashboard.overviewInsights.${key}`)}
                        </TableHead>
                      )
                    )}
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sites.map((site) => (
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
                      <TableCell className='text-right tabular-nums'>
                        {formatInt(site.totalRequests, i18n.language)}
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
                            {formatInt(site.failedCount, i18n.language)}
                          </Link>
                        ) : (
                          formatInt(site.failedCount, i18n.language)
                        )}
                      </TableCell>
                      <TableCell className='text-right tabular-nums'>
                        {formatRatio(site.successCount, site.totalRequests)}
                      </TableCell>
                      <TableCell className='text-muted-foreground text-right tabular-nums'>
                        {site.totalRequests > 0 && site.averageLatencyMs != null
                          ? `${formatInt(Math.round(site.averageLatencyMs), i18n.language)} ms`
                          : '—'}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </>
        )}
      </CardContent>
    </Card>
  )
}
