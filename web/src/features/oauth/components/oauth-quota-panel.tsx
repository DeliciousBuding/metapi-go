// metapi-go/features/oauth/components — quota section of the OAuth detail
// sheet.
//
// Split out of `oauth-detail-sheet.tsx` because the quota contract is the
// densest part of the payload: two independent windows × four numbers, plus
// snapshot-level status/source/subscription metadata. The list column can
// only afford a compact `used/limit` pair, so this panel is where
// `remaining` and `resetAt` finally surface.
//
// Honesty rules (see `../lib/oauth-quota.ts`): an unsupported provider, an
// unsupported window, a failed sync or an all-null window renders an
// explicit sentence — never a `0`, and never a green "all clear".

import { TriangleAlert as TriangleAlertIcon } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { DetailField } from '@/components/common/detail-field'
import { SoftToneBadge } from '@/components/common/soft-tone-badge'
import { Badge } from '@/components/ui/badge'
import { Notice } from '@/components/ui/notice'
import {
  formatRemainingCooldown,
  useCooldownClock,
} from '@/hooks/use-cooldown-clock'
import { toBcp47 } from '@/i18n/languages'
import { EM_DASH, formatDateTime, formatInt } from '@/lib/format'
import { cn } from '@/lib/utils'

import {
  classifyOAuthQuotaSeverity,
  hasOAuthSubscriptionDetails,
  resolveOAuthQuotaAvailability,
  resolveOAuthQuotaUsageRatio,
  resolveOAuthQuotaWindowState,
  resolveOAuthSubscriptionExpiryState,
  type OAuthQuotaSeverity,
  type OAuthQuotaSnapshot,
  type OAuthQuotaWindow,
} from '../lib/oauth-quota'

const QUOTA_SOURCE_LABEL_KEY: Record<OAuthQuotaSnapshot['source'], string> = {
  official: 'oauth.detail.quotaSourceOfficial',
  reverse_engineered: 'oauth.detail.quotaSourceReverseEngineered',
}

const QUOTA_UNAVAILABLE_MESSAGE_KEY = {
  missing: 'oauth.detail.quotaMissing',
  unsupported: 'oauth.detail.quotaUnsupported',
  error: 'oauth.detail.quotaError',
} as const

const QUOTA_METER_COLOR: Record<OAuthQuotaSeverity, string> = {
  ok: 'bg-success',
  high: 'bg-warning',
  exhausted: 'bg-destructive',
}

/**
 * One quota window (5-hour or 7-day). `reported` windows show the four
 * numbers the contract carries plus a proportional utilization bar and live
 * reset countdown (driven by the parent's shared `useCooldownClock`); the
 * other two states replace the numbers with an explicit sentence so a null is
 * never mistaken for a zero.
 */
function QuotaWindowBlock(props: {
  titleKey: string
  window: OAuthQuotaWindow | null | undefined
  locale: string
  nowMs: number
}) {
  const { t } = useTranslation()
  const windowState = resolveOAuthQuotaWindowState(props.window)
  const providerMessage = props.window?.message?.trim() || null
  const ratio = resolveOAuthQuotaUsageRatio(props.window)
  const severity = ratio !== null ? classifyOAuthQuotaSeverity(ratio) : null
  const resetCountdown =
    windowState === 'reported'
      ? formatRemainingCooldown(props.window?.resetAt, props.nowMs)
      : null

  return (
    <div className='bg-muted/30 space-y-2 rounded-lg border p-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <span className='text-muted-foreground text-xs font-medium'>
          {t(props.titleKey)}
        </span>
        <div className='flex flex-wrap items-center gap-1.5'>
          {severity === 'exhausted' && (
            <SoftToneBadge tone='destructive'>
              {t('oauth.detail.quotaExhausted')}
            </SoftToneBadge>
          )}
          {severity === 'high' && (
            <SoftToneBadge tone='warning'>
              {t('oauth.detail.quotaHighUsage')}
            </SoftToneBadge>
          )}
          {resetCountdown && (
            <SoftToneBadge tone='info'>
              {t('oauth.detail.quotaResetsIn', { duration: resetCountdown })}
            </SoftToneBadge>
          )}
        </div>
      </div>

      {windowState === 'reported' && ratio !== null && severity !== null && (
        <div
          className='bg-muted h-1.5 w-full overflow-hidden rounded-full'
          role='progressbar'
          aria-label={t(props.titleKey)}
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.round(ratio * 100)}
        >
          <div
            className={cn(
              'h-full rounded-full transition-all',
              QUOTA_METER_COLOR[severity]
            )}
            style={{ width: `${Math.round(ratio * 100)}%` }}
          />
        </div>
      )}

      {windowState === 'reported' ? (
        <dl className='grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
          <DetailField label={t('oauth.detail.quotaUsed')}>
            <span className='tabular-nums'>
              {formatInt(props.window?.used, props.locale)}
            </span>
          </DetailField>
          <DetailField label={t('oauth.detail.quotaLimit')}>
            <span className='tabular-nums'>
              {formatInt(props.window?.limit, props.locale)}
            </span>
          </DetailField>
          <DetailField label={t('oauth.detail.quotaRemaining')}>
            <span className='tabular-nums'>
              {formatInt(props.window?.remaining, props.locale)}
            </span>
          </DetailField>
          <DetailField label={t('oauth.detail.quotaResetAt')}>
            <span className='tabular-nums'>
              {formatDateTime(props.window?.resetAt, props.locale)}
            </span>
          </DetailField>
        </dl>
      ) : (
        <p className='text-muted-foreground text-xs'>
          {t(
            windowState === 'unsupported'
              ? 'oauth.detail.windowUnsupported'
              : 'oauth.detail.windowNoData'
          )}
        </p>
      )}
      {providerMessage && (
        <p className='text-muted-foreground text-xs break-words'>
          {providerMessage}
        </p>
      )}
    </div>
  )
}

