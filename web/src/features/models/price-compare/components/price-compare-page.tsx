// metapi-go/features/models/price-compare — standalone cross-site price page.
// Groups the backend's cheaper-first candidate rows by model and surfaces the
// provenance grade + best-channel recommendation for every source.

import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { ArrowRight, Scale, Search, Star } from 'lucide-react'
import { useMemo } from 'react'
import { useTranslation } from 'react-i18next'
import { ZodError } from 'zod'

import { ModelPill } from '@/components/common/model-pill'
import { PageHeader } from '@/components/common/page-header'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
} from '@/components/ui/empty'
import { Input } from '@/components/ui/input'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatCurrency, formatPrice } from '@/lib/format'
import { asStringParam } from '@/lib/helpers/searchParams'

import { usePriceWorkspace } from '../api'
import type { PriceCompareItem } from '../types'
import { CurrentPrices } from './current-prices'
import { PriceGradeBadge } from './price-grade-badge'

type ModelGroup = {
  key: string
  model: string
  rows: PriceCompareItem[]
}

export function PriceComparePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const { model, pricePage } = useSearch({
    from: '/_authenticated/price-compare',
  })
  const modelParam = asStringParam(model)?.trim() ?? ''
  const query = usePriceWorkspace({
    model: modelParam || undefined,
    limit: 200,
    pricePage: pricePage ?? 1,
  })

  const groups = useMemo<ModelGroup[]>(() => {
    const map = new Map<string, PriceCompareItem[]>()
    for (const item of query.data?.items ?? []) {
      if (item.missingPrice) continue
      const key = item.model.trim().toLowerCase() || '__unknown__'
      const list = map.get(key) ?? []
      list.push(item)
      map.set(key, list)
    }
    return [...map.entries()].map(([key, rows]) => ({
      key,
      model: rows[0]?.model ?? '',
      rows,
    }))
  }, [query.data])

  return (
    <div className='page-padding flex h-full flex-col gap-3'>
      <PageHeader
        title={t('priceCompare.page.title')}
        description={t('priceCompare.page.description')}
        actions={
          <form
            className='flex w-full gap-2 sm:w-72'
            onSubmit={(event) => {
              event.preventDefault()
              const next = String(
                new FormData(event.currentTarget).get('model') ?? ''
              ).trim()
              void navigate({
                href: next
                  ? `/price-compare?${new URLSearchParams({ model: next })}`
                  : '/price-compare',
              })
            }}
          >
            <Input
              key={modelParam}
              name='model'
              defaultValue={modelParam}
              placeholder={t('priceCompare.page.searchPlaceholder')}
              aria-label={t('priceCompare.page.searchPlaceholder')}
            />
            <Button
              type='submit'
              variant='outline'
              size='icon'
              aria-label={t('priceCompare.current.search')}
            >
              <Search aria-hidden='true' />
            </Button>
          </form>
        }
      />

      {query.data?.currentPage && query.data.currentPage.totalModels > 0 && (
        <nav
          className='flex flex-wrap items-center justify-between gap-3'
          aria-label={t('priceCompare.current.pagination')}
        >
          <p className='text-muted-foreground text-sm'>
            {t('priceCompare.current.pageSummary', {
              page: query.data.currentPage.page,
              pages: Math.max(
                1,
                Math.ceil(
                  query.data.currentPage.totalModels /
                    query.data.currentPage.pageSize
                )
              ),
              count: query.data.currentPage.totalModels,
            })}
          </p>
          <div className='flex gap-2'>
            {[-1, 1].map((delta) => (
              <Button
                key={delta}
                variant='outline'
                size='sm'
                disabled={
                  query.isFetching ||
                  (delta < 0
                    ? query.data!.currentPage!.page <= 1
                    : query.data!.currentPage!.page *
                        query.data!.currentPage!.pageSize >=
                      query.data!.currentPage!.totalModels)
                }
                onClick={() => {
                  const params = new URLSearchParams(window.location.search)
                  params.set(
                    'pricePage',
                    String(query.data!.currentPage!.page + delta)
                  )
                  void navigate({ href: `/price-compare?${params}` })
                }}
              >
                {t(
                  delta < 0
                    ? 'priceCompare.current.previous'
                    : 'priceCompare.current.next'
                )}
              </Button>
            ))}
          </div>
        </nav>
      )}

      {query.data?.pricingAccounts && !query.error && (
        <CurrentPrices data={query.data} />
      )}

      {query.isLoading && (
        <div className='text-muted-foreground flex items-center gap-2 text-sm'>
          <Spinner />
          {t('common.loading')}
        </div>
      )}

      <QueryErrorBanner
        error={query.error as Error | null}
        messageKey={
          query.error instanceof ZodError
            ? 'priceCompare.page.invalidResponse'
            : 'priceCompare.page.loadError'
        }
        onRetry={() => query.refetch()}
        isRetrying={query.isFetching}
      />

      {!query.isLoading &&
        !query.error &&
        groups.length === 0 &&
        (query.data?.pricingAccounts?.length ?? 0) === 0 && (
          // flex-none: the Empty base is flex-1, which would stretch the box
          // across the whole viewport — a content-sized empty state instead.
          <Empty className='flex-none border'>
            <EmptyHeader>
              <EmptyMedia variant='icon'>
                <Scale className='size-6' />
              </EmptyMedia>
              <EmptyDescription>
                {t('priceCompare.page.emptyDescription')}
              </EmptyDescription>
            </EmptyHeader>
            <Button
              variant='outline'
              onClick={() => void navigate({ to: '/accounts' })}
            >
              {t('priceCompare.page.emptyAction')}
            </Button>
          </Empty>
        )}

      <div className='flex flex-col gap-4'>
        {groups.length > 0 && (
          <h2 className='text-base font-semibold'>
            {t('priceCompare.current.history')}
          </h2>
        )}
        {groups.map((group) => (
          <ModelGroupCard key={group.key} group={group} />
        ))}
      </div>
    </div>
  )
}

