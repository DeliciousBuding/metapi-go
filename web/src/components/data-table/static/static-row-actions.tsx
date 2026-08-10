// metapi-go/data-table — ported from newapi
import { Pencil, Trash2 } from 'lucide-react'

import { Button } from '@/components/ui/button'
import {
  DropdownMenuItem,
  DropdownMenuShortcut,
} from '@/components/ui/dropdown-menu'

import { DataTableRowActionMenu } from '../core/row-action-menu'

type StaticRowActionsProps = {
  editLabel: string
  deleteLabel: string
  menuLabel: string
  onEdit: () => void
  onDelete: () => void
  editDisabled?: boolean
  deleteDisabled?: boolean
}

export function StaticRowActions(props: StaticRowActionsProps) {
  return (
    <div className='flex justify-end gap-1'>
      <Button
        variant='ghost'
        size='icon-sm'
        onClick={props.onEdit}
        disabled={props.editDisabled}
        aria-label={props.editLabel}
      >
        <Pencil />
      </Button>
      <DataTableRowActionMenu ariaLabel={props.menuLabel}>
        <DropdownMenuItem
          onClick={props.onDelete}
          disabled={props.deleteDisabled}
          className='text-destructive focus:text-destructive'
        >
          {props.deleteLabel}
          <DropdownMenuShortcut>
            <Trash2 size={16} />
          </DropdownMenuShortcut>
        </DropdownMenuItem>
      </DataTableRowActionMenu>
    </div>
  )
}
