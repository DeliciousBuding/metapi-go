import { Link } from '@tanstack/react-router'
import { ChartNoAxesCombined } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { toBcp47 } from '@/i18n/languages'
import type { OverviewPeriod } from '@/lib/api'
import { formatCurrency, formatInt } from '@/lib/format'

import { useOverviewReport } from '../use-overview-report'

export function ModelUsagePanel({
  period = '7d',
}: {
  period?: OverviewPeriod
}) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const query = useOverviewReport(period)
  const data = query.data
  const unavailable =
    !query.isPending &&
    !query.error &&
    (!Array.isArray(data?.models) || !data?.summary)
  return (
    <Card className='h-full min-w-0'>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          <ChartNoAxesCombined className='text-muted-foreground size-4' />
          {t('dashboard.overviewInsights.modelTitle')}
        </CardTitle>
        <CardDescription className='text-xs'>
          {t('dashboard.overviewInsights.modelDescription')}
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
          <p role='status' className='text-muted-foreground text-sm'>
            {t('dashboard.overviewInsights.unavailable')}
          </p>
        )}
        {!query.error && !unavailable && data && (
          <>
            <dl className='border-border mb-2 grid grid-cols-3 gap-3 border-b pb-3'>
              {[
                [
                  'estimatedCost',
                  formatCurrency(data.summary.totalCost, { locale }),
                ],
                ['calls', formatInt(data.summary.totalCount, locale)],
                ['tokens', formatInt(data.summary.totalTokensAll, locale)],
              ].map(([key, value]) => (
                <div key={key}>
                  <dt className='text-muted-foreground text-xs'>
                    {t(`dashboard.overviewInsights.${key}`)}
                  </dt>
                  <dd className='mt-1 font-semibold tabular-nums'>{value}</dd>
                </div>
              ))}
            </dl>
            {data.models.length === 0 && (
              <p className='text-muted-foreground py-3 text-sm'>
                {t('dashboard.overviewInsights.modelEmpty')}
              </p>
            )}
            <ul className='divide-border divide-y'>
              {data.models.map((item) => (
                <li
                  key={item.model}
                  className='flex items-center gap-3 py-2.5 text-sm'
                >
                  <span className='min-w-0 flex-1 truncate' title={item.model}>
                    {item.model && item.model !== 'other' ? (
                      <Link
                        to='/proxy-logs'
                        search={{ q: item.model, ...data.window }}
                        className='hover:text-primary hover:underline'
                      >
                        <ModelPill model={item.model} variant='inline' />
                      </Link>
                    ) : (
                      <span>
                        {item.model === 'other'
                          ? t('dashboard.overviewInsights.otherModels')
                          : item.model}
                      </span>
                    )}
                  </span>
                  <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
                    {t('dashboard.overviewInsights.callCount', {
                      count: item.calls,
                      value: formatInt(item.calls, locale),
                    })}
                  </span>
                  <span className='w-20 shrink-0 text-right font-medium tabular-nums'>
                    {formatCurrency(item.cost, {
                      locale,
                      fractionDigits: 4,
                    })}
                  </span>
                </li>
              ))}
            </ul>
          </>
        )}
      </CardContent>
    </Card>
  )
}
