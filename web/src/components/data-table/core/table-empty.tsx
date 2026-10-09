import type { ReactNode } from 'react'

import { TableCell, TableRow } from '@/components/ui/table'

import { DataTableEmptyState } from './empty-state'

type TableEmptyProps = {
  colSpan: number
  title?: string
  description?: string
  children?: ReactNode
  icon?: ReactNode
  isFiltered?: boolean
  onClearFilters?: () => void
}

export function TableEmpty({ colSpan, ...props }: TableEmptyProps) {
  return (
    <TableRow>
      <TableCell colSpan={colSpan} className='h-70 p-0'>
        <div className='sticky inset-x-0 mx-auto flex h-full w-fit max-w-full items-center justify-center'>
          <DataTableEmptyState {...props} />
        </div>
      </TableCell>
    </TableRow>
  )
}
