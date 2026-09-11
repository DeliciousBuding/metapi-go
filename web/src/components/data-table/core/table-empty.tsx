// metapi-go/data-table — TableEmpty: the empty body, and the difference between
// "there is nothing" and "you filtered everything out".
//
// Those two states need opposite actions — create something, versus clear the
// filters — so the copy, the icon and the CTA all switch on `isFiltered`.
// Collapsing them into one "no data" message is how a user ends up believing a
// table is empty when it is only narrowed.
//
// It renders as a `<TableRow>` with one cell spanning the table, not as a
// sibling of the table: the empty state has to sit inside the same scroll
// container and column geometry, or a horizontally scrolled table shows the
// message half off-screen.
import { Database, SearchX } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'
import { TableRow, TableCell } from '@/components/ui/table'

type TableEmptyProps = {
  /** Columns to span — the visible leaf count, so the cell covers the table. */
  colSpan: number
  /** Overrides for the "nothing here" state. */
  title?: string
  description?: string
  /** Defaults to a database glyph. Ignored when `isFiltered`. */
  icon?: React.ReactNode
  /** Extra content under the copy, e.g. a create button. */
  children?: React.ReactNode
  /** True when the table is empty because filters are active, not because there is no data. */
  isFiltered?: boolean
  /** Overrides for the "filtered to nothing" state. */
  filteredTitle?: string
  filteredDescription?: string
  /** Clears column + global filters; rendered as the reset CTA. */
  onClearFilters?: () => void
}

export function TableEmpty({
  colSpan,
  title,
  description,
  icon,
  children,
  isFiltered = false,
  filteredTitle,
  filteredDescription,
  onClearFilters,
}: TableEmptyProps) {
  const { t } = useTranslation()
  const resolvedTitle = isFiltered
    ? (filteredTitle ?? t('common.noResults'))
    : (title ?? t('No Data'))
  const resolvedDescription = isFiltered
    ? (filteredDescription ?? t('common.noResultsDescription'))
    : (description ?? t('No records found. Try adjusting your filters.'))
  const resolvedIcon = isFiltered ? (
    <SearchX className='size-6' />
  ) : (
    icon || <Database className='size-6' />
  )

  return (
    <TableRow>
      <TableCell colSpan={colSpan} className='h-[400px] p-0'>
        <Empty>
          <EmptyHeader>
            <EmptyMedia variant='icon'>{resolvedIcon}</EmptyMedia>
            <EmptyTitle>{resolvedTitle}</EmptyTitle>
            <EmptyDescription>{resolvedDescription}</EmptyDescription>
          </EmptyHeader>
          {isFiltered && onClearFilters ? (
            <Button variant='outline' size='sm' onClick={onClearFilters}>
              {t('common.resetFilters')}
            </Button>
          ) : (
            children
          )}
        </Empty>
      </TableCell>
    </TableRow>
  )
}
