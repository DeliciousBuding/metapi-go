import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'

import { QuietRefreshStatus } from '@/components/common/quiet-refresh-status'
import {
  SoftToneBadge,
  type SoftTone,
} from '@/components/common/soft-tone-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'

import { userPortalApi } from './api'
import { PortalOnboarding } from './components/portal-onboarding'

function budgetHealthTone(budget: {
  unlimited: boolean
  frozen: boolean
  limit: number
  used: number
  reserved: number
}): SoftTone {
  if (budget.frozen) return 'warning'
  if (budget.unlimited) return 'info'
  const remaining = Math.max(0, budget.limit - budget.used - budget.reserved)
  if (remaining <= 0) return 'destructive'
  if (budget.limit > 0 && remaining / budget.limit <= 0.15) return 'warning'
  return 'success'
}

export function PortalOverview({ userId }: { userId: number }) {
  const { t, i18n } = useTranslation()
  const budget = useQuery({
    queryKey: ['userPortal', userId, 'budget'],
    queryFn: userPortalApi.budget,
  })
  const usage = useQuery({
    queryKey: ['userPortal', userId, 'usage'],
    queryFn: userPortalApi.usage,
  })
  const number = (value: number) =>
    new Intl.NumberFormat(i18n.language, { maximumFractionDigits: 6 }).format(
      value
    )
  const updatedAt = Math.max(
    budget.dataUpdatedAt ?? 0,
    usage.dataUpdatedAt ?? 0
  )
  const hasRefreshError = Boolean(budget.isError || usage.isError)

  function renderBudgetSection() {
    if (budget.isPending) {
      return <Spinner aria-label={t('common.loading')} />
    }
    if (budget.isError) {
      return (
        <Notice tone='destructive' role='alert'>
          <span>{t('userPortal.loadFailed')}</span>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void budget.refetch()}
          >
            {t('common.retry')}
          </Button>
        </Notice>
      )
    }
    const remainingNum = Math.max(
      0,
      budget.data.limit - budget.data.used - budget.data.reserved
    )
    const remainingValue = budget.data.unlimited
      ? t('userPortal.unlimited')
      : number(remainingNum)
    const healthTone = budgetHealthTone(budget.data)
    const remainingPct =
      !budget.data.unlimited && budget.data.limit > 0
        ? Math.min(100, Math.round((remainingNum / budget.data.limit) * 100))
        : null
    let remainingBadgeLabel: string | null = null
    if (budget.data.unlimited) {
      remainingBadgeLabel = t('userPortal.unlimited')
    } else if (remainingPct !== null) {
      remainingBadgeLabel = `${remainingPct}%`
    }
    const metrics = [
      { key: 'remaining', value: remainingValue, emphasize: true },
      { key: 'used', value: number(budget.data.used), emphasize: false },
      {
        key: 'reserved',
        value: number(budget.data.reserved),
        emphasize: false,
      },
    ] as const
    return (
      <>
        {budget.data.frozen && (
          <Notice tone='warning'>
            {t('userPortal.frozenBudget', {
              count: budget.data.unknownRequests,
            })}
          </Notice>
        )}
        <div className='grid gap-3 sm:grid-cols-3'>
          {metrics.map(({ key, value, emphasize }) => (
            <Card key={key} size='sm'>
              <CardContent className='space-y-1.5'>
                <div className='flex flex-wrap items-center justify-between gap-2'>
                  <h2 className='text-muted-foreground text-xs font-medium'>
                    {t('userPortal.' + key)}
                  </h2>
                  {emphasize && remainingBadgeLabel && (
                    <SoftToneBadge tone={healthTone}>
                      {remainingBadgeLabel}
                    </SoftToneBadge>
                  )}
                </div>
                <p className='text-2xl font-semibold tracking-tight tabular-nums'>
                  {value}
                </p>
              </CardContent>
            </Card>
          ))}
        </div>
        <p className='text-muted-foreground text-xs leading-relaxed'>
          {t('userPortal.budgetHint')}
        </p>
      </>
    )
  }

  function renderUsageSection() {
    if (usage.isError) {
      return (
        <Notice tone='destructive' role='alert'>
          <span>{t('userPortal.loadFailed')}</span>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void usage.refetch()}
          >
            {t('common.retry')}
          </Button>
        </Notice>
      )
    }
    if (usage.isPending) {
      return <Spinner aria-label={t('common.loading')} />
    }
    return (
      <dl className='bg-card grid divide-y rounded-xl border sm:grid-cols-2 sm:divide-x sm:divide-y-0'>
        <div className='min-w-0 p-4'>
          <dt className='text-muted-foreground text-xs font-medium'>
            {t('userPortal.keyCount')}
          </dt>
          <dd className='mt-1 text-xl font-semibold tabular-nums'>
            {usage.data.keyCount.toLocaleString(i18n.language)}
          </dd>
        </div>
        <div className='min-w-0 p-4'>
          <dt className='text-muted-foreground text-xs font-medium'>
            {t('userPortal.recordedRequests')}
          </dt>
          <dd className='mt-1 text-xl font-semibold tabular-nums'>
            {usage.data.recordedRequests === undefined
              ? t('userPortal.unknown')
              : usage.data.recordedRequests.toLocaleString(i18n.language)}
          </dd>
          <dd className='text-muted-foreground mt-1 text-xs leading-relaxed'>
            {t('userPortal.retainedRequestsHint')}
          </dd>
        </div>
      </dl>
    )
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-end gap-2'>
        <QuietRefreshStatus updatedAt={updatedAt} hasError={hasRefreshError} />
      </div>
      <PortalOnboarding userId={userId} />
      {renderBudgetSection()}
      {renderUsageSection()}
      <div className='flex flex-wrap gap-2'>
        <Button
          render={<Link to='/user' search={{ view: 'models', page: 1 }} />}
        >
          {t('userPortal.browseModels')}
        </Button>
        <Button
          variant='outline'
          render={<Link to='/user' search={{ view: 'keys', page: 1 }} />}
        >
          {t('userPortal.createKey')}
        </Button>
      </div>
    </div>
  )
}
