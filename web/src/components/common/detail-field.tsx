// Shared label-value field for detail sheets (sites / accounts / routes /
// channels / models / checkin / proxy-logs). Stacked layout (label above
// value) — the densest arrangement, matching the GCP-console density the
// design language targets. The previous sheets each carried a private copy
// (some stacked, some in a 1/3-2/3 grid); this primitive is the single
// source of truth so icon-free field typography cannot drift again.
//
// Render as <dt>/<dd> so callers can group fields inside a <dl> (typically
// a two-column grid: `grid grid-cols-2 gap-x-3 gap-y-2 text-sm`).

import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

type DetailFieldProps = {
  label: string
  children: ReactNode
  /** Full-text tooltip for values that truncate (pass the raw value). */
  title?: string
  /** Span both columns of a two-column detail grid. */
  full?: boolean
  className?: string
}

export function DetailField({
  label,
  children,
  title,
  full = false,
  className,
}: DetailFieldProps) {
  return (
    <div
      className={cn(
        'flex min-w-0 flex-col',
        full && 'col-span-2',
        className
      )}
    >
      <dt className='text-muted-foreground text-[11px]'>{label}</dt>
      <dd className='truncate' title={title}>
        {children}
      </dd>
    </div>
  )
}
