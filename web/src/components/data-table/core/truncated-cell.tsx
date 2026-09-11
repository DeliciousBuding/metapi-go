// metapi-go/data-table — TruncatedCell: an ellipsised cell that can still be
// read.
//
// Table columns truncate constantly (site names, URLs, model descriptions) and a
// truncated value with no way to see the rest is a dead end, so this is the one
// cell every truncating column goes through. The tooltip body defaults to the
// cell's own text, which is what `textContentOf` is for: a column's `cell`
// renderer returns an arbitrary React node, and the only honest answer to "what
// does this cell say" is to concatenate its string and number leaves and ignore
// the markup. A node with no text at all (a pure icon cell) renders without a
// tooltip — an empty tooltip is worse than none.

import type * as React from 'react'

import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'

type TruncatedCellProps = {
  children: React.ReactNode
  /** Extra classes for the clipping box, e.g. a `max-w-*` to truncate against. */
  className?: string
  /** Overrides the derived text body, e.g. to spell out a formatted value. */
  tooltipContent?: React.ReactNode
}

export function TruncatedCell({
  children,
  className,
  tooltipContent,
}: TruncatedCellProps) {
  const content = tooltipContent ?? textContentOf(children)
  const clipClassName = cn('block max-w-full min-w-0 truncate', className)

  if (!content) {
    return <div className={clipClassName}>{children}</div>
  }

  return (
    <Tooltip>
      <TooltipTrigger render={<div className={clipClassName} />}>
        <div className='truncate'>{children}</div>
      </TooltipTrigger>
      <TooltipContent side='top' className='max-w-xs break-all'>
        {content}
      </TooltipContent>
    </Tooltip>
  )
}

/** The text a node reads out loud as: its string/number leaves, markup dropped. */
function textContentOf(node: React.ReactNode): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(textContentOf).join('')
  return ''
}
