// metapi-go/features/dashboard/components — shared VChart card chrome.
//
// Extracts the ~60-line container template duplicated across the 8 legacy
// charts (research/06-motion-icons-charts-responsive.md §7.3) into one shell:
// card chrome (bg-card, rounded-lg, border, shadow) + a fixed-height canvas
// viewport + a header slot. VChart renders to <canvas> inside this viewport.

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
  /** Fixed canvas height in px (default 300). */
  height?: number
  /** Actions rendered in the header (toggles, range pickers). */
  actions?: ReactNode
  /** Whether the chart data is still loading (renders a skeleton). */
  loading?: boolean
  /** The VChart / recharts canvas. */
  children: ReactNode
  className?: string
}

export function ChartShell({
  title,
  description,
  height = 300,
  actions,
  loading = false,
  children,
  className,
}: ChartShellProps) {
  return (
    <Card className={cn('flex flex-col', className)}>
      <CardHeader className='flex flex-row items-start justify-between gap-2'>
        <div className='space-y-1'>
          <CardTitle className='text-sm font-medium'>{title}</CardTitle>
          {description ? (
            <CardDescription className='text-xs'>
              {description}
            </CardDescription>
          ) : null}
        </div>
        {actions ? <div className='flex items-center gap-1'>{actions}</div> : null}
      </CardHeader>
      <CardContent className='flex-1'>
        {loading ? (
          <Skeleton
            className='w-full rounded-md'
            style={{ height }}
          />
        ) : (
          <div
            className='w-full'
            style={{ height }}
          >
            {children}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
