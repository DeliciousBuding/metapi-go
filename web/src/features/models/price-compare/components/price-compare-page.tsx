// metapi-go/features/models/price-compare — standalone cross-site price page.
// Groups the backend's cheaper-first candidate rows by model and surfaces the
// provenance grade + best-channel recommendation for every source.

import { Link } from '@tanstack/react-router'
import { ArrowRight, Search, Star } from 'lucide-react'
import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

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
import { Empty, EmptyDescription, EmptyHeader } from '@/components/ui/empty'
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

import { usePriceCompare } from '../api'
import type { PriceCompareItem } from '../types'
import { PriceGradeBadge } from './price-grade-badge'

type ModelGroup = {
  key: string
  model: string
  rows: PriceCompareItem[]
}

export function PriceComparePage() {
  const { t } = useTranslation()
  const [search, setSearch] = useState('')
  const [modelParam, setModelParam] = useState('')

  useEffect(() => {
    const timer = window.setTimeout(() => {
      setModelParam(search.trim())
    }, 350)
    return () => window.clearTimeout(timer)
  }, [search])

  const query = usePriceCompare({
    model: modelParam || undefined,
    limit: 200,
  })

  const groups = useMemo<ModelGroup[]>(() => {
    const map = new Map<string, PriceCompareItem[]>()
    for (const item of query.data ?? []) {
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
    <div className='flex h-full flex-col gap-3 p-4'>
      <div className='flex flex-wrap items-end justify-between gap-3'>
        <div>
          <h1 className='text-lg font-normal'>
            {t('priceCompare.page.title')}
          </h1>
          <p className='text-muted-foreground text-sm'>
            {t('priceCompare.page.description')}
          </p>
        </div>
        <div className='relative w-full sm:w-64'>
          <Search
            aria-hidden='true'
            className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2'
          />
          <Input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder={t('priceCompare.page.searchPlaceholder')}
            aria-label={t('priceCompare.page.searchPlaceholder')}
            className='pl-8'
          />
        </div>
      </div>

      {query.isLoading && (
        <div className='text-muted-foreground flex items-center gap-2 text-sm'>
          <Spinner />
          {t('common.loading')}
        </div>
      )}

      <QueryErrorBanner
        error={query.error as Error | null}
        messageKey='priceCompare.page.loadError'
        onRetry={() => query.refetch()}
        isRetrying={query.isFetching}
      />

      {!query.isLoading && !query.error && groups.length === 0 && (
        <Empty className='border'>
          <EmptyHeader>
            <EmptyDescription>
              {t('priceCompare.page.emptyDescription')}
            </EmptyDescription>
          </EmptyHeader>
        </Empty>
      )}

      <div className='flex flex-col gap-4'>
        {groups.map((group) => (
          <ModelGroupCard key={group.key} group={group} />
        ))}
      </div>
    </div>
  )
}

function ModelGroupCard({ group }: { group: ModelGroup }) {
  const { t } = useTranslation()
  const hasRecommended = group.rows.some((row) => row.recommended)

  return (
    <Card>
      <CardHeader>
        <CardTitle className='flex items-center gap-2 text-base font-normal'>
          <span className='font-mono'>{group.model}</span>
          {hasRecommended && (
            <Badge
              variant='secondary'
              title={t('priceCompare.recommendedHint')}
            >
              <Star aria-hidden='true' className='size-3!' />
              {t('priceCompare.recommended')}
            </Badge>
          )}
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
      </TableCell>
      <TableCell className='hidden sm:table-cell'>
        <PriceGradeBadge grade={row.source} />
      </TableCell>
      <TableCell className='text-right tabular-nums'>
        {formatPrice(row.inputPerMillion, { fractionDigits: 4 })}
      </TableCell>
      <TableCell className='hidden text-right tabular-nums sm:table-cell'>
        {formatPrice(row.outputPerMillion, { fractionDigits: 4 })}
      </TableCell>
      <TableCell
        className='hidden text-right tabular-nums sm:table-cell'
        title={
          row.missingPrice
            ? undefined
            : t('priceCompare.effectiveCostPrecision')
        }
      >
        {row.missingPrice
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
  if (row.recommended) {
    return (
      <Badge variant='default' title={t('priceCompare.recommendedHint')}>
        <Star aria-hidden='true' className='size-3!' />
        {t('priceCompare.recommended')}
      </Badge>
    )
  }
  return null
}
