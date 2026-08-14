// metapi-go/features/observability/components — header toggle that drives
// the shared auto-refresh interval (15s / 30s / Off). Reading + writing the
// interval happens through `useObservabilityAutoRefresh`; this component is
// purely presentational, mirroring the proxy-logs refresh button but as a
// segmented control so the cadence is visible at a glance.

import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import {
  useObservabilityAutoRefresh,
  type ObservabilityAutoRefreshInterval,
} from '../context/auto-refresh-context'

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

  const isMatch = (presetValue: ObservabilityAutoRefreshInterval): boolean =>
    presetValue === intervalMs

  return (
    <div
      className='bg-card flex items-center gap-1 rounded-md border p-0.5'
      role='group'
      aria-label={t('observability.autoRefresh.label')}
    >
      <RefreshCw
        className={cn(
          'text-muted-foreground size-3.5 shrink-0',
          intervalMs !== false && 'animate-spin'
        )}
        aria-hidden='true'
      />
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
  )
}
