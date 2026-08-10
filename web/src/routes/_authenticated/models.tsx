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

import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

import { api } from '@/lib/api'
import { modelsKeys, modelsSearchSchema } from '@/features/models'

export const Route = createFileRoute('/_authenticated/models')({
  validateSearch: modelsSearchSchema,
  loader: async ({ context }) => {
    await context.queryClient.prefetchQuery({
      queryKey: modelsKeys.marketplace({ refresh: false, includePricing: true }),
      queryFn: async () => {
        const result = await api.getModelsMarketplace({
          refresh: false,
          includePricing: true,
        })
        return Array.isArray(result) ? result : result?.models ?? []
      },
    })
  },
  component: lazyRouteComponent(
    () => import('@/features/models/components/models-page'),
    'ModelsPage',
  ),
})
