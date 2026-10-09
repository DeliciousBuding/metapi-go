import { useQuery } from '@tanstack/react-query'

import { api, type OverviewPeriod } from '@/lib/api'

// All analytical panels observe the same query and the server's exact window.
export function useOverviewReport(period: OverviewPeriod) {
  return useQuery({
    queryKey: ['dashboard', 'overview-report', period],
    queryFn: () => api.getOverviewReport(period),
    staleTime: 30_000,
    refetchInterval: 60_000,
  })
}
