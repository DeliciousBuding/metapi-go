// metapi-go features/checkin/components — checkin log detail side sheet.
// i18n: all user-visible strings migrated to t() calls.

import { ExternalLink } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { DetailField } from '@/components/common/detail-field'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { isValidEndpointUrl } from '@/features/sites'
import { toBcp47 } from '@/i18n/languages'

import { formatCheckinLogTime } from '../lib/checkin-time'
import { type CheckinLogRow, checkinLogRowSchema } from '../types'
import { CheckinStatusBadge } from './checkin-columns'
import { FailureReasonBadge } from './failure-reason-badge'

interface CheckinDetailSheetProps {
  row: CheckinLogRow | null
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function CheckinDetailSheet({
  row,
  open,
  onOpenChange,
}: CheckinDetailSheetProps) {
  const { t, i18n } = useTranslation()
  const locale = toBcp47(i18n.language || 'en')
  if (!row) {
    return (
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side='right' className='sm:max-w-md' />
      </Sheet>
    )
  }
  const log = checkinLogRowSchema.parse(row)
  const inner = log.checkin_logs
  const account = log.accounts
  const site = log.sites
  const reason = log.failureReason
  const handleViewAccount = () => {
    const params = new URLSearchParams()
    params.set('accountId', String(inner.accountId))
    window.location.assign(`/accounts?${params.toString()}`)
  }
  const localizedReasonTitle = reason
    ? t(`checkin.failureReason.code.${reason.code}.title`, {
        defaultValue: reason.title || '—',
      })
    : null
  const localizedActionHint =
    reason && reason.actionHint
      ? t(`checkin.failureReason.code.${reason.code}.action`, {
          defaultValue: reason.actionHint,
        })
      : null
  const localizedDetailHint =
    reason && reason.detailHint
      ? t(`checkin.failureReason.code.${reason.code}.detail`, {
          defaultValue: reason.detailHint,
        })
      : null

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 sm:max-w-md'
      >
        <SheetHeader>
          <SheetTitle className='flex min-w-0 flex-wrap items-center gap-2'>
            <span className='min-w-0 truncate'>
              {account?.username || `#${inner.accountId}`}
            </span>
            <CheckinStatusBadge status={inner.status} />
          </SheetTitle>
        </SheetHeader>
        <div className='min-w-0 flex-1 space-y-4 overflow-y-auto p-4'>
          <dl className='grid grid-cols-1 gap-x-3 gap-y-2.5 text-sm sm:grid-cols-2'>
            <DetailField label={t('checkin.detail.createdAt')}>
              <span className='tabular-nums'>
                {formatCheckinLogTime(inner.createdAt, locale)}
              </span>
            </DetailField>
            <DetailField
              label={t('checkin.detail.account')}
              title={account?.username || `#${inner.accountId}`}
            >
              <span className='tabular-nums'>
                {account?.username || `#${inner.accountId}`}
              </span>
            </DetailField>
            <DetailField
              label={t('checkin.detail.site')}
              title={site ? site.name || site.url || undefined : undefined}
            >
              {site ? site.name || site.url || '—' : '—'}
            </DetailField>
            {site?.url && (
              <DetailField label={t('checkin.detail.siteUrl')} full>
                {/^https?:\/\//i.test(site.url) &&
                isValidEndpointUrl(site.url) ? (
                  <a
                    href={site.url}
                    target='_blank'
                    rel='noopener noreferrer'
                    className='text-primary break-all underline-offset-4 hover:underline'
                  >
                    {site.url}
                  </a>
                ) : (
                  <span className='text-muted-foreground break-all'>
                    {site.url}
                  </span>
                )}
              </DetailField>
            )}
            <DetailField
              label={t('checkin.detail.reward')}
              title={inner.reward || undefined}
            >
              <span className='tabular-nums'>{inner.reward || '—'}</span>
            </DetailField>
            <DetailField label={t('checkin.detail.logId')}>
              <span className='font-mono tabular-nums'>{`#${inner.id}`}</span>
            </DetailField>
          </dl>
          {inner.message && (
            <section className='min-w-0 space-y-1.5'>
              <h3 className='text-muted-foreground text-xs font-medium'>
                {t('checkin.detail.rawResponse')}
              </h3>
              <p className='bg-muted/30 rounded-lg border p-3 font-mono text-xs leading-relaxed [overflow-wrap:anywhere] break-words whitespace-pre-wrap'>
                {inner.message}
              </p>
            </section>
          )}
          {reason && (
            <>
              <Separator />
              <section className='min-w-0 space-y-3'>
                <div className='flex flex-wrap items-center justify-between gap-2'>
                  <h3 className='text-muted-foreground text-xs font-medium'>
                    {t('checkin.detail.failureReason')}
                  </h3>
                  <FailureReasonBadge reason={reason} />
                </div>
                <dl className='grid grid-cols-1 gap-x-3 gap-y-2.5 text-sm sm:grid-cols-2'>
                  <DetailField label={t('checkin.detail.category')}>
                    {reason.category}
                  </DetailField>
                  <DetailField label={t('checkin.detail.errorCode')}>
                    <span className='font-mono text-xs break-all'>
                      {reason.code}
                    </span>
                  </DetailField>
                  <DetailField
                    label={t('checkin.detail.description')}
                    title={localizedReasonTitle || undefined}
                    full
                  >
                    {localizedReasonTitle}
                  </DetailField>
                  {localizedActionHint && (
                    <DetailField
                      label={t('checkin.detail.actionHint')}
                      title={localizedActionHint}
                      full
                    >
                      {localizedActionHint}
                    </DetailField>
                  )}
                  {localizedDetailHint && (
                    <DetailField
                      label={t('checkin.detail.detailHint')}
                      title={localizedDetailHint}
                      full
                    >
                      {localizedDetailHint}
                    </DetailField>
                  )}
                </dl>
              </section>
            </>
          )}
        </div>
        <SheetFooter>
          <Button
            onClick={handleViewAccount}
            variant='default'
            className='w-full sm:w-auto'
          >
            <ExternalLink />
            {t('checkin.detail.viewAccount')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}
