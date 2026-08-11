// metapi-go/features/models — TanStack Query hooks wrapping `lib/api.ts`.
//
// The marketplace endpoint returns the full list (no server-side pagination
// today), so `useModels` is a single query and the table does client-side
// pagination/sorting/filtering. `useModelCapabilities` is a thin selector
// over the pricing-hydrated marketplace cache: it powers the detail sheet
// with fresh capability/pricing data without a second network round-trip
// when the list has already been loaded (TanStack dedups identical keys).

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

import { api } from '@/lib/api'

import {
  modelsKeys,
  type ModelRow,
  type ModelsMarketplaceResponse,
} from './types'

/**
 * Fetch the model marketplace. The backend returns the full array; the
 * table owns client-side pagination/sorting/filtering. Pass `refresh: true`
 * to request a fresh aggregation (longer timeout), `includePricing: true`
 * to hydrate per-account pricing.
 */
export function useModels(
  options?: { refresh?: boolean; includePricing?: boolean },
  queryOptions?: Omit<
    UseQueryOptions<ModelRow[], Error, ModelRow[]>,
    'queryKey' | 'queryFn'
  >
) {
  const refresh = options?.refresh ?? false
  const includePricing = options?.includePricing ?? false
  return useQuery<ModelRow[], Error>({
    queryKey: modelsKeys.marketplace({ refresh, includePricing }),
    queryFn: async () => {
      const result = (await api.getModelsMarketplace({
        refresh,
        includePricing,
      })) as ModelsMarketplaceResponse | ModelRow[] | undefined
      const models = Array.isArray(result) ? result : (result?.models ?? [])
      return models as ModelRow[]
    },
    staleTime: 10 * 1000,
    ...queryOptions,
  })
}

/**
 * Resolve a single model's capability summary (endpoint types + tags) from
 * the pricing-hydrated marketplace cache. Returns `undefined` while loading
 * or when the model is not in the marketplace. The detail sheet uses this to
 * avoid a dedicated per-model endpoint; because the query key matches a list
 * already fetched with `includePricing`, no extra network request fires.
 */

/**
 * Collect the unique capability facets (endpoint types) across a model list,
 * sorted for a stable faceted-filter dropdown.
 */
