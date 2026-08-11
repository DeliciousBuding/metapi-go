/* eslint-disable no-nested-ternary -- status label selection uses chained ternaries */
// metapi-go features/checkin/components — checkin log detail side sheet.
// i18n: all user-visible strings migrated to t() calls.

import { ExternalLink } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import {
  Sheet,
  SheetContent,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'

import { formatCheckinLogTime } from '../lib/checkin-time'
import { type CheckinLogRow, checkinLogRowSchema } from '../types'
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
  const { t } = useTranslation()
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
  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent
        side='right'
        className='flex w-full flex-col gap-0 sm:max-w-md'
      >
        <SheetHeader>
          <SheetTitle className='flex items-center gap-2'>
            <span className='truncate'>
              {account?.username || `#${inner.accountId}`}
            </span>
            <Badge
              variant={
                inner.status === 'success'
                  ? 'default'
                  : inner.status === 'skipped'
                    ? 'secondary'
                    : 'destructive'
              }
            >
              {inner.status === 'success'
                ? t('checkin.detail.statusSuccess')
                : inner.status === 'skipped'
                  ? t('checkin.detail.statusSkipped')
                  : t('checkin.detail.statusFailed')}
            </Badge>
          </SheetTitle>
        </SheetHeader>
        <div className='flex-1 space-y-4 overflow-y-auto p-4'>
          <dl className='grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
            <DetailField label={t('checkin.detail.createdAt')}>
              {formatCheckinLogTime(inner.createdAt)}
            </DetailField>
            <DetailField label={t('checkin.detail.account')}>
              {account?.username || `#${inner.accountId}`}
            </DetailField>
            <DetailField label={t('checkin.detail.site')}>
              {site ? site.name || site.url || '—' : '—'}
            </DetailField>
            {site?.url && (
              <DetailField label={t('checkin.detail.siteUrl')}>
                <a
                  href={site.url}
                  target='_blank'
                  rel='noopener noreferrer'
                  className='text-primary underline-offset-4 hover:underline'
                >
                  {site.url}
                </a>
              </DetailField>
            )}
            <DetailField label={t('checkin.detail.reward')}>
              {inner.reward || '—'}
            </DetailField>
            <DetailField
              label={t('checkin.detail.logId')}
            >{`#${inner.id}`}</DetailField>
          </dl>
          {inner.message && (
            <div className='space-y-1'>
              <dt className='text-muted-foreground text-[11px]'>
                {t('checkin.detail.rawResponse')}
              </dt>
              <dd className='bg-muted/30 rounded-lg border p-3 font-mono text-sm break-words whitespace-pre-wrap'>
                {inner.message}
              </dd>
            </div>
          )}
          {reason && (
            <>
              <Separator />
              <div className='space-y-3'>
                <div className='flex items-center gap-2'>
                  <span className='text-muted-foreground text-[11px]'>
                    {t('checkin.detail.failureReason')}
                  </span>
                  <FailureReasonBadge reason={reason} />
                </div>
                <dl className='grid grid-cols-1 gap-y-2 text-sm'>
                  <DetailField label={t('checkin.detail.category')}>
                    {reason.category}
                  </DetailField>
                  <DetailField label={t('checkin.detail.errorCode')}>
                    {reason.code}
                  </DetailField>
                  <DetailField label={t('checkin.detail.description')}>
                    {reason.title || '—'}
                  </DetailField>
                  {reason.actionHint && (
                    <DetailField label={t('checkin.detail.actionHint')}>
                      {reason.actionHint}
                    </DetailField>
                  )}
                  {reason.detailHint && (
                    <DetailField label={t('checkin.detail.detailHint')}>
                      {reason.detailHint}
                    </DetailField>
                  )}
                </dl>
              </div>
            </>
          )}
        </div>
        <SheetFooter>
          <Button onClick={handleViewAccount} variant='default'>
            <ExternalLink />
            {t('checkin.detail.viewAccount')}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  )
}

function DetailField({
  label,
  children,
}: {
  label: string
  children: ReactNode
}) {
  return (
    <div className='flex flex-col'>
      <dt className='text-muted-foreground text-[11px]'>{label}</dt>
      <dd className='truncate'>{children}</dd>
    </div>
  )
}
