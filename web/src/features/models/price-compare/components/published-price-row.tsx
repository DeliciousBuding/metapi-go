import { Link } from '@tanstack/react-router'
import { Route } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { ModelPill } from '@/components/common/model-pill'
import {
  RowDisclosureContent,
  RowDisclosureTrigger,
  rowDisclosureClick,
} from '@/components/common/row-disclosure'
import { Badge } from '@/components/ui/badge'
import { toBcp47 } from '@/i18n/languages'
import { formatDateTime } from '@/lib/format'

import type { CurrentPrice } from '../types'
import { AccountModelPrice } from './account-model-price'
import { dimensions, price } from './current-price-format'

function officialRateClass(
  dimension: (typeof dimensions)[number],
  value: number | null | undefined
): string {
  if (value == null) return 'mt-1 text-sm font-medium tabular-nums'
  if (dimension === 'cacheRead') {
    return 'text-info-soft-fg mt-1 text-sm font-medium tabular-nums'
  }
  if (dimension === 'cacheWrite') {
    return 'text-warning-soft-fg mt-1 text-sm font-medium tabular-nums'
  }
  return 'mt-1 text-sm font-medium tabular-nums'
}

function resolveBestPublishedQuote(rows: CurrentPrice[]): {
  siteName: string
  input: number | null
  output: number | null
  cacheRead: number | null
  sampleCost: number | null
} | null {
  let best: {
    siteName: string
    input: number | null
    output: number | null
    cacheRead: number | null
    sampleCost: number | null
    score: number
  } | null = null

  for (const row of rows) {
    for (const group of row.groups) {
      const score =
        group.sampleCost ??
        group.rates.input ??
        group.rates.perRequest ??
        Number.POSITIVE_INFINITY
      if (!best || score < best.score) {
        best = {
          siteName: row.siteName,
          input: group.rates.input,
          output: group.rates.output,
          cacheRead: group.rates.cacheRead,
          sampleCost: group.sampleCost,
          score,
        }
      }
    }
  }
  return best
}

export function PublishedPriceRow({
  model,
  rows,
  official,
  isExpanded,
  onToggle,
}: {
  model: string
  rows: CurrentPrice[]
  official: CurrentPrice['official'] | undefined
  isExpanded: boolean
  onToggle: () => void
}) {
  const { t, i18n } = useTranslation()
  const bestQuote = resolveBestPublishedQuote(rows)
  const contentId = `published-price-${model.replace(/[^a-z0-9_-]/gi, '-')}`

  return (
    <div className='min-w-0'>
      <div
        className='hover:bg-muted/40 flex cursor-pointer flex-wrap items-center gap-x-4 gap-y-2 px-3 py-2.5 transition-colors sm:flex-nowrap sm:px-4'
        onClick={(event) => rowDisclosureClick(event, onToggle)}
      >
        <div className='flex min-w-0 flex-1 items-center gap-1.5 sm:flex-none sm:basis-64'>
          <RowDisclosureTrigger
            expanded={isExpanded}
            controls={contentId}
            className='max-w-full min-w-0 flex-1 px-1'
            onClick={onToggle}
          >
            <ModelPill model={model} variant='inline' className='text-sm' />
          </RowDisclosureTrigger>
        </div>

        <div className='flex min-w-0 flex-1 flex-wrap items-center gap-2 text-xs'>
          {bestQuote ? (
            <>
              <span className='truncate text-sm font-medium'>
                {bestQuote.siteName}
              </span>
              <Badge variant='secondary' className='text-2xs'>
                {rows.length}
              </Badge>
            </>
          ) : (
            <span className='text-muted-foreground'>
              {t('priceCompare.current.noQuote')}
            </span>
          )}
        </div>

        <div className='flex flex-wrap items-center gap-2 text-xs tabular-nums sm:justify-end'>
          {bestQuote && bestQuote.input != null && (
            <span className='font-medium'>
              {price(bestQuote.input)} → {price(bestQuote.output)}
            </span>
          )}
          {(!bestQuote || bestQuote.input == null) &&
            official?.rates.input != null && (
              <span className='text-muted-foreground'>
                {price(official.rates.input)} → {price(official.rates.output)}
              </span>
            )}
          {(!bestQuote || bestQuote.input == null) &&
            official?.rates.input == null && (
              <span className='text-muted-foreground'>—</span>
            )}
          {bestQuote?.cacheRead != null && (
            <Badge variant='info' className='text-2xs tabular-nums'>
              {t('priceCompare.current.cacheRead')} {price(bestQuote.cacheRead)}
            </Badge>
          )}
        </div>

        <div className='ml-auto flex shrink-0 items-center sm:ml-0'>
          <Link
            to='/token-routes'
            search={{ workspaceModel: model }}
            aria-label={t('priceCompare.goToRoutes', { model })}
            className='border-border bg-background hover:bg-muted hover:text-foreground focus-visible:border-ring focus-visible:ring-focus-ring inline-flex h-7 shrink-0 items-center justify-center gap-1 rounded-md border px-2 text-xs font-medium whitespace-nowrap transition-colors outline-none focus-visible:ring-3'
          >
            <Route className='size-3.5' aria-hidden='true' />
            <span className='hidden sm:inline'>
              {t('tokenRoutes.workspace.title')}
            </span>
          </Link>
        </div>
      </div>

      {isExpanded && (
        <RowDisclosureContent
          id={contentId}
          label={model}
          className='bg-muted/20 space-y-4 border-t px-3 py-3 sm:px-4'
        >
          <div className='bg-background space-y-3 rounded-lg border p-3'>
            <div className='flex flex-wrap justify-between gap-2'>
              <h3 className='text-sm font-medium'>
                {t('priceCompare.current.official')}
              </h3>
              <span className='text-muted-foreground text-xs'>
                {official?.source.startsWith('built-in presets')
                  ? t('priceCompare.current.builtIn', {
                      date:
                        official.source.match(/\d{4}-\d{2}-\d{2}/)?.[0] ?? '—',
                    })
                  : official?.source || t('priceCompare.current.noBaseline')}
              </span>
            </div>
            <div className='grid grid-cols-2 gap-3 sm:grid-cols-4'>
              {dimensions.map((d) => (
                <div key={d}>
                  <div className='text-muted-foreground text-xs'>
                    {t(`priceCompare.current.${d}`)}
                  </div>
                  <div className={officialRateClass(d, official?.rates[d])}>
                    {price(official?.rates[d])}
                  </div>
                </div>
              ))}
            </div>
            {official && (
              <p className='text-muted-foreground text-xs break-words'>
                {official.provider} / {official.model}
                {official.aliased && ` · ${t('priceCompare.current.alias')}`}
                {official.fetchedAt &&
                  ` · ${formatDateTime(official.fetchedAt, toBcp47(i18n.language))}`}
                {official.updatedAt &&
                  ` · ${t('priceCompare.current.catalogUpdated', { date: official.updatedAt })}`}
              </p>
            )}
          </div>

          {rows.length === 0 ? (
            <p className='text-muted-foreground text-sm'>
              {t('priceCompare.current.noQuote')}
            </p>
          ) : (
            <div className='space-y-4'>
              {rows.map((row) => (
                <AccountModelPrice
                  key={`${row.accountId}-${row.model}`}
                  row={row}
                />
              ))}
            </div>
          )}
        </RowDisclosureContent>
      )}
    </div>
  )
}
