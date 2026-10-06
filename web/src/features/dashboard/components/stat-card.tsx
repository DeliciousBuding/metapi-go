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
import { ArrowUpRight, type LucideIcon } from 'lucide-react'
import { lazy, Suspense, useMemo } from 'react'
import { useTranslation } from 'react-i18next'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { ChartConfig } from '@/components/ui/chart'
import { CountUp } from '@/components/ui/count-up'
import { IconBadge } from '@/components/ui/icon-badge'
import { KpiValue } from '@/components/ui/kpi-value'
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
        'h-full gap-2 overflow-hidden py-3',
        props.to && 'cursor-pointer transition-colors hover:bg-muted/50',
        props.className
      )}
    >
      <CardHeader className='pb-0'>
        <div className='flex items-center gap-2'>
          {Icon ? (
            <IconBadge tone={props.tone ?? 'default'} size='sm'>
              <Icon />
            </IconBadge>
          ) : null}
          <CardTitle className='text-foreground text-sm leading-5 font-semibold tracking-normal'>
            {props.title}
          </CardTitle>
          {props.to ? (
            <ArrowUpRight
              className='text-muted-foreground ms-auto size-4 shrink-0'
              aria-hidden='true'
            />
          ) : null}
        </div>
      </CardHeader>
      <CardContent className='flex flex-1 flex-col gap-2'>
        {props.loading ? (
          <div className='space-y-2'>
            <Skeleton className='h-7 w-20' />
            <Skeleton className='h-4 w-32' />
          </div>
        ) : (
          <>
            <div className='flex min-h-14 flex-col items-start gap-1.5'>
              <KpiValue size='lg'>
                {props.valueNumber !== undefined &&
                Number.isFinite(props.valueNumber) ? (
                  <CountUp
                    value={props.valueNumber}
                    format={props.valueFormat}
                  />
                ) : (
                  props.value
                )}
              </KpiValue>
              {props.hint ? (
                <span className='text-muted-foreground text-xs leading-relaxed tabular-nums'>
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
                      className='min-w-0 border-s ps-3 first:border-s-0 first:ps-0'
                    >
                      <div className='text-muted-foreground text-xs leading-4 font-medium'>
                        {detail.label}
                      </div>
                      <div
                        className={cn(
                          'mt-1 break-words text-sm leading-5 font-semibold tabular-nums [overflow-wrap:anywhere]',
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
                  <LazyStatCardSparkline data={data} config={sparkConfig} />
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
