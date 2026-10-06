import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ChartNoAxesCombined } from 'lucide-react'
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
import { api } from '@/lib/api'
import { formatCurrency, formatInt } from '@/lib/format'

export function ModelUsagePanel() {
  const { t, i18n } = useTranslation()
  const query = useQuery({
    queryKey: ['dashboard', 'overview-model-cost'],
    queryFn: () => api.getModelCostDistribution(7, 5),
    refetchInterval: 60_000,
  })
  const data = query.data
  const unavailable =
    !query.isPending &&
    !query.error &&
    (!Array.isArray(data?.items) || !data?.totals)
  return (
    <Card size='sm' className='h-full min-w-0'>
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
                  formatCurrency(data.totals.cost, { locale: i18n.language }),
                ],
                ['calls', formatInt(data.totals.calls, i18n.language)],
                ['tokens', formatInt(data.totals.tokens, i18n.language)],
              ].map(([key, value]) => (
                <div key={key}>
                  <dt className='text-muted-foreground text-xs'>
                    {t(`dashboard.overviewInsights.${key}`)}
                  </dt>
                  <dd className='mt-1 font-semibold tabular-nums'>{value}</dd>
                </div>
              ))}
            </dl>
            {data.items.length === 0 && (
              <p className='text-muted-foreground py-3 text-sm'>
                {t('dashboard.overviewInsights.modelEmpty')}
              </p>
            )}
            <ul className='divide-border divide-y'>
              {data.items.map((item) => (
                <li
                  key={item.model}
                  className='flex items-center gap-3 py-2 text-xs'
                >
                  <span
                    className='min-w-0 flex-1 truncate'
                    title={item.label || item.model}
                  >
                    {item.model && item.model !== 'other' ? (
                      <Link
                        to='/proxy-logs'
                        search={{ q: item.model, from: data.since }}
                        className='hover:text-primary font-medium hover:underline'
                      >
                        {item.label || item.model}
                      </Link>
                    ) : (
                      <span>
                        {item.model === 'other'
                          ? t('dashboard.overviewInsights.otherModels')
                          : item.label}
                      </span>
                    )}
                  </span>
                  <span className='text-muted-foreground shrink-0 tabular-nums'>
                    {t('dashboard.overviewInsights.callCount', {
                      count: item.calls,
                      value: formatInt(item.calls, i18n.language),
                    })}
                  </span>
                  <span className='w-20 shrink-0 text-right font-medium tabular-nums'>
                    {formatCurrency(item.cost, {
                      locale: i18n.language,
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
