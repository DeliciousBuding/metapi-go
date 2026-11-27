import type { UseQueryResult } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import type { RefObject } from 'react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { Button } from '@/components/ui/button'
import { Notice } from '@/components/ui/notice'
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
import { TABLE_MOBILE_MEDIA_QUERY } from '@/lib/breakpoints'

import type { userPortalApi, PersonalRequest } from '../api'
import type { readRequestSearch } from '../request-search'
import { RequestStatusBadge } from './portal-request-status'

export function RequestList({
  query,
  search,
  requestId,
  detailEntry,
  hasFilters,
}: {
  query: UseQueryResult<Awaited<ReturnType<typeof userPortalApi.requests>>>
  search: ReturnType<typeof readRequestSearch>['filters'] & {
    view: 'requests'
    page: number
    q?: string
  }
  requestId?: string
  detailEntry: RefObject<HTMLAnchorElement | null>
  hasFilters: boolean
}) {
  const { t } = useTranslation()
  const { page } = search
  const mobile = useMediaQuery(TABLE_MOBILE_MEDIA_QUERY)
  const requestLink = (item: PersonalRequest) => (
    <Link
      to='/user'
      search={{ ...search, requestId: item.id }}
      className='text-primary inline-flex max-w-full items-center font-medium wrap-anywhere hover:underline'
      onClick={(event) => {
        detailEntry.current = event.currentTarget
      }}
    >
      {item.modelRequested ? (
        <ModelPill model={item.modelRequested} variant='inline' />
      ) : (
        t('userPortal.unknown')
      )}
    </Link>
  )
  if (query.isPending) {
    return <Spinner aria-label={t('common.loading')} />
  }
  if (query.isError) {
    return (
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
    )
  }
  let content: React.ReactNode
  if (query.data.items.length === 0) {
    content = (
      <div className='text-muted-foreground bg-card flex flex-col items-center gap-3 rounded-xl border p-8 text-center text-sm'>
        <p>
          {t(
            hasFilters
              ? 'userPortal.requestFilters.noMatches'
              : 'userPortal.noRequests'
          )}
        </p>
        {!hasFilters && (
          <Button
            variant='outline'
            size='sm'
            render={<Link to='/user' search={{ view: 'models', page: 1 }} />}
          >
            {t('userPortal.browseModels')}
          </Button>
        )}
      </div>
    )
  } else if (mobile) {
    content = (
      <ul className='space-y-3'>
        {query.data.items.map((item) => (
          <li key={item.id} className='bg-card space-y-3 rounded-xl border p-4'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <h2 className='min-w-0'>{requestLink(item)}</h2>
              <RequestStatusBadge status={item.terminalStatus} />
            </div>
            <dl className='grid grid-cols-2 gap-3 text-sm'>
              <div className='col-span-2'>
                <dt className='text-muted-foreground text-xs'>
                  {t('userPortal.startedAt')}
                </dt>
                <dd className='mt-0.5 tabular-nums'>
                  {new Date(item.startedAt).toLocaleString()}
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>
                  {t('userPortal.duration')}
                </dt>
                <dd className='mt-0.5 tabular-nums'>
                  {item.latencyMs.toLocaleString()} ms
                </dd>
              </div>
              <div>
                <dt className='text-muted-foreground text-xs'>
                  {t('userPortal.attempts')}
                </dt>
                <dd className='mt-0.5 tabular-nums'>{item.attemptCount}</dd>
              </div>
            </dl>
          </li>
        ))}
      </ul>
    )
  } else {
    content = (
      <div className='bg-card overflow-hidden rounded-xl border'>
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
              <TableRow
                key={item.id}
                data-state={item.id === requestId ? 'selected' : undefined}
              >
                <TableCell className='text-muted-foreground text-xs whitespace-nowrap tabular-nums'>
                  {new Date(item.startedAt).toLocaleString()}
                </TableCell>
                <TableCell>{requestLink(item)}</TableCell>
                <TableCell>
                  <RequestStatusBadge status={item.terminalStatus} />
                </TableCell>
                <TableCell className='whitespace-nowrap tabular-nums'>
                  {item.latencyMs.toLocaleString()} ms
                </TableCell>
                <TableCell className='tabular-nums'>
                  {item.attemptCount}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    )
  }

  return (
    <>
      {content}
      <div className='flex flex-wrap items-center justify-between gap-3 text-sm'>
        <span className='text-muted-foreground tabular-nums'>
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
                search={{
                  ...search,
                  page: Math.max(1, page - 1),
                }}
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
                search={{
                  ...search,
                  page: page + 1,
                }}
              />
            }
          >
            {t('userPortal.next')}
          </Button>
        </div>
      </div>
    </>
  )
}
