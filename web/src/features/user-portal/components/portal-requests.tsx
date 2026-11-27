import { useQuery } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { ChevronDown, ChevronUp, RotateCcw, Search } from 'lucide-react'
import { useCallback, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import { QuietRefreshStatus } from '@/components/common/quiet-refresh-status'
import {
  SoftToneBadge,
  type SoftTone,
} from '@/components/common/soft-tone-badge'
import { useUrlTableState } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Notice } from '@/components/ui/notice'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '@/components/ui/select'
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
import {
  readRequestSearch,
  requestSearchHref,
  requestStatuses,
  quotaStates,
  type RequestStatus,
  type QuotaState,
} from '../request-search'
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

function requestStatusTone(status: string): SoftTone {
  if (status === 'completed') return 'success'
  if (status === 'client_cancelled') return 'neutral'
  if (
    status === 'rejected_no_route' ||
    status === 'rejected_policy' ||
    status === 'output_interrupted'
  ) {
    return 'warning'
  }
  if (status === 'upstream_failed' || status === 'gateway_failed') {
    return 'destructive'
  }
  return 'neutral'
}

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

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof HTMLElement)) return false
  const tag = target.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
  if (target.isContentEditable) return true
  const role = target.getAttribute('role')
  return (
    role === 'combobox' ||
    role === 'listbox' ||
    role === 'slider' ||
    role === 'textbox'
  )
}