function ModelGroupCard({ group }: { group: ModelGroup }) {
  const { t } = useTranslation()

  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex items-center gap-2 text-base font-normal'>
          <ModelPill
            model={group.model}
            variant='inline'
            className='text-base'
          />
        </CardTitle>
        <CardDescription>{t('priceCompare.group.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        <Table className='max-sm:[&_td]:px-1 max-sm:[&_th]:px-1'>
          <TableHeader>
            <TableRow>
              <TableHead>{t('priceCompare.columns.site')}</TableHead>
              <TableHead className='hidden sm:table-cell'>
                {t('priceCompare.columns.grade')}
              </TableHead>
              <TableHead className='text-right'>
                {t('priceCompare.columns.input')}
              </TableHead>
              <TableHead className='hidden text-right sm:table-cell'>
                {t('priceCompare.columns.output')}
              </TableHead>
              <TableHead className='hidden text-right sm:table-cell'>
                {t('priceCompare.columns.effective')}
              </TableHead>
              <TableHead>{t('priceCompare.columns.status')}</TableHead>
              <TableHead className='text-right'>
                {t('priceCompare.columns.actions')}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {group.rows.map((row) => (
              <PriceRow
                key={`${row.siteId}-${row.accountId}-${row.model}`}
                row={row}
              />
            ))}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  )
}

export function PriceRow({ row }: { row: PriceCompareItem }) {
  const { t } = useTranslation()
  return (
    <TableRow>
      <TableCell>
        <div className='font-medium'>{row.siteName}</div>
        {row.username && (
          <div className='text-muted-foreground text-xs'>{row.username}</div>
        )}
        {row.source === 'observed' && (
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('priceCompare.current.observedAverage', {
              price: formatCurrency(row.estimatedCostSample, {
                fractionDigits: 6,
              }),
              count: row.observedSamples,
            })}
          </p>
        )}
        {row.source === 'configured' && (
          <p className='text-muted-foreground mt-1 text-xs'>
            {t('priceCompare.current.configuredCost', {
              price: formatCurrency(row.estimatedCostSample, {
                fractionDigits: 6,
              }),
            })}
          </p>
        )}
      </TableCell>
      <TableCell className='hidden sm:table-cell'>
        <PriceGradeBadge grade={row.source} />
      </TableCell>
      <TableCell className='text-right tabular-nums'>
        {row.ratesSource === 'billing_details'
          ? formatPrice(row.inputPerMillion, { fractionDigits: 4 })
          : '—'}
      </TableCell>
      <TableCell className='hidden text-right tabular-nums sm:table-cell'>
        {row.ratesSource === 'billing_details'
          ? formatPrice(row.outputPerMillion, { fractionDigits: 4 })
          : '—'}
      </TableCell>
      <TableCell
        className='hidden text-right tabular-nums sm:table-cell'
        title={
          row.missingPrice ||
          row.source === 'observed' ||
          row.source === 'configured'
            ? undefined
            : t('priceCompare.effectiveCostPrecision')
        }
      >
        {row.missingPrice ||
        row.source === 'observed' ||
        row.source === 'configured'
          ? '—'
          : formatCurrency(row.estimatedCostSample, { fractionDigits: 6 })}
      </TableCell>
      <TableCell>
        <PriceRowStatus row={row} />
      </TableCell>
      <TableCell className='text-right'>
        <Button
          variant='ghost'
          size='icon-sm'
          render={<Link to='/token-routes' search={{ q: row.model }} />}
          aria-label={t('priceCompare.goToRoutes', { model: row.model })}
        >
          <ArrowRight aria-hidden='true' />
        </Button>
      </TableCell>
    </TableRow>
  )
}

function PriceRowStatus({ row }: { row: PriceCompareItem }) {
  const { t } = useTranslation()
  if (row.missingPrice) {
    return (
      <span className='text-muted-foreground inline-flex items-center gap-1 text-xs'>
        <Star aria-hidden='true' className='size-3 opacity-40' />
        {t('priceCompare.missingPrice')}
      </span>
    )
  }
  if (row.recommended && row.source === 'billing_details') {
    return (
      <Badge variant='default' title={t('priceCompare.recommendedHint')}>
        <Star aria-hidden='true' className='size-3!' />
        {t('priceCompare.recommended')}
      </Badge>
    )
  }
  return null
}
