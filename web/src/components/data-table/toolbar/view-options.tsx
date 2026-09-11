// metapi-go/data-table — DataTableViewOptions: the "which columns are visible"
// menu.
//
// Only columns with an accessor and `enableHiding` unset are offered. That
// excludes the two kinds of column a user cannot meaningfully hide: the
// selection checkbox column (no accessor) and any column the feature pinned as
// essential — hiding the identity column of a table leaves rows the user cannot
// tell apart.
//
// There is no memo here on purpose. `table` keeps one identity for the life of
// the view, so memoising on it would freeze the list against later changes for
// no saving: this filters a handful of columns.

import type { Table } from '@tanstack/react-table'
import { Columns3 } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

import { columnLabel } from '../core/column-label'

export function DataTableViewOptions<TData>({
  table,
}: {
  table: Table<TData>
}) {
  const { t } = useTranslation()
  const hideableColumns = table
    .getAllColumns()
    .filter((column) => column.accessorFn !== undefined && column.getCanHide())

  return (
    <DropdownMenu modal={false}>
      <DropdownMenuTrigger
        render={
          <Button
            variant='outline'
            className='shrink-0'
            aria-label={t('columnSettings')}
          />
        }
      >
        <Columns3 />
        {t('columnSettings')}
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end' className='w-[150px]'>
        <DropdownMenuGroup>
          <DropdownMenuLabel>{t('Toggle columns')}</DropdownMenuLabel>
          {hideableColumns.map((column) => (
            <DropdownMenuCheckboxItem
              key={column.id}
              className='capitalize'
              checked={column.getIsVisible()}
              onCheckedChange={(value) => column.toggleVisibility(!!value)}
            >
              {/* The id is the last resort: an unnamed column still has to be
                  toggleable, and its id is the only stable thing to show. */}
              {columnLabel(column.columnDef) ?? column.id}
            </DropdownMenuCheckboxItem>
          ))}
        </DropdownMenuGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
