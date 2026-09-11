// metapi-go/data-table — DataTableColumnHeader: a sortable column's header cell.
//
// The title is a dropdown trigger rather than a click-to-cycle header, because
// the cycle (none → asc → desc) is invisible: nothing tells the user a third
// click exists or what the next one does. The menu states all three explicitly
// and adds "hide column" when the column is hideable.
//
// Columns that cannot sort render as a plain title — no trigger, no caret — so a
// non-sortable header never looks clickable.

import type { Column } from '@tanstack/react-table'
import {
  ArrowDown as ArrowDownIcon,
  ArrowUp as ArrowUpIcon,
  ChevronsUpDown as CaretSortIcon,
  EyeOff as EyeNoneIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

type DataTableColumnHeaderProps<TData, TValue> = {
  column: Column<TData, TValue>
  title: React.ReactNode
}

/** Caret shown in the trigger for each sort state (`none` = unsorted). */
const SORT_ICONS = {
  asc: ArrowUpIcon,
  desc: ArrowDownIcon,
  none: CaretSortIcon,
} as const

const MENU_ICON_CLASS = 'text-muted-foreground/70 size-3.5'

export function DataTableColumnHeader<TData, TValue>({
  column,
  title,
}: DataTableColumnHeaderProps<TData, TValue>) {
  const { t } = useTranslation()

  if (!column.getCanSort()) {
    return <div>{title}</div>
  }

  const sorted = column.getIsSorted()
  const SortIcon = SORT_ICONS[sorted === false ? 'none' : sorted]

  return (
    <div className='flex items-center space-x-2'>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant='ghost'
              size='sm'
              className='data-popup-open:bg-accent -ms-3 h-8'
            />
          }
        >
          <span>{title}</span>
          <SortIcon className='ms-2 h-4 w-4' />
        </DropdownMenuTrigger>
        <DropdownMenuContent align='start'>
          {sorted !== false && (
            <>
              <DropdownMenuItem onClick={() => column.clearSorting()}>
                <CaretSortIcon className={MENU_ICON_CLASS} />
                {t('Default order')}
              </DropdownMenuItem>
              <DropdownMenuSeparator />
            </>
          )}
          <DropdownMenuItem onClick={() => column.toggleSorting(false)}>
            <ArrowUpIcon className={MENU_ICON_CLASS} />
            {t('Asc')}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={() => column.toggleSorting(true)}>
            <ArrowDownIcon className={MENU_ICON_CLASS} />
            {t('Desc')}
          </DropdownMenuItem>
          {column.getCanHide() && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuItem onClick={() => column.toggleVisibility(false)}>
                <EyeNoneIcon className={MENU_ICON_CLASS} />
                {t('Hide')}
              </DropdownMenuItem>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  )
}
