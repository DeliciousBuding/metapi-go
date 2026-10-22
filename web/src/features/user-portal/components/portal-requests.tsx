import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { Notice } from '@/components/ui/notice'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from '@/components/ui/sheet'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { useMediaQuery } from '@/hooks/use-media-query'
import { toBcp47 } from '@/i18n/languages'
import { TABLE_MOBILE_MEDIA_QUERY } from '@/lib/breakpoints'

import { userPortalApi, type PersonalRequest } from '../api'
import { CopyValue } from './portal-keys'

const knownStatuses = [
  'completed',
  'upstream_failed',
  'output_interrupted',
  'client_cancelled',
  'rejected_no_route',
  'rejected_policy',
  'gateway_failed',
]

function Status({ status }: { status: string }) {
  const { t } = useTranslation()
  return (
    <span
      className={
        status === 'completed'
          ? 'text-success-soft-fg'
          : status === 'client_cancelled'
            ? 'text-muted-foreground'
            : 'text-destructive-soft-fg'
      }
    >
      {knownStatuses.includes(status)
        ? t('userPortal.status.' + status)
        : t('userPortal.unknown')}
    </span>
  )
}

function RequestDetail({ item }: { item: PersonalRequest }) {
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
          <Status status={item.terminalStatus} />
        </dd>
      </div>
      <div>
        <dt className='text-muted-foreground text-xs'>
          {t('userPortal.nextStepTitle')}
        </dt>
        <dd className='mt-1 leading-relaxed'>
          {t(
            'userPortal.nextStep.' +
              (knownStatuses.includes(item.terminalStatus)
                ? item.terminalStatus
                : 'unknown')
          )}
        </dd>
      </div>
      <div>
        <dt className='text-muted-foreground text-xs'>
          {t('userPortal.startedAt')}
        </dt>
        <dd className='mt-1'>{new Date(item.startedAt).toLocaleString()}</dd>
      </div>
      <div>
        <dt className='text-muted-foreground text-xs'>
          {t('userPortal.duration')}
        </dt>
        <dd className='mt-1'>{item.latencyMs.toLocaleString()} ms</dd>
      </div>
      <div>
        <dt className='text-muted-foreground text-xs'>
          {t('userPortal.attempts')}
        </dt>
        <dd className='mt-1'>{item.attemptCount}</dd>
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
  const fields = [
    ['state', t('userPortal.quotaDetails.states.' + quota.state)],
    ['totalTokens', number(quota.totalTokens)],
    ['groupMultiplier', number(quota.groupMultiplier)],
    ['reserved', number(quota.reserved)],
    [
      quota.state === 'pending' || quota.state === 'unknown'
        ? 'chargedUnfinalized'
        : 'charged',
      number(quota.charged),
    ],
  ]
  return (
    <div className='min-w-0 space-y-4 leading-relaxed wrap-anywhere'>
      <dl className='grid min-w-0 gap-3'>
        {fields.map(([label, value]) => (
          <div key={label}>
            <dt className='text-muted-foreground text-xs'>
              {t('userPortal.quotaDetails.' + label)}
            </dt>
            <dd className='mt-1 tabular-nums'>{value}</dd>
          </div>
        ))}
      </dl>
      <Notice tone={quota.state === 'unknown' ? 'warning' : 'info'}>
        {t('userPortal.quotaDetails.guidance.' + quota.state)}
      </Notice>
      <div className='space-y-2 text-xs'>
        <p>{t('userPortal.quotaDetails.history')}</p>
        <p>{t('userPortal.quotaDetails.formula')}</p>
        <p>{t('userPortal.quotaDetails.rounding')}</p>
        <p>{t('userPortal.quotaDetails.attemptsHint')}</p>
        <p>{t('userPortal.quotaDetails.retryHint')}</p>
      </div>
    </div>
  )
}

