// metapi-go/data-table — DataTablePagination: the table footer's page controls.
//
// Right-aligned and container-query driven (`@container/pagination`) rather than
// viewport driven, because the table is not the whole viewport: it sits inside a
// sidebar-inset panel that can be narrowed independently of the window. Labels
// and the first/last buttons drop out as the *container* tightens, so the page
// numbers — the part that actually navigates — are the last thing to go.
//
// Every icon button carries an `sr-only` label; the glyphs alone do not name the
// action, and "chevron left" is not "previous page".

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
/** The first/last pair is the first thing dropped when the footer gets tight. */
const EDGE_BUTTON_CLASS = `${STEP_BUTTON_CLASS} @max-lg/pagination:hidden`

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
  const currentPage = pageIndex + 1
  const totalPages = table.getPageCount()
  const pageNumbers = getPageNumbers(currentPage, totalPages)

  return (
    // `overflow-clip` keeps a long page-number run from stretching the footer;
    // the 1px clip margin leaves the buttons' focus ring intact at the edge.
    <div
      className='@container/pagination flex min-w-0 items-center justify-end overflow-clip'
      style={{ overflowClipMargin: 1 }}
    >
      <div className='flex min-w-0 shrink-0 items-center gap-2 @xl/pagination:gap-3'>
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
              aria-label={t('Rows per page')}
              className='text-foreground h-8 w-[64px] font-medium tabular-nums sm:w-[70px]'
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

        <div className='flex min-w-0 shrink-0 items-center gap-1 @lg/pagination:gap-1.5 @xl/pagination:gap-2'>
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

          {pageNumbers.map((pageNumber, index) => (
            // `getPageNumbers` can emit more than one `'...'` gap, so the index is
            // part of the key: without it the two gaps collide.
            // eslint-disable-next-line react/no-array-index-key
            <div key={`${pageNumber}-${index}`} className='flex items-center'>
              {pageNumber === '...' ? (
                <span className='text-muted-foreground/60 px-0.5 text-sm @lg/pagination:px-1'>
                  ...
                </span>
              ) : (
                <Button
                  variant={currentPage === pageNumber ? 'default' : 'outline'}
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
    </div>
  )
}
