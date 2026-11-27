import { useTranslation } from 'react-i18next'

import {
  SoftToneBadge,
  type SoftTone,
} from '@/components/common/soft-tone-badge'
import { Notice } from '@/components/ui/notice'
import { toBcp47 } from '@/i18n/languages'

import type { PersonalRequest } from '../api'
import { CopyValue } from './portal-keys'
import {
  RequestStatusBadge,
  isKnownRequestStatus,
} from './portal-request-status'

function quotaStateTone(
  state: 'pending' | 'settled' | 'released' | 'unknown'
): SoftTone {
  switch (state) {
    case 'settled':
      return 'success'
    case 'pending':
      return 'info'
    case 'released':
      return 'neutral'
    case 'unknown':
      return 'warning'
  }
}

export function RequestDetail({ item }: { item: PersonalRequest }) {
  const { t } = useTranslation()
  return (
    <dl className='grid gap-5 py-4 text-sm'>
      <div>
        <dt className='text-muted-foreground text-xs'>
          {t('userPortal.requestId')}
        </dt>
        <dd className='mt-1 space-y-2'>
          <p className='font-mono wrap-anywhere'>{item.id}</p>
          <CopyValue value={item.id} label={t('userPortal.copyRequestId')} />
        </dd>
      </div>
      <div>
        <dt className='text-muted-foreground text-xs'>
          {t('userPortal.model')}
        </dt>
        <dd className='mt-1 wrap-anywhere'>
          {item.modelRequested ?? t('userPortal.unknown')}
        </dd>
      </div>
      <div>
        <dt className='text-muted-foreground text-xs'>
          {t('userPortal.result')}
        </dt>
        <dd className='mt-1'>
          <RequestStatusBadge status={item.terminalStatus} />
        </dd>
      </div>
      <div>
        <dt className='text-muted-foreground text-xs'>
          {t('userPortal.nextStepTitle')}
        </dt>
        <dd className='mt-1 leading-relaxed'>
          {t(
            'userPortal.nextStep.' +
              (isKnownRequestStatus(item.terminalStatus)
                ? item.terminalStatus
                : 'unknown')
          )}
        </dd>
      </div>
      <div>
        <dt className='text-muted-foreground text-xs'>
          {t('userPortal.supportSummary')}
        </dt>
        <dd className='mt-2'>
          <CopyValue
            label={t('userPortal.copySupportSummary')}
            value={[
              t('userPortal.requestId') + ': ' + item.id,
              t('userPortal.model') +
                ': ' +
                (item.modelRequested ?? t('userPortal.unknown')),
              t('userPortal.result') +
                ': ' +
                (isKnownRequestStatus(item.terminalStatus)
                  ? t('userPortal.status.' + item.terminalStatus)
                  : t('userPortal.unknown')),
              t('userPortal.startedAt') + ': ' + item.startedAt,
              t('userPortal.attempts') + ': ' + item.attemptCount,
              t('userPortal.quotaDetails.state') +
                ': ' +
                (item.quota
                  ? t('userPortal.quotaDetails.states.' + item.quota.state)
                  : t('userPortal.unknown')),
            ].join('\n')}
          />
        </dd>
      </div>
      <div className='grid grid-cols-2 gap-4'>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('userPortal.startedAt')}
          </dt>
          <dd className='mt-1 tabular-nums'>
            {new Date(item.startedAt).toLocaleString()}
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('userPortal.duration')}
          </dt>
          <dd className='mt-1 tabular-nums'>
            {item.latencyMs.toLocaleString()} ms
          </dd>
        </div>
        <div>
          <dt className='text-muted-foreground text-xs'>
            {t('userPortal.attempts')}
          </dt>
          <dd className='mt-1 tabular-nums'>{item.attemptCount}</dd>
        </div>
      </div>
      <div className='min-w-0 border-t pt-5'>
        <dt className='font-medium'>{t('userPortal.quotaDetails.title')}</dt>
        <dd className='mt-3'>
          <RequestQuota quota={item.quota} />
        </dd>
      </div>
    </dl>
  )
}

function RequestQuota({ quota }: { quota: PersonalRequest['quota'] }) {
  const { t, i18n } = useTranslation()
  if (quota === undefined) {
    return <p>{t('userPortal.quotaDetails.legacyUnknown')}</p>
  }
  if (quota === null) {
    return <p>{t('userPortal.quotaDetails.noRecord')}</p>
  }
  // Display stored values, including zero, without recalculating settlement.
  const number = (value: number | null) =>
    value === null
      ? t('userPortal.quotaDetails.unknown')
      : value.toLocaleString(toBcp47(i18n.language), {
          maximumFractionDigits: 20,
        })
  const fields: Array<{ label: string; value: React.ReactNode }> = [
    {
      label: 'state',
      value: (
        <SoftToneBadge tone={quotaStateTone(quota.state)}>
          {t('userPortal.quotaDetails.states.' + quota.state)}
        </SoftToneBadge>
      ),
    },
    { label: 'totalTokens', value: number(quota.totalTokens) },
    { label: 'groupMultiplier', value: number(quota.groupMultiplier) },
    { label: 'reserved', value: number(quota.reserved) },
    {
      label:
        quota.state === 'pending' || quota.state === 'unknown'
          ? 'chargedUnfinalized'
          : 'charged',
      value: number(quota.charged),
    },
  ]
  return (
    <div className='min-w-0 space-y-4 leading-relaxed wrap-anywhere'>
      <dl className='grid min-w-0 gap-3'>
        {fields.map(({ label, value }) => (
          <div key={label}>
            <dt className='text-muted-foreground text-xs'>
              {t('userPortal.quotaDetails.' + label)}
            </dt>
            <dd className='mt-1 tabular-nums'>{value}</dd>
          </div>
        ))}
      </dl>
      <Notice
        tone={quota.state === 'unknown' ? 'warning' : 'info'}
        size='compact'
      >
        {t('userPortal.quotaDetails.guidance.' + quota.state)}
      </Notice>
      <div className='text-muted-foreground space-y-2 text-xs'>
        <p>{t('userPortal.quotaDetails.history')}</p>
        <p>{t('userPortal.quotaDetails.formula')}</p>
        <p>{t('userPortal.quotaDetails.rounding')}</p>
        <p>{t('userPortal.quotaDetails.attemptsHint')}</p>
        <p>{t('userPortal.quotaDetails.retryHint')}</p>
      </div>
    </div>
  )
}
