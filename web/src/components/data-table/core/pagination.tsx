// Container queries follow the table's available width, including side panels.
// Narrow footers show a page summary; wide ones offer direct page selection.

import type { Table } from '@tanstack/react-table'
import {
  ChevronLeft as ChevronLeftIcon,
  ChevronRight as ChevronRightIcon,
  ChevronsLeft as DoubleArrowLeftIcon,
  ChevronsRight as DoubleArrowRightIcon,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn, getPageNumbers } from '@/lib/utils'

/** Page sizes offered in the selector. */
const PAGE_SIZE_OPTIONS = [10, 20, 30, 40, 50, 100] as const

/** `items` gives the Select its value→label map, so it can render the current
 *  size even before the list is opened. */
const PAGE_SIZE_SELECT_ITEMS = PAGE_SIZE_OPTIONS.map((pageSize) => ({
  value: `${pageSize}`,
  label: pageSize,
}))

const STEP_BUTTON_CLASS =
  'text-muted-foreground hover:text-foreground disabled:text-muted-foreground/50 size-8 p-0'
const EDGE_BUTTON_CLASS = `${STEP_BUTTON_CLASS} @max-xl/pagination:hidden`

type NavButtonProps = {
  className: string
  disabled: boolean
  icon: LucideIcon
  label: string
  onClick: () => void
}

function NavButton({
  className,
  disabled,
  icon: Icon,
  label,
  onClick,
}: NavButtonProps) {
  return (
    <Button
      variant='outline'
      data-touch-target='square'
      className={className}
      onClick={onClick}
      disabled={disabled}
    >
      <span className='sr-only'>{label}</span>
      <Icon className='h-4 w-4' />
    </Button>
  )
}

export function DataTablePagination<TData>({ table }: { table: Table<TData> }) {
  const { t } = useTranslation()
  const { pageIndex, pageSize } = table.getState().pagination

  // Nothing to page through: a zero-row first page renders no footer at all
  // ("Total: 0" plus dead nav buttons is pure noise). A zero-row LATER page
  // keeps the bar so the user can navigate back.
  if (table.getRowCount() === 0 && pageIndex === 0) {
    return null
  }

  const currentPage = pageIndex + 1
  const totalPages = table.getPageCount()
  const pageNumbers = getPageNumbers(currentPage, totalPages)

  return (
    <nav
      aria-label={t('Pagination')}
      data-slot='data-table-pagination'
      className='@container/pagination min-w-0'
    >
      <div className='flex min-w-0 flex-wrap items-center justify-end gap-x-3 gap-y-2'>
        <div className='flex flex-wrap items-center gap-3'>
          <div className='flex shrink-0 items-baseline gap-1.5 text-xs font-medium whitespace-nowrap sm:text-sm'>
            <span className='text-muted-foreground'>{t('Total:')}</span>
            <span className='text-foreground tabular-nums'>
              {table.getRowCount().toLocaleString()}
            </span>
          </div>

          <div className='flex shrink-0 items-center gap-1.5 @lg/pagination:gap-2'>
            <p className='text-muted-foreground hidden text-sm font-medium whitespace-nowrap @2xl/pagination:block'>
              {t('Rows per page')}
            </p>
            <Select
              items={PAGE_SIZE_SELECT_ITEMS}
              value={`${pageSize}`}
              onValueChange={(value) => table.setPageSize(Number(value))}
            >
              <SelectTrigger
                data-touch-target
                aria-label={t('Rows per page')}
                className='text-foreground h-8 w-16 font-medium tabular-nums sm:w-18'
              >
                <SelectValue placeholder={pageSize} />
              </SelectTrigger>
              <SelectContent side='top' alignItemWithTrigger={false}>
                <SelectGroup>
                  {PAGE_SIZE_OPTIONS.map((option) => (
                    <SelectItem key={option} value={`${option}`}>
                      {option}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
        </div>

        <div className='flex min-w-0 flex-wrap items-center justify-end gap-1 @lg/pagination:gap-1.5 @xl/pagination:gap-2'>
          <NavButton
            className={EDGE_BUTTON_CLASS}
            disabled={!table.getCanPreviousPage()}
            icon={DoubleArrowLeftIcon}
            label={t('Go to first page')}
            onClick={() => table.setPageIndex(0)}
          />
          <NavButton
            className={STEP_BUTTON_CLASS}
            disabled={!table.getCanPreviousPage()}
            icon={ChevronLeftIcon}
            label={t('Go to previous page')}
            onClick={() => table.previousPage()}
          />

          <span
            role='status'
            className='text-muted-foreground px-1 text-xs whitespace-nowrap tabular-nums @xl/pagination:hidden'
          >
            {t('Page {{current}} of {{total}}', {
              current: currentPage,
              total: totalPages,
            })}
          </span>

          {pageNumbers.map((pageNumber, index) => (
            // `getPageNumbers` can emit more than one `'...'` gap, so the index is
            // part of the key: without it the two gaps collide.
            // eslint-disable-next-line react/no-array-index-key
            <div
              key={`${pageNumber}-${index}`}
              className='hidden items-center @xl/pagination:flex'
            >
              {pageNumber === '...' ? (
                <span className='text-muted-foreground/60 px-0.5 text-sm @lg/pagination:px-1'>
                  ...
                </span>
              ) : (
                <Button
                  data-touch-target='square'
                  variant={currentPage === pageNumber ? 'default' : 'outline'}
                  aria-current={currentPage === pageNumber ? 'page' : undefined}
                  className={cn(
                    'h-8 min-w-8 px-2 tabular-nums',
                    currentPage === pageNumber
                      ? 'font-semibold'
                      : 'text-muted-foreground hover:text-foreground'
                  )}
                  onClick={() => table.setPageIndex((pageNumber as number) - 1)}
                >
                  <span className='sr-only'>
                    {t('Go to page {{page}}', { page: pageNumber })}
                  </span>
                  {pageNumber}
                </Button>
              )}
            </div>
          ))}

          <NavButton
            className={STEP_BUTTON_CLASS}
            disabled={!table.getCanNextPage()}
            icon={ChevronRightIcon}
            label={t('Go to next page')}
            onClick={() => table.nextPage()}
          />
          <NavButton
            className={EDGE_BUTTON_CLASS}
            disabled={!table.getCanNextPage()}
            icon={DoubleArrowRightIcon}
            label={t('Go to last page')}
            onClick={() => table.setPageIndex(totalPages - 1)}
          />
        </div>
      </div>
    </nav>
  )
}
