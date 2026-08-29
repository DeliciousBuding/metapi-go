// metapi-go/routes — models marketplace list.
//
// `validateSearch` uses `modelsSearchSchema` (the URL-state contract for the
// models page). The `brand` / `capability` multi-select filters are encoded
// as comma-separated strings in the URL and transformed into arrays for the
// validated `search` object; TanStack Router (non-strict default) does not
// rewrite the URL on transforms, so the page's own `window.location.search`
// reads stay consistent.
//
// `loader` prefetches the pricing-hydrated marketplace
// (`modelsKeys.marketplace({ refresh: false, includePricing: true })`) — the
// exact query the page's `useModels({ includePricing: true })` requests — so
// first paint renders with data. The queryFn mirrors the hook (unwrap the
// `models` array from the marketplace envelope) so the cached payload matches.

import { createFileRoute } from '@tanstack/react-router'

import {
  fetchModelsPage,
  modelsPageQueryKey,
  modelsSearchSchema,
} from '@/features/models'
import { ModelsPage } from '@/features/models/components/models-page'

export const Route = createFileRoute('/_authenticated/models')({
  validateSearch: modelsSearchSchema,
  staticData: { title: 'models.page.title' },
  loader: async ({ context, location }) => {
    const params = new URLSearchParams(location.searchStr)
    const parsed = modelsSearchSchema.safeParse({
      page: params.get('page') ?? undefined,
      pageSize: params.get('pageSize') ?? undefined,
    })
    const pageIndex = parsed.success ? (parsed.data.page ?? 0) : 0
    const pageSize = parsed.success ? (parsed.data.pageSize ?? 20) : 20
    await context.queryClient.prefetchQuery({
      queryKey: modelsPageQueryKey({
        pageIndex,
        pageSize,
        includePricing: true,
      }),
      queryFn: () =>
        fetchModelsPage({
          pageIndex,
          pageSize,
          includePricing: true,
        }),
    })
  },
  component: ModelsPage,
})
