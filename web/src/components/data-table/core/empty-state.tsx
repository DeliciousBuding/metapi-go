import { Database, SearchX } from 'lucide-react'
import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from '@/components/ui/empty'

type Props = {
  title?: string
  description?: string
  icon?: ReactNode
  children?: ReactNode
  isFiltered?: boolean
  onClearFilters?: () => void
}

// Both responsive table presentations share the same empty-state action.
export function DataTableEmptyState({
  title,
  description,
  icon,
  children,
  isFiltered = false,
  onClearFilters,
}: Props) {
  const { t } = useTranslation()
  const resolvedTitle = isFiltered
    ? t('common.noResults')
    : (title ?? t('No Data'))
  const resolvedDescription = isFiltered
    ? t('common.noResultsDescription')
    : (description ?? t('No records found. Try adjusting your filters.'))
  const resolvedIcon = isFiltered ? (
    <SearchX className='size-6' />
  ) : (
    (icon ?? <Database className='size-6' />)
  )

  return (
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
  )
}
