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
import { useTranslation } from 'react-i18next'

import { DetailField } from '@/components/common/detail-field'
import { Badge } from '@/components/ui/badge'
import { Notice } from '@/components/ui/notice'
import { toBcp47 } from '@/i18n/languages'
import { EM_DASH, formatDateTime, formatInt } from '@/lib/format'

import {
  hasOAuthSubscriptionDetails,
  resolveOAuthQuotaAvailability,
  resolveOAuthQuotaWindowState,
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

/**
 * One quota window (5-hour or 7-day). `reported` windows show the four
 * numbers the contract carries; the other two states replace the numbers
 * with a sentence so a null is never mistaken for a zero.
 */
function QuotaWindowBlock(props: {
  titleKey: string
  window: OAuthQuotaWindow | null | undefined
  locale: string
}) {
  const { t } = useTranslation()
  const windowState = resolveOAuthQuotaWindowState(props.window)
  const providerMessage = props.window?.message?.trim() || null

  return (
    <div className='bg-muted/40 rounded-lg border p-2'>
      <div className='text-muted-foreground text-[11px]'>
        {t(props.titleKey)}
      </div>
      {windowState === 'reported' ? (
        <dl className='mt-1 grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
          <DetailField label={t('oauth.detail.quotaUsed')}>
            <span className='tabular-nums'>
              {formatInt(props.window?.used)}
            </span>
          </DetailField>
          <DetailField label={t('oauth.detail.quotaLimit')}>
            <span className='tabular-nums'>
              {formatInt(props.window?.limit)}
            </span>
          </DetailField>
          <DetailField label={t('oauth.detail.quotaRemaining')}>
            <span className='tabular-nums'>
              {formatInt(props.window?.remaining)}
            </span>
          </DetailField>
          <DetailField label={t('oauth.detail.quotaResetAt')}>
            {formatDateTime(props.window?.resetAt, props.locale)}
          </DetailField>
        </dl>
      ) : (
        <p className='text-muted-foreground mt-1 text-xs'>
          {t(
            windowState === 'unsupported'
              ? 'oauth.detail.windowUnsupported'
              : 'oauth.detail.windowNoData'
          )}
        </p>
      )}
      {providerMessage && (
        <p className='text-muted-foreground mt-1 text-xs break-words'>
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

  return (
    <section>
      <h3 className='text-sm font-medium'>{t('oauth.detail.sectionQuota')}</h3>

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
          <dl className='mt-2 grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
            <DetailField label={t('oauth.detail.quotaSource')}>
              <Badge variant='outline'>
                {sourceLabelKey ? t(sourceLabelKey) : quota.source}
              </Badge>
            </DetailField>
            <DetailField label={t('oauth.detail.quotaLastSync')}>
              {formatDateTime(quota.lastSyncAt, locale)}
            </DetailField>
            <DetailField label={t('oauth.detail.lastLimitResetAt')} full>
              {formatDateTime(quota.lastLimitResetAt, locale)}
            </DetailField>
          </dl>

          {providerMessage && (
            <p className='text-muted-foreground mt-2 text-xs break-words'>
              {providerMessage}
            </p>
          )}

          <div className='mt-2 flex flex-col gap-2'>
            <QuotaWindowBlock
              titleKey='oauth.detail.windowFiveHour'
              window={quota.windows?.fiveHour}
              locale={locale}
            />
            <QuotaWindowBlock
              titleKey='oauth.detail.windowSevenDay'
              window={quota.windows?.sevenDay}
              locale={locale}
            />
          </div>

          {hasOAuthSubscriptionDetails(quota) && (
            <dl className='mt-2 grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
              <DetailField label={t('oauth.detail.subscriptionPlanType')}>
                {quota.subscription?.planType?.trim() || EM_DASH}
              </DetailField>
              <DetailField label={t('oauth.detail.subscriptionActiveStart')}>
                {formatDateTime(quota.subscription?.activeStart, locale)}
              </DetailField>
              <DetailField label={t('oauth.detail.subscriptionActiveUntil')}>
                {formatDateTime(quota.subscription?.activeUntil, locale)}
              </DetailField>
            </dl>
          )}
        </>
      )}
    </section>
  )
}
