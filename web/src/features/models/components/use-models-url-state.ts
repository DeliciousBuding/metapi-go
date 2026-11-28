import type { ColumnFiltersState } from '@tanstack/react-table'

import {
  encodeSorting,
  useUrlTableState,
  type UrlTableState,
  type UrlTableStateUpdate,
} from '@/components/data-table'
import {
  asStringParam,
  encodeStringListParam,
  parseSortingParam,
  parseStringListParam,
} from '@/lib/helpers/searchParams'

import { modelsSearchSchema } from '../lib/models-schema'

type ModelsUrlFilters = {
  capability: string[]
  endpointType: string[]
  sourceKind: string[]
}

function readSearch(searchString?: string): UrlTableState<ModelsUrlFilters> {
  if (typeof window === 'undefined') {
    return {
      q: '',
      pageIndex: 0,
      pageSize: 20,
      sorting: [],
      filters: { capability: [], endpointType: [], sourceKind: [] },
    }
  }
  const params = new URLSearchParams(searchString ?? window.location.search)
  const parsed = modelsSearchSchema.safeParse({
    q: params.get('q') ?? undefined,
    page: params.get('page') ?? undefined,
    pageSize: params.get('pageSize') ?? undefined,
    sort: params.get('sort') ?? undefined,
    capability: params.get('capability') ?? undefined,
    endpointType: params.get('endpointType') ?? undefined,
    sourceKind: params.get('sourceKind') ?? undefined,
  })
  if (!parsed.success) {
    return {
      q: '',
      pageIndex: 0,
      pageSize: 20,
      sorting: [],
      filters: { capability: [], endpointType: [], sourceKind: [] },
    }
  }
  const data = parsed.data
  return {
    q: asStringParam(data.q) ?? '',
    pageIndex: data.page ?? 0,
    pageSize: data.pageSize ?? 20,
    sorting: parseSortingParam(data.sort),
    filters: {
      capability: parseStringListParam(data.capability),
      endpointType: parseStringListParam(data.endpointType),
      sourceKind: parseStringListParam(data.sourceKind),
    },
  }
}

function buildHref(next: UrlTableStateUpdate<ModelsUrlFilters>): string {
  const current = readSearch()
  const merged: UrlTableState<ModelsUrlFilters> = {
    ...current,
    ...next,
    filters: { ...current.filters, ...next.filters },
  }
  const params = new URLSearchParams()
  if (merged.q) params.set('q', merged.q)
  if (merged.pageIndex > 0) params.set('page', String(merged.pageIndex))
  if (merged.pageSize !== 20) params.set('pageSize', String(merged.pageSize))
  const sortString = encodeSorting(merged.sorting)
  if (sortString) params.set('sort', sortString)
  const capabilityString = encodeStringListParam(merged.filters.capability)
  if (capabilityString) params.set('capability', capabilityString)
  const endpointTypeString = encodeStringListParam(merged.filters.endpointType)
  if (endpointTypeString) params.set('endpointType', endpointTypeString)
  const sourceKindString = encodeStringListParam(merged.filters.sourceKind)
  if (sourceKindString) params.set('sourceKind', sourceKindString)
  const queryString = params.toString()
  return queryString ? `/models?${queryString}` : '/models'
}

/**
 * The "feature useSearch" stage. Reads the URL on every render (cheap) and
 * hands the data-table controlled state + navigation-backed setters. The
 * shared {@link useUrlTableState} owns the router subscription and the
 * URL-sync guard.
 */
export function useModelsUrlState() {
  return useUrlTableState<ModelsUrlFilters>({
    basePath: '/models',
    read: readSearch,
    buildHref,
    // Filter changes (search / capability / endpoint-type / supply-source)
    // reset the page in the same URL transaction; TanStack's own auto-reset
    // is disabled so a single page update is issued instead of a redundant
    // bounce.
    resetPageIndexOnFilterChange: true,
    toColumnFilters: (filters) => {
      const columnFilters: ColumnFiltersState = []
      if (filters.capability.length > 0) {
        columnFilters.push({ id: 'capabilities', value: filters.capability })
      }
      if (filters.endpointType.length > 0) {
        columnFilters.push({
          id: 'endpointTypes',
          value: filters.endpointType,
        })
      }
      if (filters.sourceKind.length > 0) {
        columnFilters.push({ id: 'sourceKinds', value: filters.sourceKind })
      }
      return columnFilters
    },
    fromColumnFilters: (filters) => {
      const capabilityEntry = filters.find(
        (filter) => filter.id === 'capabilities'
      )
      const endpointTypeEntry = filters.find(
        (filter) => filter.id === 'endpointTypes'
      )
      const sourceKindEntry = filters.find(
        (filter) => filter.id === 'sourceKinds'
      )
      return {
        filters: {
          capability: Array.isArray(capabilityEntry?.value)
            ? (capabilityEntry.value as string[])
            : [],
          endpointType: Array.isArray(endpointTypeEntry?.value)
            ? (endpointTypeEntry.value as string[])
            : [],
          sourceKind: Array.isArray(sourceKindEntry?.value)
            ? (sourceKindEntry.value as string[])
            : [],
        },
      }
    },
  })
}
