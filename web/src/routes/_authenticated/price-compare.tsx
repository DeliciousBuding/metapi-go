// metapi-go/routes — standalone cross-site price comparison page.
// The page owns its model-filter state (debounced server-side `model` param)
// and reads the sorted candidate list directly from usePriceCompare.

import { createFileRoute, lazyRouteComponent } from '@tanstack/react-router'

export const Route = createFileRoute('/_authenticated/price-compare')({
  component: lazyRouteComponent(
    () =>
      import(
        '@/features/models/price-compare/components/price-compare-page'
      ),
    'PriceComparePage'
  ),
})
