// metapi-go/features/dashboard/components — overview metric stat card.
//
// Renders a metric value + an optional tiny area sparkline. The sparkline
// uses the shadcn ChartContainer (DOM-rendered recharts), which consumes CSS
// var() theme tokens via ChartStyle's --color-${key} injection — the same
// pattern the dashboard section charts (components/charts.tsx) now use.
//
// 2026-08 upgrade (audit ui-ux-2026-08): optional lucide icon rendered in an
// IconBadge, an optional tone (default/success/warning) that maps the icon
// badge onto the semantic status tokens, and an optional two-cell details
// grid (label + value) for extra information density.

import { Link } from '@tanstack/react-router'
import type { LucideIcon } from 'lucide-react'
import { lazy, Suspense, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { ChartConfig } from '@/components/ui/chart'
import { CountUp } from '@/components/ui/count-up'
import { IconBadge } from '@/components/ui/icon-badge'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'

// Loaded on demand: the sparkline is the only recharts surface reachable
// through the eager overview section. Keeping recharts behind a dynamic
// import means the dashboard entry chunk no longer downloads ~332KB of chart
// library when no sparkline renders (empty balance history, fresh install).
// The lazy reference lives at module level so its identity is stable across
// re-renders (React.lazy would otherwise remount on every render).
const LazyStatCardSparkline = lazy(() => import('./stat-card-sparkline'))

type StatCardTone = 'default' | 'success' | 'warning'

type StatCardDetail = {
  label: string
  value: string
  tone?: StatCardTone
}

type StatCardProps = {
  title: string
  value: string
  /** Optional helper line under the value (e.g. trend delta). */
  hint?: string
  /** Sparkline samples (numeric). When omitted, no sparkline renders. */
  spark?: number[]
  /** Tailwind class for the sparkline stroke (e.g. 'text-chart-1'). */
  accentClassName?: string
  /** Render skeleton placeholders while the metric is still loading. */
  loading?: boolean
  /** Numeric value to animate (CountUp). When set, overrides the static value string. */
  valueNumber?: number
  /** Formatter applied to the animated numeric value. */
  valueFormat?: (value: number) => string
  /** Optional lucide icon rendered in a soft badge next to the title. */
  icon?: LucideIcon
  /** Icon-badge tone mapped onto semantic tokens (default/success/warning). */
  tone?: StatCardTone
  /** Optional detail sub-cells (label + value) under the metric. */
  details?: StatCardDetail[]
  className?: string
  /** When set, the whole card becomes a navigable link to this route. */
  to?: string
}

const DETAIL_TONE_CLASSES: Record<StatCardTone, string> = {
  default: 'text-foreground',
  success: 'text-success-soft-fg',
  warning: 'text-warning-soft-fg',
}

const SPARK_CONFIG_BASE: ChartConfig = {
  value: {
    color: 'var(--chart-1)',
  },
}

export function StatCard(props: StatCardProps) {
  const { t } = useTranslation()
  const sparkConfig: ChartConfig = useMemo(
    () => ({
      ...SPARK_CONFIG_BASE,
      value: {
        ...SPARK_CONFIG_BASE.value,
        label: t('dashboard.statCard.trendLabel'),
      },
    }),
    [t]
  )
  const data = useMemo(
    () =>
      (props.spark ?? []).map((sample, index) => ({
        index,
        value: sample,
      })),
    [props.spark]
  )
  const Icon = props.icon

  const card = (
    <Card
      className={cn(
        'overflow-hidden h-full',
        props.to && 'cursor-pointer transition-colors hover:bg-muted/50',
        props.className
      )}
    >
      <CardHeader className='pb-2'>
        <div className='flex items-center gap-2'>
          {Icon ? (
            <IconBadge tone={props.tone ?? 'default'} size='sm'>
              <Icon />
            </IconBadge>
          ) : null}
          <CardTitle className='text-muted-foreground text-xs font-medium tracking-wide uppercase'>
            {props.title}
          </CardTitle>
        </div>
      </CardHeader>
      <CardContent className='flex flex-col gap-2'>
        {props.loading ? (
          <div className='space-y-2'>
            <Skeleton className='h-7 w-20' />
            <Skeleton className='h-4 w-32' />
          </div>
        ) : (
          <>
            <div className='flex items-end justify-between gap-2'>
              {props.valueNumber !== undefined &&
              Number.isFinite(props.valueNumber) ? (
                <CountUp
                  value={props.valueNumber}
                  format={props.valueFormat}
                  className='text-2xl font-semibold tracking-tight'
                />
              ) : (
                <span className='text-2xl font-semibold tracking-tight tabular-nums'>
                  {props.value}
                </span>
              )}
              {props.hint ? (
                <span className='text-muted-foreground text-xs tabular-nums'>
                  {props.hint}
                </span>
              ) : null}
            </div>
            <div className='flex min-h-0 flex-1 flex-col gap-2'>
              {props.details && props.details.length > 0 ? (
                <div className='grid grid-cols-2 gap-2'>
                  {props.details.map((detail) => (
                    <div
                      key={detail.label}
                      className='bg-muted/40 rounded-lg border px-2.5 py-2'
                    >
                      <div className='text-muted-foreground truncate text-[11px] leading-none font-medium'>
                        {detail.label}
                      </div>
                      <div
                        className={cn(
                          'mt-1.5 truncate text-xs font-semibold tabular-nums',
                          DETAIL_TONE_CLASSES[detail.tone ?? 'default']
                        )}
                        title={detail.value}
                      >
                        {detail.value}
                      </div>
                    </div>
                  ))}
                </div>
              ) : null}
              {data.length > 1 ? (
                <Suspense fallback={null}>
                  <LazyStatCardSparkline
                    data={data}
                    config={sparkConfig}
                    accentClassName={props.accentClassName}
                  />
                </Suspense>
              ) : null}
            </div>
          </>
        )}
      </CardContent>
    </Card>
  )

  if (props.to) {
    return (
      <Link
        to={props.to}
        className='focus-visible:ring-focus-ring block h-full rounded-xl outline-none focus-visible:ring-2 focus-visible:ring-offset-2'
      >
        {card}
      </Link>
    )
  }

  return card
}
