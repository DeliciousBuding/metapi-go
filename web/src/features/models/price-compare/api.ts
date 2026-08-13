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

export function usePriceCompare(
  params: PriceCompareParams = {},
  options?: Omit<
    UseQueryOptions<PriceCompareItem[], Error, PriceCompareItem[]>,
    'queryKey' | 'queryFn'
  >
) {
  return useQuery({
    queryKey: priceCompareQueryKeys.list(params),
    queryFn: async () => {
      const raw = await api.getModelPriceCompare(params)
      const parsed = priceCompareResponseSchema.parse(raw)
      return parsed.items
    },
    staleTime: 30 * 1000,
    ...options,
  })
}
