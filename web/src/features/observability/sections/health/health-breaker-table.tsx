import { Link } from '@tanstack/react-router'
import { CheckCircle2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { Badge } from '@/components/ui/badge'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatRemainingCooldown } from '@/hooks/use-cooldown-clock'
import { formatDateTime, formatInt } from '@/lib/format'

import type { RuntimeHealthBreaker } from '../../types'
import { BreakerActions } from './health-inspection-actions'

export function HealthBreakerTable({
  loading,
  breakers,
  locale,
  now,
}: {
  loading: boolean
  breakers: RuntimeHealthBreaker[]
  locale: string
  now: number
}) {
  const { t } = useTranslation()
  if (loading) {
    return <Skeleton className='h-40 w-full rounded-md' />
  }
  if (breakers.length === 0) {
    return (
      <Empty className='min-h-24 border'>
        <EmptyHeader>
          <EmptyMedia variant='icon' className='bg-success/10 text-success'>
            <CheckCircle2 />
          </EmptyMedia>
          <EmptyDescription>
            {t('observability.health.breakers.empty')}
          </EmptyDescription>
        </EmptyHeader>
      </Empty>
    )
  }
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>{t('observability.health.breakers.colSite')}</TableHead>
          <TableHead>{t('observability.health.breakers.colModel')}</TableHead>
          <TableHead className='text-right'>
            {t('observability.health.breakers.colLevel')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.health.breakers.colPenalty')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.health.breakers.colUntil')}
          </TableHead>
          <TableHead className='text-right'>
            {t('observability.health.breakers.colActions')}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {breakers.map((breaker) => {
          const remaining = formatRemainingCooldown(breaker.breakerUntilMs, now)
          const siteName = breaker.siteName?.trim()
          return (
            <TableRow key={`${breaker.siteId}:${breaker.model}`}>
              <TableCell className='max-w-44 tabular-nums'>
                <Link
                  to='/sites'
                  search={{ edit: breaker.siteId }}
                  className='text-primary inline-flex max-w-full items-baseline gap-1.5 font-medium hover:underline'
                  title={siteName || `#${breaker.siteId}`}
                >
                  {siteName ? (
                    <>
                      <span className='truncate'>{siteName}</span>
                      <span className='text-muted-foreground text-2xs shrink-0 font-mono font-normal'>
                        #{breaker.siteId}
                      </span>
                    </>
                  ) : (
                    `#${breaker.siteId}`
                  )}
                </Link>
              </TableCell>
              <TableCell className='truncate'>
                {breaker.model ? <ModelPill model={breaker.model} /> : '—'}
              </TableCell>
              <TableCell
                className='text-right tabular-nums'
                title={t('observability.health.breakers.levelTooltip', {
                  level: breaker.breakerLevel,
                  score: breaker.penaltyScore,
                })}
              >
                <Badge variant='warning'>L{breaker.breakerLevel}</Badge>
              </TableCell>
              <TableCell className='text-right tabular-nums'>
                {formatInt(breaker.penaltyScore, locale)}
              </TableCell>
              <TableCell className='text-right tabular-nums'>
                <div className='flex flex-col items-end gap-0.5'>
                  <span>{formatDateTime(breaker.breakerUntilMs, locale)}</span>
                  {remaining && (
                    <span className='text-warning-soft-fg text-2xs'>
                      {t('observability.health.cooldown.remaining', {
                        remaining,
                      })}
                    </span>
                  )}
                </div>
              </TableCell>
              <TableCell className='text-right'>
                <BreakerActions siteId={breaker.siteId} model={breaker.model} />
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
