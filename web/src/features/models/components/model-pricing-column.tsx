import type { ColumnDef } from '@tanstack/react-table'
import type { TFunction } from 'i18next'

import { DataTableColumnHeader } from '@/components/data-table'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { formatPrice } from '@/lib/format'
import { cn } from '@/lib/utils'

import type { ModelRow } from '../types'

function resolveLowestInputPrice(model: ModelRow): number | null {
  let lowest: number | null = null
  for (const source of model.pricingSources ?? []) {
    for (const groupKey of Object.keys(source.groupPricing ?? {})) {
      const group = source.groupPricing[groupKey]
      const input = group?.inputPerMillion
      if (typeof input === 'number' && Number.isFinite(input)) {
        if (lowest === null || input < lowest) lowest = input
      }
    }
  }
  return lowest
}

function resolveLowestOutputPrice(model: ModelRow): number | null {
  let lowest: number | null = null
  for (const source of model.pricingSources ?? []) {
    for (const groupKey of Object.keys(source.groupPricing ?? {})) {
      const group = source.groupPricing[groupKey]
      const output = group?.outputPerMillion
      if (typeof output === 'number' && Number.isFinite(output)) {
        if (lowest === null || output < lowest) lowest = output
      }
    }
  }
  return lowest
}

function resolveHighestInputPrice(model: ModelRow): number | null {
  let highest: number | null = null
  for (const source of model.pricingSources ?? []) {
    for (const groupKey of Object.keys(source.groupPricing ?? {})) {
      const group = source.groupPricing[groupKey]
      const input = group?.inputPerMillion
      if (typeof input === 'number' && Number.isFinite(input)) {
        if (highest === null || input > highest) highest = input
      }
    }
  }
  return highest
}

function resolvePriceDetail(model: ModelRow): Array<{
  site: string
  siteId: number
  price: number
  outputPrice: number | null
}> {
  const rows: Array<{
    site: string
    siteId: number
    price: number
    outputPrice: number | null
  }> = []
  for (const source of model.pricingSources ?? []) {
    let lowest: number | null = null
    let matchedOutput: number | null = null
    for (const groupKey of Object.keys(source.groupPricing ?? {})) {
      const group = source.groupPricing[groupKey]
      const input = group?.inputPerMillion
      if (typeof input === 'number' && Number.isFinite(input)) {
        if (lowest === null || input < lowest) {
          lowest = input
          matchedOutput =
            typeof group?.outputPerMillion === 'number' &&
            Number.isFinite(group.outputPerMillion)
              ? group.outputPerMillion
              : null
        }
      }
    }
    if (lowest !== null) {
      rows.push({
        site: source.siteName || `#${source.siteId}`,
        siteId: source.siteId,
        price: lowest,
        outputPrice: matchedOutput,
      })
    }
  }
  rows.sort((a, b) => a.price - b.price)
  return rows
}

/** Localized label for a pricing-source site (the synthetic catalog source
 * uses siteId 0 + siteName "catalog" as its sentinel). */
export function pricingSiteLabel(
  site: string,
  siteId: number,
  t: (key: string) => string
): string {
  if (siteId === 0 && site === 'catalog') {
    return t('models.price.catalogEstimate')
  }
  return site
}

export function modelPricingColumn(t: TFunction): ColumnDef<ModelRow> {
  return {
    id: 'priceInput',
    accessorFn: (row) => resolveLowestInputPrice(row),
    size: 120,
    meta: { mobileOrder: 50, label: t('models.columns.priceInput') },
    header: ({ column }) => (
      <DataTableColumnHeader
        column={column}
        title={t('models.columns.priceInput')}
      />
    ),
    cell: ({ row }) => {
      const price = resolveLowestInputPrice(row.original)
      if (price === null) {
        return <span className='text-muted-foreground text-sm'>—</span>
      }
      const highestInput = resolveHighestInputPrice(row.original)
      const lowestOutput = resolveLowestOutputPrice(row.original)
      const detail = resolvePriceDetail(row.original)
      const hasRange = highestInput !== null && highestInput > price
      const priceLabel = hasRange
        ? `${formatPrice(price)}–${formatPrice(highestInput)}/M`
        : `${formatPrice(price)}/M`
      const triggerNode = (
        <div className='flex flex-col gap-0.5 tabular-nums'>
          <span
            className={cn(
              'text-sm font-medium',
              detail.length > 0 &&
                'underline decoration-dotted underline-offset-2'
            )}
          >
            {priceLabel}
          </span>
          {lowestOutput !== null && (
            <span className='text-muted-foreground text-2xs'>
              → {formatPrice(lowestOutput)}/M
            </span>
          )}
        </div>
      )
      if (detail.length === 0) {
        return triggerNode
      }
      return (
        <TooltipProvider delay={200}>
          <Tooltip>
            <TooltipTrigger render={<span className='inline-flex' />}>
              {triggerNode}
            </TooltipTrigger>
            <TooltipContent side='top' className='max-w-xs'>
              <div className='flex flex-col gap-1'>
                <span className='font-medium'>
                  {t('models.columns.priceDetail')}
                </span>
                {detail.map((item) => (
                  <span
                    key={`${item.siteId}-${item.site}`}
                    className='text-xs tabular-nums'
                  >
                    {pricingSiteLabel(item.site, item.siteId, t)}:{' '}
                    {formatPrice(item.price)}/M
                    {item.outputPrice !== null
                      ? ` → ${formatPrice(item.outputPrice)}/M`
                      : ''}
                  </span>
                ))}
              </div>
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      )
    },
  }
}
