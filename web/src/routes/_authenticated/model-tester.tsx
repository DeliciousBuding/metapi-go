// metapi-go/routes — model tester (playground).
//
// No `validateSearch`: the page reads the `?model=` deep-link param directly
// from `window.location.search` (same transitional pattern the sites page
// uses), and the tester form owns the rest of its state in-memory.
//
// `loader` prefetches the base model marketplace
// (`modelsKeys.marketplace({ refresh: false, includePricing: false })`) — the
// query the tester form's `useModels()` populates its model picker from — so
// the picker renders populated on first paint. The queryFn mirrors the
// models hook (unwrap the `models` array from the marketplace envelope).

import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

import { api } from '@/lib/api'
import { modelsKeys } from '@/features/models'

export const Route = createFileRoute('/_authenticated/model-tester')({
  loader: async ({ context }) => {
    await context.queryClient.prefetchQuery({
      queryKey: modelsKeys.marketplace({ refresh: false, includePricing: false }),
      queryFn: async () => {
        const result = await api.getModelsMarketplace({
          refresh: false,
          includePricing: false,
        })
        return Array.isArray(result) ? result : result?.models ?? []
      },
    })
  },
  component: lazyRouteComponent(
    () => import('@/features/model-tester/components/model-tester-page'),
    'ModelTesterPage',
  ),
})
