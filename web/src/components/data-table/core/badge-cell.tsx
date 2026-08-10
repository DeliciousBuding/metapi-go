// metapi-go/data-table — ported from newapi
import * as React from 'react'

import { cn } from '@/lib/utils'

type BadgeCellProps = React.HTMLAttributes<HTMLDivElement>

export function BadgeCell({ className, ...props }: BadgeCellProps) {
  return (
    <div
      data-slot='badge-cell'
      className={cn(
        '-ml-1.5 flex max-w-full min-w-0 items-center gap-1 overflow-hidden [&_[data-slot=status-badge]]:max-w-full [&_[data-slot=status-badge]]:min-w-0',
        className
      )}
      {...props}
    />
  )
}
