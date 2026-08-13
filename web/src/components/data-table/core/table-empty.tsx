// metapi-go/data-table — ported from newapi
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

interface TableEmptyProps {
  /**
   * Number of columns to span
   */
  colSpan: number
  /**
   * Custom title for empty state
   * @default 'No Data'
   */
  title?: string
  /**
   * Custom description for empty state
   * @default 'No records found. Try adjusting your filters.'
   */
  description?: string
  /**
   * Custom icon component
   * @default Database icon
   */
  icon?: React.ReactNode
  /**
   * Additional content to display (e.g., buttons)
   */
  children?: React.ReactNode
  /**
   * Whether the table is empty because filters are active (vs truly no data).
   * Switches to the "no results" copy + a reset-filters CTA.
   */
  isFiltered?: boolean
  /**
   * Custom title when the table is filtered-empty.
   */
  filteredTitle?: string
  /**
   * Custom description when the table is filtered-empty.
   */
  filteredDescription?: string
  /**
   * Clears active filters (column + global). Rendered as a "Reset filters" CTA.
   */
  onClearFilters?: () => void
}

/**
 * Generic table empty state component.
 * Distinguishes "no data" from "no results after filtering" so users know
 * whether to create something or clear filters.
 */
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
