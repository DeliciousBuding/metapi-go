// metapi-go/data-table — BadgeListCell: a cell holding a list of badges that
// almost never fits.
//
// Model lists, endpoint lists, tag lists — a column like that has an unbounded
// number of values in a bounded cell, so it shows the first `max` inline, a
// "+N" chip for the rest, and the full set in a tooltip. Truncating without the
// tooltip would hide data the user came to the table for; showing everything
// would blow out the row height and with it the whole table's rhythm.
//
// The empty case renders an em dash rather than nothing, so an empty cell is
// distinguishable from a cell that failed to render.

import * as React from 'react'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { StatusBadgeList } from './status-badge'

type BadgeListCellProps = {
  /** Already-rendered badges; this cell does not know how to build them. */
  items: React.ReactNode[]
  /** How many to show inline before the "+N" chip. */
  max?: number
  tooltipClassName?: string
}

/** Negative margin cancels the badge's own `px-1.5` so text lines up with the header. */
const CELL_CLASS = '-ml-1.5 max-w-full'
const TOOLTIP_CLASS =
  'border-border bg-popover max-h-48 max-w-[320px] overflow-y-auto p-2'

export function BadgeListCell({
  items,
  max = 2,
  tooltipClassName,
}: BadgeListCellProps) {
  if (items.length === 0) {
    return <span className='text-muted-foreground text-xs'>—</span>
  }

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger render={<div className={CELL_CLASS} />}>
          <StatusBadgeList
            items={items}
            max={max}
            renderItem={(item) => item}
          />
        </TooltipTrigger>
        {items.length > max && (
          <TooltipContent
            side='top'
            className={tooltipClassName ?? TOOLTIP_CLASS}
          >
            <div className='flex flex-wrap gap-1'>{items}</div>
          </TooltipContent>
        )}
      </Tooltip>
    </TooltipProvider>
  )
}
