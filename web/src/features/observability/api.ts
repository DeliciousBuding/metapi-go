// metapi-go/features/observability — TanStack Query hooks wrapping lib/api.ts.
//
// Each section owns one query. `useUsageHeatmap` / `useSlowRequests` power
// the Overview section (reusing the existing /api/stats endpoints), and
// `useMonitorHealth` powers the Health section (the read-only
// /api/monitor/health projection).

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

import { api } from '@/lib/api'

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
  return useQuery({
    queryKey: observabilityKeys.usageHeatmap(params),
    queryFn: () =>
      api.getUsageHeatmap(params) as Promise<UsageHeatmapResponse>,
    staleTime: 30 * 1000,
    ...options,
  })
}

export function useSlowRequests(
  params: { limit?: number; minLatencyMs?: number; hours?: number } = {},
  options?: Omit<UseQueryOptions<SlowRequestsResponse>, 'queryKey' | 'queryFn'>
) {
  return useQuery({
    queryKey: observabilityKeys.slowRequests(params),
    queryFn: () =>
      api.getSlowRequests(params) as Promise<SlowRequestsResponse>,
    staleTime: 30 * 1000,
    ...options,
  })
}

export function useMonitorHealth(
  options?: Omit<UseQueryOptions<MonitorHealthResponse>, 'queryKey' | 'queryFn'>
) {
  return useQuery({
    queryKey: observabilityKeys.monitorHealth(),
    queryFn: () => api.getMonitorHealth() as Promise<MonitorHealthResponse>,
    staleTime: 15 * 1000,
    ...options,
  })
}
