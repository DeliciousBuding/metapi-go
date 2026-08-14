// metapi-go/features/proxy-logs/lib — localStorage-backed auto-refresh
// preference for the proxy logs page. Mirrors the observability auto-refresh
// pattern (the interval is persisted so a bookmarked operator view survives
// reloads), but scoped to the proxy-logs page rather than shared via context:
// proxy-logs is a single surface with no sub-sections, so a local hook is
// enough and avoids the provider boilerplate.
//
// `false` disables auto-refresh entirely; a positive number is the
// `refetchInterval` (in ms) forwarded to `useProxyLogs`. The default is Off
// so an explicit opt-in is required before background polling starts.

import { useCallback, useState } from 'react'

export type ProxyLogsAutoRefreshInterval = number | false

export const PROXY_LOGS_AUTO_REFRESH_PRESETS = [
  {
    id: '5s',
    labelKey: 'proxyLogs.page.autoRefresh.interval5s',
    value: 5_000,
  },
  {
    id: '15s',
    labelKey: 'proxyLogs.page.autoRefresh.interval15s',
    value: 15_000,
  },
  {
    id: '30s',
    labelKey: 'proxyLogs.page.autoRefresh.interval30s',
    value: 30_000,
  },
  {
    id: 'off',
    labelKey: 'proxyLogs.page.autoRefresh.intervalOff',
    value: false,
  },
] as const

const STORAGE_KEY = 'metapi-go:proxy-logs:auto-refresh'
const DEFAULT_INTERVAL_MS: ProxyLogsAutoRefreshInterval = false

function readStoredInterval(): ProxyLogsAutoRefreshInterval {
  if (typeof window === 'undefined') return DEFAULT_INTERVAL_MS
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (raw === null) return DEFAULT_INTERVAL_MS
    if (raw === 'false') return false
    const parsed = Number(raw)
    if (Number.isFinite(parsed) && parsed > 0) return parsed
    return DEFAULT_INTERVAL_MS
  } catch {
    return DEFAULT_INTERVAL_MS
  }
}

function writeStoredInterval(value: ProxyLogsAutoRefreshInterval) {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem(STORAGE_KEY, String(value))
  } catch {
    /* localStorage unavailable — keep the in-memory state only. */
  }
}

/**
 * Resolve the proxy-logs auto-refresh interval (in ms, or `false` to disable)
 * and a setter that persists the choice to localStorage. Re-mounts (route
 * navigation, reloads) restore the last choice so an operator's preferred
 * cadence survives without re-toggling each visit.
 */
export function useProxyLogsAutoRefresh(): {
  intervalMs: ProxyLogsAutoRefreshInterval
  setIntervalMs: (value: ProxyLogsAutoRefreshInterval) => void
} {
  const [intervalMs, setIntervalMsState] =
    useState<ProxyLogsAutoRefreshInterval>(readStoredInterval)

  const setIntervalMs = useCallback(
    (value: ProxyLogsAutoRefreshInterval) => {
      setIntervalMsState(value)
      writeStoredInterval(value)
    },
    []
  )

  return { intervalMs, setIntervalMs }
}
