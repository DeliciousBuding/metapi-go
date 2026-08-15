// metapi-go/routes — model tester (playground).
//
// `validateSearch` types the single deep-link param `?model=` (from the
// marketplace "try it" link) so the page can read it via `useSearch()` instead
// of `window.location.search`. The tester form owns the rest of its state
// in-memory.
//
// `loader` prefetches the base model marketplace
// (`modelsKeys.marketplace({ refresh: false, includePricing: false })`) — the
// query the tester form's `useModels()` populates its model picker from — so
// the picker renders populated on first paint. The queryFn mirrors the
// models hook (unwrap the `models` array from the marketplace envelope).

import { createFileRoute } from '@tanstack/react-router'
import { z } from 'zod'

import { ModelTesterPage } from '@/features/model-tester/components/model-tester-page'
import { modelsKeys } from '@/features/models'
import { api } from '@/lib/api'
import { stringSearchParam } from '@/lib/helpers/searchParams'

// Tolerant deep-link param: the router JSON-parses search values, so a
// stale `?model=123` arrives as a number and must not throw a route error.
export const modelTesterSearchSchema = z.object({
  model: stringSearchParam,
})

export const Route = createFileRoute('/_authenticated/model-tester')({
  validateSearch: modelTesterSearchSchema,
  staticData: { title: 'modelTester.page.title' },
  loader: async ({ context }) => {
    await context.queryClient.prefetchQuery({
      queryKey: modelsKeys.marketplace({
        refresh: false,
        includePricing: false,
      }),
      queryFn: async () => {
        const result = await api.getModelsMarketplace({
          refresh: false,
          includePricing: false,
        })
        return Array.isArray(result) ? result : (result?.models ?? [])
      },
    })
  },
  component: ModelTesterPage,
})
