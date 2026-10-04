// metapi-go/data-table — MobileCardList: the narrow-viewport rendering of a table.
//
// Not a grid of cards: rows sit in one bordered container separated by dividers,
// because at phone width the gaps between separate cards cost more vertical space
// than they add clarity. Per-row content is `CardRowContent`, shared with the
// desktop card grid so the two cannot drift.
//
// Selection stays on the explicit checkbox. A tap on the card surface must not
// select a row (or expose bulk actions) at the end of a touch scroll.
import type { Table } from '@tanstack/react-table'
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

import { renderCellContent, tableHasCompactMeta } from './card-cell-utils'
import { CardRowContent } from './card-row-content'

interface MobileCardListProps<TData> {
  table: Table<TData>
  isLoading?: boolean
  emptyTitle?: string
  emptyDescription?: string
  emptyAction?: React.ReactNode
  /** Per-entity empty-state icon (defaults to a generic database glyph). */
  emptyIcon?: React.ReactNode
}

/** The bordered, divided shell both skeletons share with the real list. */
function SkeletonList({ children }: { children: React.ReactNode }) {
  return (
    <div className='divide-y overflow-hidden rounded-lg border'>{children}</div>
  )
}

const SKELETON_ROWS = [1, 2, 3, 4, 5]

function ListSkeleton() {
  return (
    <SkeletonList>
      {SKELETON_ROWS.map((i) => (
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
    </SkeletonList>
  )
}

function FallbackListSkeleton() {
  return (
    <SkeletonList>
      {SKELETON_ROWS.map((i) => (
        <div key={i} className='space-y-1.5 px-3 py-2.5'>
          {[1, 2, 3].map((j) => (
            <div key={j} className='flex items-center justify-between'>
              <Skeleton className='h-2.5 w-16' />
              <Skeleton className='h-3.5 w-28' />
            </div>
          ))}
        </div>
      ))}
    </SkeletonList>
  )
}

export function MobileCardList<TData>(props: MobileCardListProps<TData>) {
  const {
    table,
    isLoading = false,
    emptyTitle,
    emptyDescription,
    emptyAction,
    emptyIcon,
  } = props
  const { t } = useTranslation()

  const resolvedEmptyTitle = emptyTitle ?? t('No Data')
  const resolvedEmptyDescription = emptyDescription ?? t('No data available')

  // No memo: `getVisibleLeafColumns()` returns a fresh array each render, so a
  // dependency on it invalidates every time and the memo only ever added an
  // eslint-disable. The check itself is a `.some()` over the visible columns,
  // which TanStack already caches.
  const hasCompactMeta = tableHasCompactMeta(table)

  if (isLoading) {
    return hasCompactMeta ? <ListSkeleton /> : <FallbackListSkeleton />
  }

  const rows = table.getRowModel().rows
  const hasSelectColumn = table
    .getVisibleLeafColumns()
    .some((column) => column.id === 'select')

  if (rows.length === 0) {
    return (
      <div className='rounded-lg border p-6'>
        <Empty className='border-none p-0'>
          <EmptyHeader>
            <EmptyMedia variant='icon'>
              {emptyIcon ?? <Database className='size-6' />}
            </EmptyMedia>
            <EmptyTitle>{resolvedEmptyTitle}</EmptyTitle>
            <EmptyDescription>{resolvedEmptyDescription}</EmptyDescription>
          </EmptyHeader>
          {emptyAction}
        </Empty>
      </div>
    )
  }

  return (
    <div className='divide-y overflow-hidden rounded-lg border'>
      {rows.map((row) => {
        const isSelected = row.getIsSelected()
        const selectCell = hasSelectColumn
          ? row.getVisibleCells().find((cell) => cell.column.id === 'select')
          : undefined
        return (
          <div
            key={row.id}
            data-mobile-card-row
            data-state={isSelected ? 'selected' : undefined}
            className='[background-color:var(--data-table-card-bg,var(--table-row))] px-3 py-2.5 transition-colors data-[state=selected]:bg-(--table-row-selected-bg)'
          >
            <div className='flex items-start gap-1.5'>
              {selectCell && (
                <div className='flex size-7 shrink-0 items-center justify-center [&_[data-slot=checkbox]]:after:-inset-3'>
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
