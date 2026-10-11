// metapi-go/routes — models marketplace list.
//
// `validateSearch` uses `modelsSearchSchema` (the URL-state contract for the
// models page). The capability / endpoint-type / supply-source multi-select
// filters are encoded as comma-separated strings in the URL and transformed
// into arrays for the validated `search` object; TanStack Router (non-strict
// default) does not rewrite the URL on transforms, so the page's own
// `window.location.search` reads stay consistent.
//
// `loader` prefetches exactly the page query the component will issue —
// pagination, search, filters, and sort all key the server-side page query —
// so first paint renders with data instead of a skeleton round-trip.

import { createFileRoute } from '@tanstack/react-router'

import { encodeSorting } from '@/components/data-table'
import {
  fetchModelsPage,
  modelsPageQueryKey,
  modelsSearchSchema,
} from '@/features/models'
import { ModelsPage } from '@/features/models/components/models-page'
import {
  asStringParam,
  parseSortingParam,
  parseStringListParam,
} from '@/lib/helpers/searchParams'

export const Route = createFileRoute('/_authenticated/models')({
  validateSearch: modelsSearchSchema,
  staticData: { title: 'models.page.title' },
  loader: async ({ context, location }) => {
    const params = new URLSearchParams(location.searchStr)
    const parsed = modelsSearchSchema.safeParse({
      q: params.get('q') ?? undefined,
      page: params.get('page') ?? undefined,
      pageSize: params.get('pageSize') ?? undefined,
      sort: params.get('sort') ?? undefined,
      capability: params.get('capability') ?? undefined,
      endpointType: params.get('endpointType') ?? undefined,
      sourceKind: params.get('sourceKind') ?? undefined,
    })
    const data = parsed.success ? parsed.data : null
    const pageParams = {
      pageIndex: data?.page ?? 0,
      pageSize: data?.pageSize ?? 20,
      includePricing: true,
      q: asStringParam(data?.q) ?? '',
      capabilities: parseStringListParam(data?.capability),
      endpointTypes: parseStringListParam(data?.endpointType),
      sourceKinds: parseStringListParam(data?.sourceKind),
      sort: encodeSorting(parseSortingParam(data?.sort)),
    }
    await context.queryClient.prefetchQuery({
      queryKey: modelsPageQueryKey(pageParams),
      queryFn: () => fetchModelsPage(pageParams),
    })
  },
  component: ModelsPage,
})
