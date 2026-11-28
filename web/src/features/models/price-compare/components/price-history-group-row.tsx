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
import {
  Table,
  TableBody,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatCurrency, formatPrice } from '@/lib/format'

import { PriceGradeBadge } from './price-grade-badge'
import type { ModelGroup } from './price-history-groups'
import { PriceRow } from './price-history-row'

export function ModelGroupRow({
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
