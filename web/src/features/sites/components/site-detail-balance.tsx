import { useTranslation } from 'react-i18next'

import { DetailField } from '@/components/common/detail-field'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import {
  formatAbsoluteDateTime,
  formatCurrency,
  formatRelativeTime,
} from '@/lib/format'

import { resolveSiteBalanceUsd } from '../lib/site-balance'
import type { Site } from '../types'

/**
 * Renders only when the backend provided at least one displayable balance /
 * subscription value — a site without this data keeps the plain registry
 * view. The balance row prefers `totalBalance` and falls back to the
 * subscription summary's remaining USD (shared with the list column via
 * `resolveSiteBalanceUsd`); the separate "Remaining" row is suppressed in
 * the fallback case so the same number is never shown twice.
 */
export function SiteBalanceSection({
  site,
  locale,
}: {
  site: Site
  locale: string
}) {
  const { t } = useTranslation()
  const summary = site.subscriptionSummary ?? null
  const balanceUsd = resolveSiteBalanceUsd(site)
  const planNames = summary?.planNames ?? []
  const hasSubscriptionDetail =
    planNames.length > 0 ||
    typeof summary?.totalUsedUsd === 'number' ||
    typeof summary?.totalRemainingUsd === 'number' ||
    Boolean(summary?.nextExpiresAt)
  if (balanceUsd === null && !hasSubscriptionDetail) {
    return null
  }
  const showRemainingRow =
    typeof site.totalBalance === 'number' &&
    typeof summary?.totalRemainingUsd === 'number'
  return (
    <>
      <section>
        <h3 className='text-sm font-medium'>
          {t('sites.detail.balanceTitle')}
        </h3>
        <dl className='mt-2 grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
          {balanceUsd !== null && (
            <DetailField label={t('sites.detail.balance')}>
              <span className='tabular-nums'>{formatCurrency(balanceUsd)}</span>
            </DetailField>
          )}
          {planNames.length > 0 && (
            <DetailField label={t('sites.detail.plans')}>
              <span className='flex flex-wrap gap-1'>
                {planNames.map((planName) => (
                  <Badge key={planName} variant='outline'>
                    {planName}
                  </Badge>
                ))}
              </span>
            </DetailField>
          )}
          {typeof summary?.totalUsedUsd === 'number' && (
            <DetailField label={t('sites.detail.monthlyUsage')}>
              <span className='tabular-nums'>
                {formatCurrency(summary.totalUsedUsd)}
                {typeof summary.totalMonthlyLimitUsd === 'number'
                  ? ` / ${formatCurrency(summary.totalMonthlyLimitUsd)}`
                  : ''}
              </span>
            </DetailField>
          )}
          {showRemainingRow && (
            <DetailField label={t('sites.detail.remaining')}>
              <span className='tabular-nums'>
                {formatCurrency(summary?.totalRemainingUsd)}
              </span>
            </DetailField>
          )}
          {summary?.nextExpiresAt && (
            <DetailField
              label={t('sites.detail.nextExpires')}
              title={
                formatAbsoluteDateTime(summary.nextExpiresAt, locale) ||
                undefined
              }
            >
              {formatRelativeTime(summary.nextExpiresAt, locale)}
            </DetailField>
          )}
        </dl>
      </section>
      <Separator />
    </>
  )
}
