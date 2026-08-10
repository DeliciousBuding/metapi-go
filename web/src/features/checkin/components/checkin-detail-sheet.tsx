// metapi-go features/checkin/components — checkin log detail side sheet.
//
// Shows the full checkin record (untruncated message, reward, timestamps)
// plus the structured failureReason breakdown (code / category / title /
// actionHint / detailHint) and a footer CTA to jump to the account detail.
// Mirrors the accounts feature's AccountDetailSheet layout.

import { ExternalLink } from 'lucide-react'
import type { ReactNode } from 'react'

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

import {
  type CheckinLogRow,
  checkinLogRowSchema,
} from '../types'
import { formatCheckinLogTime } from '../lib/checkin-time'
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
            <Badge variant={inner.status === 'success' ? 'default' : inner.status === 'skipped' ? 'secondary' : 'destructive'}>
              {inner.status === 'success'
                ? '成功'
                : inner.status === 'skipped'
                  ? '跳过'
                  : '失败'}
            </Badge>
          </SheetTitle>
        </SheetHeader>

        <div className='flex-1 space-y-4 overflow-y-auto p-4'>
          <dl className='grid grid-cols-2 gap-x-3 gap-y-2 text-sm'>
            <DetailField label='签到时间'>
              {formatCheckinLogTime(inner.createdAt)}
            </DetailField>
            <DetailField label='账号'>
              {account?.username || `#${inner.accountId}`}
            </DetailField>
            <DetailField label='站点'>
              {site ? site.name || site.url || '—' : '—'}
            </DetailField>
            {site?.url && (
              <DetailField label='站点地址'>
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
            <DetailField label='奖励'>
              {inner.reward || '—'}
            </DetailField>
            <DetailField label='日志 ID'>
              {`#${inner.id}`}
            </DetailField>
          </dl>

          {inner.message && (
            <div className='space-y-1'>
              <dt className='text-[11px] text-muted-foreground'>原始响应</dt>
              <dd className='whitespace-pre-wrap break-words rounded-lg border bg-muted/30 p-3 text-sm font-mono'>
                {inner.message}
              </dd>
            </div>
          )}

          {reason && (
            <>
              <Separator />
              <div className='space-y-3'>
                <div className='flex items-center gap-2'>
                  <span className='text-[11px] text-muted-foreground'>
                    失败原因
                  </span>
                  <FailureReasonBadge reason={reason} />
                </div>
                <dl className='grid grid-cols-1 gap-y-2 text-sm'>
                  <DetailField label='分类'>
                    {reason.category}
                  </DetailField>
                  <DetailField label='错误代码'>
                    {reason.code}
                  </DetailField>
                  <DetailField label='描述'>
                    {reason.title || '—'}
                  </DetailField>
                  {reason.actionHint && (
                    <DetailField label='建议操作'>
                      {reason.actionHint}
                    </DetailField>
                  )}
                  {reason.detailHint && (
                    <DetailField label='详细信息'>
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
            查看账号
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
      <dt className='text-[11px] text-muted-foreground'>{label}</dt>
      <dd className='truncate'>{children}</dd>
    </div>
  )
}
