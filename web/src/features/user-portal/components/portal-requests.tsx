import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
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

import { userPortalApi, type PersonalRequest } from '../api'

function Status({ status }: { status: string }) {
  const { t } = useTranslation()
  const known = [
    'completed',
    'upstream_failed',
    'output_interrupted',
    'client_cancelled',
    'rejected_no_route',
    'rejected_policy',
    'gateway_failed',
  ]
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
      {known.includes(status)
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
        <dd className='mt-1 font-mono wrap-anywhere'>{item.id}</dd>
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
    </dl>
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
  const query = useQuery({
    queryKey: ['userPortal', userId, 'requests', page],
    queryFn: () => userPortalApi.requests(page),
  })
  const detail = useQuery({
    queryKey: ['userPortal', userId, 'request', requestId],
    queryFn: () => userPortalApi.request(requestId!),
    enabled: !!requestId,
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
                      <TableCell>
                        <Link
                          to='/user'
                          search={{
                            view: 'requests',
                            page,
                            requestId: item.id,
                          }}
                          className='text-primary font-medium hover:underline'
                        >
                          {item.modelRequested ?? t('userPortal.unknown')}
                        </Link>
                      </TableCell>
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
        <SheetContent>
          <SheetHeader>
            <SheetTitle>{t('userPortal.requestDetail')}</SheetTitle>
            <SheetDescription>{t('userPortal.requestsHint')}</SheetDescription>
          </SheetHeader>
          <div className='overflow-y-auto px-4'>
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
