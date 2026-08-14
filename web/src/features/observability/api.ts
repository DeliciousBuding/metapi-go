// metapi-go/features/observability — TanStack Query hooks wrapping lib/api.ts.
//
// Each section owns one query. `useUsageHeatmap` / `useSlowRequests` power
// the Overview section (reusing the existing /api/stats endpoints), and
// `useMonitorHealth` powers the Health section (the read-only
// /api/monitor/health projection).
//
// Auto-refresh: every hook reads the shared auto-refresh interval from
// `useObservabilityAutoRefresh` (15s by default, toggled to 30s or Off from
// the observability header) and applies it as `refetchInterval` so health /
// heatmap / slow-request data stays fresh instead of fetching once on mount.
// Callers can still override `refetchInterval` per-hook via `options`.

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

import { api } from '@/lib/api'

import { useObservabilityAutoRefresh } from './context/auto-refresh-context'
import {
  observabilityKeys,
  type MonitorHealthResponse,
  type SlowRequestsResponse,
  type UsageHeatmapResponse,
} from './types'

export function useUsageHeatmap(
  params: { days?: number; dimension?: 'site' | 'model' } = {},
  options?: Omit<UseQueryOptions<UsageHeatmapResponse>, 'queryKey' | 'queryFn'>
) {
  const { intervalMs } = useObservabilityAutoRefresh()
  return useQuery({
    queryKey: observabilityKeys.usageHeatmap(params),
    queryFn: () => api.getUsageHeatmap(params) as Promise<UsageHeatmapResponse>,
    staleTime: 30 * 1000,
    refetchInterval: intervalMs,
    ...options,
  })
}

export function useSlowRequests(
  params: { limit?: number; minLatencyMs?: number; hours?: number } = {},
  options?: Omit<UseQueryOptions<SlowRequestsResponse>, 'queryKey' | 'queryFn'>
) {
  const { intervalMs } = useObservabilityAutoRefresh()
  return useQuery({
    queryKey: observabilityKeys.slowRequests(params),
    queryFn: () => api.getSlowRequests(params) as Promise<SlowRequestsResponse>,
    staleTime: 30 * 1000,
    refetchInterval: intervalMs,
    ...options,
  })
}

export function useMonitorHealth(
  options?: Omit<UseQueryOptions<MonitorHealthResponse>, 'queryKey' | 'queryFn'>
) {
  const { intervalMs } = useObservabilityAutoRefresh()
  return useQuery({
    queryKey: observabilityKeys.monitorHealth(),
    queryFn: () => api.getMonitorHealth() as Promise<MonitorHealthResponse>,
    staleTime: 15 * 1000,
    refetchInterval: intervalMs,
    ...options,
  })
}
