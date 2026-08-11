// metapi-go/data-table — ported from newapi
import * as React from 'react'

import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

import { StatusBadgeList } from './status-badge'

interface BadgeListCellProps {
  items: React.ReactNode[]
  max?: number
  tooltipClassName?: string
}

/**
 * Table cell renderer for a list of badges with overflow tooltip.
 * Displays up to `max` badges inline; remaining items appear in a tooltip.
 * Applies -ml-1.5 to compensate for badge px-1.5 and align with column header.
 */
export function BadgeListCell({
  items,
  max = 2,
  tooltipClassName,
}: BadgeListCellProps) {
  if (items.length === 0) {
    return <span className='text-muted-foreground text-xs'>-</span>
  }

  const showTooltip = items.length > max

  return (
    <TooltipProvider>
      <Tooltip>
        <TooltipTrigger render={<div className='-ml-1.5 max-w-full' />}>
          <StatusBadgeList
            items={items}
            max={max}
            renderItem={(item) => item}
          />
        </TooltipTrigger>
        {showTooltip && (
          <TooltipContent
            side='top'
            className={
              tooltipClassName ??
              'border-border bg-popover max-h-48 max-w-[320px] overflow-y-auto p-2'
            }
          >
            <div className='flex flex-wrap gap-1'>{items}</div>
          </TooltipContent>
        )}
      </Tooltip>
    </TooltipProvider>
  )
}
