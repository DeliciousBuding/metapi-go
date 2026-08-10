// metapi-go/data-table — ported from newapi
import { MoreHorizontal } from 'lucide-react'
import * as React from 'react'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'

type DataTableRowActionMenuProps = {
  children: React.ReactNode
  ariaLabel: string
  contentClassName?: string
  modal?: boolean
  onOpenChange?: (open: boolean) => void
}

export function DataTableRowActionMenu(props: DataTableRowActionMenuProps) {
  return (
    <DropdownMenu modal={props.modal} onOpenChange={props.onOpenChange}>
      <DropdownMenuTrigger
        render={
          <Button
            variant='ghost'
            size='icon'
            className='data-popup-open:bg-muted'
            aria-label={props.ariaLabel}
          />
        }
      >
        <MoreHorizontal aria-hidden='true' />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align='end'
        className={cn('w-48', props.contentClassName)}
      >
        {props.children}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
