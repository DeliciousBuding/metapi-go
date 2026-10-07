// metapi-go/ui — KPI value display component. Single source for dashboard/
// observability numeric metric sizing: text-3xl (default) / text-2xl / text-xl.
// All variants share the canonical recipe font-medium + tracking-tight +
// tabular-nums; consumers compose children (e.g. CountUp) instead of
// re-declaring the classes. CJK tracking is neutralised by the
// `:root[lang|='zh']` override in theme.css.

import { cn } from '@/lib/utils'

type KpiValueSize = 'lg' | 'md' | 'sm'

const sizeClass: Record<KpiValueSize, string> = {
  lg: 'text-3xl',
  md: 'text-2xl',
  sm: 'text-xl',
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
        'font-mono leading-tight font-semibold tracking-tight tabular-nums',
        sizeClass[size],
        className
      )}
    >
      {children}
    </span>
  )
}
