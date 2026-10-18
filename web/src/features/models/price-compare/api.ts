// metapi-go/features/models/price-compare — TanStack Query hooks.
// The backend returns the full sorted candidate list (cheaper first); the
// page groups by model for the cross-site view.

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import { api } from '@/lib/api'

import {
  priceCompareQueryKeys,
  priceCompareResponseSchema,
  type PriceCompareItem,
} from './types'

export type PriceCompareParams = {
  model?: string
  days?: number
  limit?: number
  topModels?: number
  exactModel?: boolean
  pricePage?: number
  pricePageSize?: number
}

async function fetchPriceCompare(
  params: PriceCompareParams = {}
): Promise<PriceCompareItem[]> {
  const raw = await api.getModelPriceCompare(params)
  const parsed = priceCompareResponseSchema.parse(raw)
  return parsed.items
}

export function priceCompareQueryOptions(params: PriceCompareParams = {}) {
  return {
    queryKey: priceCompareQueryKeys.list(params),
    queryFn: () => fetchPriceCompare(params),
    staleTime: 30 * 1000,
  }
}

export function usePriceWorkspace(params: PriceCompareParams = {}) {
  return useQuery({
    queryKey: [...priceCompareQueryKeys.list(params), 'workspace'],
    queryFn: async () =>
      priceCompareResponseSchema.parse(await api.getModelPriceCompare(params)),
    staleTime: 30_000,
  })
}

export function useRefreshAccountPricing() {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (accountId: number) => api.refreshAccountPricing(accountId),
    onSettled: () =>
      client.invalidateQueries({ queryKey: priceCompareQueryKeys.all }),
  })
}