export function PortalRequests({
  userId,
  page,
  requestId,
  onCloseDetail,
}: {
  userId: number
  page: number
  requestId?: string
  onCloseDetail: () => void
}) {
  const { t } = useTranslation()
  const detailEntry = useRef<HTMLAnchorElement | null>(null)
  const mobile = useMediaQuery(TABLE_MOBILE_MEDIA_QUERY)
  const requestLink = (item: PersonalRequest) => (
    <Link
      to='/user'
      search={{ view: 'requests', page, requestId: item.id }}
      className='text-primary font-medium wrap-anywhere hover:underline'
      onClick={(event) => {
        detailEntry.current = event.currentTarget
      }}
    >
      {item.modelRequested ?? t('userPortal.unknown')}
    </Link>
  )
  const query = useQuery({
    queryKey: ['userPortal', userId, 'requests', page],
    queryFn: () => userPortalApi.requests(page),
    // Calls made in a terminal do not invalidate the browser's overview cache.
    refetchOnMount: 'always',
  })
  const detail = useQuery({
    queryKey: ['userPortal', userId, 'request', requestId],
    queryFn: () => userPortalApi.request(requestId!),
    enabled: !!requestId,
    refetchOnMount: 'always',
  })
  return (
    <div className='space-y-4'>
      <p className='text-muted-foreground text-sm'>
        {t('userPortal.requestsHint')}
      </p>
      {query.isPending ? (
        <Spinner aria-label={t('common.loading')} />
      ) : query.isError ? (
        <Notice tone='destructive' role='alert'>
          <span>{t('userPortal.loadFailed')}</span>
          <Button
            variant='outline'
            size='sm'
            onClick={() => void query.refetch()}
          >
            {t('common.retry')}
          </Button>
        </Notice>
      ) : (
        <>
          {query.data.items.length === 0 ? (
            <p className='text-muted-foreground rounded-xl border p-8 text-center'>
              {t('userPortal.noRequests')}
            </p>
          ) : mobile ? (
            <ul className='space-y-3'>
              {query.data.items.map((item) => (
                <li key={item.id} className='space-y-3 rounded-xl border p-4'>
                  <h2>{requestLink(item)}</h2>
                  <dl className='grid grid-cols-2 gap-3 text-sm'>
                    <div className='col-span-2'>
                      <dt className='text-muted-foreground text-xs'>
                        {t('userPortal.startedAt')}
                      </dt>
                      <dd className='mt-1'>
                        {new Date(item.startedAt).toLocaleString()}
                      </dd>
                    </div>
                    <div>
                      <dt className='text-muted-foreground text-xs'>
                        {t('userPortal.result')}
                      </dt>
                      <dd className='mt-1'>
                        <Status status={item.terminalStatus} />
                      </dd>
                    </div>
                    <div>
                      <dt className='text-muted-foreground text-xs'>
                        {t('userPortal.duration')}
                      </dt>
                      <dd className='mt-1'>
                        {item.latencyMs.toLocaleString()} ms
                      </dd>
                    </div>
                    <div>
                      <dt className='text-muted-foreground text-xs'>
                        {t('userPortal.attempts')}
                      </dt>
                      <dd className='mt-1'>{item.attemptCount}</dd>
                    </div>
                  </dl>
                </li>
              ))}
            </ul>
          ) : (
            <div className='overflow-hidden rounded-xl border'>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('userPortal.startedAt')}</TableHead>
                    <TableHead>{t('userPortal.model')}</TableHead>
                    <TableHead>{t('userPortal.result')}</TableHead>
                    <TableHead>{t('userPortal.duration')}</TableHead>
                    <TableHead>{t('userPortal.attempts')}</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {query.data.items.map((item) => (
                    <TableRow key={item.id}>
                      <TableCell className='whitespace-nowrap'>
                        {new Date(item.startedAt).toLocaleString()}
                      </TableCell>
                      <TableCell>{requestLink(item)}</TableCell>
                      <TableCell>
                        <Status status={item.terminalStatus} />
                      </TableCell>
                      <TableCell>
                        {item.latencyMs.toLocaleString()} ms
                      </TableCell>
                      <TableCell>{item.attemptCount}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
          <div className='flex flex-wrap items-center justify-between gap-3 text-sm'>
            <span className='text-muted-foreground'>
              {t('userPortal.pagination', {
                page,
                total: Math.max(
                  1,
                  Math.ceil(query.data.total / query.data.pageSize)
                ),
                count: query.data.total,
              })}
            </span>
            <div className='flex gap-2'>
              <Button
                variant='outline'
                size='sm'
                disabled={page <= 1}
                render={
                  <Link
                    to='/user'
                    search={{ view: 'requests', page: Math.max(1, page - 1) }}
                  />
                }
              >
                {t('userPortal.previous')}
              </Button>
              <Button
                variant='outline'
                size='sm'
                disabled={page * query.data.pageSize >= query.data.total}
                render={
                  <Link
                    to='/user'
                    search={{ view: 'requests', page: page + 1 }}
                  />
                }
              >
                {t('userPortal.next')}
              </Button>
            </div>
          </div>
        </>
      )}
      <Sheet
        open={!!requestId}
        onOpenChange={(open) => {
          if (!open) onCloseDetail()
        }}
      >
        <SheetContent finalFocus={detailEntry}>
          <SheetHeader>
            <SheetTitle>{t('userPortal.requestDetail')}</SheetTitle>
            <SheetDescription>{t('userPortal.requestsHint')}</SheetDescription>
          </SheetHeader>
          <div className='min-h-0 min-w-0 flex-1 overflow-y-auto px-4 pb-4'>
            {detail.isPending ? (
              <Spinner aria-label={t('common.loading')} />
            ) : detail.isError ? (
              <Notice tone='destructive' role='alert'>
                <span>{t('userPortal.loadFailed')}</span>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => void detail.refetch()}
                >
                  {t('common.retry')}
                </Button>
              </Notice>
            ) : detail.data ? (
              <RequestDetail item={detail.data} />
            ) : null}
          </div>
        </SheetContent>
      </Sheet>
    </div>
  )
}
