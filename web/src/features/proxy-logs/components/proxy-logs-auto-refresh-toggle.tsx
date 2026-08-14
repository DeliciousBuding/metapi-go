// metapi-go/features/proxy-logs/components — header toggle that drives the
// page-level auto-refresh interval (5s / 15s / 30s / Off). Mirrors the
// observability auto-refresh segmented control so the cadence is visible at
// a glance; the interval is persisted to localStorage via
// `useProxyLogsAutoRefresh` so a bookmarked operator view survives reloads.

import { RefreshCw } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

import {
  PROXY_LOGS_AUTO_REFRESH_PRESETS,
  useProxyLogsAutoRefresh,
  type ProxyLogsAutoRefreshInterval,
} from '../lib/use-proxy-logs-auto-refresh'

export function ProxyLogsAutoRefreshToggle() {
  const { t } = useTranslation()
  const { intervalMs, setIntervalMs } = useProxyLogsAutoRefresh()

  const isMatch = (presetValue: ProxyLogsAutoRefreshInterval): boolean =>
    presetValue === intervalMs

  return (
    <div
      className='bg-card flex items-center gap-1 rounded-md border p-0.5'
      role='group'
      aria-label={t('proxyLogs.page.autoRefresh.label')}
    >
      <RefreshCw
        className={cn(
          'text-muted-foreground size-3.5 shrink-0',
          intervalMs !== false && 'animate-spin'
        )}
        aria-hidden='true'
      />
      {PROXY_LOGS_AUTO_REFRESH_PRESETS.map((preset) => {
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
