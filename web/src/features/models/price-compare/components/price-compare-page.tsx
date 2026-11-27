// metapi-go/features/models/price-compare — standalone cross-site price page.
// Groups the backend's cheaper-first candidate rows by model into a dense,
// keyboard-operable RowDisclosure matrix: collapsed rows surface the winning
// site, provenance grade, lowest/average cost spread, and a direct route
// workspace link; expanding reveals every site's full input/output/sample cost
// breakdown without burying the viewport in tall cards.

import { Link, useNavigate, useSearch } from '@tanstack/react-router'
import { ArrowRight, Route, Scale, Search, Star } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ZodError } from 'zod'

import { ModelPill } from '@/components/common/model-pill'
import { PageHeader } from '@/components/common/page-header'
import { QueryErrorBanner } from '@/components/common/query-error-banner'
import {
  RowDisclosureContent,
  RowDisclosureTrigger,
  rowDisclosureClick,
} from '@/components/common/row-disclosure'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
  bestRow: PriceCompareItem
  minSample: number | null
  avgSample: number | null
  spreadPct: number | null
}

function summarizeGroup(key: string, rows: PriceCompareItem[]): ModelGroup {
  const bestRow =
    rows.find((row) => row.recommended && row.source === 'billing_details') ??
    rows[0]
  if (!bestRow) {
    throw new Error(`Expected non-empty price group for ${key}`)
  }
  const sampleCosts = rows
    .filter(
      (row) =>
        !row.missingPrice &&
        Number.isFinite(row.estimatedCostSample) &&
        row.estimatedCostSample > 0
    )
    .map((row) => row.estimatedCostSample)

  const minSample = sampleCosts.length > 0 ? Math.min(...sampleCosts) : null
  const avgSample =
    sampleCosts.length > 0
      ? sampleCosts.reduce((sum, value) => sum + value, 0) / sampleCosts.length
      : null
  const spreadPct =
    minSample !== null && avgSample !== null && avgSample > minSample
      ? Math.round(((avgSample - minSample) / avgSample) * 100)
      : null

  return {
    key,
    model: bestRow.model,
    rows,
    bestRow,
    minSample,
    avgSample,
    spreadPct,
  }
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

  const [expandedKeys, setExpandedKeys] = useState<ReadonlySet<string>>(
    () => new Set()
  )
  const toggleGroup = useCallback((key: string) => {
    setExpandedKeys((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }, [])

  const groups = useMemo<ModelGroup[]>(() => {
    const map = new Map<string, PriceCompareItem[]>()
    for (const item of query.data?.items ?? []) {
      if (item.missingPrice) continue
      const key = item.model.trim().toLowerCase() || '__unknown__'
      const list = map.get(key) ?? []
      list.push(item)
      map.set(key, list)
    }
    return [...map.entries()].map(([key, rows]) => summarizeGroup(key, rows))
  }, [query.data])

  const currentPage = query.data?.currentPage

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

      {currentPage && currentPage.totalModels > 0 && (
        <nav
          className='flex flex-wrap items-center justify-between gap-3'
          aria-label={t('priceCompare.current.pagination')}
        >
          <p className='text-muted-foreground text-sm tabular-nums'>
            {t('priceCompare.current.pageSummary', {
              page: currentPage.page,
              pages: Math.max(
                1,
                Math.ceil(currentPage.totalModels / currentPage.pageSize)
              ),
              count: currentPage.totalModels,
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
                    ? currentPage.page <= 1
                    : currentPage.page * currentPage.pageSize >=
                      currentPage.totalModels)
                }
                onClick={() => {
                  const params = new URLSearchParams(window.location.search)
                  params.set('pricePage', String(currentPage.page + delta))
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

      {groups.length > 0 && (
        <section className='flex flex-col gap-2.5'>
          <div className='flex flex-wrap items-baseline justify-between gap-2'>
            <h2 className='text-base font-semibold'>
              {t('priceCompare.current.history')}
            </h2>
            <p className='text-muted-foreground text-xs'>
              {t('priceCompare.group.description')}
            </p>
          </div>

          <div className='bg-card divide-y overflow-hidden rounded-xl border'>
            {groups.map((group) => (
              <ModelGroupRow
                key={group.key}
                group={group}
                expanded={expandedKeys.has(group.key)}
                onToggle={() => toggleGroup(group.key)}
              />
            ))}
          </div>
        </section>
      )}
    </div>
  )
}

function ModelGroupRow({
  group,
  expanded,
  onToggle,
}: {
  group: ModelGroup
  expanded: boolean
  onToggle: () => void
}) {
  const { t } = useTranslation()
  const contentId = `price-group-${group.key.replace(/[^a-z0-9_-]/gi, '-')}`
  const { bestRow, minSample, avgSample, spreadPct } = group

  return (
    <div className='min-w-0'>
      <div
        className='hover:bg-muted/40 flex cursor-pointer flex-wrap items-center gap-x-4 gap-y-2 px-3 py-2.5 transition-colors sm:flex-nowrap sm:px-4'
        onClick={(event) => rowDisclosureClick(event, onToggle)}
      >
        {/* Model identity + disclosure button */}
        <div className='flex min-w-0 flex-1 items-center gap-1.5 sm:flex-none sm:basis-64'>
          <RowDisclosureTrigger
            expanded={expanded}
            controls={contentId}
            className='max-w-full min-w-0 flex-1 px-1'
            onClick={onToggle}
          >
            <ModelPill
              model={group.model}
              variant='inline'
              className='text-sm'
            />
          </RowDisclosureTrigger>
        </div>

        {/* Winning site + provenance grade */}
        <div className='flex min-w-0 flex-1 flex-wrap items-center gap-2 text-xs'>
          <span className='truncate text-sm font-medium'>
            {bestRow.siteName}
          </span>
          <PriceGradeBadge grade={bestRow.source} />
          {group.rows.length > 1 && (
            <span className='text-muted-foreground tabular-nums'>
              · {group.rows.length}
            </span>
          )}
        </div>

        {/* Lowest vs average price spread */}
        <div className='flex flex-wrap items-center gap-2 text-xs tabular-nums sm:justify-end'>
          {bestRow.ratesSource === 'billing_details' && (
            <span className='font-medium'>
              {formatPrice(bestRow.inputPerMillion, { fractionDigits: 4 })} →{' '}
              {formatPrice(bestRow.outputPerMillion, { fractionDigits: 4 })}
            </span>
          )}
          {bestRow.ratesSource !== 'billing_details' && minSample !== null && (
            <span className='font-medium'>
              {formatCurrency(minSample, { fractionDigits: 6 })}
            </span>
          )}
          {bestRow.ratesSource !== 'billing_details' && minSample === null && (
            <span className='text-muted-foreground'>—</span>
          )}
          {avgSample !== null &&
            minSample !== null &&
            group.rows.length > 1 && (
              <span className='text-muted-foreground'>
                / {formatCurrency(avgSample, { fractionDigits: 6 })}
              </span>
            )}
          {spreadPct !== null && spreadPct > 0 && (
            <Badge variant='success' className='text-2xs tabular-nums'>
              -{spreadPct}%
            </Badge>
          )}
        </div>

        {/* Route workspace deep-link */}
        <div className='ml-auto flex shrink-0 items-center gap-1 sm:ml-0'>
          <Link
            to='/token-routes'
            search={{ workspaceModel: group.model }}
            aria-label={t('priceCompare.goToRoutes', { model: group.model })}
            className='border-border bg-background hover:bg-muted hover:text-foreground focus-visible:border-ring focus-visible:ring-focus-ring inline-flex h-7 shrink-0 items-center justify-center gap-1 rounded-md border px-2 text-xs font-medium whitespace-nowrap transition-colors outline-none focus-visible:ring-3'
          >
            <Route className='size-3.5' aria-hidden='true' />
            <span className='hidden sm:inline'>
              {t('tokenRoutes.workspace.title')}
            </span>
          </Link>
        </div>
      </div>

      {expanded && (
        <RowDisclosureContent
          id={contentId}
          label={group.model}
          className='bg-muted/20 border-t px-3 py-2.5 sm:px-4'
        >
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
        </RowDisclosureContent>
      )}
    </div>
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
