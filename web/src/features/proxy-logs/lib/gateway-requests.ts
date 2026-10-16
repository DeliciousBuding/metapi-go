import { useQuery } from '@tanstack/react-query'

import { type GatewayRequestsQuery, api } from '@/lib/api'

const gatewayRequestsKeys = {
  all: ['gateway-requests'] as const,
  list: (params: GatewayRequestsQuery) =>
    ['gateway-requests', 'list', params] as const,
  detail: (id: string | null) => ['gateway-requests', 'detail', id] as const,
}

export function useGatewayRequests(
  params: GatewayRequestsQuery,
  interval: number | false
) {
  return useQuery({
    queryKey: gatewayRequestsKeys.list(params),
    queryFn: () => api.getGatewayRequests(params),
    refetchInterval: interval,
  })
}

export function useGatewayRequest(id: string | null) {
  return useQuery({
    queryKey: gatewayRequestsKeys.detail(id),
    queryFn: () => {
      if (id === null) throw new Error('Request detail requires an ID')
      return api.getGatewayRequest(id)
    },
    enabled: id !== null,
  })
}
