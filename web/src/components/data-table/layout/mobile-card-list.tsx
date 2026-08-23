// metapi-go/data-table — ported from newapi
import type { Row, Table } from '@tanstack/react-table'
import { Database } from 'lucide-react'
import * as React from 'react'
import { useTranslation } from 'react-i18next'

import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

import { renderCellContent, tableHasCompactMeta } from './card-cell-utils'
import { CardRowContent } from './card-row-content'

interface MobileCardListProps<TData> {
  table: Table<TData>
  isLoading?: boolean
  emptyTitle?: string
  emptyDescription?: string
  getRowKey?: (row: Row<TData>) => string | number
  getRowClassName?: (row: Row<TData>) => string | undefined
}

function ListSkeleton() {
  return (
    <div className='divide-y overflow-hidden rounded-lg border'>
      {[1, 2, 3, 4, 5].map((i) => (
        <div key={i} className='px-3 py-2.5'>
          <div className='flex items-center justify-between'>
            <Skeleton className='h-4 w-32' />
            <Skeleton className='h-5 w-16 rounded-md' />
          </div>
          <div className='mt-1.5 grid grid-cols-2 gap-2'>
            <div className='flex-1'>
              <Skeleton className='mb-1 h-2 w-8' />
              <Skeleton className='h-4 w-full' />
            </div>
            <div className='flex-1'>
              <Skeleton className='mb-1 h-2 w-8' />
              <Skeleton className='h-4 w-full' />
            </div>
          </div>
        </div>
      ))}
    </div>
  )
}

function FallbackListSkeleton() {
  return (
    <div className='divide-y overflow-hidden rounded-lg border'>
      {[1, 2, 3, 4, 5].map((i) => (
        <div key={i} className='space-y-1.5 px-3 py-2.5'>
          {[1, 2, 3].map((j) => (
            <div key={j} className='flex items-center justify-between'>
              <Skeleton className='h-2.5 w-16' />
              <Skeleton className='h-3.5 w-28' />
            </div>
          ))}
        </div>
      ))}
    </div>
  )
}

/**
 * Mobile-optimized list view for table data.
 *
 * Renders rows inside a single bordered container with dividers —
 * a Vercel/Stripe-style list rather than individual cards.
 *
 * Per-row content is shared with the desktop card view via
 * {@link CardRowContent}; see `card-row-content.tsx` for the column-meta
 * extensions (`mobileTitle`, `mobileBadge`, `mobileHidden`).
 */
export function MobileCardList<TData>(props: MobileCardListProps<TData>) {
  const {
    table,
    isLoading = false,
    emptyTitle,
    emptyDescription,
    getRowKey,
    getRowClassName,
  } = props
  const { t } = useTranslation()

  const resolvedEmptyTitle = emptyTitle ?? t('No Data')
  const resolvedEmptyDescription = emptyDescription ?? t('No data available')

  const visibleColumns = table.getVisibleLeafColumns()
  const hasCompactMeta = React.useMemo(
    () => tableHasCompactMeta(table),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [visibleColumns]
  )

  if (isLoading) {
    return hasCompactMeta ? <ListSkeleton /> : <FallbackListSkeleton />
  }

  const rows = table.getRowModel().rows
  const hasSelectColumn = table
    .getVisibleLeafColumns()
    .some((column) => column.id === 'select')

  if (!rows || rows.length === 0) {
    return (
      <div className='rounded-lg border p-6'>
        <Empty className='border-none p-0'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              <Database className='size-6' />
            </EmptyMedia>
            <EmptyTitle>{resolvedEmptyTitle}</EmptyTitle>
            <EmptyDescription>{resolvedEmptyDescription}</EmptyDescription>
          </EmptyHeader>
        </Empty>
      </div>
    )
  }

  return (
    <div className='divide-y overflow-hidden rounded-lg border'>
      {rows.map((row) => {
        const key = getRowKey ? getRowKey(row) : row.id
        const isSelected = row.getIsSelected()
        const selectCell = hasSelectColumn
          ? row.getVisibleCells().find((cell) => cell.column.id === 'select')
          : undefined
        return (
          <div
            key={key}
            data-state={isSelected ? 'selected' : undefined}
            className={cn(
              '[background-color:var(--data-table-card-bg,var(--table-row))] px-3 py-2.5 transition-colors data-[state=selected]:bg-(--table-row-selected-bg)',
              getRowClassName?.(row)
            )}
            onClick={
              selectCell
                ? (event) => {
                    const target = event.target as HTMLElement
                    if (
                      target.closest(
                        'button, a, input, [data-slot=checkbox], [data-slot=dropdown-menu-content], [role=menuitem]'
                      )
                    ) {
                      return
                    }
                    row.toggleSelected()
                  }
                : undefined
            }
          >
            <div className='flex items-start gap-1.5'>
              {selectCell && (
                <div className='shrink-0 pt-0.5'>
                  {renderCellContent(selectCell)}
                </div>
              )}
              <div className='min-w-0 flex-1'>
                <CardRowContent row={row} compact={hasCompactMeta} />
              </div>
            </div>
          </div>
        )
      })}
    </div>
  )
}