function Status({ status }: { status: string }) {
  const { t } = useTranslation()
  return (
    <SoftToneBadge tone={requestStatusTone(status)}>
      {knownStatuses.includes(status)
        ? t('userPortal.status.' + status)
        : t('userPortal.unknown')}
    </SoftToneBadge>
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
                (knownStatuses.includes(item.terminalStatus)
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
  const navigate = useNavigate()
  const detailEntry = useRef<HTMLAnchorElement | null>(null)
  const state = useUrlTableState({
    basePath: '/user',
    read: readRequestSearch,
    buildHref: requestSearchHref,
    toColumnFilters: () => [],
    fromColumnFilters: () => ({}),
    resetPageIndexOnFilterChange: true,
  })
  const filters = { search: state.globalFilter || undefined, ...state.filters }
  const hasFilters = Boolean(
    state.globalFilter || state.filters.status || state.filters.quotaState
  )
  const mobile = useMediaQuery(TABLE_MOBILE_MEDIA_QUERY)
  const requestLink = (item: PersonalRequest) => (
    <Link
      to='/user'
      search={{
        view: 'requests',
        page,
        q: state.globalFilter || undefined,
        ...state.filters,
        requestId: item.id,
      }}
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
  const query = useQuery({
    queryKey: hasFilters
      ? ['userPortal', userId, 'requests', page, filters]
      : ['userPortal', userId, 'requests', page],
    queryFn: () => userPortalApi.requests(page, filters),
    // Calls made in a terminal do not invalidate the browser's overview cache.
    refetchOnMount: 'always',
  })
  const detail = useQuery({
    queryKey: ['userPortal', userId, 'request', requestId],
    queryFn: () => {
      if (!requestId) {
        return Promise.reject(new Error('Missing request ID'))
      }
      return userPortalApi.request(requestId)
    },
    enabled: Boolean(requestId),
    refetchOnMount: 'always',
  })
  const items = query.data?.items ?? []
  const currentIndex = requestId
    ? items.findIndex((item) => item.id === requestId)
    : -1
  const prevItem = currentIndex > 0 ? items[currentIndex - 1] : undefined
  const nextItem =
    currentIndex >= 0 && currentIndex + 1 < items.length
      ? items[currentIndex + 1]
      : undefined
  const navigateToRequest = useCallback(
    (targetId: string) => {
      void navigate({
        to: '/user',
        search: {
          view: 'requests',
          page,
          q: state.globalFilter || undefined,
          ...state.filters,
          requestId: targetId,
        },
        replace: true,
      })
    },
    [navigate, page, state.globalFilter, state.filters]
  )
  useEffect(() => {
    if (!requestId || (!prevItem && !nextItem)) return
    function handleKeyDown(event: KeyboardEvent) {
      if (
        event.metaKey ||
        event.ctrlKey ||
        event.altKey ||
        event.shiftKey ||
        event.isComposing ||
        event.keyCode === 229
      ) {
        return
      }
      if (isEditableTarget(event.target)) return
      if (event.key === 'ArrowUp' && prevItem) {
        event.preventDefault()
        navigateToRequest(prevItem.id)
      } else if (event.key === 'ArrowDown' && nextItem) {
        event.preventDefault()
        navigateToRequest(nextItem.id)
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => {
      window.removeEventListener('keydown', handleKeyDown)
    }
  }, [requestId, prevItem, nextItem, navigateToRequest])
  const { ensurePageInRange } = state
  useEffect(() => {
    if (query.isSuccess && !query.isFetching && !requestId)
      ensurePageInRange(
        Math.max(1, Math.ceil(query.data.total / query.data.pageSize))
      )
  }, [
    query.isSuccess,
    query.isFetching,
    query.data,
    requestId,
    ensurePageInRange,
  ])
  function renderRequestsBody() {
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
            <li
              key={item.id}
              className='bg-card space-y-3 rounded-xl border p-4'
            >
              <div className='flex flex-wrap items-center justify-between gap-2'>
                <h2 className='min-w-0'>{requestLink(item)}</h2>
                <Status status={item.terminalStatus} />
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
                    <Status status={item.terminalStatus} />
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
                    view: 'requests',
                    page: Math.max(1, page - 1),
                    q: state.globalFilter || undefined,
                    ...state.filters,
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
                    view: 'requests',
                    page: page + 1,
                    q: state.globalFilter || undefined,
                    ...state.filters,
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

  function renderDetailBody() {
    if (detail.isPending) {
      return <Spinner aria-label={t('common.loading')} />
    }
    if (detail.isError) {
      return (
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
      )
    }
    if (!detail.data) return null
    return <RequestDetail item={detail.data} />
  }

  return (
    <div className='space-y-4'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <p className='text-muted-foreground text-sm'>
          {t('userPortal.requestsHint')}
        </p>
        <QuietRefreshStatus
          updatedAt={query.dataUpdatedAt}
          hasError={query.isError}
        />
      </div>
      <div className='flex flex-wrap items-center gap-2'>
        <div className='relative min-w-0 flex-1 basis-60'>
          <Search
            aria-hidden='true'
            className='text-muted-foreground pointer-events-none absolute start-3 top-2 size-4'
          />
          <Input
            className='h-8 ps-9'
            aria-label={t('userPortal.requestFilters.search')}
            placeholder={t('userPortal.requestFilters.search')}
            maxLength={200}
            value={state.globalFilter}
            onChange={(event) => state.onGlobalFilterChange(event.target.value)}
          />
        </div>
        <Select
          value={state.filters.status ?? 'all'}
          onValueChange={(value) =>
            state.updateUrlState({
              filters: {
                status:
                  value === 'all'
                    ? undefined
                    : (value as RequestStatus | undefined),
              },
              pageIndex: 0,
            })
          }
        >
          <SelectTrigger
            size='sm'
            aria-label={t('userPortal.requestFilters.status')}
          >
            <SelectValue>
              {state.filters.status
                ? t('userPortal.status.' + state.filters.status)
                : t('userPortal.requestFilters.allResults')}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>
              {t('userPortal.requestFilters.allResults')}
            </SelectItem>
            {requestStatuses.map((status) => (
              <SelectItem key={status} value={status}>
                {t('userPortal.status.' + status)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Select
          value={state.filters.quotaState ?? 'all'}
          onValueChange={(value) =>
            state.updateUrlState({
              filters: {
                quotaState:
                  value === 'all'
                    ? undefined
                    : (value as QuotaState | undefined),
              },
              pageIndex: 0,
            })
          }
        >
          <SelectTrigger
            size='sm'
            aria-label={t('userPortal.requestFilters.quota')}
          >
            <SelectValue>
              {state.filters.quotaState
                ? t(
                    'userPortal.requestFilters.quotaStates.' +
                      state.filters.quotaState
                  )
                : t('userPortal.requestFilters.allQuota')}
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='all'>
              {t('userPortal.requestFilters.allQuota')}
            </SelectItem>
            {quotaStates.map((status) => (
              <SelectItem key={status} value={status}>
                {t('userPortal.requestFilters.quotaStates.' + status)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {hasFilters && (
          <Button
            size='sm'
            variant='ghost'
            onClick={() =>
              state.updateUrlState({
                q: '',
                filters: { status: undefined, quotaState: undefined },
                pageIndex: 0,
              })
            }
          >
            <RotateCcw aria-hidden='true' />
            {t('userPortal.requestFilters.clear')}
          </Button>
        )}
      </div>
      {renderRequestsBody()}
      <Sheet
        open={Boolean(requestId)}
        onOpenChange={(open) => {
          if (!open) onCloseDetail()
        }}
      >
        <SheetContent finalFocus={detailEntry} className='sm:max-w-lg'>
          <SheetHeader>
            <div className='flex items-center justify-between gap-2'>
              <SheetTitle>{t('userPortal.requestDetail')}</SheetTitle>
              {(prevItem || nextItem) && (
                <div className='flex items-center gap-1'>
                  <Button
                    type='button'
                    size='icon-xs'
                    variant='outline'
                    aria-label={t('requestLogs.prevRecord')}
                    disabled={!prevItem}
                    onClick={() => prevItem && navigateToRequest(prevItem.id)}
                  >
                    <ChevronUp aria-hidden='true' className='size-3.5' />
                  </Button>
                  <Button
                    type='button'
                    size='icon-xs'
                    variant='outline'
                    aria-label={t('requestLogs.nextRecord')}
                    disabled={!nextItem}
                    onClick={() => nextItem && navigateToRequest(nextItem.id)}
                  >
                    <ChevronDown aria-hidden='true' className='size-3.5' />
                  </Button>
                </div>
              )}
            </div>
            <SheetDescription>{t('userPortal.requestsHint')}</SheetDescription>
          </SheetHeader>
          <div className='min-h-0 min-w-0 flex-1 overflow-y-auto px-4 pb-4'>
            {renderDetailBody()}
          </div>
        </SheetContent>
      </Sheet>
    </div>
  )
}
