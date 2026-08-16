// metapi-go/features/models/price-compare — TanStack Query hooks.
// The backend returns the full sorted candidate list (cheaper first); the
// page groups by model for the cross-site view.

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

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

export function usePriceCompare(
  params: PriceCompareParams = {},
  options?: Omit<
    UseQueryOptions<PriceCompareItem[], Error, PriceCompareItem[]>,
    'queryKey' | 'queryFn'
  >
) {
  return useQuery({
    ...priceCompareQueryOptions(params),
    ...options,
  })
}
