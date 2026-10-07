import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { ArrowUpRight, Bell } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { api } from '@/lib/api'
import { attentionLabel, type AttentionResponse } from '@/lib/attention-label'

import { resolveAttentionTarget } from '../../availability/attention-target'

const severityVariant = {
  critical: 'destructive',
  warning: 'warning',
  info: 'info',
} as const

export function AttentionPanel() {
  const { t } = useTranslation()
  const query = useQuery({
    queryKey: ['dashboard', 'overview-attention'],
    queryFn: () => api.getAttention(6) as Promise<AttentionResponse>,
    refetchInterval: 60_000,
  })
  const unavailable =
    !query.isPending && !query.error && !Array.isArray(query.data?.items)
  return (
    <Card className='h-full'>
      <CardHeader>
        <CardTitle className='flex items-center gap-2'>
          <Bell className='text-muted-foreground size-4' />
          {t('dashboard.overviewInsights.attentionTitle')}
          {query.data?.total != null && (
            <Badge variant='secondary' className='ml-auto tabular-nums'>
              {query.data.total}
            </Badge>
          )}
        </CardTitle>
        <CardDescription className='text-xs'>
          {t('dashboard.overviewInsights.attentionDescription')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <QueryErrorBanner
          error={query.error}
          messageKey='dashboard.overviewInsights.loadError'
          onRetry={() => void query.refetch()}
          isRetrying={query.isFetching}
        />
        {query.isPending && <Skeleton className='h-24 w-full' />}
        {unavailable && (
          <p role='status' className='text-muted-foreground text-sm'>
            {t('dashboard.overviewInsights.unavailable')}
          </p>
        )}
        {!query.error && query.data?.items?.length === 0 && (
          <p className='text-muted-foreground py-3 text-sm'>
            {t('dashboard.overviewInsights.attentionEmpty')}
          </p>
        )}
        {!query.error && (
          <ul className='divide-border divide-y'>
            {query.data?.items?.slice(0, 6).map((item) => {
              const target = resolveAttentionTarget(item.target)
              const label = attentionLabel(item, t)
              return (
                <li
                  key={`${item.category}-${item.target}-${item.createdAt}-${item.label}`}
                  className='flex items-start gap-2 py-2'
                >
                  <Badge
                    variant={severityVariant[item.severity]}
                    className='mt-0.5 shrink-0'
                  >
                    {t(`dashboard.overviewInsights.${item.severity}`)}
                  </Badge>
                  {target ? (
                    <Link
                      {...target}
                      className='hover:text-primary flex min-w-0 flex-1 items-start gap-1 text-sm leading-5'
                    >
                      <span className='min-w-0 break-words'>{label}</span>
                      <ArrowUpRight className='mt-1 ml-auto size-3 shrink-0' />
                    </Link>
                  ) : (
                    <span className='text-sm leading-5'>{label}</span>
                  )}
                </li>
              )
            })}
          </ul>
        )}
        <Link
          to='/dashboard/$section'
          params={{ section: 'availability' }}
          className='text-primary mt-2 inline-flex items-center gap-1 text-xs hover:underline'
        >
          {t('dashboard.overviewInsights.allAttention')}
          <ArrowUpRight className='size-3' />
        </Link>
      </CardContent>
    </Card>
  )
}