export function OAuthQuotaPanel(props: {
  quota: OAuthQuotaSnapshot | null | undefined
}) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  const availability = resolveOAuthQuotaAvailability(props.quota)
  const quota = props.quota
  const providerMessage = quota?.providerMessage?.trim() || null
  const lastError = quota?.lastError?.trim() || null
  // A source the frontend does not know yet must fall back to the raw wire
  // value rather than an empty translation lookup.
  const sourceLabelKey = quota ? QUOTA_SOURCE_LABEL_KEY[quota.source] : null

  const deadlines = useMemo(
    () => [
      quota?.windows?.fiveHour?.resetAt,
      quota?.windows?.sevenDay?.resetAt,
    ],
    [quota?.windows?.fiveHour?.resetAt, quota?.windows?.sevenDay?.resetAt]
  )
  const nowMs = useCooldownClock(deadlines)
  const expiryState = resolveOAuthSubscriptionExpiryState(
    quota?.subscription?.activeUntil,
    nowMs
  )

  return (
    <section>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h3 className='text-sm font-medium'>
          {t('oauth.detail.sectionQuota')}
        </h3>
        {quota && (
          <Badge variant='outline'>
            {sourceLabelKey ? t(sourceLabelKey) : quota.source}
          </Badge>
        )}
      </div>

      {availability !== 'reported' && (
        <Notice tone='warning' size='compact' className='mt-2'>
          <TriangleAlertIcon aria-hidden='true' className='mt-0.5 size-3.5' />
          <div className='min-w-0'>
            <p className='break-words'>
              {t(QUOTA_UNAVAILABLE_MESSAGE_KEY[availability])}
            </p>
            {lastError && <p className='mt-1 break-words'>{lastError}</p>}
          </div>
        </Notice>
      )}

      {quota && (
        <>
          <dl className='mt-2.5 grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
            <DetailField label={t('oauth.detail.quotaLastSync')}>
              <span className='tabular-nums'>
                {formatDateTime(quota.lastSyncAt, locale)}
              </span>
            </DetailField>
            <DetailField label={t('oauth.detail.lastLimitResetAt')}>
              <span className='tabular-nums'>
                {formatDateTime(quota.lastLimitResetAt, locale)}
              </span>
            </DetailField>
          </dl>

          {providerMessage && (
            <p className='text-muted-foreground mt-2 text-xs break-words'>
              {providerMessage}
            </p>
          )}

          <div className='mt-2.5 flex flex-col gap-2.5'>
            <QuotaWindowBlock
              titleKey='oauth.detail.windowFiveHour'
              window={quota.windows?.fiveHour}
              locale={locale}
              nowMs={nowMs}
            />
            <QuotaWindowBlock
              titleKey='oauth.detail.windowSevenDay'
              window={quota.windows?.sevenDay}
              locale={locale}
              nowMs={nowMs}
            />
          </div>

          {hasOAuthSubscriptionDetails(quota) && (
            <dl className='mt-3 grid grid-cols-2 gap-x-3 gap-y-2 border-t pt-3 text-sm'>
              <DetailField label={t('oauth.detail.subscriptionPlanType')}>
                {quota.subscription?.planType?.trim() || EM_DASH}
              </DetailField>
              <DetailField label={t('oauth.detail.subscriptionActiveStart')}>
                <span className='tabular-nums'>
                  {formatDateTime(quota.subscription?.activeStart, locale)}
                </span>
              </DetailField>
              <DetailField
                label={t('oauth.detail.subscriptionActiveUntil')}
                full
              >
                <span className='inline-flex flex-wrap items-center gap-2 tabular-nums'>
                  <span>
                    {formatDateTime(quota.subscription?.activeUntil, locale)}
                  </span>
                  {expiryState === 'expired' && (
                    <SoftToneBadge tone='destructive'>
                      {t('oauth.detail.subscriptionExpired')}
                    </SoftToneBadge>
                  )}
                  {expiryState === 'expiringSoon' && (
                    <SoftToneBadge tone='warning'>
                      {t('oauth.detail.subscriptionExpiringSoon')}
                    </SoftToneBadge>
                  )}
                </span>
              </DetailField>
            </dl>
          )}
        </>
      )}
    </section>
  )
}
