// metapi-go/ui — KPI value display component. Single source for dashboard/
// observability numeric metric sizing: text-2xl (default) / text-xl / text-lg.
// All variants share the canonical recipe font-semibold + tracking-tight +
// tabular-nums; consumers compose children (e.g. CountUp) instead of
// re-declaring the classes. CJK tracking is neutralised by the
// `:root[lang|='zh']` override in theme.css.

import { cn } from '@/lib/utils'

type KpiValueSize = 'lg' | 'md' | 'sm'

const sizeClass: Record<KpiValueSize, string> = {
  lg: 'text-2xl',
  md: 'text-xl',
  sm: 'text-lg',
}

export function KpiValue({
  size = 'lg',
  className,
  children,
}: {
  size?: KpiValueSize
  className?: string
  children: React.ReactNode
}) {
  return (
    <span
      className={cn(
        'font-semibold tracking-tight tabular-nums',
        sizeClass[size],
        className
      )}
    >
      {children}
    </span>
  )
}
