import type { ReactNode } from 'react'

import { cn } from '@/lib/utils'

const toneClasses = {
  success: 'text-success',
  danger: 'text-destructive',
} as const

export function SummaryStrip({
  children,
  className,
}: {
  children: ReactNode
  className?: string
}) {
  return (
    <div
      className={cn(
        'bg-muted/20 flex flex-wrap items-center gap-x-5 gap-y-1 rounded-lg border px-3 py-2',
        className
      )}
    >
      {children}
    </div>
  )
}

export function SummaryStat({
  label,
  value,
  tone,
}: {
  label: string
  value: string
  tone?: 'success' | 'danger'
}) {
  const valueToneClass = tone ? toneClasses[tone] : 'text-foreground'
  return (
    <span className='inline-flex items-baseline gap-2 whitespace-nowrap'>
      <span className='text-muted-foreground text-xs'>{label}</span>
      <span className={`text-sm font-semibold tabular-nums ${valueToneClass}`}>
        {value}
      </span>
    </span>
  )
}
