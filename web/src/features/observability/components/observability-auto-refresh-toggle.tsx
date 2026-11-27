// metapi-go/features/observability/components — header toggle that drives
// the shared auto-refresh interval (15s / 30s / Off). Reading + writing the
// interval happens through `useObservabilityAutoRefresh`; this component is
// purely presentational, mirroring the proxy-logs refresh button but as a
// segmented control so the cadence is visible at a glance.

import { useIsFetching, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { QuietRefreshStatus } from '@/components/common/quiet-refresh-status'
import { Button } from '@/components/ui/button'
import { useDelayedSpin } from '@/hooks/use-quiet-refresh'
import { cn } from '@/lib/utils'

import {
  useObservabilityAutoRefresh,
  type ObservabilityAutoRefreshInterval,
} from '../context/auto-refresh-context'
import { observabilityKeys } from '../types'

const OBSERVABILITY_AUTO_REFRESH_PRESETS = [
  {
    id: '15s',
    labelKey: 'observability.autoRefresh.interval15s',
    value: 15_000,
  },
  {
    id: '30s',
    labelKey: 'observability.autoRefresh.interval30s',
    value: 30_000,
  },
  {
    id: 'off',
    labelKey: 'observability.autoRefresh.intervalOff',
    value: false,
  },
] as const

export function ObservabilityAutoRefreshToggle() {
  const { t } = useTranslation()
  const { intervalMs, setIntervalMs } = useObservabilityAutoRefresh()
  const queryClient = useQueryClient()
  const isFetching = useIsFetching({ queryKey: observabilityKeys.all }) > 0
  const showSpin = useDelayedSpin(isFetching)
  const [freshness, setFreshness] = useState<{
    updatedAt: number
    hasError: boolean
  }>({ updatedAt: 0, hasError: false })

  useEffect(() => {
    const sync = () => {
      const queries = queryClient
        .getQueryCache()
        .findAll({ queryKey: observabilityKeys.all })
      let latestUpdatedAt = 0
      let hasError = false
      for (const query of queries) {
        if (query.state.status === 'error') hasError = true
        if (query.state.dataUpdatedAt > latestUpdatedAt) {
          latestUpdatedAt = query.state.dataUpdatedAt
        }
      }
      setFreshness((prev) =>
        prev.updatedAt === latestUpdatedAt && prev.hasError === hasError
          ? prev
          : { updatedAt: latestUpdatedAt, hasError }
      )
    }
    sync()
    return queryClient.getQueryCache().subscribe(sync)
  }, [queryClient])

  const isMatch = (presetValue: ObservabilityAutoRefreshInterval): boolean =>
    presetValue === intervalMs

  return (
    <div className='flex flex-wrap items-center gap-2'>
      <QuietRefreshStatus
        updatedAt={freshness.updatedAt}
        hasError={freshness.hasError}
      />
      <div
        className='bg-card flex items-center gap-1 rounded-md border p-0.5'
        role='group'
        aria-label={t('observability.autoRefresh.label')}
      >
        <Button
          type='button'
          variant='ghost'
          size='icon-xs'
          aria-label={t('observability.autoRefresh.refreshNow')}
          disabled={isFetching}
          onClick={() => {
            void queryClient.invalidateQueries({
              queryKey: observabilityKeys.all,
            })
          }}
        >
          <RefreshCw
            className={cn(
              'text-muted-foreground size-3.5 shrink-0',
              showSpin && 'animate-spin'
            )}
            aria-hidden='true'
          />
        </Button>
        {OBSERVABILITY_AUTO_REFRESH_PRESETS.map((preset) => {
          const active = isMatch(preset.value)
          return (
            <Button
              key={preset.id}
              type='button'
              variant={active ? 'default' : 'ghost'}
              size='sm'
              className='h-7 px-2 text-xs font-medium'
              aria-pressed={active}
              onClick={() => setIntervalMs(preset.value)}
            >
              {t(preset.labelKey)}
            </Button>
          )
        })}
      </div>
    </div>
  )
}
