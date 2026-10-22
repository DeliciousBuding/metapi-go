import { useQuery } from '@tanstack/react-query'
import { Link, useLocation, useNavigate } from '@tanstack/react-router'
import { ArrowRight, Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import {
  useUrlTableState,
  type UrlTableState,
  type UrlTableStateUpdate,
} from '@/components/data-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Notice } from '@/components/ui/notice'
import { Spinner } from '@/components/ui/spinner'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'

import { type PersonalModel, userPortalApi } from '../api'
import { connectionExample } from '../connection-example'
import { CopyValue } from './portal-keys'

type Filters = Record<string, never>
const allowedProtocols = [
  'chat/completions',
  'responses',
  'messages',
  'embeddings',
] as const

type AllowedProtocol = (typeof allowedProtocols)[number]

const capabilityKeys = [
  'boundedText',
  'functionTools',
  'media',
  'hostedTools',
  'sessions',
] as const

function readModelContext(search: string): {
  model?: string
  protocol?: AllowedProtocol
} {
  const params = new URLSearchParams(search)
  const model = params.get('model')?.trim()
  const protocol = params.get('protocol')
  return {
    model: model ? model.slice(0, 200) : undefined,
    protocol: allowedProtocols.includes(protocol as AllowedProtocol)
      ? (protocol as AllowedProtocol)
      : undefined,
  }
}

function modelContextHref(
  search: string,
  context: { model?: string; protocol?: AllowedProtocol }
) {
  const params = new URLSearchParams(search)
  if (context.model) params.set('model', context.model)
  else params.delete('model')
  if (context.protocol) params.set('protocol', context.protocol)
  else params.delete('protocol')
  return '/user?' + params.toString()
}

function readSearch(search: string): UrlTableState<Filters> {
  const params = new URLSearchParams(search)
  const page = Number(params.get('page') ?? 1)
  return {
    q: (params.get('q') ?? '').slice(0, 200),
    pageIndex:
      Number.isSafeInteger(page) && page > 0 && page <= 1_000_000
        ? page - 1
        : 0,
    pageSize: 20,
    sorting: [],
    filters: {},
  }
}
function buildHref(
  next: UrlTableStateUpdate<Filters>,
  search = window.location.search
) {
  const state = { ...readSearch(search), ...next }
  const params = new URLSearchParams(search)
  params.set('view', 'models')
  params.set('page', String(state.pageIndex + 1))
  if (state.q) params.set('q', state.q)
  else params.delete('q')
  return '/user?' + params.toString()
}
function useModelSearch() {
  return useUrlTableState<Filters>({
    basePath: '/user',
    read: readSearch,
    buildHref,
    resetPageIndexOnFilterChange: true,
    toColumnFilters: () => [],
    fromColumnFilters: () => ({}),
  })
}

function ReferencePrice({ model }: { model: PersonalModel }) {
  const { t, i18n } = useTranslation()
  const price = model.referencePrice
  const number = new Intl.NumberFormat(i18n.language, {
    maximumFractionDigits: 8,
  })
  return (
    <div className='space-y-3'>
      <h3 className='text-sm font-medium'>
        {t('userPortal.catalog.referencePrice')}
      </h3>
      {price ? (
        <dl className='grid grid-cols-2 gap-3 text-sm'>
          {(['input', 'output', 'cacheRead', 'cacheWrite'] as const).map(
            (field) => (
              <div key={field}>
                <dt className='text-muted-foreground'>
                  {t('userPortal.catalog.' + field)}
                </dt>
                <dd className='tabular-nums'>
                  {price[field] === null
                    ? t('userPortal.unknown')
                    : number.format(price[field])}
                </dd>
              </div>
            )
          )}
        </dl>
      ) : (
        <p className='text-muted-foreground text-sm'>
          {t('userPortal.catalog.priceUnknown')}
        </p>
      )}
      <p className='text-muted-foreground text-xs'>
        {t('userPortal.catalog.priceHint')}
      </p>
      {price?.updatedAt && (
        <p className='text-muted-foreground text-xs'>
          {t('userPortal.catalog.priceUpdated', { date: price.updatedAt })}
        </p>
      )}
    </div>
  )
}

function ModelConnection({ model }: { model: PersonalModel }) {
  const { t } = useTranslation()
  const locationSearch = useLocation({ select: (loc) => loc.searchStr })
  const navigate = useNavigate()
  const requestedProtocol = readModelContext(locationSearch).protocol
  const protocol = model.protocols.includes(requestedProtocol as never)
    ? requestedProtocol
    : model.protocols[0]
  const baseURL = window.location.origin + '/v1'
  function selectProtocol(next: string) {
    if (!model.protocols.includes(next as never)) return
    void navigate({
      href: modelContextHref(locationSearch, {
        model: model.id,
        protocol: next as AllowedProtocol,
      }),
    })
  }
  return (
    <div className='space-y-5'>
      <ReferencePrice model={model} />
      <div className='space-y-2 border-t pt-4'>
        <h3 className='font-medium'>{t('userPortal.catalog.firstCall')}</h3>
        <p className='text-muted-foreground text-sm'>
          {t('userPortal.catalog.exampleHint')}
        </p>
        <Label htmlFor='model-base-url'>{t('userPortal.endpoint')}</Label>
        <Input id='model-base-url' value={baseURL} readOnly />
        <div className='flex flex-wrap gap-2'>
          <CopyValue value={baseURL} label={t('userPortal.copyEndpoint')} />
          <CopyValue value={model.id} label={t('userPortal.copyModel')} />
        </div>
      </div>
      {protocol ? (
        <div className='min-w-0 space-y-3'>
          <Tabs value={protocol} onValueChange={selectProtocol}>
            <TabsList
              className='grid w-full grid-cols-2 gap-1 group-data-horizontal/tabs:h-auto sm:inline-flex sm:w-fit'
              aria-label={t('userPortal.catalog.protocol')}
            >
              {model.protocols.map((p) => (
                <TabsTrigger key={p} value={p} className='h-auto min-h-8'>
                  {p}
                </TabsTrigger>
              ))}
            </TabsList>
            <TabsContent value={protocol} className='space-y-3'>
              <p className='text-muted-foreground text-sm'>
                {t('userPortal.catalog.environmentKey')}
              </p>
              <pre
                className='bg-muted max-h-72 overflow-auto rounded-lg border p-3 text-xs'
                tabIndex={0}
                aria-label={t('userPortal.catalog.example')}
              >
                <code>
                  {connectionExample(
                    window.location.origin,
                    model.id,
                    protocol
                  )}
                </code>
              </pre>
              <CopyValue
                value={connectionExample(
                  window.location.origin,
                  model.id,
                  protocol
                )}
                label={t('userPortal.catalog.copyExample')}
              />
            </TabsContent>
          </Tabs>
        </div>
      ) : (
        <Notice tone='info'>{t('userPortal.catalog.noExample')}</Notice>
      )}
      <Notice tone='info'>{t('userPortal.catalog.verifyHint')}</Notice>
      <div className='flex flex-wrap gap-2'>
        <Button
          render={
            <Link
              to='/user'
              search={{
                view: 'keys',
                page: 1,
                model: model.id,
                protocol: protocol as AllowedProtocol | undefined,
              }}
            />
          }
        >
          {t('userPortal.createKey')}
          <ArrowRight aria-hidden='true' />
        </Button>
        <Button
          variant='outline'
          render={<Link to='/user' search={{ view: 'requests', page: 1 }} />}
        >
          {t('userPortal.nav.requests')}
        </Button>
      </div>
    </div>
  )
}

export function PortalModels({ userId }: { userId: number }) {
  const { t, i18n } = useTranslation()
  const state = useModelSearch()
  const locationSearch = useLocation({ select: (loc) => loc.searchStr })
  const navigate = useNavigate()
  const context = readModelContext(locationSearch)
  const query = useQuery({
    queryKey: ['userPortal', userId, 'models'],
    queryFn: userPortalApi.models,
    refetchOnMount: 'always',
  })
  const filtered = (query.data?.items ?? []).filter((item) =>
    item.id.toLowerCase().includes(state.globalFilter.trim().toLowerCase())
  )
  const pageCount = Math.max(1, Math.ceil(filtered.length / 20))
  const pageIndex = Math.min(state.pagination.pageIndex, pageCount - 1)
  // Do not keep a stale model's permissions or connection example after refresh.
  const selected = !query.isError
    ? query.data?.items.find((item) => item.id === context.model)
    : undefined
  function selectModel(model: PersonalModel) {
    const requestedProtocol = context.protocol
    const protocol = model.protocols.includes(requestedProtocol as never)
      ? requestedProtocol
      : undefined
    void navigate({
      href: modelContextHref(locationSearch, { model: model.id, protocol }),
    })
  }
  return (
    <div className='space-y-5'>
      <p className='text-muted-foreground text-sm'>
        {t('userPortal.modelsHint')}
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
          {context.model && !selected && !query.isFetching && (
            <Notice tone='warning'>
              <p className='wrap-anywhere'>
                {t('userPortal.catalog.selectedUnavailable', {
                  model: context.model,
                })}
              </p>
              <Button
                size='sm'
                variant='outline'
                onClick={() =>
                  void navigate({ href: modelContextHref(locationSearch, {}) })
                }
              >
                {t('userPortal.catalog.clearSelection')}
              </Button>
            </Notice>
          )}
          <Card>
            <CardContent className='space-y-3'>
              <h2 className='font-medium'>
                {t('userPortal.capabilities.title')}
              </h2>
              <p className='text-muted-foreground text-sm leading-relaxed'>
                {t('userPortal.capabilities.intro')}
              </p>
              <ul className='grid gap-2 text-sm sm:grid-cols-2'>
                {capabilityKeys.map((key) => (
                  <li className='flex gap-2' key={key}>
                    <span aria-hidden='true' className='text-primary mt-0.5'>
                      •
                    </span>
                    <span>{t('userPortal.capabilities.' + key)}</span>
                  </li>
                ))}
              </ul>
              <p className='text-muted-foreground text-xs leading-relaxed'>
                {t('userPortal.capabilities.protocols')}
              </p>
            </CardContent>
          </Card>
          <Card>
            <CardContent className='space-y-2'>
              <h2 className='font-medium'>
                {t('userPortal.catalog.quotaTitle')}
              </h2>
              <p className='text-sm'>
                {t('userPortal.catalog.quotaFormula', {
                  multiplier: new Intl.NumberFormat(i18n.language).format(
                    query.data.quota.groupMultiplier
                  ),
                })}
              </p>
              <p className='text-muted-foreground text-xs'>
                {t('userPortal.catalog.quotaHint')}
              </p>
            </CardContent>
          </Card>
          {query.data.items.length === 0 ? (
            <Notice tone='info'>{t('userPortal.noModels')}</Notice>
          ) : (
            <>
              <div className='flex flex-wrap items-center gap-3'>
                <div className='relative min-w-0 flex-1'>
                  <Search
                    className='text-muted-foreground pointer-events-none absolute start-3 top-2.5 size-4'
                    aria-hidden='true'
                  />
                  <Input
                    aria-label={t('userPortal.catalog.search')}
                    placeholder={t('userPortal.catalog.search')}
                    value={state.globalFilter}
                    maxLength={200}
                    onChange={(e) => state.onGlobalFilterChange(e.target.value)}
                    className='ps-9'
                  />
                </div>
                <span className='text-muted-foreground text-sm'>
                  {t('userPortal.catalog.count', { count: filtered.length })}
                </span>
              </div>
              {filtered.length === 0 ? (
                <Notice tone='info'>
                  <span>{t('userPortal.catalog.noMatches')}</span>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => state.onGlobalFilterChange('')}
                  >
                    {t('userPortal.catalog.clearSearch')}
                  </Button>
                </Notice>
              ) : (
                <div className='divide-border divide-y rounded-xl border'>
                  {filtered
                    .slice(pageIndex * 20, (pageIndex + 1) * 20)
                    .map((model) => (
                      <div
                        className='flex flex-wrap items-center justify-between gap-3 p-4'
                        key={model.routeId + ':' + model.id}
                      >
                        <div className='min-w-0 flex-1 space-y-2'>
                          <ModelPill model={model.id} variant='inline' />
                          <div className='flex flex-wrap gap-1.5'>
                            {model.protocols.map((p) => (
                              <Badge key={p} variant='secondary'>
                                {p}
                              </Badge>
                            ))}
                          </div>
                        </div>
                        <Button
                          variant='outline'
                          size='sm'
                          onClick={() => selectModel(model)}
                          aria-label={t('userPortal.catalog.connectModel', {
                            model: model.id,
                          })}
                        >
                          {t('userPortal.catalog.details')}
                          <ArrowRight aria-hidden='true' />
                        </Button>
                      </div>
                    ))}
                </div>
              )}
              {pageCount > 1 && (
                <div className='flex items-center justify-between gap-3 text-sm'>
                  <span>
                    {t('userPortal.pagination', {
                      page: pageIndex + 1,
                      total: pageCount,
                      count: filtered.length,
                    })}
                  </span>
                  <div className='flex gap-2'>
                    <Button
                      variant='outline'
                      size='sm'
                      disabled={pageIndex === 0}
                      onClick={() =>
                        state.onPaginationChange({
                          pageIndex: pageIndex - 1,
                          pageSize: 20,
                        })
                      }
                    >
                      {t('userPortal.previous')}
                    </Button>
                    <Button
                      variant='outline'
                      size='sm'
                      disabled={pageIndex + 1 >= pageCount}
                      onClick={() =>
                        state.onPaginationChange({
                          pageIndex: pageIndex + 1,
                          pageSize: 20,
                        })
                      }
                    >
                      {t('userPortal.next')}
                    </Button>
                  </div>
                </div>
              )}
            </>
          )}
        </>
      )}
      <Dialog
        open={!!selected}
        onOpenChange={(open) => {
          if (!open)
            void navigate({
              href: modelContextHref(locationSearch, {}),
            })
        }}
      >
        <DialogContent className='max-h-[90svh] overflow-y-auto sm:max-w-2xl'>
          <DialogHeader>
            <DialogTitle className='wrap-anywhere'>{selected?.id}</DialogTitle>
            <DialogDescription>
              {t('userPortal.catalog.detailHint')}
            </DialogDescription>
          </DialogHeader>
          {selected && (
            <ModelConnection
              key={selected.id + ':' + selected.protocols.join(',')}
              model={selected}
            />
          )}
        </DialogContent>
      </Dialog>
    </div>
  )
}
