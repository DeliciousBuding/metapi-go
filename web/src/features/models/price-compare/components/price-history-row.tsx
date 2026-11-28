import { Link } from '@tanstack/react-router'
import { ArrowRight, Star } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { TableCell, TableRow } from '@/components/ui/table'
import { formatCurrency, formatPrice } from '@/lib/format'

import type { PriceCompareItem } from '../types'
import { PriceGradeBadge } from './price-grade-badge'

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
