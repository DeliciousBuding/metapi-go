// The page owns polling. This control only changes its persisted preference.

import { Check, ChevronDown, Timer } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'

import {
  PROXY_LOGS_AUTO_REFRESH_PRESETS,
  type ProxyLogsAutoRefreshInterval,
} from '../lib/use-proxy-logs-auto-refresh'

type ProxyLogsAutoRefreshToggleProps = {
  intervalMs: ProxyLogsAutoRefreshInterval
  setIntervalMs: (value: ProxyLogsAutoRefreshInterval) => void
}

export function ProxyLogsAutoRefreshToggle({
  intervalMs,
  setIntervalMs,
}: ProxyLogsAutoRefreshToggleProps) {
  const { t } = useTranslation()

  const preset = PROXY_LOGS_AUTO_REFRESH_PRESETS.find(
    (item) => item.value === intervalMs
  )
  const label = preset ? t(preset.labelKey) : `${Number(intervalMs) / 1000}s`

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={<Button variant='outline' size='sm' />}
        aria-label={`${t('proxyLogs.page.autoRefresh.label')}: ${label}`}
      >
        <Timer className='size-3.5' aria-hidden='true' />
        <span className='text-muted-foreground'>
          {t('proxyLogs.page.autoRefresh.label')}
        </span>
        {label}
        <ChevronDown
          className='text-muted-foreground size-3.5'
          aria-hidden='true'
        />
      </DropdownMenuTrigger>
      <DropdownMenuContent align='end'>
        {PROXY_LOGS_AUTO_REFRESH_PRESETS.map((item) => (
          <DropdownMenuItem
            key={item.id}
            onClick={() => setIntervalMs(item.value)}
          >
            <Check
              className={
                intervalMs === item.value ? 'size-3.5' : 'invisible size-3.5'
              }
              aria-hidden='true'
            />
            {t(item.labelKey)}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
