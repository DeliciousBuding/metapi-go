import { Coins, Gauge, RadioTower, Repeat2, Shuffle, Timer } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { cn } from '@/lib/utils'

import type { RouteSummaryRow } from '../types'
import { normalizeRoutingStrategy, routingStrategyLabel } from '../utils'

const STRATEGY_ICONS = {
  weighted: Shuffle,
  round_robin: Repeat2,
  stable_first: RadioTower,
  least_busy: Gauge,
  lowest_latency: Timer,
  lowest_cost: Coins,
} as const

export function RoutesModelSummary({
  route,
  readOnly,
}: {
  route: RouteSummaryRow
  readOnly: boolean
}) {
  const { t } = useTranslation()
  const strategyLabel = t(routingStrategyLabel(route.routingStrategy))
  const StrategyIcon =
    STRATEGY_ICONS[normalizeRoutingStrategy(route.routingStrategy)]
  const total = route.channelCount ?? 0
  const enabled = route.enabledChannelCount ?? 0
  const partial = enabled < total
  const sites = route.siteNames ?? []
  let channelLabel: string
  if (total === 0) {
    channelLabel = t('tokenRoutes.columns.channelZeroNeedChannels')
  } else if (partial) {
    channelLabel = t('tokenRoutes.columns.channelDisabledCount', {
      count: total - enabled,
    })
  } else {
    channelLabel = t('tokenRoutes.columns.channelEnabledSummary', {
      enabled,
      total,
    })
  }
  let channelTone = 'text-foreground'
  if (enabled === 0) channelTone = 'text-muted-foreground'
  else if (partial) channelTone = 'text-warning'

  return (
    <div className='text-muted-foreground text-2xs mt-0.5 flex min-w-0 flex-wrap items-center gap-x-1.5 gap-y-0.5 sm:flex-nowrap sm:overflow-hidden'>
      <code className='max-w-44 truncate font-mono' title={route.modelPattern}>
        {route.modelPattern}
      </code>
      {!readOnly && (
        <>
          <span aria-hidden='true'>·</span>
          <span
            className={cn(
              'inline-flex shrink-0 items-center gap-1 tabular-nums',
              channelTone
            )}
            title={channelLabel}
          >
            <RadioTower className='size-3' aria-hidden='true' />
            {enabled}/{total}
          </span>
          <span aria-hidden='true'>·</span>
          <span
            className='inline-flex shrink-0 items-center gap-1'
            title={strategyLabel}
          >
            <StrategyIcon className='size-3' aria-hidden='true' />
            <span className='hidden truncate sm:inline'>{strategyLabel}</span>
          </span>
        </>
      )}
      {sites.length > 0 && (
        <>
          <span aria-hidden='true' className='hidden sm:inline'>
            ·
          </span>
          <span
            className='hidden min-w-0 truncate sm:inline'
            title={sites.join(' → ')}
          >
            {sites.slice(0, 2).join(' → ')}
            {sites.length > 2 && ` +${sites.length - 2}`}
          </span>
        </>
      )}
    </div>
  )
}
