// metapi-go/features/models — TanStack Query hooks wrapping `lib/api.ts`.
//
// The marketplace list is server-driven: page, page-size, search, filters,
// and sort are all query parameters, and the paged response carries the true
// total plus whole-marketplace facet counts, so a page-1 search also finds
// page-2 models. `useModelCapabilities`-style selectors keep reading the
// shared page cache; the `?model=<name>` deep link resolves through the
// exact-name detail endpoint instead of scanning the loaded page.

import { useQuery, type UseQueryOptions } from '@tanstack/react-query'

import { api } from '@/lib/api'

import {
  modelsKeys,
  type ModelRow,
  type ModelsMarketplaceFacets,
  type ModelsMarketplaceResponse,
} from './types'

/** One server-side page of the pricing-hydrated marketplace. */
export type ModelsPageData = {
  items: ModelRow[]
  total: number
  /** Whole-marketplace facet counts; absent only from a pre-contract server. */
  facets: ModelsMarketplaceFacets | null
}

/** Every URL-owned input that shapes one marketplace page query. */
export type ModelsPageParams = {
  pageIndex: number
  pageSize: number
  includePricing: boolean
  q: string
  capabilities: string[]
  endpointTypes: string[]
  sourceKinds: string[]
  /** Table sort wire form (`column:asc|desc`); '' keeps the name.asc default. */
  sort: string
}

/** Query key for one server-side models marketplace page. */
export function modelsPageQueryKey(params: ModelsPageParams) {
  return [
    ...modelsKeys.all,
    'marketplace',
    'page',
    {
      pageIndex: params.pageIndex,
      pageSize: params.pageSize,
      includePricing: params.includePricing,
      q: params.q,
      capabilities: params.capabilities,
      endpointTypes: params.endpointTypes,
      sourceKinds: params.sourceKinds,
      sort: params.sort,
    },
  ] as const
}

/**
 * Fetch + shape one server-side models marketplace page. Without ?page the
 * backend keeps the legacy response; this fetcher always sends page/pageSize
 * and therefore always receives the paged envelope. A missing/malformed
 * total degrades to the returned page length.
 */
export async function fetchModelsPage(
  params: ModelsPageParams
): Promise<ModelsPageData> {
  const response = await api.getModelsMarketplace({
    page: params.pageIndex + 1,
    pageSize: params.pageSize,
    includePricing: params.includePricing,
    q: params.q || undefined,
    capability: params.capabilities.join(',') || undefined,
    endpointType: params.endpointTypes.join(',') || undefined,
    sourceKind: params.sourceKinds.join(',') || undefined,
    sort: params.sort || undefined,
  })
  const envelope = !Array.isArray(response)
    ? (response as {
        items?: unknown[]
        total?: number
        facets?: ModelsMarketplaceFacets
      })
    : null
  const items = envelope?.items ?? []
  const total =
    envelope &&
    typeof envelope.total === 'number' &&
    Number.isFinite(envelope.total)
      ? envelope.total
      : items.length
  return {
    items: items as ModelRow[],
    total,
    facets: envelope?.facets ?? null,
  }
}

/** Fetch one server-side marketplace page by URL-owned table state. */
export function useModelsPage(
  params: ModelsPageParams,
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

/** One fully hydrated marketplace row, resolved by exact model name. */
export type ModelDetailData = { model: ModelRow }

/**
 * Exact-name detail lookup for the one-shot `?model=<name>` deep link: the
 * referenced model opens even when it is not on the currently loaded page.
 * A 404 arrives as a query error carrying the server's "model not found"
 * message; the page turns that into an explicit notice, not a silent drop.
 * Retrying is pointless for a stable negative answer, so it is disabled.
 */
export function useModelDetail(name: string | null) {
  return useQuery<ModelDetailData, Error>({
    queryKey: modelsKeys.detail(name ?? ''),
    enabled: Boolean(name),
    retry: false,
    staleTime: 10 * 1000,
    queryFn: async () => {
      const result = await api.getModelMarketplaceDetail(name ?? '')
      if (!result.model) {
        throw new Error('model not found')
      }
      return { model: result.model as ModelRow }
    },
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
