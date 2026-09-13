// metapi-go/data-table — BadgeListCell: a cell holding a list of badges that
// almost never fits.
//
// Model lists, endpoint lists, tag lists — a column like that has an unbounded
// number of values in a bounded cell, so it shows the first `max` inline, a
// "+N" chip for the rest, and the full set in a tooltip. Truncating without the
// tooltip would hide data the user came to the table for; showing everything
// would blow out the row height and with it the whole table's rhythm.
//
// The badges themselves arrive pre-rendered: which badge component a column uses
// is the feature's business, not the table's.
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
import { cn } from '@/lib/utils'

type BadgeListCellProps = {
  /** Already-rendered badges. */
  items: React.ReactNode[]
  /** How many to show inline before the "+N" chip. */
  max?: number
}

/** Negative margin cancels the badge's own `px-1.5` so text lines up with the header. */
const CELL_CLASS = '-ml-1.5 max-w-full'
const TOOLTIP_CLASS =
  'border-border bg-popover max-h-48 max-w-[320px] overflow-y-auto p-2'

export function BadgeListCell({ items, max = 2 }: BadgeListCellProps) {
  if (items.length === 0) {
    return <span className='text-muted-foreground text-xs'>—</span>
  }

  const overflow = items.length - max

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger render={<div className={CELL_CLASS} />}>
          {/* Rendered as the caller's own elements, keys and all: wrapping each
              in a Fragment would mean inventing a key for nodes that already
              have one. */}
          <div className='flex max-w-full min-w-0 items-center gap-1 overflow-hidden'>
            {items.slice(0, max)}
            {overflow > 0 && <OverflowChip count={overflow} />}
          </div>
        </TooltipTrigger>
        {overflow > 0 && (
          <TooltipContent side='top' className={TOOLTIP_CLASS}>
            <div className='flex flex-wrap gap-1'>{items}</div>
          </TooltipContent>
        )}
      </Tooltip>
    </TooltipProvider>
  )
}

/**
 * The "+N" chip. A muted text pill with no background, so a column of badges
 * does not turn into a wall of chips and the count does not compete with the
 * values it is summarising.
 */
function OverflowChip({ count }: { count: number }) {
  const label = `+${count}`

  return (
    <span
      title={label}
      className={cn(
        'inline-flex w-fit max-w-full min-w-0 shrink items-center font-medium tracking-normal whitespace-nowrap transition-colors',
        'rounded-4xl h-5 gap-1 px-1.5 text-2xs leading-none',
        'text-muted-foreground',
        'shrink-0'
      )}
    >
      <span className='min-w-0 truncate leading-normal'>{label}</span>
    </span>
  )
}
