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
/** One server-side page of the pricing-hydrated marketplace. */
export type ModelsPageData = {
  items: ModelRow[]
  total: number
}

/** Query key for one server-side models marketplace page. */
export function modelsPageQueryKey(params: {
  pageIndex: number
  pageSize: number
  includePricing: boolean
}) {
  return [
    ...modelsKeys.all,
    'marketplace',
    'page',
    {
      pageIndex: params.pageIndex,
      pageSize: params.pageSize,
      includePricing: params.includePricing,
    },
  ] as const
}

/**
 * Fetch + shape one server-side models marketplace page. Without ?page the
 * backend keeps the legacy response; this fetcher always sends page/pageSize
 * and therefore always receives the paged envelope. A missing/malformed
 * total degrades to the returned page length.
 */
export async function fetchModelsPage(params: {
  pageIndex: number
  pageSize: number
  includePricing: boolean
}): Promise<ModelsPageData> {
  const response = await api.getModelsMarketplace({
    page: params.pageIndex + 1,
    pageSize: params.pageSize,
    includePricing: params.includePricing,
  })
  const envelope = !Array.isArray(response)
    ? (response as { items?: unknown[]; total?: number })
    : null
  const items = envelope?.items ?? []
  const total =
    envelope && typeof envelope.total === "number" && Number.isFinite(envelope.total)
      ? envelope.total
      : items.length
  return { items: items as ModelRow[], total }
}

/** Fetch one server-side marketplace page by URL-owned table state. */
export function useModelsPage(
  params: { pageIndex: number; pageSize: number; includePricing: boolean },
  options?: Omit<UseQueryOptions<ModelsPageData>, 'queryKey' | 'queryFn'>
) {
  return useQuery<ModelsPageData>({
    queryKey: modelsPageQueryKey(params),
    queryFn: () => fetchModelsPage(params),
    placeholderData: (previous) => previous,
    staleTime: 10 * 1000,
    ...options,
  })
}

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
