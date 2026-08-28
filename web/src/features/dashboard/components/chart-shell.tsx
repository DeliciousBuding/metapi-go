// metapi-go/features/dashboard/components — shared chart card chrome.
//
// Extracts the ~60-line container template duplicated across the 8 legacy
// charts (research/06-motion-icons-charts-responsive.md §7.3) into one shell:
// card chrome (bg-card, rounded-lg, border, shadow) + a fixed-height chart
// viewport + a header slot. Holds either a recharts SVG chart (dashboard
// sections) or a lightweight DOM chart in the same viewport.

import type { ReactNode } from 'react'

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

type ChartShellProps = {
  title: string
  description?: string
  /** Fixed chart viewport height in px (default 300). */
  height?: number
  /** Actions rendered in the header (toggles, range pickers). */
  actions?: ReactNode
  /** Whether the chart data is still loading (renders a skeleton). */
  loading?: boolean
  /** The chart (recharts SVG or lightweight DOM chart). */
  children: ReactNode
  /**
   * Optional screen-reader-only data summary rendered next to the chart
   * viewport (S10, #1035). Hidden until loading finishes, same as the chart.
   */
  summary?: ReactNode
  className?: string
}

export function ChartShell({
  title,
  description,
  height = 300,
  actions,
  loading = false,
  children,
  summary,
  className,
}: ChartShellProps) {
  return (
    <Card className={cn('flex flex-col', className)}>
      <CardHeader className='flex flex-row items-start justify-between gap-2'>
        <div className='space-y-1'>
          <CardTitle className='text-sm font-medium'>{title}</CardTitle>
          {description ? (
            <CardDescription className='text-xs'>{description}</CardDescription>
          ) : null}
        </div>
        {actions ? (
          <div className='flex items-center gap-1'>{actions}</div>
        ) : null}
      </CardHeader>
      <CardContent className='flex-1' aria-busy={loading}>
        {loading ? (
          <Skeleton className='w-full rounded-md' style={{ height }} />
        ) : (
          <>
            <div className='w-full' style={{ height }}>
              {children}
            </div>
            {summary}
          </>
        )}
      </CardContent>
    </Card>
  )
}
